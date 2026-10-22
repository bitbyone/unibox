package unibox

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/tobola/unibox/internal/unibox/api"
)

type Manager struct {
	Paths    Paths
	Config   Config
	Runtimes map[string]Runtime
	Log      io.Writer

	managed    sync.Once
	managedErr error
}

// syncManaged brings the environments repository's managed files to this
// unibox's own once per command, before a profile is read for a check, a
// build or a task, so an image is built from them. A repository init did
// not lay out is left alone.
func (m *Manager) syncManaged() error {
	m.managed.Do(func() {
		root := m.Config.Environments
		if root == "" || !LaidOut(root) {
			return
		}
		written, updated, removed, err := SyncManaged(root)
		if err != nil {
			m.managedErr = fmt.Errorf("cannot bring the environments repository's managed files to unibox's own: %w", err)
			return
		}
		for _, rel := range append(written, updated...) {
			fmt.Fprintf(m.Log, "updated %s to unibox's own\n", rel)
		}
		for _, rel := range removed {
			fmt.Fprintf(m.Log, "removed %s: unibox no longer has it\n", rel)
		}
	})
	return m.managedErr
}

func Open(paths Paths, log io.Writer) (*Manager, error) {
	c, err := paths.LoadConfig()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = io.Discard
	}
	sbx := &SBX{Cache: filepath.Join(paths.Cache, "sbx")}
	sbx.SSHAgent, _ = c.SSHAgent("sbx")
	if home, err := os.UserHomeDir(); err == nil {
		sbx.Bindings = filepath.Join(home, ".config", "sbx", "credentials.yaml")
	}
	msb := &MSB{Cache: filepath.Join(paths.Cache, "msb"), Log: log}
	return &Manager{Paths: paths, Config: c, Runtimes: map[string]Runtime{"sbx": sbx, "msb": msb}, Log: log}, nil
}

// runtimeOrder is the order a new sandbox's runtime is chosen in when none
// is set. An experimental runtime is never chosen for the user; it is used
// when set as default_runtime or asked for with --runtime.
var runtimeOrder = []string{"sbx"}

func (m *Manager) Runtime(name string) (Runtime, error) {
	if name == "" {
		name = m.Config.DefaultRuntime
	}
	if name == "" {
		name = m.defaultRuntime()
		if name == "" {
			return nil, fmt.Errorf("sbx is not installed; install Docker Sandboxes and run sbx login, or try the experimental msb runtime with unibox config set default_runtime msb")
		}
	}
	r, ok := m.Runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime %q is not supported; unibox runtime ls lists them", name)
	}
	if !m.Config.Enabled(name) {
		return nil, fmt.Errorf("runtime %s is disabled; enable it with unibox config set runtimes.%s.on true", name, name)
	}
	return r, nil
}

// DefaultRuntime is the runtime a new sandbox gets when none is asked for:
// the configured one, else the first enabled, present and supported one.
func (m *Manager) DefaultRuntime() string {
	if m.Config.DefaultRuntime != "" {
		return m.Config.DefaultRuntime
	}
	return m.defaultRuntime()
}

