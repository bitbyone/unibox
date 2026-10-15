package unibox

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed all:scaffold
var scaffoldFS embed.FS

// InitReport says what Init wrote and what it found already there and kept.
type InitReport struct {
	Root          string
	Written, Kept []string
	// Completed are files init brought up to date, adding what they lacked.
	Completed []string
	// Migrated says a profile was brought from an older format, then what
	// of it could not be carried over.
	Migrated []string
}

// Init lays the environments repository's mandatory structure into root and,
// given a name, adds one profile: the shared runner and its installation
// scripts, the unibox instructions text, and the profile's own files with the
// chosen agents installed. It writes only what is missing - a file already
// there is kept word for word - so it is safe on a repository in use.
func Init(root, name string, agents []string, cpus, memory string) (InitReport, error) {
	report := InitReport{Root: root}
	existing := false
	if name != "" {
		if err := ValidName(name); err != nil {
			return report, err
		}
		_, err := os.Stat(filepath.Join(root, "sandboxes", name, "sandbox.yaml"))
		existing = err == nil
		if agents == nil && !existing {
			agents = []string{"copilot"}
		}
	}
	if name != "" && !existing {
		if len(agents) == 0 {
			return report, fmt.Errorf("choose at least one agent for the profile")
		}
		for _, id := range agents {
			if _, err := FindAgent(id); err != nil {
				return report, err
			}
		}
		if err := (Resources{Amount(cpus), memory}).validate(); err != nil {
			return report, err
		}
	}
	place := func(rel string, data []byte, mode os.FileMode) error {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Lstat(path); err == nil {
			report.Kept = append(report.Kept, rel)
			return nil
		}
		if err := AtomicWrite(path, data, mode); err != nil {
			return err
		}
		report.Written = append(report.Written, rel)
		return nil
	}
	err := fs.WalkDir(scaffoldFS, "scaffold", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, "scaffold/")
		dest := rel
		if rel == ".install" || strings.HasPrefix(rel, "installations/") {
			dest = "_shared/" + rel
		}
		data, err := scaffoldFS.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if rel == ".install" || strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		return place(dest, data, mode)
	})
	if err != nil {
		return report, err
	}
	if name == "" {
		if err := os.MkdirAll(filepath.Join(root, "sandboxes"), 0o755); err != nil {
			return report, err
		}
		return report, CheckLayout(root)
	}
	if existing {
		return completeProfile(root, name, agents, report, place)
	}
	dir := "sandboxes/" + name
	lines := []string{"base", "tools/nvm"}
	declared := []string{}
	for _, id := range agents {
		lines = append(lines, "agents/"+id)
		declared = append(declared, id)
	}
	// Each agent's first questions - a theme, onboarding, whether to trust a
	// folder - would come in every new sandbox; its seed answers them once.
	seeds, err := agentSeeds(agents)
	if err != nil {
		return report, err
	}
	// The profile is built as a node tree and laid out by the arrangement
	// every change of it gets, so it reads the same written by init or by
	// a later command: each agent with how it is credited and its seeds.
	raw := fmt.Sprintf("# %s: describe the environment here. version is the profile's format;\n# unibox init migrates an older one.\nversion: %d\ndockerfile: Dockerfile\nresources: {cpus: %s, memory: %s}\nagents: [%s]\ninstructions: [unibox]\n", name, ProfileFormat, cpus, memory, strings.Join(declared, ", "))
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return report, err
	}
	for _, id := range agents {
		a, _ := FindAgent(id)
		SetCoAuthor(doc.Content[0], id, a.CoAuthor)
	}
	for _, sd := range seeds {
		SetAgentConfig(doc.Content[0], sd.agent, sd.file.Name, sd.file.SeedPath(sd.agent))
	}
	ArrangeProfile(&doc, name)
	profileText, err := encodeProfile(&doc)
	if err != nil {
		return report, err
	}
	profileYAML := string(profileText)
	dockerfile := fmt.Sprintf(`# The image is composed by _shared/install: list the installation scripts,
# one per line. The sbx build passes BASE=docker/sandbox-templates:shell
# (.sbx/build.args); the default keeps this buildable by hand.
ARG BASE=ubuntu:24.04
FROM ${BASE}

ARG UNIBOX_RUNTIME
USER root

RUN --mount=type=bind,source=_shared,target=/unibox /unibox/.install <<EOF
%s
EOF

# The shell template ships an agent user; a plain ubuntu base needs one made.
RUN id agent >/dev/null 2>&1 || useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent
`, strings.Join(lines, "\n"))
	instructions := fmt.Sprintf("# %s\n\nWhat the agent should know when it works in this profile.\n", name)
	for _, f := range []struct {
		rel  string
		data string
		mode os.FileMode
	}{
		{dir + "/sandbox.yaml", profileYAML, 0o644},
		{dir + "/Dockerfile", dockerfile, 0o644},
		{dir + "/instructions.md", instructions, 0o644},
		{dir + "/.sbx/build.args", "BASE=docker/sandbox-templates:shell\n", 0o644},
	} {
		if err := place(f.rel, []byte(f.data), f.mode); err != nil {
			return report, err
		}
	}
	for _, sd := range seeds {
		if err := place(dir+"/"+sd.file.SeedPath(sd.agent), sd.data, 0o644); err != nil {
			return report, err
		}
	}
	if _, err := LoadProfile(root, name); err != nil {
		return report, fmt.Errorf("the repository did not come out loadable: %w", err)
	}
	return report, nil
}

