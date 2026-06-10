package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"strings"
	"encoding/json"
	"path/filepath"
)

type Disk struct {
    Label string `json:"label"`
    UUID  string `json:"uuid"`
    Mount string `json:"mount"`
}

type Config struct {
    Source   	Disk   `json:"source"`
    Replica  	Disk   `json:"replica"`
    Excludes 	string `json:"excludes"`
    Trash    	string `json:"trash"`
	Marker 		string `json:"marker"`
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

const (
	ADD 	= "add"
	MODIFY 	= "mod"
	DELETE 	= "del"
)

type Change struct {
	action string
	path string
}

type Worker interface {
	Diff(src string, dist string, exclds string) 	[]Change
	Sync(src string, dist string, exclds string)	[]Change
}

func CheckMount(mount, uuid string) error {
    out, err := exec.Command("findmnt", "-no", "UUID", mount).Output()
    if err != nil {
		return fmt.Errorf("error: nothing mount in %s", mount)
    }
    got := strings.TrimSpace(string(out))
    if got != uuid {
		return fmt.Errorf("different disc in %s. expect: %s, got: %s. err: %w", mount, uuid, got, err)
    }
    return err
}

func CheckMarker(mount, marker string) error {
	_, err := os.ReadFile(filepath.Join(mount, marker))
    if err != nil {
        return fmt.Errorf("marker missing on %s: %w", mount, err)
    }
	return err
}

type Rsync struct { }

func (r Rsync) checkRsync() {
	if _, err := exec.LookPath("rsync"); err != nil {
		fmt.Fprintln(os.Stderr, "rsync not found in PATH, install it (e.g. sudo apt install rsync)")
		os.Exit(1)
    }
}

func (r Rsync) Sync(src string, dst string, exclds string) []Change {
	return r.run(false, src, dst, exclds)
}

func (r Rsync) Diff(src string, dst string, exclds string) []Change {
	return r.run(true, src, dst, exclds)
}

func (r Rsync) run(dryRun bool, src string, dst string, exclds string) []Change {
	r.checkRsync()
	result := []Change{}

	args := []string{"-rti", "--delete", "--modify-window=2", "--exclude-from=" + exclds}
	if dryRun {
		args = append(args, "-n")
	}
	cmd := exec.Command("rsync", append(args, src, dst)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("err rsync:", err)
		os.Exit(1)
	}

	lines := strings.Split(string(output), "\n")
	for _,line := range lines {
		if len(line) == 0 {
			continue
		}
		prefix, path, ok := strings.Cut(line, " ")
		if ok {
			if prefix[0] == '*' {
				result = append(result, Change{action:DELETE, path:path[2:]})
			} else if line[0] == '>' || line[0] == 'c' {
				if len(prefix) >= 3 && prefix[2] == '+' {
					result = append(result, Change{action:ADD, path:path})
				} else {
					result = append(result, Change{action:MODIFY, path:path})
				}
			}
		}
	}
	return result
}

func main() {
	if len(os.Args) != 2 {
		help()
		os.Exit(2)
	}
	cfg,err := loadConfig("/home/gnom/replicator/config.json")
	if err != nil {
		fmt.Println("err loading config:", err)
		os.Exit(1)
	}

	if 	(CheckMount(cfg.Source.Mount, cfg.Source.UUID) != nil) ||
		(CheckMount(cfg.Replica.Mount, cfg.Replica.UUID) != nil) ||
		(CheckMarker(cfg.Source.Mount, cfg.Marker) != nil) ||
		(CheckMarker(cfg.Replica.Mount, cfg.Marker) != nil) {
		fmt.Println("gabella")
		os.Exit(1)
	}
	// danger window, must be process
	var worker Worker = Rsync{}

	var changes []Change
	switch os.Args[1] {
	case "status":
		changes = worker.Diff(cfg.Source.Mount, cfg.Replica.Mount, cfg.Excludes)
	case "sync":
		changes = worker.Sync(cfg.Source.Mount, cfg.Replica.Mount, cfg.Excludes)
		syscall.Sync()
	default:
		fmt.Println("replct: unknown command: " + os.Args[1])
		help()
		os.Exit(2)
	}
	for _, elem := range changes {
		fmt.Println(elem)
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