// defaultRuntime is the first enabled runtime that is present and not
// experimental, in runtimeOrder, then by name.
func (m *Manager) defaultRuntime() string {
	order := slices.Clone(runtimeOrder)
	for _, id := range sortedKeys(m.Runtimes) {
		if !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for _, id := range order {
		r, ok := m.Runtimes[id]
		if !ok || !m.Config.Enabled(id) {
			continue
		}
		if p, ok := r.(Present); ok && !p.Present() {
			continue
		}
		if IsExperimental(r) {
			continue
		}
		return id
	}
	return ""
}

// profile loads a profile with its person's values in it.
func (m *Manager) profile(name string) (Profile, error) {
	p, err := LoadProfile(m.Config.Environments, name)
	if err != nil {
		return p, err
	}
	vars, err := m.Paths.LoadVars()
	if err != nil {
		return p, err
	}
	return p.withSecrets(vars).withVars(vars.For(p.Name))
}

// Environments says where the environments repository is, or, when there is
// none to use, what to do about it.
func (m *Manager) Environments() (string, error) {
	root := m.Config.Environments
	if root == "" {
		return "", fmt.Errorf("no environments repository is configured; start with unibox init --dir <path>, or unibox init -i to be asked")
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return root, fmt.Errorf("environments repository %s does not exist; unibox init --dir %s lays it out again, or unibox config unset environments forgets it", root, root)
	}
	return root, nil
}

// Profile loads a profile of the environments repository.
func (m *Manager) Profile(name string) (Profile, error) { return m.profile(name) }

// RecordAccess remembers a profile's host access as seen, so the next
// change of it is told against this one.
func (m *Manager) RecordAccess(name string) error {
	p, err := m.profile(name)
	if err != nil {
		return err
	}
	return m.Paths.State(func(s *State) error {
		s.Approvals[p.ApprovalKey()] = p.AccessHash()
		s.ApprovedFields[p.ApprovalKey()] = p.HostAccess
		return nil
	})
}
func (m *Manager) Approval(name string) (HostAccess, HostAccess, error) {
	p, err := m.profile(name)
	if err != nil {
		return HostAccess{}, HostAccess{}, err
	}
	s, err := m.Paths.ReadState()
	return s.ApprovedFields[p.ApprovalKey()], p.HostAccess, err
}
func buildKey(p Profile, r Runtime) string { return p.ApprovalKey() + ":" + r.Name() }
func imageTag(p Profile) string            { return "unibox/" + p.Name + ":" + p.Revision[:16] }

func (m *Manager) Built(ctx context.Context, p Profile, r Runtime) (BuildRecord, error) {
	s, err := m.Paths.ReadState()
	if err != nil {
		return BuildRecord{}, err
	}
	record, ok := s.Builds[buildKey(p, r)]
	if !ok || record.Revision != p.Revision {
		return BuildRecord{}, Fail(3, "image_required", "profile %s needs a build; run unibox profile build %s --runtime %s", p.Name, p.Name, r.Name())
	}
	images, err := r.Images(ctx)
	if err != nil {
		return BuildRecord{}, err
	}
	for _, im := range images {
		if im.Reference == imageTag(p) {
			return record, nil
		}
	}
	return BuildRecord{}, Fail(3, "image_required", "profile %s image is missing from %s; run unibox profile build %s", p.Name, r.Name(), p.Name)
}
func (m *Manager) Build(ctx context.Context, name, rt string, force bool) error {
	if err := m.syncManaged(); err != nil {
		return err
	}
	p, err := m.profile(name)
	if err != nil {
		return err
	}
	if err = p.missingVarsError(); err != nil {
		return err
	}
	r, err := m.Runtime(rt)
	if err != nil {
		return err
	}
	l, err := LockFile(filepath.Join(m.Paths.Config, "locks", "build-"+buildKey(p, r)+".lock"), false, false)
	if err != nil {
		return err
	}
	defer l.Close()
	if !force {
		if _, err = m.Built(ctx, p, r); err == nil {
			fmt.Fprintf(m.Log, "image for %s is current\n", name)
			return nil
		}
	}
	fmt.Fprintf(m.Log, "building %s for %s\n", name, r.Name())
	id, err := r.Build(ctx, p, imageTag(p), m.Log)
	if err != nil {
		return err
	}
	if err = m.Paths.State(func(s *State) error { s.Builds[buildKey(p, r)] = BuildRecord{p.Revision, id, p.Files}; return nil }); err != nil {
		return err
	}
	m.pruneBuilds(ctx, r, p)
	return nil
}

func (m *Manager) Profiles(ctx context.Context) ([]api.Profile, error) {
	names, err := ProfileNames(m.Config.Environments)
	if err != nil {
		return nil, err
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return nil, err
	}
	out := []api.Profile{}
	for _, name := range names {
		row := api.Profile{Name: name, Images: map[string]string{}}
		p, err := m.profile(name)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		row.Agents = p.Agents
		row.Revision = p.Revision
		row.Approved = !p.HostFacing() || state.Approvals[p.ApprovalKey()] == p.AccessHash()
		row.LastCommit, _ = git(ctx, p.Root, "log", "-1", "--format=%h %s", "--", name)
		for id, r := range m.Runtimes {
			if !m.Config.Enabled(id) {
				continue
			}
			row.Images[id] = "not built"
			if b, ok := state.Builds[buildKey(p, r)]; ok {
				row.Images[id] = "outdated"
				if b.Revision == p.Revision {
					row.Images[id] = "built"
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (m *Manager) sandbox(name string) (api.Sandbox, error) {
	if err := ValidName(name); err != nil {
		return api.Sandbox{}, err
	}
	s, err := m.Paths.ReadState()
	if err != nil {
		return api.Sandbox{}, err
	}
	b, ok := s.Sandboxes[name]
	if !ok {
		return b, fmt.Errorf("sandbox %s is not registered; create it with unibox sandbox create", name)
	}
	return b, nil
}
func (m *Manager) saveBox(b api.Sandbox) error {
	return m.Paths.State(func(s *State) error { s.Sandboxes[b.Name] = b; return nil })
}
func (m *Manager) guard(name string) (*FileLock, error) {
	return LockFile(filepath.Join(m.Paths.Config, "locks", name+"-guard.lock"), false, false)
}
func (m *Manager) usage(name string, shared bool) (*FileLock, error) {
	return LockFile(filepath.Join(m.Paths.Config, "locks", name+"-usage.lock"), shared, false)
}

// A task's name is unique across sandboxes, so naming the task is enough to
// find it. State from before that rule may still hold one name twice.
// TaskSandbox names the sandbox holding a task.
func (m *Manager) TaskSandbox(task string) (string, error) {
	return m.taskSandbox(m.taskID(task), false)
}

// TaskID is the ID of the task a name says, its own or its alias.
func (m *Manager) TaskID(name string) string { return m.taskID(name) }

// taskID is the ID of the task a name says: its own, or one the user gave
// it as an alias. A name of no task is itself, for a task to be made.
func (m *Manager) taskID(name string) string {
	s, err := m.Paths.ReadState()
	if err != nil || name == "" {
		return name
	}
	for _, b := range s.Sandboxes {
		for _, t := range b.Tasks {
			if t.ID == name {
				return name
			}
		}
	}
	for _, b := range s.Sandboxes {
		for _, t := range b.Tasks {
			if t.Alias == name {
				return t.ID
			}
		}
	}
	return name
}

// AliasTask gives a task a name of the user's, taken wherever its ID is
// and shown in its place; the task keeps its ID and its directory. A name
// is unique across sandboxes, as a task's ID is; the task's own ID takes
// the alias away.
func (m *Manager) AliasTask(task, alias string) (api.Task, error) {
	if err := ValidName(alias); err != nil {
		return api.Task{}, err
	}
	id := m.taskID(task)
	name, err := m.taskSandbox(id, false)
	if err != nil {
		return api.Task{}, err
	}
	if s, err := m.Paths.ReadState(); err == nil {
		for _, b := range s.Sandboxes {
			for _, t := range b.Tasks {
				if t.ID != id && (t.ID == alias || t.Alias == alias) {
					return api.Task{}, fmt.Errorf("%s names task %s already, in sandbox %s; a task's name is unique across sandboxes", alias, t.ID, b.Name)
				}
			}
		}
	}
	guard, err := m.guard(name)
	if err != nil {
		return api.Task{}, err
	}
	defer guard.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return api.Task{}, err
	}
	i := slices.IndexFunc(b.Tasks, func(t api.Task) bool { return t.ID == id })
	if i < 0 {
		return api.Task{}, fmt.Errorf("task %s not found", task)
	}
	b.Tasks[i].Alias = alias
	if alias == id {
		b.Tasks[i].Alias = ""
	}
	if err = m.saveBox(b); err != nil {
		return api.Task{}, err
	}
	return b.Tasks[i], nil
}

func (m *Manager) taskSandbox(task string, preparing bool) (string, error) {
	s, err := m.Paths.ReadState()
	if err != nil {
		return "", err
	}
	found := []string{}
	for name, b := range s.Sandboxes {
		for _, t := range b.Tasks {
			if t.ID == task {
				found = append(found, name)
			}
		}
	}
	slices.Sort(found)
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) > 1:
		return "", fmt.Errorf("task %s exists in sandboxes %s; remove the one not wanted with unibox task rm", task, strings.Join(found, ", "))
	case preparing:
		return "", nil
	}
	return "", fmt.Errorf("task %s does not exist; unibox task ls lists them", task)
}

func (m *Manager) Definition(req Request) (api.Sandbox, Runtime, Profile, error) {
	if err := m.syncManaged(); err != nil {
		return api.Sandbox{}, nil, Profile{}, err
	}
	if req.Sandbox == "" && req.NewSandbox == "" && req.Profile == "" && req.Task != "" {
		name, err := m.taskSandbox(m.taskID(req.Task), len(req.Repositories) > 0)
		if err != nil {
			return api.Sandbox{}, nil, Profile{}, err
		}
		req.Sandbox = name
	}
	name := req.Sandbox
	if name == "" {
		name = req.NewSandbox
	}
	// --sandbox names a sandbox that is there, or one to make from
	// --profile when it is not: one flag, whichever it turns out to be.
	if name != "" && req.NewSandbox == "" && req.Profile != "" {
		if s, err := m.Paths.ReadState(); err == nil {
			if _, known := s.Sandboxes[name]; !known {
				req.NewSandbox = name
			}
		}
	}
	if name != "" && req.NewSandbox == "" {
		b, err := m.sandbox(name)
		if err != nil {
			return b, nil, Profile{}, fmt.Errorf("sandbox %s does not exist; pass -p to make it (unibox profile ls lists them)", name)
		}
		if req.Runtime != "" && req.Runtime != b.Runtime {
			return b, nil, Profile{}, fmt.Errorf("sandbox %s belongs to runtime %s", name, b.Runtime)
		}
		if req.Profile != "" && req.Profile != b.Profile {
			return b, nil, Profile{}, fmt.Errorf("sandbox %s uses profile %s", name, b.Profile)
		}
		r, err := m.Runtime(b.Runtime)
		if err != nil {
			return b, nil, Profile{}, err
		}
		p, err := m.profile(b.Profile)
		return b, r, p, err
	}
	if req.Profile == "" {
		if name == "" {
			return api.Sandbox{}, nil, Profile{}, fmt.Errorf("no sandbox chosen; pass -s (unibox sandbox ls lists them) or -p to make one (unibox profile ls), or set one with unibox context use")
		}
		return api.Sandbox{}, nil, Profile{}, fmt.Errorf("new sandbox %s needs -p; unibox profile ls lists them", name)
	}
	if name == "" {
		name = req.Profile
	}
	if err := ValidName(name); err != nil {
		return api.Sandbox{}, nil, Profile{}, err
	}
	r, err := m.Runtime(req.Runtime)
	if err != nil {
		return api.Sandbox{}, nil, Profile{}, err
	}
	p, err := m.profile(req.Profile)
	if err != nil {
		return api.Sandbox{}, nil, Profile{}, err
	}
	if _, err = p.InstructionText(req.Instructions); err != nil {
		return api.Sandbox{}, nil, Profile{}, err
	}
	b := api.Sandbox{Name: name, Runtime: r.Name(), RuntimeName: "unibox-" + name, Profile: p.Name, Instructions: req.Instructions, State: "missing", Tasks: []api.Task{}, Runs: []api.Run{}}
	return b, r, p, nil
}

func (m *Manager) Check(ctx context.Context, req Request) (api.Check, error) {
	req.Task = m.taskID(req.Task)
	if err := m.syncManaged(); err != nil {
		return api.Check{}, err
	}
	result := api.Check{APIVersion: api.Version, Ready: true, Needs: []api.Need{}, Repositories: []api.Repository{}}
	b, r, p, err := m.Definition(req)
	if err != nil {
		return result, err
	}
	if lines := m.AccessChanges(p); len(lines) > 0 {
		result.Needs = append(result.Needs, api.Need{Code: "profile_changed", Warning: true, Message: fmt.Sprintf("profile %s changed what it asks of the host: %s", p.Name, strings.Join(lines, ", "))})
	}
	if err = r.Available(ctx); err != nil {
		result.Needs = append(result.Needs, api.Need{Code: "runtime_unavailable", Message: err.Error()})
	} else if _, err = m.Built(ctx, p, r); err != nil {
		result.Needs = append(result.Needs, api.Need{Code: "image_required", Message: err.Error()})
	}
	if req.Agent != "" && !slices.Contains(p.Agents, req.Agent) {
		return result, fmt.Errorf("profile %s does not offer agent %s", p.Name, req.Agent)
	}
	// A secret the profile names has to be stored before the runtime can run
	// the command that reads it, so it is checked before the logins are.
	if refs := slices.DeleteFunc(p.SecretRefs(), func(n string) bool { return p.ReadBy(n) != "" }); len(refs) > 0 {
		if store, err := PlatformSecrets(); err != nil {
			result.Needs = append(result.Needs, api.Need{Code: "secret_missing", Message: err.Error()})
		} else {
			for _, name := range refs {
				if ok, err := store.Has(ctx, name); err != nil {
					result.Needs = append(result.Needs, api.Need{Code: "secret_missing", Message: err.Error()})
				} else if !ok {
					result.Needs = append(result.Needs, api.Need{Code: "secret_missing", Message: fmt.Sprintf("profile %s names secret %s, which %s does not hold; run unibox agent login -p %s or unibox secret set %s", p.Name, name, store.Name(), p.Name, name)})
				}
			}
		}
	}
	if err := p.missingVarsError(); err != nil {
		result.Needs = append(result.Needs, api.Need{Code: "var_missing", Message: err.Error()})
	}
	vars, services := p.Unused()
	if len(vars) > 0 {
		result.Needs = append(result.Needs, api.Need{Code: "var_unused", Warning: true, Message: fmt.Sprintf("profile %s declares vars nothing refers to: %s; refer to them as ${vars.<name>} or take them out", p.Name, strings.Join(vars, ", "))})
	}
	for _, s := range services {
		result.Needs = append(result.Needs, api.Need{Code: "service_unused", Service: s, Warning: true, Message: fmt.Sprintf("no variable of profile %s carries service %s's placeholder, so no tool sends its token; add - NAME: ${services.%s} to env", p.Name, s, s)})
	}
	if IsExperimental(r) {
		result.Needs = append(result.Needs, api.Need{Code: "runtime_experimental", Warning: true, Message: fmt.Sprintf("runtime %s is experimental; sbx is the supported runtime", r.Name())})
	}
	if a, ok := r.(Authenticator); ok {
		result.Needs = append(result.Needs, a.AuthNeeds(ctx, b.RuntimeName, p)...)
	}
	// A runtime without a credential proxy cannot keep a service's token off
	// the agent; it may still run, but the user is told the token will be
	// readable inside the sandbox so a narrow, short-lived one is chosen.
	if len(p.Services) > 0 && !r.Capabilities().CredentialProxy {
		for name := range p.Services {
			result.Needs = append(result.Needs, api.Need{Code: "credential_in_container", Service: name, Warning: true, Message: fmt.Sprintf("runtime %s has no credential proxy; the %s token will be readable inside the sandbox - use a narrow, short-lived one", r.Name(), name)})
		}
	}
	req.services = p.Services
	repos, needs, err := ResolveRepositories(ctx, req)
	if err != nil {
		return result, err
	}
	result.Repositories = repos
	result.Needs = append(result.Needs, needs...)
	for _, r := range repos {
		if r.HostPath == "" {
			result.Needs = append(result.Needs, api.Need{Code: "remote_check_pending", Repository: r.Name, Message: "remote branch and credentials will be checked inside the sandbox during preparation", Warning: true})
		}
	}
	for _, n := range result.Needs {
		if !n.Warning {
			result.Ready = false
		}
	}
	return result, nil
}

// Login takes an agent's sign-in as the profile declares it, on the user's
// terminal, through the runtime that keeps it.
func (m *Manager) Login(ctx context.Context, req Request, agent string, in io.Reader, out, errout io.Writer) error {
	_, r, p, err := m.Definition(req)
	if err != nil {
		return err
	}
	a, ok := r.(Authenticator)
	if !ok {
		return fmt.Errorf("runtime %s keeps no logins; sign %s in inside the sandbox", r.Name(), agent)
	}
	auth, ok := p.Auth[agent]
	if !ok {
		return fmt.Errorf("profile %s declares no auth for %s", p.Name, agent)
	}
	return a.Login(ctx, agent, auth, in, out, errout)
}

// startBox starts a stopped sandbox whose runtime does not start one when a
// command runs in it.
func (m *Manager) startBox(ctx context.Context, r Runtime, b api.Sandbox, box Box) error {
	s, ok := r.(Starter)
	if !ok || box.State == "running" {
		return nil
	}
	fmt.Fprintf(m.Log, "starting sandbox %s\n", b.Name)
	return s.Start(ctx, b.RuntimeName, m.Log)
}

func (m *Manager) findBox(ctx context.Context, r Runtime, b api.Sandbox) (Box, bool, error) {
	boxes, err := r.List(ctx)
	if err != nil {
		return Box{}, false, err
	}
	for _, v := range boxes {
		if v.Name == b.RuntimeName {
			if b.RuntimeID != "" && v.ID != "" && b.RuntimeID != v.ID {
				return v, true, fmt.Errorf("sandbox %s was replaced outside unibox; inspect it before removing its unibox registration", b.Name)
			}
			return v, true, nil
		}
	}
	return Box{}, false, nil
}

func (m *Manager) ensure(ctx context.Context, b *api.Sandbox, r Runtime, p Profile, force, rebuild bool) error {
	if err := r.Available(ctx); err != nil {
		return err
	}
	if err := p.missingVarsError(); err != nil {
		return err
	}
	if err := m.noteAccess(p, m.Log); err != nil {
		return err
	}
	if rebuild {
		if err := m.Build(ctx, p.Name, r.Name(), true); err != nil {
			return err
		}
	}
	build, err := m.Built(ctx, p, r)
	if err != nil {
		return err
	}
	instructions, err := p.InstructionText(b.Instructions)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for _, k := range p.Env {
		value, ok := os.LookupEnv(k)
		if !ok {
			return fmt.Errorf("profile requires %s; set it before starting", k)
		}
		env[k] = value
	}
	// Network rules change live and instructions before every session, so
	// neither is part of what makes a sandbox.
	settings := settingsSnapshot(p, r)
	revision := digest(struct {
		Profile json.RawMessage
		Image   string
		Env     map[string]string
	}{settings, build.Image, env})
	instructionRevision := digest(instructions)
	var exclusive *FileLock
	defer func() {
		if exclusive != nil {
			exclusive.Close()
		}
	}()
	box, exists, err := m.findBox(ctx, r, *b)
	if err != nil {
		return err
	}
	// Ports switched for a sandbox last while it runs; it starts with the
	// profile's.
	if (!exists || box.State != "running") && len(b.PortSwitches) > 0 {
		b.PortSwitches = nil
		if err = m.saveBox(*b); err != nil {
			return err
		}
	}
	created := false
	if force || !exists || b.Revision != revision {
		created = true
		exclusive, err = m.usage(b.Name, false)
		if err != nil {
			return fmt.Errorf("sandbox %s needs recreation but is in use; retry after its sessions finish", b.Name)
		}
		if exists {
			if err = m.safeBox(ctx, *b, r); err != nil {
				return err
			}
			reasons := settingsChanges(b.Settings, settings)
			if b.Image != "" && b.Image != build.Image {
				reasons = append(reasons, "the image was rebuilt")
			}
			if force {
				reasons = append(reasons, "asked for")
			}
			if len(reasons) == 0 {
				reasons = append(reasons, "the profile changed")
			}
			fmt.Fprintf(m.Log, "recreating sandbox %s: %s\n", b.Name, strings.Join(reasons, ", "))
			if err = r.Remove(ctx, b.RuntimeName); err != nil {
				return err
			}
		}
		b.RuntimeID = ""
		b.Revision = ""
		b.State = "creating"
		if err = m.saveBox(*b); err != nil {
			return err
		}
		kind := ""
		if len(p.Agents) == 1 {
			kind = p.Agents[0]
		}
		if err = r.Create(ctx, Spec{Name: b.RuntimeName, Image: imageTag(p), Resources: p.Resources, Access: p.HostAccess, Agent: kind, Agents: p.Agents, OAuth: p.OAuthAgent()}, m.Log); err != nil {
			return err
		}
		box, ok, err := m.findBox(ctx, r, *b)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("runtime did not list the new sandbox; inspect it with its own CLI")
		}
		b.RuntimeID = box.ID
		m.pruneKits(ctx, r, b.RuntimeName)
		cmd := r.Exec(ctx, b.RuntimeName, Command{User: "root", Args: []string{"sh", "-c", "set -eu; mkdir -p /work /var/lib/unibox/runs; chown -R agent:agent /work /var/lib/unibox"}})
		if err = runLogged(cmd, m.Log); err != nil {
			return err
		}
		for i := range b.Tasks {
			b.Tasks[i].State = "pending"
		}
		b.Revision = revision
		b.InstructionRevision = ""
		b.Memory = p.Memory
		b.Image, b.Settings = build.Image, settings
		// Create applied the profile's rules; its ports are published below.
		b.NetworkAllow, b.NetworkDeny = networkRules(p)
		b.Ports, b.PortSwitches = nil, nil
		b.State = "running"
		if err = m.saveBox(*b); err != nil {
			return err
		}
	}
	if !created {
		if err = m.startBox(ctx, r, *b, box); err != nil {
			return err
		}
		if f, ok := r.(SecretRefresher); ok && box.State == "running" {
			if err = f.RefreshSecrets(ctx, b.RuntimeName, m.Log); err != nil {
				return err
			}
		}
	}
	if err = m.syncNetwork(ctx, b, r, p); err != nil {
		return err
	}
	if err = m.syncPorts(ctx, b, r, p); err != nil {
		return err
	}
	if b.InstructionRevision != instructionRevision && exclusive == nil {
		exclusive, err = m.usage(b.Name, false)
		if err != nil {
			return fmt.Errorf("sandbox instructions changed while a session is active; retry when it finishes")
		}
	}
	if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(initializeScript(p, instructions))}), m.Log); err != nil {
		return err
	}
	if err = m.seedAgentConfigs(ctx, *b, r, p); err != nil {
		return err
	}
	b.InstructionRevision = instructionRevision
	return m.saveBox(*b)
}

