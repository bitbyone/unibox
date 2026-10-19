package unibox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tobola/unibox/internal/unibox/api"
)

// A template is an environments repository others copy profiles from. A
// copied profile is the user's to change; what the template's profiles
// share - its _shared install scripts and instruction texts - is kept under
// .templates/<template> in the user's repository, rewritten only from the
// template, so it never mixes with the user's own _shared and one template
// never meets another's.

// TemplateSource is .templates/<template>/source.yaml: where the template
// comes from and which of its commits is there.
type TemplateSource struct {
	Source  string    `yaml:"source"`
	Commit  string    `yaml:"commit,omitempty"`
	Fetched time.Time `yaml:"fetched"`
}

// Template is a template's repository as fetched: a directory to read and
// what to do with it once read.
type Template struct {
	Dir, Source, Commit string
	cleanup             func()
}

func (t Template) Close() {
	if t.cleanup != nil {
		t.cleanup()
	}
}

// FetchTemplate gets a template: a local directory as it is, anything else
// a git repository cloned shallowly into a temporary directory. The clone
// runs as the user's own git, with their credentials, as it is the user's
// choice to read from it.
func FetchTemplate(ctx context.Context, source string) (Template, error) {
	if path, err := ExpandPath(source); err == nil {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			abs, err := filepath.Abs(path)
			if err != nil {
				return Template{}, err
			}
			commit, _ := templateGit(ctx, abs, "rev-parse", "HEAD")
			return Template{Dir: abs, Source: abs, Commit: commit}, nil
		}
	}
	dir, err := os.MkdirTemp("", "unibox-template-")
	if err != nil {
		return Template{}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	clone := exec.CommandContext(ctx, "git", "clone", "--quiet", "--depth", "1", "--", source, dir)
	clone.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := clone.CombinedOutput(); err != nil {
		cleanup()
		return Template{}, fmt.Errorf("could not clone template %s; check the URL and your access to it: %s", source, strings.TrimSpace(string(out)))
	}
	commit, _ := templateGit(ctx, dir, "rev-parse", "HEAD")
	return Template{Dir: dir, Source: source, Commit: commit, cleanup: cleanup}, nil
}

func templateGit(ctx context.Context, dir string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}

var nonName = regexp.MustCompile(`[^a-z0-9-]+`)

// TemplateNameOf is the name a template is kept under when none is given:
// its repository's or directory's, made a unibox name.
func TemplateNameOf(source string) string {
	base := strings.TrimSuffix(strings.TrimRight(source, "/"), ".git")
	if i := strings.LastIndexAny(base, "/:"); i >= 0 {
		base = base[i+1:]
	}
	name := strings.Trim(nonName.ReplaceAllString(strings.ToLower(base), "-"), "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "template-" + name
	}
	return strings.Trim(name, "-")
}

