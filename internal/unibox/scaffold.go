package unibox

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

// templates holds every file unibox puts into an environments repository,
// as files rather than strings: managed/ mirrors .shared, which unibox owns
// and keeps at its own version; user/ the user's _shared, written once with
// an example of each kind; profile/ what init writes once into a new
// profile, the user's from then on; seeds/ the agents' configuration seeds,
// per agent under the file's own name.
//
//go:embed all:templates
var templatesFS embed.FS

// InitReport says what Init wrote and what it found already there and kept.
type InitReport struct {
	Root          string
	Written, Kept []string
	// Updated and Removed are managed files brought to this unibox's own:
	// rewritten, and taken out because this unibox no longer has them.
	Updated, Removed []string
	// Completed are files init brought up to date, adding what they lacked.
	Completed []string
	// Migrated says a profile was brought from an older format, then what
	// of it could not be carried over.
	Migrated []string
	// Relaid says what moved from the repository layout of an older unibox.
	Relaid []string
}

// managedFiles are the files unibox owns in an environments repository,
// all under .shared, by their path in it.
func managedFiles() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(templatesFS, "templates/managed", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := templatesFS.ReadFile(path)
		out[strings.TrimPrefix(path, "templates/managed/")] = data
		return err
	})
	return out, err
}

func managedMode(rel string) os.FileMode {
	if strings.HasSuffix(rel, ".sh") {
		return 0o755
	}
	return 0o644
}

// LaidOut says whether init laid the repository out, so .shared is
// unibox's to keep current.
func LaidOut(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".shared"))
	return err == nil && info.IsDir()
}

// SyncManaged brings .shared to this unibox's own: it writes what is
// missing or differs, whatever was edited in it, and takes out anything
// else there - .shared is unibox's alone; the user's own go in _shared.
func SyncManaged(root string) (written, updated, removed []string, err error) {
	files, err := managedFiles()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, rel := range sortedKeys(files) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		mode := managedMode(rel)
		old, readErr := os.ReadFile(path)
		info, statErr := os.Stat(path)
		if readErr == nil && statErr == nil && bytes.Equal(old, files[rel]) && info.Mode().Perm() == mode {
			continue
		}
		if err = AtomicWrite(path, files[rel], mode); err != nil {
			return nil, nil, nil, err
		}
		if readErr == nil {
			updated = append(updated, rel)
		} else {
			written = append(written, rel)
		}
	}
	shared := filepath.Join(root, ".shared")
	dirs := []string{}
	err = filepath.WalkDir(shared, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != shared {
				dirs = append(dirs, path)
			}
			return nil
		}
		if _, ours := files[rel]; ours {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		removed = append(removed, rel)
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	// Directories a newer unibox emptied go too, deepest first.
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
	return written, updated, removed, nil
}

// managedHeaders are the lines unibox has put at the top of its scripts,
// in every wording, so a script from an older layout compares by its body.
var managedHeaders = []string{
	"# Managed by unibox: init, check and build rewrite it to unibox's own. Put\n# your own installation scripts beside it under another name.\n",
	"# Managed by unibox: init, check and build rewrite .shared to unibox's own.\n# Your own scripts go in _shared/install; one of the same name wins there.\n",
	"# Managed by unibox: init, check and build rewrite .shared to unibox's own.\n# Your own scripts go in _shared/@install; one of the same name wins there.\n",
}

func withoutHeader(b []byte) string {
	s := string(b)
	for _, h := range managedHeaders {
		s = strings.Replace(s, h, "", 1)
	}
	return s
}

// generatedRunLines are the lines a Dockerfile init wrote to run the
// installation scripts, in each older form, and newRunLines what replaces
// them; the list of names that follows them stays, as @install reads it.
var generatedRunLines = []string{
	"RUN --mount=type=bind,source=_shared,target=/unibox /unibox/.install <<EOF",
	"RUN --mount=type=bind,target=/unibox /unibox/.shared/install.sh <<EOF",
}

const newRunLines = "COPY --from=unibox / /\nRUN @install <<EOF"

// generatedComments are the comments init wrote above them, each with the
// one that replaces it.
var generatedComments = map[string]string{
	"# The image is composed by _shared/install: list the installation scripts,\n# one per line. The sbx build passes BASE=docker/sandbox-templates:shell\n# (.sbx/build.args); the default keeps this buildable by hand.\n":                                                                              dockerfileComment,
	"# The image is composed by .shared/install.sh: list the installation scripts,\n# one per line: each runs from _shared/@install, yours, or .shared/@install,\n# unibox's. The sbx build passes BASE=docker/sandbox-templates:shell\n# (.sbx/build.args); the default keeps this buildable by hand.\n": dockerfileComment,
}

const dockerfileComment = "# The image is composed by @install: it runs the installation scripts it\n# names, in order, each from _shared/@install, yours, or .shared/@install,\n# unibox's; COPY --from=unibox puts it and them into the image. The sbx\n# build passes BASE=docker/sandbox-templates:shell (.sbx/build.args).\n"