// mergeJSON lays the seed over the current configuration: a key the seed
// names wins, maps merge recursively, a list keeps what the agent added to
// it - a folder trusted during a session - with the seed's items joined,
// and everything else - the agent's own state, projects, paths - stays as
// it is.
func mergeJSON(current, seed map[string]any) map[string]any {
	for key, value := range seed {
		if sm, ok := value.(map[string]any); ok {
			if cm, ok := current[key].(map[string]any); ok {
				current[key] = mergeJSON(cm, sm)
				continue
			}
		}
		if sl, ok := value.([]any); ok {
			if cl, ok := current[key].([]any); ok {
				for _, item := range sl {
					if !slices.ContainsFunc(cl, func(c any) bool { return fmt.Sprint(c) == fmt.Sprint(item) }) {
						cl = append(cl, item)
					}
				}
				current[key] = cl
				continue
			}
		}
		current[key] = value
	}
	return current
}

// splitJSONComments takes the leading // lines off a configuration - copilot
// heads its config.json with them - so the rest parses as JSON and they can
// be put back as they were.
func splitJSONComments(text string) (head, body string) {
	lines := strings.SplitAfter(text, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "//") {
		i++
	}
	return strings.Join(lines[:i], ""), strings.Join(lines[i:], "")
}

func (m *Manager) seedAgentConfigs(ctx context.Context, b api.Sandbox, r Runtime, p Profile) error {
	for _, entry := range sortedKeys(p.AgentConfig) {
		file := p.AgentConfig[entry]
		agent, name := splitAgentConfigKey(entry)
		_, f, err := ConfigFile(agent, name)
		if err != nil {
			return err
		}
		data, err := readInside(p.Root, filepath.Join(p.Dir, file))
		if err != nil {
			return fmt.Errorf("profile %s: config of %s: %w", p.Name, agent, err)
		}
		seed, err := parseSeed(f, data)
		if err != nil {
			return fmt.Errorf("profile %s: config %s of %s: %v", p.Name, file, agent, err)
		}
		if err = m.mergeConfigFile(ctx, b, r, agent, f, seed); err != nil {
			return err
		}
	}
	return nil
}

