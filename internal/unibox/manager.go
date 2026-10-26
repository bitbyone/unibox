package unibox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

// Manager keeps unibox's sandboxes: it creates each from an env with sbx,
// and owns what sbx does not know of it - its tasks, their mounts and
// sessions, and the ports the user named.
type Manager struct {
	Paths  Paths
	Config Config
	SBX    Runtime
	Log    io.Writer
	// In, Out and Err are the user's terminal, which sbx env create and
	// sbx secret set talk to directly.
	In       io.Reader
	Out, Err io.Writer
	// kits are the kits read so far, by reference and args: reading one
	// from a registry takes seconds, and a list drawn again after each
	// action asks for the same ones.
	kits sync.Map
}

func Open(paths Paths, log io.Writer) (*Manager, error) {
	c, err := paths.LoadConfig()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = io.Discard
	}
	if err := paths.retireState(log); err != nil {
		return nil, err
	}
	sbx := &SBX{}
	on, _ := c.SSHAgent()
	sbx.AllowSSHAgent(on)
	return &Manager{Paths: paths, Config: c, SBX: sbx, Log: log, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, nil
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
		return b, Fail(4, "sandbox_not_found", "sandbox %s is not unibox's; unibox sandbox ls lists them, unibox sandbox create makes one", name)
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

// TaskSandbox names the sandbox holding a task. A task's name is unique
// across sandboxes, so naming the task is enough to find it.
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

// Definition is the sandbox a request works in and the env it is, or is
// to be, created from. A sandbox unibox knows keeps its env; one it does
// not is made from the env asked for, named as the env names it unless
// the request names it.
func (m *Manager) Definition(req Request) (api.Sandbox, Env, error) {
	if req.Sandbox == "" && req.Env == "" && req.Task != "" {
		name, err := m.taskSandbox(m.taskID(req.Task), len(req.Repositories) > 0)
		if err != nil {
			return api.Sandbox{}, Env{}, err
		}
		req.Sandbox = name
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return api.Sandbox{}, Env{}, err
	}
	name := req.Sandbox
	if name == "" && req.Env != "" {
		env, err := m.Config.LoadEnv(req.Env)
		if err != nil {
			return api.Sandbox{}, Env{}, err
		}
		name = env.SandboxName()
	}
	if name == "" {
		return api.Sandbox{}, Env{}, fmt.Errorf("no sandbox chosen; pass -s (unibox sandbox ls lists them) or -e to work in an env's (unibox env ls), or set one with unibox context use")
	}
	if err := ValidName(name); err != nil {
		return api.Sandbox{}, Env{}, err
	}
	if b, ok := state.Sandboxes[name]; ok {
		if req.Env != "" && req.Env != b.Env {
			return b, Env{}, fmt.Errorf("sandbox %s is made from env %s, not %s", name, b.Env, req.Env)
		}
		env, err := m.Config.LoadEnv(b.Env)
		if err != nil {
			return b, env, fmt.Errorf("sandbox %s: %w", name, err)
		}
		return b, env, nil
	}
	if req.Env == "" {
		return api.Sandbox{}, Env{}, fmt.Errorf("sandbox %s does not exist; pass -e to create it from an env (unibox env ls lists them)", name)
	}
	env, err := m.Config.LoadEnv(req.Env)
	if err != nil {
		return api.Sandbox{}, Env{}, err
	}
	b := api.Sandbox{Name: name, Env: env.Name, Files: env.Config.Files, State: "missing", Tasks: []api.Task{}, Runs: []api.Run{}}
	return b, env, nil
}

// args are the person's values for an env's args, those it declares, as
// they go to sbx, and a failure naming what is missing or refused.
func (m *Manager) args(env Env) (map[string]string, error) {
	values, err := m.Paths.LoadEnvValues()
	if err != nil {
		return nil, err
	}
	mine := values.ArgsFor(env)
	if missing := env.SBX.Missing(mine); len(missing) > 0 {
		a := env.SBX.Args[missing[0]]
		hint := ""
		if a.Description != "" {
			hint = " (" + a.Description + ")"
		}
		return nil, Fail(4, "arg_missing", "env %s needs %s%s; set it with unibox var set -e %s %s <value>", env.Name, strings.Join(missing, ", "), hint, env.Name, missing[0])
	}
	if _, err := env.SBX.CheckArgs(mine); err != nil {
		return nil, Fail(4, "arg_invalid", "env %s: %v", env.Name, err)
	}
	return env.SBX.Declared(mine), nil
}

// resources are what the person gives an env's sandboxes of this host,
// resolved into what sbx takes; a cap is said to log.
func (m *Manager) resources(env Env, log io.Writer) (Resources, error) {
	values, err := m.Paths.LoadEnvValues()
	if err != nil {
		return Resources{}, err
	}
	r, err := values.ResourcesFor(env.Name).Resolve(HostResources(), log)
	if err != nil {
		return r, Fail(4, "resources_invalid", "env %s: %v", env.Name, err)
	}
	return r, nil
}

// fingerprint stands for what a sandbox of the env would be created from
// now, args and resources included.
func (m *Manager) fingerprint(env Env, args map[string]string) (string, error) {
	r, err := m.resources(env, io.Discard)
	if err != nil {
		return "", err
	}
	return env.SBX.Fingerprint(args, r)
}

// createFiles are what sbx env create and plan read for an env: the
// person's args in a file, and the env's files with one of unibox's last
// giving the resources. done removes what was written.
func (m *Manager) createFiles(env Env, args map[string]string, log io.Writer) (argsFile string, files []string, done func(), err error) {
	removes := []func(){}
	done = func() {
		for _, f := range removes {
			f()
		}
	}
	if len(args) > 0 {
		f, remove, err := WriteArgsFile(filepath.Join(m.Paths.Cache, "args"), args)
		if err != nil {
			return "", nil, done, err
		}
		removes = append(removes, remove)
		argsFile = f
	}
	r, err := m.resources(env, log)
	if err != nil {
		return "", nil, done, err
	}
	f, remove, err := WriteResourcesFile(filepath.Join(m.Paths.Cache, "args"), r)
	if err != nil {
		return "", nil, done, err
	}
	removes = append(removes, remove)
	files = slices.Clone(env.SBX.Files)
	if f != "" {
		files = append(files, f)
	}
	return argsFile, files, done, nil
}

// Plan has sbx check an env's files with the person's args, changing
// nothing, and gives the line saying what creating or updating its
// sandbox would do; a failure carries sbx's first line of error.
func (m *Manager) Plan(ctx context.Context, env Env) (string, error) {
	args, err := m.args(env)
	if err != nil {
		return "", err
	}
	file, files, done, err := m.createFiles(env, args, io.Discard)
	defer done()
	if err != nil {
		return "", err
	}
	out, err := m.SBX.EnvPlan(ctx, env.SandboxName(), file, files)
	if err != nil {
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return "", fmt.Errorf("%s", strings.TrimPrefix(strings.TrimPrefix(line, "Error: "), "error: "))
			}
		}
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "to destroy") {
			return strings.TrimSpace(line), nil
		}
	}
	return "nothing to do", nil
}

