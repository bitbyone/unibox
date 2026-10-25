package unibox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// An sbxenv file is the team's and sbx's: unibox never writes, generates or
// merges one with anything of its own. It reads the little it needs - the
// args a person supplies, the kits and the bindings - and leaves every other
// field to sbx, so that a newer sbx adding fields does not break unibox.

// EnvArg is an argument an sbxenv file declares.
type EnvArg struct {
	Required    bool    `json:"required,omitempty"`
	Default     *string `json:"default,omitempty"`
	Pattern     string  `json:"pattern,omitempty"`
	Description string  `json:"description,omitempty"`
}

// EnvKit is one entry of an sbxenv's kits: an image reference, or a
// directory beside the file, which Dir then names on the host.
type EnvKit struct {
	Source string `json:"source"`
	Dir    string `json:"dir,omitempty"`
	// Args are what the env gives the kit's own args, as written: a
	// value, or ${{ env.args.<name> }}.
	Args map[string]string `json:"args,omitempty"`
}

// SBXEnv is what unibox reads of an env's files, merged in their order as sbx
// merges them.
type SBXEnv struct {
	Files    []string          `json:"files"`
	Name     string            `json:"name,omitempty"`
	Agent    string            `json:"agent,omitempty"`
	Args     map[string]EnvArg `json:"args,omitempty"`
	Kits     []EnvKit          `json:"kits,omitempty"`
	Bindings []string          `json:"bindings,omitempty"`
}

var argName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// ReadSBXEnv reads an env's files. Later files override what earlier ones
// say, field by field for an arg; kits add up, as sbx concatenates lists.
func ReadSBXEnv(files []string) (SBXEnv, error) {
	e := SBXEnv{Args: map[string]EnvArg{}}
	if len(files) == 0 {
		return e, fmt.Errorf("an env needs at least one sbxenv file; add one with unibox env add")
	}
	bindings := map[string]bool{}
	for _, file := range files {
		path, err := ExpandPath(file)
		if err != nil {
			return e, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return e, fmt.Errorf("cannot read sbxenv file %s: %w", file, err)
		}
		// Only these fields are unibox's to read; the rest is sbx's.
		var doc struct {
			Name     string                          `yaml:"name"`
			Agent    string                          `yaml:"agent"`
			Args     map[string]map[string]yaml.Node `yaml:"args"`
			Kits     []yaml.Node                     `yaml:"kits"`
			Bindings map[string]yaml.Node            `yaml:"bindings"`
		}
		if err = yaml.Unmarshal(b, &doc); err != nil {
			return e, fmt.Errorf("%s is not valid YAML: %w", file, err)
		}
		e.Files = append(e.Files, path)
		if doc.Name != "" {
			e.Name = doc.Name
		}
		if doc.Agent != "" {
			e.Agent = doc.Agent
		}
		for name, fields := range doc.Args {
			if !argName.MatchString(name) {
				return e, fmt.Errorf("%s: arg %q is not a name sbx takes", file, name)
			}
			a := e.Args[name]
			for key, v := range fields {
				switch key {
				case "required":
					if err = v.Decode(&a.Required); err != nil {
						return e, fmt.Errorf("%s: args.%s.required is true or false", file, name)
					}
				case "default":
					s := v.Value
					a.Default = &s
				case "pattern":
					a.Pattern = v.Value
				case "description":
					a.Description = v.Value
				}
			}
			e.Args[name] = a
		}
		for _, k := range doc.Kits {
			source := k.Value
			var args map[string]string
			if k.Kind == yaml.MappingNode {
				var entry struct {
					Source string            `yaml:"source"`
					Args   map[string]string `yaml:"args"`
				}
				_ = k.Decode(&entry)
				source, args = entry.Source, entry.Args
			}
			if source == "" {
				return e, fmt.Errorf("%s: a kit without a source; give it as a reference or {source: ...}", file)
			}
			kit := EnvKit{Source: source, Args: args}
			if localKit(source) {
				dir := source
				if !filepath.IsAbs(dir) {
					dir = filepath.Join(filepath.Dir(path), dir)
				}
				kit.Dir = filepath.Clean(dir)
			}
			if !slices.ContainsFunc(e.Kits, func(k EnvKit) bool { return k.Source == kit.Source && k.Dir == kit.Dir }) {
				e.Kits = append(e.Kits, kit)
			}
		}
		for name := range doc.Bindings {
			bindings[name] = true
		}
	}
	e.Bindings = sortedKeys(bindings)
	return e, nil
}

