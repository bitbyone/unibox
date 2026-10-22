package unibox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

func taskID(repos []api.Repository) string {
	name := "task"
	if len(repos) > 0 {
		name = repos[0].Name
		if repos[0].HostPath != "" {
			candidate := strings.ToLower(filepath.Base(repos[0].HostPath))
			if ValidName(candidate) == nil {
				name = candidate
			}
		}
	}
	if len(name) > 40 {
		name = name[:40]
	}
	type identity struct{ URL, Branch, Host string }
	ids := []identity{}
	for _, r := range repos {
		ids = append(ids, identity{r.URL, r.Branch, r.HostPath})
	}
	return name + "-" + digest(ids)[:8]
}
func checkNeeds(needs []api.Need) error {
	for _, n := range needs {
		if !n.Warning {
			return Fail(4, n.Code, "%s: %s", n.Repository, n.Message)
		}
	}
	return nil
}

func (m *Manager) task(ctx context.Context, req Request, b *api.Sandbox, p Profile, resume bool) (api.Task, error) {
	// A task resumed by name already knows its agent; asking again would
	// make the record pointless.
	if req.Agent == "" && req.Task != "" && len(req.Repositories) == 0 {
		for _, t := range b.Tasks {
			if t.ID == req.Task {
				req.Agent = t.Agent
			}
		}
	}
	if req.Agent == "" {
		// A task opened in a shell still names the agent a later resume
		// runs: the profile's first.
		if len(p.Agents) == 1 || req.Shell && len(p.Agents) > 0 {
			req.Agent = p.Agents[0]
		} else {
			return api.Task{}, fmt.Errorf("profile %s offers %s; pass --agent", p.Name, strings.Join(p.Agents, ", "))
		}
	}
	if !slices.Contains(p.Agents, req.Agent) {
		return api.Task{}, fmt.Errorf("profile %s does not offer agent %s", p.Name, req.Agent)
	}
	if len(req.Repositories) == 0 && req.Task != "" {
		for _, t := range b.Tasks {
			if t.ID == req.Task {
				t.Agent = req.Agent
				return t, nil
			}
		}
		return api.Task{}, fmt.Errorf("task %s does not exist; supply --repo to prepare it", req.Task)
	}
	if len(req.Repositories) == 0 {
		return api.Task{}, fmt.Errorf("no repository selected; pass --repo <path-or-url>, or unibox resume <task> for a prepared one")
	}
	req.services = p.Services
	repos, err := resolveForTask(ctx, req)
	if err != nil {
		return api.Task{}, err
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return api.Task{}, err
	}
	id := req.Task
	if id == "" {
		id = taskID(repos)
		// The same repositories in another sandbox are another task, and a
		// task's name has to say which one without naming the sandbox.
		if other := taskOwner(state, id); other != "" && other != b.Name {
			id = id[:strings.LastIndex(id, "-")+1] + digest([]string{id, b.Name})[:8]
		}
	}
	if err = ValidName(id); err != nil {
		return api.Task{}, err
	}
	if other := taskOwner(state, id); other != "" && other != b.Name {
		return api.Task{}, fmt.Errorf("task %s already exists in sandbox %s; choose another --task name", id, other)
	}
	t := api.Task{ID: id, Sandbox: b.Name, Directory: "/work/" + id, Repositories: repos, Agent: req.Agent, State: "pending"}
	for _, old := range b.Tasks {
		if old.ID == id {
			// A task grown by add-repo is still the task its first
			// repositories name; they find it with all it holds now.
			if digest(old.Repositories) != digest(repos) {
				if !containsRepositories(old.Repositories, repos) {
					return t, fmt.Errorf("task %s belongs to different repositories, branches or authors; choose another --task name", id)
				}
				t.Repositories = old.Repositories
			}
			if resume && old.State == "ready" {
				old.Agent = req.Agent
				return old, nil
			}
		}
	}
	return t, nil
}