// Check says what a request needs before its task can start.
func (m *Manager) Check(ctx context.Context, req Request) (api.Check, error) {
	req.Task = m.taskID(req.Task)
	result := api.Check{APIVersion: api.Version, Ready: true, Needs: []api.Need{}, Repositories: []api.Repository{}}
	b, env, err := m.Definition(req)
	if err != nil {
		return result, err
	}
	if err = m.SBX.Available(ctx); err != nil {
		result.Needs = append(result.Needs, api.Need{Code: "runtime_unavailable", Message: err.Error()})
	}
	if args, err := m.args(env); err != nil {
		code := "arg_missing"
		if f, ok := err.(*Failure); ok {
			code = f.Code
		}
		result.Needs = append(result.Needs, api.Need{Code: code, Message: err.Error()})
	} else if b.Fingerprint != "" {
		if fp, err := m.fingerprint(env, args); err == nil && fp != b.Fingerprint {
			result.Needs = append(result.Needs, api.Need{Code: "env_changed", Warning: true, Message: fmt.Sprintf("env %s changed since sandbox %s was created; unibox sandbox recreate %s applies it", env.Name, b.Name, b.Name)})
		}
	}
	req.decide()
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

// Box is a sandbox as sbx lists it.
func (m *Manager) findBox(ctx context.Context, b api.Sandbox) (Box, bool, error) {
	boxes, err := m.SBX.List(ctx)
	if err != nil {
		return Box{}, false, err
	}
	for _, v := range boxes {
		if v.Name == b.Name {
			if b.ID != "" && v.ID != "" && b.ID != v.ID {
				return v, true, fmt.Errorf("sandbox %s was replaced outside unibox; inspect it, then unibox sandbox rm --force %s forgets it", b.Name, b.Name)
			}
			return v, true, nil
		}
	}
	return Box{}, false, nil
}

// ensure brings a sandbox to where a session can start in it: created from
// its env when sbx has none, recreated when asked, its ports as the env
// says. A change of the env is told, never applied unasked: recreating
// discards what the sandbox holds outside its tasks.
func (m *Manager) ensure(ctx context.Context, b *api.Sandbox, env Env, recreate bool) error {
	if err := m.SBX.Available(ctx); err != nil {
		return err
	}
	args, err := m.args(env)
	if err != nil {
		return err
	}
	fingerprint, err := m.fingerprint(env, args)
	if err != nil {
		return fmt.Errorf("env %s: %w", env.Name, err)
	}
	box, exists, err := m.findBox(ctx, *b)
	if err != nil {
		return err
	}
	if exists && b.State == "missing" {
		return Fail(4, "sandbox_taken", "sbx has a sandbox %s that unibox did not create; unibox sandbox create --adopt %s -e %s takes it over, or name another with -s", b.Name, b.Name, env.Name)
	}
	if exists && recreate {
		exclusive, err := m.usage(b.Name, false)
		if err != nil {
			return fmt.Errorf("sandbox %s is in use; retry after its sessions finish", b.Name)
		}
		defer exclusive.Close()
		if err = m.safeBox(ctx, *b); err != nil {
			return err
		}
		fmt.Fprintf(m.Log, "removing sandbox %s to create it again from env %s\n", b.Name, env.Name)
		if err = m.removeBox(ctx, *b); err != nil {
			return err
		}
		exists = false
	}
	switch {
	case !exists:
		if err = m.create(ctx, b, env, args, fingerprint); err != nil {
			return err
		}
	case b.State == "creating":
		// sbx made it, and unibox stopped before readying it.
		if err = m.ready(ctx, b, env, box, fingerprint); err != nil {
			return err
		}
	default:
		if b.Fingerprint != fingerprint {
			fmt.Fprintf(m.Log, "env %s changed since sandbox %s was created; unibox sandbox recreate %s applies it\n", env.Name, b.Name, b.Name)
		}
		// Ports switched for a sandbox last while it runs; it starts with
		// the env's.
		if box.State != "running" && (len(b.PortSwitches) > 0 || len(b.PortMaps) > 0) {
			b.PortSwitches, b.PortMaps = nil, nil
		}
	}
	if err = m.syncPorts(ctx, b, env); err != nil {
		return err
	}
	return m.saveBox(*b)
}

// create has sbx create a sandbox from its env, on the user's terminal,
// and readies it for tasks.
func (m *Manager) create(ctx context.Context, b *api.Sandbox, env Env, args map[string]string, fingerprint string) error {
	file, files, done, err := m.createFiles(env, args, m.Log)
	defer done()
	if err != nil {
		return err
	}
	b.State = "creating"
	b.ID, b.Fingerprint = "", ""
	b.Env, b.Files = env.Name, env.SBX.Files
	if err := m.saveBox(*b); err != nil {
		return err
	}
	fmt.Fprintf(m.Log, "creating sandbox %s from env %s\n", b.Name, env.Name)
	if err := m.SBX.EnvCreate(ctx, b.Name, file, files, m.In, m.Out, m.Err); err != nil {
		return err
	}
	box, ok, err := m.findBox(ctx, *b)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("sbx did not list the new sandbox %s; inspect it with sbx ls", b.Name)
	}
	return m.ready(ctx, b, env, box, fingerprint)
}

