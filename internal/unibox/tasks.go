package unibox

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

func taskID(repos []api.Repository) string { return taskIDNamed("", repos) }

// taskIDNamed is a task's name after the repository named lead, else its
// first, and a digest of them all.
func taskIDNamed(lead string, repos []api.Repository) string {
	name := "task"
	if len(repos) > 0 {
		first := repos[0]
		if i := slices.IndexFunc(repos, func(r api.Repository) bool { return r.Name == lead }); i >= 0 {
			first = repos[i]
		}
		name = first.Name
		if first.HostPath != "" {
			candidate := strings.ToLower(filepath.Base(first.HostPath))
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

func (m *Manager) task(ctx context.Context, req Request, b *api.Sandbox, resume bool) (api.Task, error) {
	if len(req.Repositories) == 0 && req.Task != "" {
		for _, t := range b.Tasks {
			if t.ID == req.Task {
				return t, nil
			}
		}
		return api.Task{}, fmt.Errorf("task %s does not exist; supply --repo to prepare it", req.Task)
	}
	if len(req.Repositories) == 0 {
		return api.Task{}, fmt.Errorf("no repository selected; pass --repo <path-or-url>, or unibox resume <task> for a prepared one")
	}
	repos, err := resolveForTask(ctx, req)
	if err != nil {
		return api.Task{}, err
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return api.Task{}, err
	}
	// A task is named by the repositories asked for; those the env has
	// every task take leave its name be, so a task made before they were
	// there is found still.
	own := req.own(repos)
	id := req.Task
	if id == "" {
		// Named after a repository worked in, not one only read.
		lead := ""
		if i := slices.IndexFunc(own, func(r api.Repository) bool { return !req.contextNames()[r.Name] }); i >= 0 {
			lead = own[i].Name
		}
		id = taskIDNamed(lead, own)
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
	t := api.Task{ID: id, Sandbox: b.Name, Directory: "/work/" + id, Repositories: repos, State: "pending"}
	for _, old := range b.Tasks {
		if old.ID == id {
			// A task grown by add-repo is still the task its first
			// repositories name; they find it with all it holds now.
			// It holds what it was given, where a branch made in the
			// sandbox started included.
			if !containsRepositories(old.Repositories, own) {
				return t, fmt.Errorf("task %s belongs to different repositories, branches or authors; choose another --task name", id)
			}
			t.Repositories = old.Repositories
			if resume && old.State == "ready" {
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
		if err = pushRepositories(ctx, repos, req.contextNames()); err != nil {
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
	// A repository asked alone to push gets its branch on the forge: made
	// there for a clone, pushed from the host for a mounted checkout, once
	// it is on it. One with no forge is refused before a checkout moves.
	push := req.names(func(in RepoInput) bool { return in.Push && !in.Context })
	for _, r := range repos {
		if !push[r.Name] {
			continue
		}
		origin := r.URL != ""
		if r.HostPath != "" {
			_, err := git(ctx, r.HostPath, "remote", "get-url", "origin")
			origin = err == nil
		}
		if !origin {
			return nil, fmt.Errorf("%s has no origin to push branch %s to; leave its push off, or give it an origin", cmp.Or(r.HostPath, r.Name), r.Branch)
		}
	}
	if req.Mount {
		if err = switchMounted(ctx, repos); err != nil {
			return nil, err
		}
	}
	if len(push) > 0 {
		these := slices.DeleteFunc(slices.Clone(repos), func(r api.Repository) bool { return !push[r.Name] })
		if err = pushRepositories(ctx, these, nil); err != nil {
			return nil, err
		}
	}
	return repos, nil
}

// containsRepositories says whether every one of want is among have, as it is.
func containsRepositories(have, want []api.Repository) bool {
	have, want = withoutBase(have), withoutBase(want)
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
	_, env, err := m.Definition(req)
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
	// A task's directories are mounted or cloned as its first ones were;
	// an address, which no mount can take, is cloned beside mounted ones.
	req.Mount = slices.ContainsFunc(t.Repositories, func(r api.Repository) bool { return r.Mounted })
	req.Clone = !req.Mount
	if req.Mount {
		for i, in := range req.Repositories {
			if p, err := ExpandPath(in.Source); err != nil || !isDir(p) {
				req.Repositories[i].Clone = true
			}
		}
	}
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
	if err = m.ensure(ctx, &b, env, false); err != nil {
		return t, nil, err
	}
	usage, err := m.usage(b.Name, true)
	if err != nil {
		return t, nil, err
	}
	defer usage.Close()
	part := t
	part.Repositories = added
	fmt.Fprintf(m.Log, "adding %d repositories to task %s\n", len(added), t.ID)
	if err = m.mountTask(ctx, b, part); err != nil {
		return t, nil, err
	}
	if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(prepareScript(part))}), m.Log); err != nil {
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
		noteRecent(s, b.Env, added, time.Now())
		return nil
	})
	return t, added, err
}

// The guard serializes preparation and lifecycle changes. Usage locks survive
// the guard while sessions run, allowing other tasks to prepare beside them.
func (m *Manager) prepare(ctx context.Context, req Request, resume bool) (api.Sandbox, api.Task, *FileLock, *FileLock, error) {
	req.Task = m.taskID(req.Task)
	b, env, err := m.Definition(req)
	if err != nil {
		return b, api.Task{}, nil, nil, err
	}
	guard, err := m.guard(b.Name)
	if err != nil {
		return b, api.Task{}, nil, nil, err
	}
	defer guard.Close()
	if old, err := m.sandbox(b.Name); err == nil {
		b = old
	}
	req.decide()
	t, err := m.task(ctx, req, &b, resume)
	if err != nil {
		return b, t, nil, nil, err
	}
	taskLock, err := LockFile(filepath.Join(m.Paths.Config, "locks", b.Name+"-task-"+t.ID+".lock"), false, false)
	if err != nil {
		// A shell joins a task whose agent works in it: the task is ready,
		// so nothing is prepared that the agent could be in the middle of.
		if !(resume && req.Shell && t.State == "ready") {
			return b, t, nil, nil, fmt.Errorf("task %s already has an active session; wait for it to finish, or join it with unibox resume %s --shell", t.ID, t.ID)
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
	if err = m.ensure(ctx, &b, env, false); err != nil {
		return b, t, nil, nil, err
	}
	usage, err = m.usage(b.Name, true)
	if err != nil {
		return b, t, nil, nil, err
	}
	// A recreation made every task pending, including a previously ready one.
	for _, saved := range b.Tasks {
		if saved.ID == t.ID && saved.State != "ready" {
			t.State = "pending"
		}
	}
	// The agent asked for, else the one the task ran last, else the env's.
	t.Agent = cmp.Or(req.Agent, t.Agent, b.Agent)
	if !resume || t.State != "ready" {
		t.State = "preparing"
		replaceTask(&b, t)
		if err = m.claimTask(b, t.ID); err != nil {
			return b, t, nil, nil, err
		}
		fmt.Fprintf(m.Log, "preparing task %s\n", t.ID)
		if err = m.mountTask(ctx, b, t); err != nil {
			return b, t, nil, nil, err
		}
		if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(prepareScript(t))}), m.Log); err != nil {
			return b, t, nil, nil, err
		}
		t.State = "ready"
	} else if err = m.remount(ctx, b, t); err != nil {
		return b, t, nil, nil, err
	}
	replaceTask(&b, t)
	// A task's repositories are those the person works with, offered again
	// beside the index the next time one is picked.
	err = m.Paths.State(func(s *State) error {
		s.Sandboxes[b.Name] = b
		noteRecent(s, b.Env, t.Repositories, time.Now())
		return nil
	})
	if err != nil {
		return b, t, nil, nil, err
	}
	success = true
	return b, t, usage, taskLock, nil
}

func (m *Manager) Prepare(ctx context.Context, req Request) (api.Prepared, error) {
	b, t, usage, taskLock, err := m.prepare(ctx, req, false)
	if err != nil {
		return api.Prepared{}, err
	}
	defer usage.Close()
	defer taskLock.Close()
	return api.Prepared{APIVersion: api.Version, Sandbox: b.Name, Task: t.ID, Directory: t.Directory, State: "ready"}, nil
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

func (m *Manager) finishRun(b api.Sandbox, run api.Run, lease, usage, taskLock *FileLock, after func()) {
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
	if after != nil {
		after()
	}
	m.stopIdle(b)
}

// stopIdle stops a sandbox no session uses any longer, when the person
// has unibox do so.
func (m *Manager) stopIdle(b api.Sandbox) {
	if !m.Config.StopIdle() {
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
	if _, err := guestOutput(ctx, m.SBX, b.Name, "bash", "-c", guestIdleScript); err != nil {
		fmt.Fprintln(m.Log, "sandbox kept running: a guest session may still be active")
		return
	}
	if err = m.SBX.Stop(ctx, b.Name); err != nil {
		fmt.Fprintln(m.Log, "cannot stop idle sandbox:", err)
	} else {
		fmt.Fprintf(m.Log, "sandbox %s stopped: no active sessions\n", b.Name)
	}
}

// sshEnv is what a session needs to use the host's SSH agent sbx forwards:
// a host key it has not seen is taken, as the guest has no known_hosts.
func (m *Manager) sshEnv() []string {
	if on, _ := m.Config.SSHAgent(); !on {
		return nil
	}
	return []string{"GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=accept-new"}
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
	b, t, usage, taskLock, err := m.prepare(ctx, req, true)
	if err != nil {
		return err
	}
	var command []string
	agent := ""
	if !req.Shell {
		agent = t.Agent
		if command, err = m.agentCommand(ctx, b, agent); err != nil {
			usage.Close()
			taskLock.Close()
			return err
		}
	}
	run, lease, err := m.recordRun(b, t, agent)
	if err != nil {
		usage.Close()
		taskLock.Close()
		return err
	}
	// A task asked to go goes once its session is over and its lock is
	// let go, before an idle sandbox is stopped; one with work the forge
	// lacks stays, and says so.
	var after func()
	if req.Remove {
		after = func() {
			if err := m.removeTask(context.WithoutCancel(ctx), b.Name, t.ID, false); err != nil {
				fmt.Fprintf(m.Log, "task %s kept: %v; unibox task rm %s removes it once its work is safe\n", t.ID, err, t.ID)
				return
			}
			fmt.Fprintf(m.Log, "removed task %s\n", t.ID)
		}
	}
	defer m.finishRun(b, run, lease, usage, taskLock, after)
	// What the task's agents are told is written before each session; with
	// it, a session starts above the repositories, where AGENTS.md is.
	instructions := ""
	if env, err := m.Config.LoadEnv(b.Env); err == nil {
		instructions = TaskInstructions(t, env)
	}
	dir := sessionDir(t, instructions)
	told := instructionsScript(t, instructions)
	if req.Shell {
		// A shell is the user's own session: no agent is credited in its
		// commits.
		return RunChild(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", told + sessionScript(run.ID, withEnv(m.sshEnv(), []string{"bash", "-l"}))}, Directory: dir, Interactive: true, Input: in, Output: out, Error: errout}))
	}
	a := FindAgent(agent)
	args := append(command, extra...)
	env := slices.Clone(a.Env)
	if a.CoAuthor != "" {
		name, _, _ := strings.Cut(a.CoAuthor, " <")
		env = append(env, "UNIBOX_AGENT="+name, "UNIBOX_CO_AUTHOR="+a.CoAuthor)
	}
	env = append(env, m.sshEnv()...)
	args = withEnv(env, args)
	if a.Trust != nil && a.Config != "" {
		dirs := []string{"/work", t.Directory}
		for _, repo := range t.Repositories {
			dirs = append(dirs, path.Join(t.Directory, repo.Name))
		}
		if err := m.mergeAgentConfig(ctx, b, a, a.Trust(dirs)); err != nil {
			fmt.Fprintf(errout, "unibox: %s may ask whether to trust the directory: %v\n", a.ID, err)
		}
	}
	script := told + sessionScript(run.ID, args)
	if a.Setup != "" {
		script = "(\n" + a.Setup + ") || echo 'unibox: preparing " + a.ID + " failed; it may ask whether to trust the directory' >&2\n" + script
	}
	cmd := m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", script}, Directory: dir, Interactive: true, Input: in, Output: out, Error: errout})
	err = RunChild(cmd)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if code < 0 {
			code = 128
		}
		if code == 125 {
			// sbx 0.50 says so when it could not run the command at all.
			return &Failure{code, "exec_failed", fmt.Sprintf("sbx could not start %s in sandbox %s; unibox doctor says what is wrong", agent, b.Name)}
		}
		return &Failure{code, "agent_exit", fmt.Sprintf("agent exited with status %d", code)}
	}
	return err
}

// agentCommand is how a sandbox starts an agent: as the env's kit for it
// says, else as sbx's own kit would, when the sandbox has the agent's
// command - an image with several agents, say.
func (m *Manager) agentCommand(ctx context.Context, b api.Sandbox, id string) ([]string, error) {
	if id == "" {
		return nil, Fail(4, "agent_missing", "no kit of sandbox %s's env runs an agent; open a shell with --shell", b.Name)
	}
	if id == b.Agent && len(b.AgentCommand) > 0 {
		return slices.Clone(b.AgentCommand), nil
	}
	if c, ok := b.Agents[id]; ok {
		return slices.Clone(c), nil
	}
	a := FindAgent(id)
	if err := ValidName(a.Command); err != nil {
		return nil, fmt.Errorf("agent %q: %w", id, err)
	}
	if m.SBX.Exec(ctx, b.Name, Command{Args: []string{"sh", "-c", "command -v " + a.Command}}).Run() != nil {
		known := slices.Sorted(maps.Keys(b.Agents))
		if len(known) == 0 && b.Agent != "" {
			known = []string{b.Agent}
		}
		return nil, Fail(4, "agent_missing", "sandbox %s has no %s; its env runs %s - pick one with --agent, or have the team add a kit for %s", b.Name, id, strings.Join(known, ", "), id)
	}
	return append([]string{a.Command}, a.Args...), nil
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
	if err = m.SBX.Available(ctx); err != nil {
		guard.Close()
		return err
	}
	_, exists, err := m.findBox(ctx, b)
	if err != nil || !exists {
		guard.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("sbx has no sandbox %s; unibox sandbox recreate %s creates it again", name, name)
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
	defer m.finishRun(b, run, lease, usage, nil, nil)
	return RunChild(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", sessionScript(run.ID, withEnv(m.sshEnv(), []string{"bash", "-l"}))}, Interactive: true, Input: in, Output: out, Error: errout}))
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
	box, exists, err := m.findBox(ctx, b)
	if err != nil {
		return err
	}
	if !exists || box.State == "stopped" {
		return nil
	}
	if _, err = guestOutput(ctx, m.SBX, b.Name, "bash", "-c", guestIdleScript); err != nil {
		return Fail(4, "guest_busy", "a guest session may still be active; inspect it with unibox sandbox shell")
	}
	return m.SBX.Stop(ctx, b.Name)
}
func (m *Manager) Remove(ctx context.Context, name, task string, force bool) error {
	return m.remove(ctx, name, task, force)
}

// remove takes a task or a sandbox away. Forced, it neither waits for the
// sessions nor checks for unpushed work: what is in the sandbox goes, the
// host's mounted directories never.
func (m *Manager) remove(ctx context.Context, name, task string, force bool) error {
	task = m.taskID(task)
	if task != "" {
		return m.removeTask(ctx, name, task, force)
	}
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
	if task == "" && b.Adopted && !force {
		return fmt.Errorf("sandbox %s was adopted, made outside unibox; unibox sandbox rm --force %s removes it", name, name)
	}
	_, exists, err := m.findBox(ctx, b)
	if err != nil && !force {
		return err
	}
	if exists && !force {
		if err = m.safeBox(ctx, b); err != nil {
			return err
		}
	}
	if exists {
		if err = m.removeBox(ctx, b); err != nil {
			return err
		}
	}
	if err = m.Paths.State(func(s *State) error { delete(s.Sandboxes, name); return nil }); err != nil {
		return err
	}
	// Resources and network rules given to this sandbox alone go with it.
	if err = m.forgetNetwork(name); err != nil {
		return err
	}
	if v, err := m.Paths.LoadEnvValues(); err == nil && v.SandboxResources[name] != (Resources{}) {
		return m.Paths.SetSandboxResources(name, Resources{})
	}
	return nil
}

// removeTask takes a task out of its sandbox while other tasks there may
// work on: it waits for no sandbox, only for its own session to be over,
// and checks its own clones for unpushed work. Forced, it checks neither.
func (m *Manager) removeTask(ctx context.Context, name, task string, force bool) error {
	if err := ValidName(task); err != nil {
		return err
	}
	l, err := m.guard(name)
	if err != nil {
		return err
	}
	defer l.Close()
	own, err := LockFile(filepath.Join(m.Paths.Config, "locks", name+"-task-"+task+".lock"), false, false)
	if err != nil && !force {
		return fmt.Errorf("task %s has an active session; finish it first, or remove it with --force", task)
	}
	if own != nil {
		defer own.Close()
	}
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	_, exists, err := m.findBox(ctx, b)
	if err != nil && !force {
		return err
	}
	if !force {
		for _, r := range b.Runs {
			if r.Task == task && m.runActive(r) {
				return fmt.Errorf("task %s has an active session; finish it first, or remove it with --force", task)
			}
		}
	}
	index := slices.IndexFunc(b.Tasks, func(t api.Task) bool { return t.ID == task })
	if index < 0 {
		return fmt.Errorf("task %s not found", task)
	}
	removed := b.Tasks[index]
	// Its clones are checked for unpushed work before anything goes.
	if exists && !force {
		if err := runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(safeToRemoveScript(removed))}), m.Log); err != nil {
			return Fail(4, "unpushed_work", "task %s may contain unpushed work; preserve it before removing it", task)
		}
	}
	if exists {
		// Mounted directories come out before anything is deleted, and
		// the guest checks they did: rm -rf would go into a mount and
		// delete the host's files.
		if err = m.unmountTask(ctx, b, removed); err != nil {
			return err
		}
		if s := unmountedScript(removed); s != "" {
			if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", s}}), m.Log); err != nil {
				return err
			}
			// sbx made the mount points as root; rmdir takes only
			// empty ones.
			points := []string{"bash", "-c", `for d; do [ ! -d "$d" ] || rmdir -- "$d"; done`, "bash"}
			for _, mt := range mounts(removed) {
				points = append(points, mt[1])
			}
			if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{User: "root", Args: points}), m.Log); err != nil {
				return err
			}
		}
		if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"rm", "-rf", "--", removed.Directory}}), m.Log); err != nil {
			return err
		}
	}
	// Only this task's record goes; a session started meanwhile in another
	// task of the sandbox has recorded its run in the same state.
	return m.Paths.State(func(s *State) error {
		box := s.Sandboxes[name]
		box.Tasks = slices.DeleteFunc(box.Tasks, func(t api.Task) bool { return t.ID == task })
		s.Sandboxes[name] = box
		return nil
	})
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