// mergeAgentConfig lays a seed over the JSON file the agent's Trust goes to.
func (m *Manager) mergeAgentConfig(ctx context.Context, b api.Sandbox, r Runtime, a Agent, seed map[string]any) error {
	return m.mergeConfigFile(ctx, b, r, a.ID, AgentFile{Name: path.Base(a.Config), Path: a.Config}, seed)
}

// mergeConfigFile lays a seed over one of an agent's configuration files in
// the guest home, JSON or TOML, writing only when something changes.
func (m *Manager) mergeConfigFile(ctx context.Context, b api.Sandbox, r Runtime, agent string, f AgentFile, seed map[string]any) error {
	out, err := runtimeOutput(ctx, r, b.RuntimeName, "sh", "-c", "cat \"$HOME/"+f.Path+"\" 2>/dev/null || true")
	if err != nil {
		return err
	}
	var merged []byte
	if f.TOML() {
		merged, err = mergeTOML(out, seed)
	} else {
		merged, err = mergeJSONText(out, seed)
	}
	if err != nil {
		return fmt.Errorf("agent %s configuration in sandbox %s: %v; repair or remove ~/%s before seeding it", agent, b.Name, err, f.Path)
	}
	if merged == nil {
		return nil
	}
	script := "set -eu\nmkdir -p \"$HOME/" + path.Dir(f.Path) + "\"\nprintf %s " + Quote(base64.StdEncoding.EncodeToString(merged)) + " | base64 -d > \"$HOME/" + f.Path + "\"\n"
	return runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(script)}), m.Log)
}

