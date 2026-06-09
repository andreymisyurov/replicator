package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"encoding/json"
)

type Disk struct {
    Label string `json:"label"`
    UUID  string `json:"uuid"`
    Mount string `json:"mount"`
}

type Config struct {
    Source   Disk   `json:"source"`
    Replica  Disk   `json:"replica"`
    Excludes string `json:"excludes"`
    Trash    string `json:"trash"`
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
	MODIFY 	= "modify"
	DELETE 	= "delete"
)

type Change struct {
	action string
	path string
}

type Worker interface {
	Diff(src string, dist string, exclds string) 	[]Change
	Sync(src string, dist string)					bool
}

func CheckMount(uuid, mount string) bool {
    out, err := exec.Command("findmnt", "-no", "UUID", mount).Output()
    if err != nil {
		fmt.Println("error: nothing mount in " + mount)
		return false
    }
    got := strings.TrimSpace(string(out))
    if got != uuid {
        fmt.Println("different disc in " + mount + ". expect: " + uuid + ", got: " + got)
		return false
    }
    return true
}

type Rsync struct { }

func (r Rsync) Sync(src string, dst string) bool {

	return true
}

func (r Rsync) Diff(src string, dst string, exclds string) []Change {
	result := []Change{}

	cmd := exec.Command("rsync", "-rtni", "--delete", "--modify-window=2", "--exclude-from="+exclds, src, dst)
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
				result = append(result, Change{action:DELETE, path:path})
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

	if !CheckMount(cfg.Source.UUID, cfg.Source.Mount) || !CheckMount(cfg.Replica.UUID, cfg.Replica.Mount) {
		os.Exit(1)
	var worker Worker = Rsync{}

	switch os.Args[1] {
	case "status":
		arr := worker.Diff(cfg.Source.Mount, cfg.Replica.Mount, cfg.Excludes)
		for _, elem := range arr {
			fmt.Println(elem)
		}
	case "sync":
		worker.Sync(cfg.Source.Mount, cfg.Replica.Mount)
		fmt.Println("sync: not implemented yet")
	default:
		fmt.Println("replct: unknown command: " + os.Args[1])
		help()
		os.Exit(2)
	}
}

func help() {
	fmt.Fprint(os.Stderr, `replct - one-way disk replicator

Usage:
  replct <command>

Commands:
  status   compare source and replica, report divergence (read-only)
  sync     replicate changes from source to replica
`)
}