// MountDrift compares what a running sandbox has mounted under /work
// with its ready tasks' mounts: missing are a task's mounts the sandbox
// lacks, which the task's next start makes again, and stray the mount
// points no task has, each as host path and target.
func (m *Manager) MountDrift(ctx context.Context, b api.Sandbox) (missing, stray [][2]string, err error) {
	info, err := guestOutput(ctx, m.SBX, b.Name, "cat", "/proc/self/mountinfo")
	if err != nil {
		return nil, nil, err
	}
	found := guestMounts(info)
	mounted := map[string]bool{}
	for _, mt := range found {
		mounted[mt[1]] = true
	}
	known := map[string]bool{}
	for _, t := range b.Tasks {
		for _, mt := range mounts(t) {
			known[mt[1]] = true
			if t.State == "ready" && !mounted[mt[1]] {
				missing = append(missing, mt)
			}
		}
	}
	for _, mt := range found {
		if !known[mt[1]] {
			stray = append(stray, mt)
		}
	}
	return missing, stray, nil
}

// remount mounts again what a ready task has lost of its mounts, as
// after sbx umount by hand, so its agent never starts on an empty
// directory.
func (m *Manager) remount(ctx context.Context, b api.Sandbox, t api.Task) error {
	if len(mounts(t)) == 0 {
		return nil
	}
	missing, _, err := m.MountDrift(ctx, b)
	if err != nil {
		return err
	}
	for _, mt := range missing {
		if !slices.Contains(mounts(t), mt) {
			continue
		}
		if err := m.SBX.Mount(ctx, b.Name, mt[0], mt[1]); err != nil {
			return err
		}
		fmt.Fprintf(m.Log, "mounted %s at %s in sandbox %s again\n", mt[0], mt[1], b.Name)
	}
	return nil
}