// mergeJSONText lays a seed over a JSON configuration, keeping its leading
// // lines; nil when nothing changes.
func mergeJSONText(text string, seed map[string]any) ([]byte, error) {
	current := map[string]any{}
	head, body := splitJSONComments(text)
	if strings.TrimSpace(head) != "" {
		head = strings.TrimRight(head, "\n") + "\n"
	}
	if strings.TrimSpace(body) != "" {
		if err := json.Unmarshal([]byte(body), &current); err != nil {
			return nil, fmt.Errorf("not valid JSON")
		}
	}
	before, _ := json.Marshal(current)
	mergedMap := mergeJSON(current, seed)
	after, _ := json.Marshal(mergedMap)
	if bytes.Equal(before, after) {
		return nil, nil
	}
	merged, err := json.MarshalIndent(mergedMap, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(head), append(merged, '\n')...), nil
}

// mergeTOML lays a seed over a TOML configuration; nil when nothing
// changes. The blocks between unibox's markers - written before each
// session by the agent's own setup - are kept where they were: one before
// any table stays at the top, the rest go after.
func mergeTOML(text string, seed map[string]any) ([]byte, error) {
	var top, tail, body strings.Builder
	inBlock, table := false, false
	var block *strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# unibox:begin"):
			inBlock = true
			block = &tail
			if !table {
				block = &top
			}
			block.WriteString(line)
		case strings.HasPrefix(trimmed, "# unibox:end"):
			inBlock = false
			block.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				block.WriteString("\n")
			}
		case inBlock:
			block.WriteString(line)
		default:
			if strings.HasPrefix(trimmed, "[") {
				table = true
			}
			body.WriteString(line)
		}
	}
	current := map[string]any{}
	if err := toml.Unmarshal([]byte(body.String()), &current); err != nil {
		return nil, fmt.Errorf("not valid TOML")
	}
	before, _ := json.Marshal(current)
	mergedMap := mergeJSON(current, seed)
	after, _ := json.Marshal(mergedMap)
	if bytes.Equal(before, after) {
		return nil, nil
	}
	merged, err := toml.Marshal(mergedMap)
	if err != nil {
		return nil, err
	}
	return []byte(top.String() + string(merged) + tail.String()), nil
}