// Profiles are the template's profiles.
func (t Template) Profiles() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(t.Dir, "sandboxes"))
	if err != nil {
		return nil, fmt.Errorf("%s is no environments repository: it has no sandboxes directory", t.Source)
	}
	out := []string{}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(t.Dir, "sandboxes", e.Name(), "sandbox.yaml")); e.IsDir() && err == nil {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// sharedFiles are the template's shared files as they go into
// .templates/<template>: its _shared install scripts and instruction texts.
func (t Template) sharedFiles() (map[string]SnapshotFile, error) {
	out := map[string]SnapshotFile{}
	for _, dir := range []string{"@install", "@instructions"} {
		from := filepath.Join(t.Dir, "_shared", dir)
		if _, err := os.Stat(from); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("template %s: %s is a link; a template's shared files are files", t.Source, path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(filepath.Join(t.Dir, "_shared"), path)
			out[filepath.ToSlash(rel)] = SnapshotFile{data, uint32(info.Mode().Perm())}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// TemplateChanges is what writing a template's shared files would change
// in .templates/<template>.
type TemplateChanges struct {
	Added, Changed, Removed []string
}

func (c TemplateChanges) Empty() bool { return len(c.Added)+len(c.Changed)+len(c.Removed) == 0 }

func templateDir(root, name string) string { return filepath.Join(root, ".templates", name) }

// ReadTemplateSource reads where a kept template comes from.
func ReadTemplateSource(root, name string) (TemplateSource, error) {
	var s TemplateSource
	data, err := os.ReadFile(filepath.Join(templateDir(root, name), "source.yaml"))
	if err != nil {
		return s, fmt.Errorf("the environments repository keeps no template %s; unibox template ls lists those it does", name)
	}
	return s, yaml.Unmarshal(data, &s)
}

// SharedChanges compares the template's shared files with those kept.
func (t Template) SharedChanges(root, name string) (TemplateChanges, error) {
	var c TemplateChanges
	files, err := t.sharedFiles()
	if err != nil {
		return c, err
	}
	kept := map[string][]byte{}
	dir := templateDir(root, name)
	for _, sub := range []string{"@install", "@instructions"} {
		for rel, data := range readTree(filepath.Join(dir, sub)) {
			kept[sub+"/"+rel] = data
		}
	}
	for _, rel := range sortedKeys(files) {
		old, ok := kept[rel]
		switch {
		case !ok:
			c.Added = append(c.Added, rel)
		case !bytes.Equal(old, files[rel].Data):
			c.Changed = append(c.Changed, rel)
		}
	}
	for _, rel := range sortedKeys(kept) {
		if _, ok := files[rel]; !ok {
			c.Removed = append(c.Removed, rel)
		}
	}
	return c, nil
}

// WriteShared puts the template's shared files into .templates/<name>,
// replacing what was there: the directory is the template's alone. A
// directory kept from another source is not taken over.
func (t Template) WriteShared(root, name string) error {
	if err := ValidName(name); err != nil {
		return fmt.Errorf("template name: %w", err)
	}
	dir := templateDir(root, name)
	if old, err := ReadTemplateSource(root, name); err == nil && old.Source != t.Source {
		return fmt.Errorf("template %s comes from %s, not %s; name this one otherwise with --template", name, old.Source, t.Source)
	}
	files, err := t.sharedFiles()
	if err != nil {
		return err
	}
	// The shared files are replaced whole; the bases of the profiles copied
	// from the template, and any conflict left to resolve, stay.
	for _, old := range []string{"@install", "@instructions", "source.yaml"} {
		if err = os.RemoveAll(filepath.Join(dir, old)); err != nil {
			return err
		}
	}
	for rel, f := range files {
		mode := os.FileMode(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err = AtomicWrite(filepath.Join(dir, filepath.FromSlash(rel)), f.Data, mode); err != nil {
			return err
		}
	}
	src, err := yaml.Marshal(TemplateSource{Source: t.Source, Commit: t.Commit, Fetched: time.Now().UTC().Truncate(time.Second)})
	if err != nil {
		return err
	}
	header := []byte("# Where template " + name + " comes from. unibox writes this directory from the\n# template and replaces it on unibox template update; your own changes go in\n# _shared, which wins.\n")
	return AtomicWrite(filepath.Join(dir, "source.yaml"), append(header, src...), 0o644)
}

// CopyProfile copies the template's profile use into the repository as
// name, marked as copied from the template, as the template has it. It
// writes nothing when the profile does not then load.
func (t Template) CopyProfile(root, name, use, template string) (InitReport, error) {
	if err := ValidName(name); err != nil {
		return InitReport{}, err
	}
	from := filepath.Join(t.Dir, "sandboxes", use)
	if _, err := os.Stat(filepath.Join(from, "sandbox.yaml")); err != nil {
		profiles, _ := t.Profiles()
		return InitReport{}, fmt.Errorf("template %s has no profile %s; it has %s", t.Source, use, strings.Join(profiles, ", "))
	}
	to := filepath.Join(root, "sandboxes", name)
	if _, err := os.Stat(to); err == nil {
		return InitReport{}, fmt.Errorf("profile %s exists already; choose another name for the copy", name)
	}
	if err := t.WriteShared(root, template); err != nil {
		return InitReport{}, err
	}
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("template profile %s: %s is a link; a profile's files are files", use, rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if rel == "sandbox.yaml" {
			if data, err = markTemplate(data, name, template+"/"+use); err != nil {
				return fmt.Errorf("template profile %s: %w", use, err)
			}
		}
		return AtomicWrite(filepath.Join(to, rel), data, info.Mode().Perm())
	})
	undo := func(err error) (InitReport, error) {
		os.RemoveAll(to)
		return InitReport{}, err
	}
	if err != nil {
		return undo(err)
	}
	// The copy stays as the template has it, but for a template of an
	// older format, which is brought up to this one.
	report := InitReport{Root: root}
	format, notes, err := migrateProfile(root, name)
	if err != nil {
		return undo(err)
	}
	if format < ProfileFormat {
		report.Migrated = append(append(report.Migrated, fmt.Sprintf("sandboxes/%s/sandbox.yaml from format %d to %d", name, format, ProfileFormat)), notes...)
	}
	if _, err = LoadProfile(root, name); err != nil {
		return undo(fmt.Errorf("the copy of template profile %s does not load, so it was not kept: %w", use, err))
	}
	// The copy as written is the base an update merges the user's changes
	// and the template's from.
	if err = writeTree(baseDir(root, template, name), readTree(to), nil); err != nil {
		return undo(err)
	}
	return report, nil
}

// markTemplate writes where a copied profile comes from into it.
func markTemplate(text []byte, name, template string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("sandbox.yaml is no mapping")
	}
	setMapValue(doc.Content[0], "template", scalarNode(template))
	ArrangeProfile(&doc, name)
	return encodeProfile(&doc)
}

// ProfileDiff is how a profile differs from the template profile it was
// copied from: the files of either that the other lacks or has otherwise.
// sandbox.yaml is compared as unibox lays it out, without the template line.
func (t Template) ProfileDiff(root string, p Profile) ([]string, error) {
	from := filepath.Join(t.Dir, "sandboxes", p.TemplateProfile())
	if _, err := os.Stat(from); err != nil {
		return nil, fmt.Errorf("template %s has no profile %s any more", t.Source, p.TemplateProfile())
	}
	theirs, ours := readTree(from), readTree(p.Dir)
	normal := func(b []byte) []byte {
		var doc yaml.Node
		if yaml.Unmarshal(b, &doc) != nil || len(doc.Content) != 1 {
			return b
		}
		// Both are laid out as unibox lays a profile out, so only what
		// the profile says is compared, not unibox's comments.
		deleteMapKey(doc.Content[0], "template")
		ArrangeProfile(&doc, p.Name)
		out, _ := encodeProfile(&doc)
		return out
	}
	differ := []string{}
	for _, rel := range sortedKeys(theirs) {
		a, b := theirs[rel], ours[rel]
		if rel == "sandbox.yaml" {
			a, b = normal(a), normal(b)
		}
		if _, ok := ours[rel]; !ok || !bytes.Equal(a, b) {
			differ = append(differ, rel)
		}
	}
	for _, rel := range sortedKeys(ours) {
		if _, ok := theirs[rel]; !ok && !slices.Contains(differ, rel) {
			differ = append(differ, rel)
		}
	}
	return differ, nil
}

// TemplateProfiles are the profiles of the repository copied from a
// template, by name.
func TemplateProfiles(root, template string) []Profile {
	names, _ := ProfileNames(root)
	out := []Profile{}
	for _, n := range names {
		if p, err := LoadProfile(root, n); err == nil && p.TemplateName() == template {
			out = append(out, p)
		}
	}
	return out
}

// KeptTemplates are the templates .templates keeps.
func KeptTemplates(root string) []string {
	entries, _ := os.ReadDir(filepath.Join(root, ".templates"))
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Inspect tells what the template holds and how it stands to the
// repository at root - empty for none - without writing anything.
func (t Template) Inspect(root, name string) (api.Template, error) {
	out := api.Template{APIVersion: api.Version, Name: name, Source: t.Source, Commit: t.Commit, Shared: []string{}, Profiles: []api.TemplateProfile{}}
	files, err := t.sharedFiles()
	if err != nil {
		return out, err
	}
	out.Shared = sortedKeys(files)
	profiles, err := t.Profiles()
	if err != nil {
		return out, err
	}
	for _, n := range profiles {
		row := api.TemplateProfile{Name: n, Agents: []string{}, Vars: map[string]string{}, Services: []api.TemplateService{}, Secrets: []string{}, Env: []string{}, Instructions: []string{}}
		data, _ := os.ReadFile(filepath.Join(t.Dir, "sandboxes", n, "sandbox.yaml"))
		row.Format = profileVersion(data)
		if root != "" {
			_, err := os.Stat(filepath.Join(root, "sandboxes", n))
			row.Exists = err == nil
		}
		p, err := LoadProfile(t.Dir, n)
		if err != nil {
			row.Error = err.Error()
			out.Profiles = append(out.Profiles, row)
			continue
		}
		row.Image, row.Agents, row.Instructions = p.Image, p.Agents, p.Instructions
		for k, v := range p.Vars {
			row.Vars[k] = v
		}
		for _, s := range sortedKeys(p.Services) {
			row.Services = append(row.Services, api.TemplateService{Name: s, Hosts: p.Services[s].ProxyHosts(), Git: p.Services[s].Git})
		}
		row.Secrets = p.SecretRefs()
		row.Env = append(slices.Clone(p.Env), sortedKeys(p.EnvValues)...)
		out.Profiles = append(out.Profiles, row)
	}
	if root != "" {
		if kept, err := ReadTemplateSource(root, name); err == nil {
			k := &api.TemplateKept{Source: kept.Source, Commit: kept.Commit, Profiles: []string{}}
			for _, p := range TemplateProfiles(root, name) {
				k.Profiles = append(k.Profiles, p.Name)
			}
			out.Kept = k
		}
	}
	return out, nil
}

// readTree reads a directory's files by their slash path under it, without
// .git; nil for a directory that is not there.
func readTree(dir string) map[string][]byte {
	out := map[string][]byte{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)], _ = os.ReadFile(path)
		return nil
	})
	return out
}

func writeTree(dir string, files map[string][]byte, modes map[string]os.FileMode) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for rel, data := range files {
		mode := modes[rel]
		if mode == 0 {
			mode = 0o644
		}
		if err := AtomicWrite(filepath.Join(dir, filepath.FromSlash(rel)), data, mode); err != nil {
			return err
		}
	}
	return nil
}