// guestMounts are the mounts under /work in a mountinfo, where unibox
// mounts tasks' directories, as host path and target: sbx shares a host
// directory with its path as the mount's root. mountinfo escapes a space
// and the like as an octal \NNN.
func guestMounts(info string) [][2]string {
	out := [][2]string{}
	for _, line := range strings.Split(info, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		mt := [2]string{unescapeMountinfo(f[3]), unescapeMountinfo(f[4])}
		if strings.HasPrefix(mt[1], "/work/") && !slices.Contains(out, mt) {
			out = append(out, mt)
		}
	}
	return out
}

func unescapeMountinfo(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// mountTask mounts a task's host directories into its sandbox, at their
// places in the task, before the task is prepared there.
func (m *Manager) mountTask(ctx context.Context, b api.Sandbox, t api.Task) error {
	// Made by the agent first, the task's directory stays the agent's: sbx
	// makes a mount's missing parents as root.
	if len(mounts(t)) > 0 {
		if err := runQuiet(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"mkdir", "-p", t.Directory}})); err != nil {
			return err
		}
	}
	for _, mt := range mounts(t) {
		if err := m.SBX.Mount(ctx, b.Name, mt[0], mt[1]); err != nil {
			return err
		}
		fmt.Fprintf(m.Log, "mounted %s at %s in sandbox %s\n", mt[0], mt[1], b.Name)
		if err := m.tellMount(mt[0]); err != nil {
			return err
		}
	}
	return nil
}