func (m *Manager) AttachInstructions(name string, names []string) error {
	guard, err := m.guard(name)
	if err != nil {
		return err
	}
	defer guard.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	p, err := m.profile(b.Profile)
	if err != nil {
		return err
	}
	if _, err = p.InstructionText(names); err != nil {
		return err
	}
	b.Instructions = names
	return m.saveBox(b)
}

func (m *Manager) Create(ctx context.Context, req Request) (api.Sandbox, error) {
	if req.NewSandbox == "" {
		req.NewSandbox = req.Sandbox
		req.Sandbox = ""
	}
	b, r, p, err := m.Definition(req)
	if err != nil {
		return b, err
	}
	l, err := m.guard(b.Name)
	if err != nil {
		return b, err
	}
	defer l.Close()
	s, err := m.Paths.ReadState()
	if err != nil {
		return b, err
	}
	if _, ok := s.Sandboxes[b.Name]; ok {
		return b, fmt.Errorf("sandbox %s already exists; use run or recreate", b.Name)
	}
	if _, exists, err := m.findBox(ctx, r, b); err != nil {
		return b, err
	} else if exists {
		return b, fmt.Errorf("runtime name %s is already in use; choose another sandbox name", b.RuntimeName)
	}
	if err = m.ensure(ctx, &b, r, p, false, false); err != nil {
		return b, err
	}
	return b, nil
}

