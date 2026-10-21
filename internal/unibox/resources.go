package unibox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Resources is what a sandbox gets of the host: each a fixed amount or a
// share of the host's, as "50%", worked out when the sandbox is created.
// In JSON they read as the two fields they were before resources grouped
// them, so a sandbox's recorded settings stay comparable.
type Resources struct {
	CPUs   Amount `yaml:"cpus" json:"cpus"`
	Memory string `yaml:"memory" json:"memory"`
}

// Amount is a resource as a profile says it: a number, a size such as 8g,
// or a share of the host's such as 50%.
type Amount string

func (a *Amount) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: a resource is a number, a size or a share such as 50%%", n.Line)
	}
	*a = Amount(n.Value)
	return nil
}

// MarshalJSON writes a whole number as a number, as cpus always was.
func (a Amount) MarshalJSON() ([]byte, error) {
	if a == "" {
		return []byte("0"), nil
	}
	if n, err := strconv.Atoi(string(a)); err == nil {
		return json.Marshal(n)
	}
	return json.Marshal(string(a))
}

func (a *Amount) UnmarshalJSON(b []byte) error {
	var n int
	if json.Unmarshal(b, &n) == nil {
		*a = ""
		if n != 0 {
			*a = Amount(strconv.Itoa(n))
		}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*a = Amount(s)
	return nil
}

var (
	cpuPattern    = regexp.MustCompile(`^[1-9][0-9]*$`)
	memoryPattern = regexp.MustCompile(`^[1-9][0-9]*[kKmMgG]?$`)
	sharePattern  = regexp.MustCompile(`^([1-9][0-9]*)%$`)
)

// share is the percentage an amount says, or 0 when it is not one.
func (a Amount) share() int {
	m := sharePattern.FindStringSubmatch(string(a))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// Validate says whether the resources are of a form unibox takes.
func (r Resources) Validate() error { return r.validate() }

func (r Resources) validate() error {
	if r.CPUs != "" && !cpuPattern.MatchString(string(r.CPUs)) && !validShare(r.CPUs) {
		return fmt.Errorf("resources.cpus is a number of CPUs or a share of the host's from 1%% to 100%%, e.g. 4 or 50%%")
	}
	if r.Memory != "" && !memoryPattern.MatchString(r.Memory) && !validShare(Amount(r.Memory)) {
		return fmt.Errorf("resources.memory is a size or a share of the host's from 1%% to 100%%, e.g. 8g or 50%%")
	}
	return nil
}

func validShare(a Amount) bool {
	n := a.share()
	return n >= 1 && n <= 100
}

// Host is what the host has, for resolving shares.
type Host struct {
	CPUs   int
	Memory int64 // bytes
}

// HostResources reads the host's CPUs and memory; memory is 0 when it
// cannot be read.
func HostResources() Host {
	h := Host{CPUs: runtime.NumCPU()}
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			h.Memory, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		}
	case "linux":
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
					kb, _ := strconv.ParseInt(f[1], 10, 64)
					h.Memory = kb * 1024
				}
			}
		}
	}
	return h
}

// Resolve turns shares into amounts of this host: CPUs rounded, at least
// one; memory in whole MiB.
func (r Resources) Resolve(h Host) (cpus int, memory string, err error) {
	if n := r.CPUs.share(); n > 0 {
		cpus = max(1, (h.CPUs*n+50)/100)
	} else if r.CPUs != "" {
		cpus, _ = strconv.Atoi(string(r.CPUs))
	}
	memory = r.Memory
	if n := Amount(r.Memory).share(); n > 0 {
		if h.Memory <= 0 {
			return 0, "", fmt.Errorf("the host's memory could not be read for resources.memory %s; give a size such as 8g", r.Memory)
		}
		memory = fmt.Sprintf("%dm", h.Memory*int64(n)/100>>20)
	}
	return cpus, memory, nil
}

// memoryBytes reads a size as sbx takes it, in binary units.
func memoryBytes(s string) int64 {
	if s == "" {
		return 0
	}
	unit := int64(1)
	switch s[len(s)-1] {
	case 'k', 'K':
		unit = 1 << 10
	case 'm', 'M':
		unit = 1 << 20
	case 'g', 'G':
		unit = 1 << 30
	}
	n, _ := strconv.ParseInt(strings.TrimRight(s, "kKmMgG"), 10, 64)
	return n * unit
}