func baseDir(root, template, profile string) string {
	return filepath.Join(templateDir(root, template), "bases", profile)
}

func conflictDir(root, template, profile string) string {
	return filepath.Join(templateDir(root, template), "conflicts", profile)
}

// theirs is the template's profile as a copy of it would be written: its
// files, with the template line in sandbox.yaml.
func (t Template) theirs(p Profile) (map[string][]byte, error) {
	files := readTree(filepath.Join(t.Dir, "sandboxes", p.TemplateProfile()))
	if len(files) == 0 {
		return nil, fmt.Errorf("template %s has no profile %s any more", t.Source, p.TemplateProfile())
	}
	if y, ok := files["sandbox.yaml"]; ok {
		marked, err := markTemplate(y, p.Name, p.Template)
		if err != nil {
			return nil, err
		}
		files["sandbox.yaml"] = marked
	}
	return files, nil
}

// ProfileMerge is what an update does to one profile copied from the
// template: the files it writes or removes, and those both the user and the
// template changed in the same place.
type ProfileMerge struct {
	Profile Profile
	// Write are the profile's files with their merged content; nil
	// removes one.
	Write map[string][]byte
	// Conflicts are the files both changed alike, with the template's
	// version and the merge with markers, when one could be made.
	Conflicts map[string][2][]byte
	// NoBase says the profile was copied before unibox kept a base, so a
	// file both sides have otherwise is a conflict.
	NoBase bool
	theirs map[string][]byte
}