// agentSeed is the seed unibox's table gives one configuration file of an
// agent, encoded as the file is written.
type agentSeed struct {
	agent string
	file  AgentFile
	data  []byte
}

func agentSeeds(agents []string) ([]agentSeed, error) {
	out := []agentSeed{}
	for _, id := range agents {
		a, err := FindAgent(id)
		if err != nil {
			return nil, err
		}
		for _, f := range a.Configs {
			if f.Seed == nil {
				continue
			}
			var data []byte
			if f.TOML() {
				data, err = toml.Marshal(f.Seed)
			} else {
				data, err = json.MarshalIndent(f.Seed, "", "  ")
				data = append(data, '\n')
			}
			if err != nil {
				return nil, err
			}
			out = append(out, agentSeed{id, f, data})
		}
	}
	return out, nil
}

// completeProfile brings an existing profile up to what init writes today,
// adding only what it lacks: the seeds of its agents and their config
// entries, how each agent is credited, and the layout. What
// the profile says already - a seed of its own, a co-author, any section -
// stays as it is. Its agents are its own; init does not change them.
func completeProfile(root, name string, agents []string, report InitReport, place func(string, []byte, os.FileMode) error) (InitReport, error) {
	from, notes, err := migrateProfile(root, name)
	if err != nil {
		return report, err
	}
	if from < ProfileFormat {
		report.Migrated = append(report.Migrated, fmt.Sprintf("sandboxes/%s/sandbox.yaml from format %d to %d", name, from, ProfileFormat))
		report.Migrated = append(report.Migrated, notes...)
	}
	p, err := LoadProfile(root, name)
	var other formatError
	if errors.As(err, &other) {
		return report, err
	}
	if err != nil {
		return report, fmt.Errorf("profile %s exists but does not load; repair it before init completes it: %w", name, err)
	}
	if agents != nil {
		a, b := slices.Clone(agents), slices.Clone(p.Agents)
		slices.Sort(a)
		slices.Sort(b)
		if !slices.Equal(a, b) {
			return report, fmt.Errorf("profile %s declares %s; init does not change a profile's agents - edit its agents and Dockerfile, or leave out --agents", name, strings.Join(p.Agents, ", "))
		}
	}
	rel := "sandboxes/" + name + "/sandbox.yaml"
	file := filepath.Join(root, filepath.FromSlash(rel))
	before, err := os.ReadFile(file)
	if err != nil {
		return report, err
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(before, &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return report, fmt.Errorf("profile %s is not a YAML mapping; repair sandbox.yaml by hand", name)
	}
	m := doc.Content[0]
	seeds, err := agentSeeds(p.Agents)
	if err != nil {
		return report, err
	}
	for _, sd := range seeds {
		if _, ok := p.AgentConfig[agentConfigKey(sd.agent, sd.file.Name)]; ok {
			continue
		}
		dest := sd.file.SeedPath(sd.agent)
		if err := place("sandboxes/"+name+"/"+dest, sd.data, 0o644); err != nil {
			return report, err
		}
		SetAgentConfig(m, sd.agent, sd.file.Name, dest)
	}
	for _, id := range p.Agents {
		if _, ok := p.Git.CoAuthors[id]; !ok {
			a, _ := FindAgent(id)
			SetCoAuthor(m, id, a.CoAuthor)
		}
	}
	ArrangeProfile(&doc, name)
	after, err := encodeProfile(&doc)
	if err != nil {
		return report, err
	}
	if bytes.Equal(before, after) {
		report.Kept = append(report.Kept, rel)
		return report, nil
	}
	info, err := os.Stat(file)
	if err != nil {
		return report, err
	}
	if err = AtomicWrite(file, after, info.Mode().Perm()); err != nil {
		return report, err
	}
	if _, err = LoadProfile(root, name); err != nil {
		if restore := AtomicWrite(file, before, info.Mode().Perm()); restore != nil {
			return report, fmt.Errorf("%w; and the profile could not be restored: %v", err, restore)
		}
		return report, fmt.Errorf("completing profile %s would break it, so it was not kept: %w", name, err)
	}
	report.Completed = append(report.Completed, rel)
	return report, nil
}