// ready makes a sandbox sbx created ready for tasks, and records it.
func (m *Manager) ready(ctx context.Context, b *api.Sandbox, env Env, box Box, fingerprint string) error {
	b.ID = box.ID
	if err := runLogged(m.SBX.Exec(ctx, b.Name, Command{User: "root", Args: []string{"bash", "-c", setupScript}}), m.Log); err != nil {
		return err
	}
	b.Agent, b.AgentCommand, b.Agents = m.agentOf(ctx, env)
	for i := range b.Tasks {
		b.Tasks[i].State = "pending"
	}
	b.Fingerprint = fingerprint
	b.Ports, b.PortSwitches, b.PortMaps = nil, nil, nil
	b.State = "running"
	return m.saveBox(*b)
}

// agentOf reads which agent an env's sandbox runs and how: the workload
// kit's entrypoint and default arguments. The kit the env's agent names is
// read first; the first kit of the sandbox kind is the workload, the
// default agent, and every such kit's agent is one a task may choose.
func (m *Manager) agentOf(ctx context.Context, env Env) (string, []string, map[string][]string) {
	kits := slices.Clone(env.SBX.Kits)
	slices.SortStableFunc(kits, func(a, b EnvKit) int {
		ma, mb := kitName(a.Source) == env.SBX.Agent, kitName(b.Source) == env.SBX.Agent
		switch {
		case ma && !mb:
			return -1
		case mb && !ma:
			return 1
		}
		return 0
	})
	values, _ := m.Paths.LoadEnvValues()
	id, command, all := "", []string(nil), map[string][]string{}
	for _, k := range kits {
		ref := k.Source
		if k.Dir != "" {
			ref = k.Dir
		}
		kit, err := m.SBX.KitInspect(ctx, ref, env.SBX.KitArgs(k, values.ArgsFor(env)))
		if err != nil {
			fmt.Fprintf(m.Log, "%v\n", err)
			continue
		}
		if kit.Kind != "sandbox" || len(kit.Sandbox.Entrypoint) == 0 {
			continue
		}
		c := append(slices.Clone(kit.Sandbox.Entrypoint), kit.Sandbox.Command.Default...)
		a := AgentFor(kit.Sandbox.Entrypoint).ID
		if _, seen := all[a]; seen {
			continue
		}
		all[a] = c
		if id == "" {
			id, command = a, c
		}
	}
	if id == "" {
		fmt.Fprintf(m.Log, "no kit of env %s runs an agent; unibox run --shell opens a shell instead\n", env.Name)
		return "", nil, nil
	}
	return id, command, all
}