// taskOwner names the sandbox holding a task, "" when none does.
func taskOwner(s State, id string) string {
	for name, b := range s.Sandboxes {
		for _, t := range b.Tasks {
			if t.ID == id {
				return name
			}
		}
	}
	return ""
}

// claimTask saves a sandbox with a new task in one step with the check that
// no other sandbox has taken the name meanwhile; each sandbox's guard only
// serializes its own preparations.
func (m *Manager) claimTask(b api.Sandbox, id string) error {
	return m.Paths.State(func(s *State) error {
		for name, other := range s.Sandboxes {
			if name == b.Name {
				continue
			}
			for _, t := range other.Tasks {
				if t.ID == id {
					return fmt.Errorf("task %s already exists in sandbox %s; choose another --task name", id, name)
				}
			}
		}
		s.Sandboxes[b.Name] = b
		return nil
	})
}

func replaceTask(b *api.Sandbox, t api.Task) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == t.ID {
			b.Tasks[i] = t
			return
		}
	}
	b.Tasks = append(b.Tasks, t)
}

// resolveForTask turns a request's repositories into the forge clones a task
// holds, pushing first when asked, and refuses what would lose local work.
func resolveForTask(ctx context.Context, req Request) ([]api.Repository, error) {
	repos, needs, err := ResolveRepositories(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Push {
		if err = PushRepositories(ctx, repos); err != nil {
			return nil, err
		}
		repos, needs, err = ResolveRepositories(ctx, req)
		if err != nil {
			return nil, err
		}
	}
	if err = checkNeeds(needs); err != nil {
		return nil, err
	}
	if req.Mount {
		if err = switchMounted(ctx, repos); err != nil {
			return nil, err
		}
	}
	return repos, nil
}

// containsRepositories says whether every one of want is among have, as it is.
func containsRepositories(have, want []api.Repository) bool {
	for _, w := range want {
		if !slices.ContainsFunc(have, func(h api.Repository) bool { return digest(h) == digest(w) }) {
			return false
		}
	}
	return true
}

// AddRepositories clones more repositories into a task that is already
// prepared, beside the ones it holds. Nothing is mounted: each is an
// independent forge clone in the task's directory, so an agent running in
// the task sees it at once and needs no restart. The task lock is left to
// the session that holds it; the sandbox guard serializes this with other
// preparations, and the task's record grows so removal still checks every
// clone for unpushed work.
func (m *Manager) AddRepositories(ctx context.Context, req Request) (api.Task, []api.Repository, error) {
	req.Task = m.taskID(req.Task)
	if req.Task == "" {
		return api.Task{}, nil, fmt.Errorf("name the task to add to; unibox task ls lists them")
	}
	if len(req.Repositories) == 0 {
		return api.Task{}, nil, fmt.Errorf("no repository selected; pass --repo <path-or-url>")
	}
	name, err := m.taskSandbox(req.Task, false)
	if err != nil {
		return api.Task{}, nil, err
	}
	req.Sandbox = name
	_, r, p, err := m.Definition(req)
	if err != nil {
		return api.Task{}, nil, err
	}
	guard, err := m.guard(name)
	if err != nil {
		return api.Task{}, nil, err
	}
	defer guard.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return api.Task{}, nil, err
	}
	var t api.Task
	for _, v := range b.Tasks {
		if v.ID == req.Task {
			t = v
		}
	}
	if t.ID == "" {
		return api.Task{}, nil, fmt.Errorf("task %s does not exist; unibox task ls lists them", req.Task)
	}
	req.services = p.Services
	repos, err := resolveForTask(ctx, req)
	if err != nil {
		return t, nil, err
	}
	added := []api.Repository{}
	for _, nr := range repos {
		i := slices.IndexFunc(t.Repositories, func(o api.Repository) bool { return o.Name == nr.Name })
		switch {
		case i < 0:
			added = append(added, nr)
		case digest(t.Repositories[i]) != digest(nr):
			return t, nil, fmt.Errorf("task %s already has a repository named %s; give the new one another name with --repo name=source", t.ID, nr.Name)
		}
	}
	if len(added) == 0 {
		return t, added, nil
	}
	if err = m.ensure(ctx, &b, r, p, false, false); err != nil {
		return t, nil, err
	}
	usage, err := m.usage(b.Name, true)
	if err != nil {
		return t, nil, err
	}
	defer usage.Close()
	a, _ := FindAgent(t.Agent)
	part := t
	part.Repositories = added
	fmt.Fprintf(m.Log, "adding %d repositories to task %s\n", len(added), t.ID)
	if err = m.mountTask(ctx, b, r, part); err != nil {
		return t, nil, err
	}
	if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(prepareScript(part, p, a))}), m.Log); err != nil {
		return t, nil, err
	}
	t.Repositories = append(t.Repositories, added...)
	// Only this task's record changes; a session started meanwhile has
	// recorded its run in the same state and must keep it.
	err = m.Paths.State(func(s *State) error {
		box := s.Sandboxes[b.Name]
		for _, v := range box.Tasks {
			if v.ID == t.ID {
				v.Repositories = t.Repositories
				replaceTask(&box, v)
				t = v
			}
		}
		s.Sandboxes[b.Name] = box
		return nil
	})
	return t, added, err
}