// TemplateUpdate is what unibox template update does: the template's shared
// files, and every profile copied from it, all at once or not at all.
type TemplateUpdate struct {
	Template string
	Shared   TemplateChanges
	Profiles []ProfileMerge
}

func (u TemplateUpdate) Conflicted() bool {
	for _, m := range u.Profiles {
		if len(m.Conflicts) > 0 {
			return true
		}
	}
	return false
}

// PlanUpdate merges each profile copied from the template three ways: the
// base it was copied as, the user's, and the template's now. A file one side
// changed takes that side; one both changed is merged by git merge-file, and
// a conflict there is reported, never written.
func (t Template) PlanUpdate(ctx context.Context, root, template string) (TemplateUpdate, error) {
	u := TemplateUpdate{Template: template}
	var err error
	if u.Shared, err = t.SharedChanges(root, template); err != nil {
		return u, err
	}
	for _, p := range TemplateProfiles(root, template) {
		theirs, err := t.theirs(p)
		if err != nil {
			return u, fmt.Errorf("profile %s: %w", p.Name, err)
		}
		ours := readTree(p.Dir)
		base := readTree(baseDir(root, template, p.Name))
		m := ProfileMerge{Profile: p, Write: map[string][]byte{}, Conflicts: map[string][2][]byte{}, NoBase: len(base) == 0, theirs: theirs}
		names := map[string]bool{}
		for _, tree := range []map[string][]byte{base, ours, theirs} {
			for rel := range tree {
				names[rel] = true
			}
		}
		for _, rel := range sortedKeys(names) {
			b, inBase := base[rel]
			o, inOurs := ours[rel]
			th, inTheirs := theirs[rel]
			same := func(x []byte, inX bool, y []byte, inY bool) bool { return inX == inY && bytes.Equal(x, y) }
			switch {
			case same(o, inOurs, th, inTheirs):
				// Alike already.
			case !m.NoBase && same(th, inTheirs, b, inBase):
				// Only the user changed it.
			case !m.NoBase && same(o, inOurs, b, inBase):
				if inTheirs {
					m.Write[rel] = th
				} else {
					m.Write[rel] = nil
				}
			case m.NoBase || !inBase || !inOurs || !inTheirs:
				m.Conflicts[rel] = [2][]byte{th, nil}
			default:
				merged, clean, err := mergeFile(ctx, o, b, th)
				if err != nil {
					return u, err
				}
				if clean {
					m.Write[rel] = merged
				} else {
					m.Conflicts[rel] = [2][]byte{th, merged}
				}
			}
		}
		u.Profiles = append(u.Profiles, m)
	}
	return u, nil
}