func (m *Manager) safeBox(ctx context.Context, b api.Sandbox, r Runtime) error {
	// The checks run in the guest, which a runtime that does not start a
	// sandbox on its own has to be started for.
	if box, ok, err := m.findBox(ctx, r, b); err != nil {
		return err
	} else if ok {
		if err = m.startBox(ctx, r, b, box); err != nil {
			return err
		}
	}
	if err := runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(guestIdleScript)}), m.Log); err != nil {
		return Fail(4, "guest_busy", "sandbox %s may still have a running guest session; inspect it with unibox sandbox shell", b.Name)
	}
	for _, t := range b.Tasks {
		if err := runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(safeToRemoveScript(t))}), m.Log); err != nil {
			return Fail(4, "unpushed_work", "task %s may contain unpushed work; preserve it before recreating or removing the sandbox", t.ID)
		}
	}
	return nil
}

func (m *Manager) Recreate(ctx context.Context, name string, rebuild bool) error {
	l, err := m.guard(name)
	if err != nil {
		return err
	}
	defer l.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return err
	}
	p, err := m.profile(b.Profile)
	if err != nil {
		return err
	}
	if !rebuild {
		if _, err = m.Built(ctx, p, r); err != nil {
			if err = m.Build(ctx, p.Name, r.Name(), false); err != nil {
				return err
			}
		}
	}
	if err = m.ensure(ctx, &b, r, p, true, rebuild); err != nil {
		return err
	}
	for i := range b.Tasks {
		a, err := FindAgent(b.Tasks[i].Agent)
		if err != nil {
			return err
		}
		// A recreated sandbox has none of the mounts its tasks had.
		if err = m.mountTask(ctx, b, r, b.Tasks[i]); err != nil {
			return err
		}
		if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(prepareScript(b.Tasks[i], p, a))}), m.Log); err != nil {
			return err
		}
		b.Tasks[i].State = "ready"
		if err = m.saveBox(b); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) List(ctx context.Context) ([]api.Sandbox, error) {
	s, err := m.Paths.ReadState()
	if err != nil {
		return nil, err
	}
	out := []api.Sandbox{}
	for _, b := range s.Sandboxes {
		r, err := m.Runtime(b.Runtime)
		if err != nil {
			b.State = "unavailable"
		} else {
			box, ok, err := m.findBox(ctx, r, b)
			switch {
			case err != nil:
				b.State = "unknown"
			case !ok:
				b.State = "missing"
			default:
				b.State = box.State
				b.Memory = box.Memory
			}
		}
		runs := []api.Run{}
		for _, run := range b.Runs {
			if m.runActive(run) {
				runs = append(runs, run)
			}
		}
		b.Runs = runs
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b api.Sandbox) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *Manager) runLock(id string) (*FileLock, error) {
	return LockFile(filepath.Join(m.Paths.Config, "locks", "run-"+id+".lock"), false, false)
}
func (m *Manager) runActive(run api.Run) bool {
	l, err := m.runLock(run.ID)
	if err != nil {
		return true
	}
	l.Close()
	return false
}
func newRunID() string { return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()) }