// The guard serializes preparation and lifecycle changes. Usage locks survive
// the guard while sessions run, allowing other tasks to prepare beside them.
func (m *Manager) prepare(ctx context.Context, req Request, resume bool) (api.Sandbox, api.Task, Runtime, *FileLock, *FileLock, error) {
	req.Task = m.taskID(req.Task)
	b, r, p, err := m.Definition(req)
	if err != nil {
		return b, api.Task{}, r, nil, nil, err
	}
	guard, err := m.guard(b.Name)
	if err != nil {
		return b, api.Task{}, r, nil, nil, err
	}
	defer guard.Close()
	state, err := m.Paths.ReadState()
	if err != nil {
		return b, api.Task{}, r, nil, nil, err
	}
	if old, ok := state.Sandboxes[b.Name]; ok {
		b = old
	} else {
		if _, exists, err := m.findBox(ctx, r, b); err != nil {
			return b, api.Task{}, r, nil, nil, err
		} else if exists {
			return b, api.Task{}, r, nil, nil, fmt.Errorf("runtime sandbox %s is not owned by unibox; choose another name", b.RuntimeName)
		}
	}
	t, err := m.task(ctx, req, &b, p, resume)
	if err != nil {
		return b, t, r, nil, nil, err
	}
	taskLock, err := LockFile(filepath.Join(m.Paths.Config, "locks", b.Name+"-task-"+t.ID+".lock"), false, false)
	if err != nil {
		// A shell joins a task whose agent works in it: the task is ready,
		// so nothing is prepared that the agent could be in the middle of.
		if !(resume && req.Shell && t.State == "ready") {
			return b, t, r, nil, nil, fmt.Errorf("task %s already has an active session; wait for it to finish, or join it with unibox resume %s --shell", t.ID, t.ID)
		}
		fmt.Fprintf(m.Log, "joining task %s beside its running session\n", t.ID)
		taskLock = nil
	}
	success := false
	var usage *FileLock
	defer func() {
		if !success {
			taskLock.Close()
			if usage != nil {
				usage.Close()
			}
		}
	}()
	if err = m.ensure(ctx, &b, r, p, false, false); err != nil {
		return b, t, r, nil, nil, err
	}
	usage, err = m.usage(b.Name, true)
	if err != nil {
		return b, t, r, nil, nil, err
	}
	// A recreation made every task pending, including a previously ready one.
	for _, saved := range b.Tasks {
		if saved.ID == t.ID && saved.State != "ready" {
			t.State = "pending"
		}
	}
	if !resume || t.State != "ready" {
		t.State = "preparing"
		replaceTask(&b, t)
		if err = m.claimTask(b, t.ID); err != nil {
			return b, t, r, nil, nil, err
		}
		a, _ := FindAgent(t.Agent)
		fmt.Fprintf(m.Log, "preparing task %s\n", t.ID)
		if err = m.mountTask(ctx, b, r, t); err != nil {
			return b, t, r, nil, nil, err
		}
		if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-s"}, Input: strings.NewReader(prepareScript(t, p, a))}), m.Log); err != nil {
			return b, t, r, nil, nil, err
		}
		t.State = "ready"
	}
	replaceTask(&b, t)
	if err = m.saveBox(b); err != nil {
		return b, t, r, nil, nil, err
	}
	success = true
	return b, t, r, usage, taskLock, nil
}