// mergeFile merges the user's file and the template's from their base with
// git merge-file; clean says no part conflicted.
func mergeFile(ctx context.Context, ours, base, theirs []byte) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "unibox-merge-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)
	paths := []string{}
	for i, data := range [][]byte{ours, base, theirs} {
		path := filepath.Join(dir, fmt.Sprint(i))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, false, err
		}
		paths = append(paths, path)
	}
	cmd := exec.CommandContext(ctx, "git", "merge-file", "-p", "-L", "yours", "-L", "as copied", "-L", "template", paths[0], paths[1], paths[2])
	out, err := cmd.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out, true, nil
	case errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() < 128:
		return out, false, nil
	}
	return nil, false, fmt.Errorf("git merge-file failed: %w", err)
}

// WriteConflicts leaves what the user needs to resolve each conflict beside
// the template, never in the profile, which keeps working as it was: the
// template's version as <file>.template and a merge with markers as
// <file>.merge. The ones of an earlier update go first.
func (u TemplateUpdate) WriteConflicts(root string) ([]string, error) {
	written := []string{}
	for _, m := range u.Profiles {
		dir := conflictDir(root, u.Template, m.Profile.Name)
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		for _, rel := range sortedKeys(m.Conflicts) {
			c := m.Conflicts[rel]
			if c[0] != nil {
				path := filepath.Join(dir, filepath.FromSlash(rel)+".template")
				if err := AtomicWrite(path, c[0], 0o644); err != nil {
					return nil, err
				}
				written = append(written, path)
			}
			if c[1] != nil {
				path := filepath.Join(dir, filepath.FromSlash(rel)+".merge")
				if err := AtomicWrite(path, c[1], 0o644); err != nil {
					return nil, err
				}
				written = append(written, path)
			}
		}
	}
	return written, nil
}

// Apply writes the update: the merged files of every profile, its new base,
// and the template's shared files. Every profile must load afterwards, or
// all of it is put back as it was.
func (u TemplateUpdate) Apply(root string, t Template) error {
	if u.Conflicted() {
		return fmt.Errorf("the update of template %s has conflicts to resolve first", u.Template)
	}
	type saved struct {
		path string
		data []byte
		mode os.FileMode
		was  bool
	}
	undo := []saved{}
	restore := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			s := undo[i]
			if s.was {
				_ = AtomicWrite(s.path, s.data, s.mode)
			} else {
				_ = os.Remove(s.path)
			}
		}
	}
	for _, m := range u.Profiles {
		for _, rel := range sortedKeys(m.Write) {
			path := filepath.Join(m.Profile.Dir, filepath.FromSlash(rel))
			s := saved{path: path, mode: 0o644}
			if info, err := os.Stat(path); err == nil {
				s.data, _ = os.ReadFile(path)
				s.mode, s.was = info.Mode().Perm(), true
			}
			undo = append(undo, s)
			var err error
			if m.Write[rel] == nil {
				err = os.Remove(path)
			} else {
				mode := s.mode
				if th, ok := m.theirs[rel]; ok && !s.was && th != nil {
					mode = 0o644
				}
				err = AtomicWrite(path, m.Write[rel], mode)
			}
			if err != nil {
				restore()
				return err
			}
		}
	}
	for _, m := range u.Profiles {
		if _, err := LoadProfile(root, m.Profile.Name); err != nil {
			restore()
			return fmt.Errorf("profile %s would not load after the update, so nothing was written: %w", m.Profile.Name, err)
		}
	}
	if err := t.WriteShared(root, u.Template); err != nil {
		restore()
		return err
	}
	for _, m := range u.Profiles {
		if err := writeTree(baseDir(root, u.Template, m.Profile.Name), m.theirs, nil); err != nil {
			return err
		}
		_ = os.RemoveAll(conflictDir(root, u.Template, m.Profile.Name))
	}
	// Gone with the last conflict; one still there keeps it.
	_ = os.Remove(filepath.Join(templateDir(root, u.Template), "conflicts"))
	return nil
}