// fixDockerfiles rewrites, in every profile's Dockerfile, the lines an
// older init wrote to run the installation scripts into the @install of
// today. A Dockerfile that runs them otherwise is named, to be changed by
// hand.
func fixDockerfiles(root string) ([]string, error) {
	notes := []string{}
	profiles, _ := os.ReadDir(filepath.Join(root, "sandboxes"))
	for _, e := range profiles {
		file := filepath.Join(root, "sandboxes", e.Name(), "Dockerfile")
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		text := string(data)
		for _, line := range generatedRunLines {
			text = strings.ReplaceAll(text, line, newRunLines)
		}
		for old, now := range generatedComments {
			text = strings.Replace(text, old, now, 1)
		}
		if text != string(data) {
			if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
				return notes, err
			}
			notes = append(notes, "sandboxes/"+e.Name()+"/Dockerfile runs @install")
		} else if strings.Contains(text, "/unibox/.install") || strings.Contains(text, ".shared/install.sh") {
			notes = append(notes, "sandboxes/"+e.Name()+"/Dockerfile still runs an older runner; run the scripts with COPY --from=unibox / / and RUN @install <names>")
		}
	}
	return notes, nil
}

// relayOut moves a repository from the layout older unibox gave it -
// _shared/.install, _shared/installations, instructions/ - to .shared and
// _shared/{@install,@instructions}. A script or text that is unibox's own
// is dropped, as .shared has it; one that differs is kept as the user's,
// where it wins over unibox's. Best effort: what it cannot do it says.
func relayOut(root string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, "_shared", ".install")); err != nil {
		return nil, nil
	}
	files, err := managedFiles()
	if err != nil {
		return nil, err
	}
	notes := []string{}
	move := func(fromDir, toDir, managedDir string) error {
		from := filepath.Join(root, filepath.FromSlash(fromDir))
		return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(from, path)
			rel = filepath.ToSlash(rel)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if ours, ok := files[managedDir+"/"+rel]; ok && withoutHeader(ours) == withoutHeader(data) {
				return os.Remove(path)
			}
			to := filepath.Join(root, filepath.FromSlash(toDir), filepath.FromSlash(rel))
			if _, err := os.Stat(to); err == nil {
				notes = append(notes, fmt.Sprintf("%s/%s left where it is: %s/%s is there already", fromDir, rel, toDir, rel))
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			if err := os.Rename(path, to); err != nil {
				return err
			}
			note := fmt.Sprintf("moved %s/%s to %s/%s", fromDir, rel, toDir, rel)
			if _, ok := files[managedDir+"/"+rel]; ok {
				note += ", which differs from unibox's and now wins over it"
			}
			notes = append(notes, note)
			return nil
		})
	}
	if err := move("_shared/installations", "_shared/@install", ".shared/@install"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return notes, err
	}
	if err := move("instructions", "_shared/@instructions", ".shared/@instructions"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return notes, err
	}
	for _, dir := range []string{"_shared/installations", "instructions"} {
		removeEmpty(filepath.Join(root, filepath.FromSlash(dir)))
	}
	for _, f := range []string{"_shared/.install", "_shared/.unibox-managed"} {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(f)))
	}
	return notes, nil
}

// removeEmpty takes out a directory tree that holds no file.
func removeEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			removeEmpty(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(dir)
}

// profileTemplate renders one of templates/profile.
func profileTemplate(name string, data any) ([]byte, error) {
	src, err := templatesFS.ReadFile("templates/profile/" + name)
	if err != nil {
		return nil, err
	}
	t, err := template.New(name).Funcs(template.FuncMap{"join": strings.Join}).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = t.Execute(&out, data)
	return out.Bytes(), err
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
	relaid, err := relayOut(root)
	report.Relaid = relaid
	if err != nil {
		return report, err
	}
	fixed, err := fixDockerfiles(root)
	report.Relaid = append(report.Relaid, fixed...)
	if err != nil {
		return report, err
	}
	written, updated, removed, err := SyncManaged(root)
	if err != nil {
		return report, err
	}
	report.Written = append(report.Written, written...)
	report.Updated, report.Removed = updated, removed
	// The user's own shared structure, with an example of each kind, is
	// written once; from then on it is theirs.
	err = fs.WalkDir(templatesFS, "templates/user", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "templates/user/")
		return place(rel, data, managedMode(rel))
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
	fields := map[string]any{"Name": name, "Format": ProfileFormat, "CPUs": cpus, "Memory": memory, "Agents": declared, "Installations": lines}
	raw, err := profileTemplate("sandbox.yaml.tmpl", fields)
	if err != nil {
		return report, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
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
	files := []struct {
		rel, template string
		data          []byte
	}{{rel: dir + "/sandbox.yaml", data: profileText}, {rel: dir + "/Dockerfile", template: "Dockerfile.tmpl"}, {rel: dir + "/instructions.md", template: "instructions.md.tmpl"}, {rel: dir + "/.sbx/build.args", template: ".sbx/build.args"}}
	for _, f := range files {
		data := f.data
		if f.template != "" {
			if data, err = profileTemplate(f.template, fields); err != nil {
				return report, err
			}
		}
		if err := place(f.rel, data, 0o644); err != nil {
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
			data, err := templatesFS.ReadFile("templates/seeds/" + id + "/" + f.Name)
			if errors.Is(err, fs.ErrNotExist) {
				continue
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