// kitName is a kit's name as an sbxenv's agent names it: the last part of
// its reference, without a tag or digest.
func kitName(source string) string {
	name := path.Base(source)
	if i := strings.IndexAny(name, ":@"); i >= 0 {
		name = name[:i]
	}
	return name
}

// Create makes a sandbox from an env without a task in it. Adopt records a
// sandbox sbx has already, made outside unibox, which is then removed only
// when forced.
func (m *Manager) Create(ctx context.Context, req Request, adopt bool) (api.Sandbox, error) {
	b, env, err := m.Definition(req)
	if err != nil {
		return b, err
	}
	l, err := m.guard(b.Name)
	if err != nil {
		return b, err
	}
	defer l.Close()
	// A creation that did not finish is tried again.
	if b.State != "missing" && b.State != "creating" {
		return b, fmt.Errorf("sandbox %s exists already; unibox run works in it, unibox sandbox recreate creates it again", b.Name)
	}
	if adopt {
		box, ok, err := m.findBox(ctx, b)
		if err != nil {
			return b, err
		}
		if !ok {
			return b, fmt.Errorf("sbx has no sandbox %s to adopt; leave out --adopt to create it", b.Name)
		}
		b.ID, b.Adopted, b.State = box.ID, true, box.State
		if args, err := m.args(env); err == nil {
			b.Fingerprint, _ = m.fingerprint(env, args)
		}
		if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{User: "root", Args: []string{"bash", "-c", setupScript}}), m.Log); err != nil {
			return b, err
		}
		b.Agent, b.AgentCommand, b.Agents = m.agentOf(ctx, env)
		return b, m.saveBox(b)
	}
	return b, m.ensure(ctx, &b, env, false)
}