func (m *Manager) Prepare(ctx context.Context, req Request) (api.Prepared, error) {
	b, t, _, usage, taskLock, err := m.prepare(ctx, req, false)
	if err != nil {
		return api.Prepared{}, err
	}
	defer usage.Close()
	defer taskLock.Close()
	return api.Prepared{APIVersion: api.Version, Sandbox: b.Name, Runtime: b.Runtime, RuntimeName: b.RuntimeName, Task: t.ID, Directory: t.Directory, State: "ready"}, nil
}

func (m *Manager) recordRun(b api.Sandbox, t api.Task, agent string) (api.Run, *FileLock, error) {
	run := api.Run{ID: newRunID(), Sandbox: b.Name, Task: t.ID, Agent: agent, PID: os.Getpid(), Pane: os.Getenv("HERDR_PANE_ID"), Socket: os.Getenv("HERDR_SOCKET_PATH"), Since: time.Now()}
	l, err := m.runLock(run.ID)
	if err != nil {
		return run, nil, err
	}
	err = m.Paths.State(func(s *State) error {
		box := s.Sandboxes[b.Name]
		active := []api.Run{}
		for _, old := range box.Runs {
			if m.runActive(old) {
				active = append(active, old)
			}
		}
		box.Runs = append(active, run)
		s.Sandboxes[b.Name] = box
		return nil
	})
	if err != nil {
		l.Close()
		return run, nil, err
	}
	return run, l, nil
}

func (m *Manager) finishRun(b api.Sandbox, r Runtime, run api.Run, lease, usage, taskLock *FileLock) {
	lease.Close()
	usage.Close()
	if taskLock != nil {
		taskLock.Close()
	}
	if err := m.Paths.State(func(s *State) error {
		box, ok := s.Sandboxes[b.Name]
		if !ok {
			return nil
		}
		box.Runs = slices.DeleteFunc(box.Runs, func(v api.Run) bool { return v.ID == run.ID })
		s.Sandboxes[b.Name] = box
		return nil
	}); err != nil {
		fmt.Fprintln(m.Log, "cannot update session record:", err)
	}
	if !m.Config.StopIdle(r.Name()) {
		return
	}
	guard, err := m.guard(b.Name)
	if err != nil {
		return
	}
	defer guard.Close()
	exclusive, err := m.usage(b.Name, false)
	if err != nil {
		return
	}
	defer exclusive.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := runtimeOutput(ctx, r, b.RuntimeName, "sh", "-c", guestIdleScript); err != nil {
		fmt.Fprintln(m.Log, "sandbox kept running: a guest session may still be active")
		return
	}
	if err = r.Stop(ctx, b.RuntimeName); err != nil {
		fmt.Fprintln(m.Log, "cannot stop idle sandbox:", err)
	} else {
		fmt.Fprintf(m.Log, "sandbox %s stopped: no active sessions\n", b.Name)
	}
}

func RunChild(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			return err
		case sig := <-sigs:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}
}

