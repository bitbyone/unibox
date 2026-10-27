package unibox

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Resources are what a sandbox gets of the machine: CPUs and memory,
// absolute or as a share of the host. A team may say what an env's
// sandboxes start from in its unibox.yaml files, a share being the one
// thing that fits every machine; the person's in vars.toml - for every
// env, one, or one sandbox - win over it. sbx takes no share, so unibox
// works it out on the host at each create and hands sbx the amount, in an
// sbxenv file of its own given last.
type Resources struct {
	CPUs   string `toml:"cpus,omitempty" json:"cpus,omitempty" yaml:"cpus"`
	Memory string `toml:"memory,omitempty" json:"memory,omitempty" yaml:"memory"`
}

// ReadTeamResources are the resources a team gives an env's sandboxes: the
// repository's unibox.yaml, then the file beside each of the env's sbxenv
// files over it, a later file's over an earlier one's.
func ReadTeamResources(sbxenvFiles []string) (Resources, error) {
	var out Resources
	err := readTeamFiles(sbxenvFiles, func(name, _ string, doc teamDoc) error {
		for _, r := range ResourceNames {
			if v := doc.Resources.Get(r); v != "" {
				if err := ValidResource(r, v); err != nil {
					return fmt.Errorf("%s: resources: %w", name, err)
				}
			}
		}
		out = doc.Resources.Over(out)
		return nil
	})
	return out, err
}

// ResourceNames are the resources a person may set.
var ResourceNames = []string{"cpus", "memory"}

var (
	cpuPattern    = regexp.MustCompile(`^[0-9]+$`)
	memoryPattern = regexp.MustCompile(`^[1-9][0-9]*[kKmMgG]?$`)
	sharePattern  = regexp.MustCompile(`^([1-9][0-9]*)%$`)
)

// share is the percentage a value says, or 0 when it is not one.
func share(v string) int {
	m := sharePattern.FindStringSubmatch(v)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// Get and Set name a resource as vars.toml does.
func (r Resources) Get(name string) string {
	if name == "cpus" {
		return r.CPUs
	}
	return r.Memory
}

func (r *Resources) Set(name, value string) {
	if name == "cpus" {
		r.CPUs = value
	} else {
		r.Memory = value
	}
}

// Over is r, with what it leaves unset taken from base.
func (r Resources) Over(base Resources) Resources {
	if r.CPUs == "" {
		r.CPUs = base.CPUs
	}
	if r.Memory == "" {
		r.Memory = base.Memory
	}
	return r
}

// ValidResource says whether a value can be kept for a resource.
func ValidResource(name, value string) error {
	n := share(value)
	switch name {
	case "cpus":
		if !cpuPattern.MatchString(value) && (n < 1 || n > 100) {
			return fmt.Errorf("cpus is a number of CPUs, 0 for all of them, or a share of the host's from 1%% to 100%%, e.g. 4 or 50%%")
		}
	case "memory":
		if !memoryPattern.MatchString(value) && (n < 1 || n > 100) {
			return fmt.Errorf("memory is a size or a share of the host's from 1%% to 100%%, e.g. 16g or 75%%")
		}
	default:
		return fmt.Errorf("%q is no resource; set %s", name, strings.Join(ResourceNames, " or "))
	}
	return nil
}

// Host is what the host has, for resolving shares.
type Host struct {
	CPUs   int
	Memory int64 // bytes, 0 when it could not be read
}

// HostResources reads the host's CPUs and memory.
func HostResources() Host {
	h := Host{CPUs: runtime.NumCPU()}
	if m, err := hostMemory(); err == nil {
		h.Memory = int64(m)
	}
	return h
}

// Resolve gives the amounts sbx takes on this host: a share of the CPUs as
// a count, rounded and at least one, and all of them as 0, which sbx reads
// as every CPU; memory in MiB, kept within the 75% of the host's sbx gives
// a sandbox (sbx create --help), so asking for all of it still starts one.
// What it caps it says to log.
func (r Resources) Resolve(h Host, log io.Writer) (Resources, error) {
	out := r
	if n := share(r.CPUs); n == 100 {
		out.CPUs = "0"
	} else if n > 0 {
		out.CPUs = strconv.Itoa(max(1, (h.CPUs*n+50)/100))
	}
	if r.Memory == "" {
		return out, nil
	}
	if n := share(r.Memory); n > 0 {
		if h.Memory <= 0 {
			return out, fmt.Errorf("the host's memory could not be read for memory %s; give a size such as 16g", r.Memory)
		}
		out.Memory = fmt.Sprintf("%dm", h.Memory*int64(n)/100>>20)
	}
	limit := max(h.Memory*3/4, 512<<20)
	if h.Memory > 0 && memoryBytes(out.Memory) > limit {
		capped := fmt.Sprintf("%dm", limit>>20)
		fmt.Fprintf(log, "memory %s is more than sbx gives a sandbox, 75%% of the host's; using %s\n", r.Memory, capped)
		out.Memory = capped
	}
	return out, nil
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

// fingerprint is what the resources add to a sandbox's fingerprint;
// nothing when none is set, so a sandbox made before keeps its own.
func (r Resources) fingerprint() []string {
	out := []string{}
	for _, name := range ResourceNames {
		if v := r.Get(name); v != "" {
			out = append(out, name, v)
		}
	}
	return out
}

// WriteResourcesFile writes the sbxenv file that gives sbx the resolved
// resources, read after the env's own so that it wins, and returns it with
// what removes it; with none set there is no file.
func WriteResourcesFile(dir string, r Resources) (string, func(), error) {
	if r.CPUs == "" && r.Memory == "" {
		return "", func() {}, nil
	}
	var b strings.Builder
	b.WriteString("# Written by unibox from your resources in vars.toml.\nschemaVersion: \"1\"\nsandboxOptions:\n")
	if r.CPUs != "" {
		fmt.Fprintf(&b, "  cpus: %s\n", r.CPUs)
	}
	if r.Memory != "" {
		fmt.Fprintf(&b, "  memory: %q\n", r.Memory)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(dir, "resources-*.sbxenv.yaml")
	if err != nil {
		return "", nil, err
	}
	remove := func() { os.Remove(f.Name()) }
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		remove()
		return "", nil, err
	}
	return f.Name(), remove, nil
}