// safeBox refuses while a session runs in the sandbox or a task's clone
// holds work only the sandbox has.
func (m *Manager) safeBox(ctx context.Context, b api.Sandbox) error {
	if err := runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(guestIdleScript)}), m.Log); err != nil {
		return Fail(4, "guest_busy", "sandbox %s may still have a running guest session; inspect it with unibox sandbox shell", b.Name)
	}
	for _, t := range b.Tasks {
		if err := runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(safeToRemoveScript(t))}), m.Log); err != nil {
			return Fail(4, "unpushed_work", "task %s may contain unpushed work; preserve it before recreating or removing the sandbox", t.ID)
		}
	}
	return nil
}

// removeBox takes a sandbox out of sbx: its tasks' host directories first,
// so nothing of the host goes with it, then the sandbox through its env,
// which takes what sbx made for it alone. A sandbox whose env files are
// gone is removed by its name.
func (m *Manager) removeBox(ctx context.Context, b api.Sandbox) error {
	for _, t := range b.Tasks {
		if err := m.unmountTask(ctx, b, t); err != nil {
			return err
		}
	}
	if !b.Adopted && len(b.Files) > 0 {
		readable := true
		for _, f := range b.Files {
			if _, err := os.Stat(f); err != nil {
				readable = false
			}
		}
		if readable {
			file := ""
			if env, err := m.Config.LoadEnv(b.Env); err == nil {
				if args, err := m.args(env); err == nil && len(args) > 0 {
					f, remove, err := WriteArgsFile(filepath.Join(m.Paths.Cache, "args"), args)
					if err == nil {
						defer remove()
						file = f
					}
				}
			}
			if err := m.SBX.EnvRemove(ctx, b.Name, file, b.Files, m.Log); err == nil {
				return nil
			}
			fmt.Fprintf(m.Log, "sbx env rm did not remove sandbox %s; removing it by its name\n", b.Name)
		}
	}
	return m.SBX.Remove(ctx, b.Name)
}

// Recreate creates a sandbox again from its env as it is now, and puts its
// tasks back: their clones are checked for unpushed work first, and made
// again; their host directories are mounted again.
func (m *Manager) Recreate(ctx context.Context, name string) error {
	l, err := m.guard(name)
	if err != nil {
		return err
	}
	defer l.Close()
	b, env, err := m.Definition(Request{Sandbox: name})
	if err != nil {
		return err
	}
	if b.Adopted {
		return fmt.Errorf("sandbox %s was adopted, so unibox does not know how it was made; recreate it with sbx, or remove it with unibox sandbox rm --force", name)
	}
	if err = m.ensure(ctx, &b, env, true); err != nil {
		return err
	}
	return m.restoreTasks(ctx, &b)
}

// restoreTasks prepares a sandbox's tasks again after it was created anew.
func (m *Manager) restoreTasks(ctx context.Context, b *api.Sandbox) error {
	for i := range b.Tasks {
		if err := m.mountTask(ctx, *b, b.Tasks[i]); err != nil {
			return err
		}
		if err := runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(prepareScript(b.Tasks[i]))}), m.Log); err != nil {
			return err
		}
		b.Tasks[i].State = "ready"
		if err := m.saveBox(*b); err != nil {
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
	boxes, listErr := m.SBX.List(ctx)
	out := []api.Sandbox{}
	for _, b := range s.Sandboxes {
		i := slices.IndexFunc(boxes, func(v Box) bool { return v.Name == b.Name })
		switch {
		case listErr != nil:
			b.State = "unknown"
		case i < 0:
			b.State = "missing"
		default:
			b.State = boxes[i].State
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