func (m *Manager) Run(ctx context.Context, req Request, in io.Reader, out, errout io.Writer, extra []string) error {
	if req.Shell && len(extra) > 0 {
		return fmt.Errorf("a shell takes no agent arguments; leave out what follows --")
	}
	if len(req.Scripts) > 0 {
		_, _, p, err := m.Definition(req)
		if err != nil {
			return err
		}
		if err = p.knownScripts(req.Scripts); err != nil {
			return err
		}
	}
	b, t, r, usage, taskLock, err := m.prepare(ctx, req, true)
	if err != nil {
		return err
	}
	agent := t.Agent
	if req.Shell {
		agent = ""
	}
	run, lease, err := m.recordRun(b, t, agent)
	if err != nil {
		usage.Close()
		taskLock.Close()
		return err
	}
	defer m.finishRun(b, r, run, lease, usage, taskLock)
	p, err := m.profile(b.Profile)
	if err != nil {
		return err
	}
	dir := t.Directory
	if len(t.Repositories) == 1 {
		dir = path.Join(dir, t.Repositories[0].Name)
	}
	if len(req.Scripts) > 0 {
		env := append([]string{"UNIBOX_SANDBOX=" + b.Name, "UNIBOX_TASK=" + t.ID, "UNIBOX_TASK_DIR=" + t.Directory}, p.SessionEnv()...)
		if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", scriptsScript(p, req.Scripts, env)}, Directory: dir}), errout); err != nil {
			what := t.Agent
			if req.Shell {
				what = "the shell"
			}
			return fmt.Errorf("a script failed, so %s was not started: %w", what, err)
		}
	}
	if req.Shell {
		// A shell is the user's own session: no agent is credited in its
		// commits.
		return RunChild(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", sessionScript(run.ID, withEnv(append(p.SessionEnv(), sshEnv(r)...), []string{"bash", "-l"}))}, Directory: dir, Interactive: true, Input: in, Output: out, Error: errout}))
	}
	a, _ := FindAgent(t.Agent)
	args, agentEnv := p.AgentCommand(a)
	args = append(args, extra...)
	// The commit hook credits the agent of this session, not the one the
	// task was prepared with, as the profile names it.
	name, _, _ := strings.Cut(a.CoAuthor, " <")
	env := append(agentEnv, "UNIBOX_AGENT="+name, "UNIBOX_CO_AUTHOR="+p.CoAuthor(a.ID))
	env = append(env, p.SessionEnv()...)
	if s, ok := r.(SessionEnver); ok {
		env = append(env, s.SessionEnv(p, a.ID)...)
	}
	args = withEnv(env, args)
	if a.Trust != nil && a.Config != "" {
		dirs := []string{"/work", t.Directory}
		for _, repo := range t.Repositories {
			dirs = append(dirs, path.Join(t.Directory, repo.Name))
		}
		if err := m.mergeAgentConfig(ctx, b, r, a, a.Trust(dirs)); err != nil {
			fmt.Fprintf(errout, "unibox: %s may ask whether to trust the directory: %v\n", a.ID, err)
		}
	}
	script := sessionScript(run.ID, args)
	if a.Setup != "" {
		script = "(\n" + a.Setup + ") || echo 'unibox: preparing " + a.ID + " failed; it may ask for a login' >&2\n" + script
	}
	cmd := r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", script}, Directory: dir, Interactive: true, Input: in, Output: out, Error: errout})
	err = RunChild(cmd)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if code < 0 {
			code = 128
		}
		return &Failure{code, "agent_exit", fmt.Sprintf("agent exited with status %d", code)}
	}
	return err
}

// withEnv runs a command with the variables set, through env(1).
func withEnv(env, args []string) []string {
	if len(env) == 0 {
		return args
	}
	return append(append([]string{"env"}, env...), args...)
}