// tellMount says, the first time a directory is mounted, that the agent
// writes into its .git: a hook or a setting there runs on the host by the
// next git command in it. --clone keeps the host's checkout out of reach.
func (m *Manager) tellMount(dir string) error {
	return m.Paths.State(func(s *State) error {
		if slices.Contains(s.Mounted, dir) {
			return nil
		}
		s.Mounted = append(s.Mounted, dir)
		fmt.Fprintf(m.Log, "note: the agent can write to %s, its .git included; a hook or setting it leaves there runs on this host with your next git command in it. Look at what it changed, or use --clone to keep it out of the checkout.\n", dir)
		return nil
	})
}

// unmountTask takes a task's host directories out of its sandbox.
func (m *Manager) unmountTask(ctx context.Context, b api.Sandbox, t api.Task) error {
	for _, mt := range mounts(t) {
		if err := m.SBX.Unmount(ctx, b.Name, mt[0], mt[1]); err != nil {
			return fmt.Errorf("%w; the task is kept so nothing of the host is deleted", err)
		}
		fmt.Fprintf(m.Log, "unmounted %s from sandbox %s\n", mt[0], b.Name)
	}
	return nil
}

// TaskID is the name a task of these repositories gets when it is given
// none, after the one named lead - else its first - with a digest of their
// forge addresses, branches and checkouts.
func TaskID(lead string, repos []api.Repository) string { return taskIDNamed(lead, repos) }

// withoutBase is a repository as its task is known by: where a branch
// made in the sandbox started, and whether it is only read, are no part of
// it.
func withoutBase(repos []api.Repository) []api.Repository {
	out := slices.Clone(repos)
	for i := range out {
		out[i].Base, out[i].Context = "", false
	}
	return out
}