// localKit says a kit's source is a directory on the host rather than an
// image reference, as sbx tells them apart: by a path's leading dot or
// slash.
func localKit(source string) bool {
	return strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~")
}

// SandboxName is the name sbx gives the env's sandbox when unibox names
// none: the file's own, or the env's.
func (e SBXEnv) SandboxName(env string) string {
	if e.Name != "" {
		return e.Name
	}
	return env
}

// Missing are the args a person has to supply and has not: required ones
// with no default.
func (e SBXEnv) Missing(values map[string]string) []string {
	out := []string{}
	for _, name := range sortedKeys(e.Args) {
		a := e.Args[name]
		if _, ok := values[name]; ok {
			continue
		}
		if a.Required && a.Default == nil {
			out = append(out, name)
		}
	}
	return out
}

// CheckArgs says what of a person's values the env would not take: a value
// its pattern refuses. Values for args it does not declare are left out of
// what goes to sbx, and named here.
func (e SBXEnv) CheckArgs(values map[string]string) (unknown []string, err error) {
	for _, name := range sortedKeys(values) {
		a, ok := e.Args[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		if a.Pattern == "" {
			continue
		}
		// sbx matches the whole value; a pattern Go cannot read is left to
		// sbx to judge.
		re, rerr := regexp.Compile(`^(?:` + a.Pattern + `)$`)
		if rerr == nil && !re.MatchString(values[name]) {
			return unknown, fmt.Errorf("arg %s is %q, which does not match %s; change it with unibox var set %s <value>", name, values[name], a.Pattern, name)
		}
	}
	return unknown, nil
}

// Declared are the values for args the env declares, which alone go to sbx:
// it refuses an argument a file does not declare.
func (e SBXEnv) Declared(values map[string]string) map[string]string {
	out := map[string]string{}
	for name, v := range values {
		if _, ok := e.Args[name]; ok {
			out[name] = v
		}
	}
	return out
}

// Fingerprint stands for everything a sandbox is created from: the env's
// files, its local kits' files and the person's args. Image kits are named
// by their reference, which the files carry already.
func (e SBXEnv) Fingerprint(args map[string]string) (string, error) {
	h := sha256.New()
	add := func(parts ...string) {
		for _, p := range parts {
			fmt.Fprintf(h, "%d:%s", len(p), p)
		}
	}
	for _, file := range e.Files {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		add("file", file, string(b))
	}
	for _, k := range e.Kits {
		if k.Dir == "" {
			continue
		}
		files := []string{}
		err := filepath.WalkDir(k.Dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("kit %s: %w", k.Source, err)
		}
		sort.Strings(files)
		for _, path := range files {
			b, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			rel, _ := filepath.Rel(k.Dir, path)
			add("kit", k.Dir, rel, string(b))
		}
	}
	for _, name := range sortedKeys(args) {
		add("arg", name, args[name])
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// WriteArgsFile writes the values for sbx's --env-args-file into a file
// only the user reads, and returns it with what removes it.
func WriteArgsFile(dir string, values map[string]string) (string, func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(dir, "env-args-*")
	if err != nil {
		return "", nil, err
	}
	remove := func() { os.Remove(f.Name()) }
	var b strings.Builder
	for _, name := range sortedKeys(values) {
		fmt.Fprintf(&b, "%s=%s\n", name, values[name])
	}
	if err = f.Chmod(0o600); err == nil {
		_, err = f.WriteString(b.String())
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		remove()
		return "", nil, err
	}
	return f.Name(), remove, nil
}

var envArgRef = regexp.MustCompile(`\$\{\{\s*env\.args\.([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// KitArgs are the values a kit's args get from the env, with the person's
// values, then the env's defaults, put for ${{ env.args.<name> }}. One
// neither gives is a placeholder: unibox only reads a kit, and what a kit
// declares does not depend on its args.
func (e SBXEnv) KitArgs(k EnvKit, values map[string]string) map[string]string {
	if len(k.Args) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, v := range k.Args {
		out[name] = envArgRef.ReplaceAllStringFunc(v, func(ref string) string {
			arg := envArgRef.FindStringSubmatch(ref)[1]
			if v, ok := values[arg]; ok {
				return v
			}
			if d := e.Args[arg].Default; d != nil {
				return *d
			}
			return "unset"
		})
	}
	return out
}