func (m *Manager) Shell(ctx context.Context, name string, in io.Reader, out, errout io.Writer) error {
	guard, err := m.guard(name)
	if err != nil {
		return err
	}
	b, err := m.sandbox(name)
	if err != nil {
		guard.Close()
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		guard.Close()
		return err
	}
	if err = r.Available(ctx); err != nil {
		guard.Close()
		return err
	}
	box, exists, err := m.findBox(ctx, r, b)
	if err != nil || !exists {
		guard.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("sandbox is missing; run unibox sandbox recreate %s", name)
	}
	if err = m.startBox(ctx, r, b, box); err != nil {
		guard.Close()
		return err
	}
	usage, err := m.usage(name, true)
	guard.Close()
	if err != nil {
		return err
	}
	run, lease, err := m.recordRun(b, api.Task{}, "")
	if err != nil {
		usage.Close()
		return err
	}
	defer m.finishRun(b, r, run, lease, usage, nil)
	env := sshEnv(r)
	if p, err := m.profile(b.Profile); err == nil {
		env = append(p.SessionEnv(), env...)
	}
	return RunChild(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", sessionScript(run.ID, withEnv(env, []string{"bash", "-l"}))}, Interactive: true, Input: in, Output: out, Error: errout}))
}

// RunScripts runs a profile's scripts in its sandbox at once, in /work,
// counted as a session as a shell is, so an idle sandbox is stopped after.
func (m *Manager) RunScripts(ctx context.Context, name string, scripts []string, out io.Writer) error {
	guard, err := m.guard(name)
	if err != nil {
		return err
	}
	b, err := m.sandbox(name)
	if err != nil {
		guard.Close()
		return err
	}
	p, err := m.profile(b.Profile)
	if err == nil {
		err = p.knownScripts(scripts)
	}
	if err != nil {
		guard.Close()
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		guard.Close()
		return err
	}
	if err = r.Available(ctx); err != nil {
		guard.Close()
		return err
	}
	box, exists, err := m.findBox(ctx, r, b)
	if err != nil || !exists {
		guard.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("sandbox is missing; run unibox sandbox recreate %s", name)
	}
	if err = m.startBox(ctx, r, b, box); err != nil {
		guard.Close()
		return err
	}
	usage, err := m.usage(name, true)
	guard.Close()
	if err != nil {
		return err
	}
	run, lease, err := m.recordRun(b, api.Task{}, "")
	if err != nil {
		usage.Close()
		return err
	}
	defer m.finishRun(b, r, run, lease, usage, nil)
	env := append([]string{"UNIBOX_SANDBOX=" + b.Name}, p.SessionEnv()...)
	return runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", sessionScript(run.ID, []string{"sh", "-c", scriptsScript(p, scripts, env)})}, Directory: "/work"}), out)
}

// SandboxScripts are the scripts of a sandbox's profile, by name.
func (m *Manager) SandboxScripts(name string) (string, map[string]string, error) {
	b, err := m.sandbox(name)
	if err != nil {
		return "", nil, err
	}
	p, err := m.profile(b.Profile)
	if err != nil {
		return "", nil, err
	}
	return p.Name, p.Scripts, nil
}

func (m *Manager) Stop(ctx context.Context, name string) error {
	l, err := m.guard(name)
	if err != nil {
		return err
	}
	defer l.Close()
	u, err := m.usage(name, false)
	if err != nil {
		return err
	}
	defer u.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return err
	}
	box, exists, err := m.findBox(ctx, r, b)
	if err != nil {
		return err
	}
	if !exists || box.State == "stopped" {
		return nil
	}
	if _, err = runtimeOutput(ctx, r, b.RuntimeName, "sh", "-c", guestIdleScript); err != nil {
		return Fail(4, "guest_busy", "a guest session may still be active; inspect it with unibox sandbox shell")
	}
	return r.Stop(ctx, b.RuntimeName)
}
func (m *Manager) Remove(ctx context.Context, name, task string) error {
	return m.remove(ctx, name, task, false)
}

