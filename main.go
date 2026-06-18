package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	ADD    = "add"
	MODIFY = "mod"
	DELETE = "del"
)

type Change struct {
	action string
	path   string
}

// work with config
type Disk struct {
	Label string `json:"label"`
	UUID  string `json:"uuid"`
	Mount string `json:"mount"`
}

type Config struct {
	Source    Disk   `json:"source"`
	Replica   Disk   `json:"replica"`
	Excludes  string `json:"excludes"`
	Trash     string `json:"trash"`
	Marker    string `json:"marker"`
	MaxDelete int    `json:"maxDelete"`
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// main interface
type Worker interface {
	Diff(cfg Config) []Change
	Sync(cfg Config) []Change
}

// security checks
func checkMount(mount, uuid string) error {
	out, err := exec.Command("findmnt", "-no", "UUID", mount).Output()
	if err != nil {
		return fmt.Errorf("error: nothing mount in %s", mount)
	}
	str := string(out[:len(out)-1])
	str = str[strings.LastIndex(str, "\n")+1:]
	if str != uuid {
		return fmt.Errorf("different disc in %s. expect: %s, got: %s", mount, uuid, str)
	}
	return nil
}

func checkMarker(mount, marker string) error {
	_, err := os.ReadFile(filepath.Join(mount, marker))
	if err != nil {
		return fmt.Errorf("marker missing on %s: %w", mount, err)
	}
	return err
}

func checkDeleteLimit(arr []Change, limit int) error {
	dels := 0
	for _, c := range arr {
		if c.action == DELETE {
			dels++
		}
	}
	if dels > limit {
		return fmt.Errorf("refusing to sync: %d deletions planned, limit %d. please, do it manually with -f(--force) flag", dels, limit)
	}
	return nil
}

// rsync way
type Rsync struct{}

func (r Rsync) checkRsync() {
	if _, err := exec.LookPath("rsync"); err != nil {
		fmt.Fprintln(os.Stderr, "rsync not found in PATH, install it (e.g. sudo apt install rsync)")
		os.Exit(1)
	}
}

func (r Rsync) Sync(cfg Config) []Change {
	return r.run(false, cfg)
}

func (r Rsync) Diff(cfg Config) []Change {
	return r.run(true, cfg)
}

func (r Rsync) run(dryRun bool, cfg Config) []Change {
	r.checkRsync()
	result := []Change{}

	args := []string{"-rti", "--delete", "--modify-window=2", "--exclude-from=" + cfg.Excludes, "--backup", "--backup-dir=" + cfg.Trash}
	if dryRun {
		args = append(args, "-n")
	}
	cmd := exec.Command("rsync", append(args, cfg.Source.Mount, cfg.Replica.Mount)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("err rsync:", err)
		os.Exit(1)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		prefix, path, ok := strings.Cut(line, " ")
		if ok {
			if prefix[0] == '*' {
				result = append(result, Change{action: DELETE, path: path[2:]})
			} else if line[0] == '>' || line[0] == 'c' {
				if len(prefix) >= 3 && prefix[2] == '+' {
					result = append(result, Change{action: ADD, path: path})
				} else {
					result = append(result, Change{action: MODIFY, path: path})
				}
			}
		}
	}
	return result
}

func main() {
	force := false
	flag.BoolVar(&force, "force", false, "skip the deletion limit")
	flag.BoolVar(&force, "f", false, "skip the deletion limit(shorthand)")
	flag.Parse()
	if flag.NArg() != 1 {
		help()
		os.Exit(2)
	}


	cfg, err := loadConfig("/home/gnom/replicator/config.json")
	if err != nil {
		fmt.Println("err loading config:", err)
		os.Exit(1)
	}

	if (checkMount(cfg.Source.Mount, cfg.Source.UUID) != nil) ||
		(checkMount(cfg.Replica.Mount, cfg.Replica.UUID) != nil) ||
		(checkMarker(cfg.Source.Mount, cfg.Marker) != nil) ||
		(checkMarker(cfg.Replica.Mount, cfg.Marker) != nil) {
		fmt.Println("gabella")
		os.Exit(1)
	}
	// danger window, must be process
	var worker Worker = Rsync{}

	changes := worker.Diff(cfg)
	if len(changes) == 0 {
		fmt.Println("nothing to sync")
		os.Exit(0)
	}
	switch flag.Arg(0) {
	case "status":
		for _, elem := range changes {
			fmt.Println(elem)
		}
	case "sync":
		if force == false {
			if err := checkDeleteLimit(changes, cfg.MaxDelete); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		changes = worker.Sync(cfg)
		syscall.Sync()
	default:
		fmt.Println("replct: unknown command: " + os.Args[1])
		help()
		os.Exit(2)
	}
}

func help() {
	fmt.Fprint(os.Stderr, `replct - one-way disk replicator

Usage:
  replicator <command>

Commands:
  status   compare source and replica, report divergence (read-only)
  sync     replicate changes from source to replica
`)
}