// remove takes a task or a sandbox away. Forced, it neither waits for the
// sessions nor checks for unpushed work: what is in the sandbox goes, the
// host's mounted directories never.
func (m *Manager) remove(ctx context.Context, name, task string, force bool) error {
	task = m.taskID(task)
	l, err := m.guard(name)
	if err != nil {
		return err
	}
	defer l.Close()
	u, err := m.usage(name, false)
	if err != nil && !force {
		return err
	}
	if u != nil {
		defer u.Close()
	}
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return err
	}
	_, exists, err := m.findBox(ctx, r, b)
	if err != nil {
		return err
	}
	if exists && !force {
		if err = m.safeBox(ctx, b, r); err != nil {
			return err
		}
	}
	if task != "" {
		if err = ValidName(task); err != nil {
			return err
		}
		index := slices.IndexFunc(b.Tasks, func(t api.Task) bool { return t.ID == task })
		if index < 0 {
			return fmt.Errorf("task %s not found", task)
		}
		removed := b.Tasks[index]
		if exists {
			// Mounted directories come out before anything is deleted, and
			// the guest checks they did: rm -rf would go into a mount and
			// delete the host's files.
			if err = m.unmountTask(ctx, b, r, removed); err != nil {
				return err
			}
			if s := unmountedScript(removed); s != "" {
				if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"sh", "-c", s}}), m.Log); err != nil {
					return err
				}
			}
			if err = runLogged(r.Exec(ctx, b.RuntimeName, Command{Args: []string{"rm", "-rf", "--", removed.Directory}}), m.Log); err != nil {
				return err
			}
		}
		b.Tasks = slices.Delete(b.Tasks, index, index+1)
		return m.saveBox(b)
	}
	if exists {
		if err = r.Remove(ctx, b.RuntimeName); err != nil {
			return err
		}
	}
	return m.Paths.State(func(s *State) error { delete(s.Sandboxes, name); return nil })
}

// mounts are a task's mounted repositories: each host directory and the
// place in the task it is mounted at.
func mounts(t api.Task) [][2]string {
	out := [][2]string{}
	for _, r := range t.Repositories {
		if r.Mounted {
			out = append(out, [2]string{r.HostPath, path.Join(t.Directory, r.Name)})
		}
	}
	return out
}

// mountTask mounts a task's host directories into its sandbox, at their
// places in the task, before the task is prepared there.
func (m *Manager) mountTask(ctx context.Context, b api.Sandbox, r Runtime, t api.Task) error {
	list := mounts(t)
	if len(list) == 0 {
		return nil
	}
	mounter, ok := r.(Mounter)
	if !ok {
		return fmt.Errorf("runtime %s cannot mount host directories; leave out --mount", r.Name())
	}
	for _, mt := range list {
		if err := mounter.Mount(ctx, b.RuntimeName, mt[0], mt[1]); err != nil {
			return err
		}
		fmt.Fprintf(m.Log, "mounted %s at %s in sandbox %s\n", mt[0], mt[1], b.Name)
	}
	return nil
}

// unmountTask takes a task's host directories out of its sandbox.
func (m *Manager) unmountTask(ctx context.Context, b api.Sandbox, r Runtime, t api.Task) error {
	list := mounts(t)
	if len(list) == 0 {
		return nil
	}
	mounter, ok := r.(Mounter)
	if !ok {
		return fmt.Errorf("runtime %s cannot unmount host directories; the task is kept so nothing of the host is deleted", r.Name())
	}
	for _, mt := range list {
		if err := mounter.Unmount(ctx, b.RuntimeName, mt[0], mt[1]); err != nil {
			return fmt.Errorf("%w; the task is kept so nothing of the host is deleted", err)
		}
		fmt.Fprintf(m.Log, "unmounted %s from sandbox %s\n", mt[0], b.Name)
	}
	return nil
}
