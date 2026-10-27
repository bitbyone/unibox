package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// taskRepo is a repository of the env's index as a new task would take
// it: picked or not, worked in or read, its branch pushed or not. One that
// is a directory of the host is mounted, and local holds it with what a
// mount takes from it: the top of its checkout, its forge and its branch.
type taskRepo struct {
	name                          string
	repo                          unibox.IndexedRepo
	picked, context, push, preset bool
	local, top, origin, current   string
}

// taskPickModel picks a new task's repositories, each worked in or read,
// the branch the ones worked in get and whether it is pushed, with the
// task's name: all on one screen, so what each repository gets is seen
// before the task starts and nothing is asked after. A context repository
// stays on its ref, or a mounted one on the branch it is on; a work one
// gets the branch typed after b, made where it is missing - on the forge
// when pushed, else in the sandbox or the host's checkout alone.
type taskPickModel struct {
	env string
	// adding is the task picked repositories are added to, which keeps its
	// name; "" for a new task.
	adding    string
	rows      []taskRepo
	query     string
	filtering bool
	cursor    int
	// branch and task are typed after b and t; editing is which, while
	// typed.
	branch, task string
	editing      string
	input        string
	// existing are the tasks unibox has, which a name of theirs resumes.
	existing      map[string]bool
	note          string
	done, gaveUp  bool
	width, height int
}

func (m taskPickModel) Init() tea.Cmd { return nil }

func (m taskPickModel) visible() []int {
	texts := make([]string, len(m.rows))
	for i, r := range m.rows {
		texts[i] = r.name + " " + r.repo.Description
	}
	return fuzzyFilter(m.query, texts)
}

// branchOf is the branch a repository gets, and whether it is the task's
// own branch rather than the repository's.
func (m taskPickModel) branchOf(r taskRepo) (string, bool) {
	switch {
	case !r.context && m.branch != "":
		return m.branch, true
	case r.local != "":
		return r.current, false
	}
	return r.repo.Ref, false
}

// lead is the repository a task is named after: one picked by hand and
// worked in, else any worked in, else the first; neither one only read
// nor one every task takes tells two tasks apart.
func (m taskPickModel) lead() string {
	for _, want := range []func(taskRepo) bool{
		func(r taskRepo) bool { return !r.context && !r.preset },
		func(r taskRepo) bool { return !r.context },
	} {
		for _, r := range m.rows {
			if r.picked && want(r) {
				return r.name
			}
		}
	}
	return ""
}

// taskName is the name the task gets: the one typed, else the one its
// repositories make - mounted ones first, as a task holds them - "" while
// a branch is still asked for.
func (m taskPickModel) taskName() string {
	if m.task != "" {
		return m.task
	}
	mounted, cloned := []api.Repository{}, []api.Repository{}
	for _, r := range m.rows {
		if !r.picked {
			continue
		}
		b, _ := m.branchOf(r)
		if r.local != "" {
			if b == "" || r.top == "" {
				return ""
			}
			mounted = append(mounted, api.Repository{Name: r.name, URL: r.origin, Branch: b, HostPath: r.top})
			continue
		}
		// One cloned on the forge's default branch is named by that, as
		// unibox knows it here: not at all.
		url, err := unibox.CloneURL(r.repo.URL)
		if err != nil {
			return ""
		}
		cloned = append(cloned, api.Repository{Name: r.name, URL: url, Branch: b})
	}
	repos := append(mounted, cloned...)
	if len(repos) == 0 {
		return ""
	}
	return unibox.TaskID(m.lead(), repos)
}

func (m taskPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyCtrlC {
		m.gaveUp = true
		return m, tea.Quit
	}
	if m.editing != "" {
		switch key.Type {
		case tea.KeyEsc:
			m.editing = ""
		case tea.KeyEnter:
			value := strings.TrimSpace(m.input)
			if m.editing == "task" {
				if value != "" && unibox.ValidName(value) != nil {
					m.note = "a task's name is letters, digits and dashes"
					return m, nil
				}
				m.task = value
			} else {
				m.branch = value
			}
			m.editing, m.note = "", ""
			return m, nil
		case tea.KeyBackspace:
			if r := []rune(m.input); len(r) > 0 {
				m.input = string(r[:len(r)-1])
			}
		case tea.KeyRunes:
			m.input += string(key.Runes)
		}
		return m, nil
	}
	if m.filtering {
		switch key.Type {
		case tea.KeyEsc:
			m.filtering, m.query, m.cursor = false, "", 0
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyBackspace:
			if r := []rune(m.query); len(r) > 0 {
				m.query, m.cursor = string(r[:len(r)-1]), 0
			}
		case tea.KeyUp:
			m.cursor = max(m.cursor-1, 0)
		case tea.KeyDown:
			m.cursor = min(m.cursor+1, max(len(m.visible())-1, 0))
		case tea.KeySpace:
			m.query, m.cursor = m.query+" ", 0
		case tea.KeyRunes:
			m.query, m.cursor = m.query+string(key.Runes), 0
		}
		return m, nil
	}
	m.note = ""
	under := -1
	if v := m.visible(); m.cursor < len(v) {
		under = v[m.cursor]
	}
	switch key.String() {
	case "esc":
		if m.query != "" {
			m.query, m.cursor = "", 0
			return m, nil
		}
		m.gaveUp = true
		return m, tea.Quit
	case "q":
		m.gaveUp = true
		return m, tea.Quit
	case "enter":
		for _, r := range m.rows {
			if r.picked {
				m.done = true
				return m, tea.Quit
			}
		}
		m.note = "pick a repository with space first"
	case "/":
		m.filtering = true
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, max(len(m.visible())-1, 0))
	case " ", "x":
		if under >= 0 {
			if r := m.rows[under]; r.local != "" && r.top == "" {
				m.note = r.repo.URL + " is no directory of this machine"
				return m, nil
			}
			m.rows[under].picked = !m.rows[under].picked
			return m, nil
		}
	case "c":
		if under >= 0 {
			m.rows[under].context = !m.rows[under].context
			return m, nil
		}
	case "p":
		if under >= 0 {
			if m.rows[under].context {
				m.note = "a context repository keeps its branch, and nothing is pushed there"
				return m, nil
			}
			m.rows[under].push = !m.rows[under].push
		}
	case "b":
		m.editing, m.input = "branch", m.branch
	case "t":
		if m.adding == "" {
			m.editing, m.input = "task", m.task
		}
	}
	return m, nil
}

func (m taskPickModel) View() string {
	if m.done || m.gaveUp {
		return ""
	}
	visible := m.visible()
	picked, working, pushed := 0, 0, 0
	for _, r := range m.rows {
		if !r.picked {
			continue
		}
		picked++
		if _, own := m.branchOf(r); own {
			working++
			if r.push {
				pushed++
			}
		}
	}
	field := func(name, value, empty string) string {
		if m.editing == name {
			return cursorMark() + m.input + "_"
		}
		if value == "" {
			return portHelp.Render(empty)
		}
		return portCursor.Render(value)
	}
	// The task's name stands in the title's corner, as it will be.
	name := m.taskName()
	corner := "task " + portCursor.Render(name)
	switch {
	case name == "":
		corner = ""
	case m.existing[name]:
		corner += " " + portHelp.Render("(exists; it is resumed)")
	case m.task == "":
		corner += " " + portHelp.Render("(generated)")
	}
	title := listTitle.Render("New task in env " + m.env)
	if m.adding != "" {
		title, corner = listTitle.Render("Add repositories to task "+m.adding), ""
	}
	gap := 3
	if m.width > 0 {
		gap = max(m.width-1-lipgloss.Width(title)-lipgloss.Width(corner), 3)
	}
	lines := []string{
		title + strings.Repeat(" ", gap) + corner,
		"",
		"  branch  " + field("branch", m.branch, "none: each its ref, its checkout's branch or the forge's default"),
	}
	if m.adding == "" {
		lines = append(lines, "  task    "+field("task", m.task, "generated from its repositories and branches"))
	}
	lines = append(lines, "")
	if m.filtering || m.query != "" {
		typing := ""
		if m.filtering {
			typing = "_"
		}
		lines = append(lines, cursorMark()+"/"+m.query+typing+"  "+portHelp.Render(fmt.Sprintf("%d of %d", len(visible), len(m.rows))), "")
	}
	table := [][]string{{"", "REPO", "ROLE", "FROM", "BRANCH", "PUSH", "ABOUT"}}
	for _, i := range visible {
		r := m.rows[i]
		box := portHelp.Render("•")
		if r.picked {
			box = portOn.Render("✓")
		}
		role := "work"
		if r.context {
			role = portOff.Render("context")
		}
		from := "clone"
		if r.local != "" {
			from = "mount"
		}
		b, own := m.branchOf(r)
		branch := cmp.Or(b, portHelp.Render("default"))
		push := ""
		if own {
			push = portHelp.Render("no")
			if r.push {
				push = portOn.Render("yes")
			}
		}
		about := cmp.Or(r.repo.Description, r.repo.URL)
		if r.local != "" && r.top == "" {
			about = portErr.Render("no such directory: " + r.repo.URL)
		}
		if r.repo.Default == unibox.RepoAlways {
			about = "always · " + about
		}
		table = append(table, []string{box, r.name, role, from, branch, push, about})
	}
	if len(visible) == 0 {
		lines = append(lines, portHelp.Render("  nothing matches"))
	} else {
		rows := strings.Split(strings.TrimSuffix(drawTable(table, m.cursor), "\n"), "\n")
		body := rows[1:]
		if m.height > 0 {
			room := max(m.height-len(lines)-6, 1)
			if room < len(body) {
				first := min(max(m.cursor-room/2, 0), len(body)-room)
				body = body[first : first+room]
			}
		}
		lines = append(append(lines, rows[0]), body...)
	}
	summary := fmt.Sprintf("%d picked", picked)
	if m.branch != "" {
		summary += fmt.Sprintf(" · branch %s in %d, pushed from %d", m.branch, working, pushed)
	}
	lines = append(lines, "", portHelp.Render(summary))
	if m.note != "" {
		lines = append(lines, portErr.Render(m.note))
	}
	switch {
	case m.editing != "":
		lines = append(lines, keyHelp("enter", "keep it", "esc", "leave it as it was"))
	case m.filtering:
		lines = append(lines, keyHelp("↑/↓", "move", "enter", "keep the filter", "esc", "clear it"))
	default:
		if m.adding != "" {
			lines = append(lines, keyHelp("↑/↓ j/k", "move", "space", "pick", "c", "work/context", "p", "push", "b", "branch", "/", "filter", "enter", "add them", "q", "cancel"))
		} else {
			lines = append(lines, keyHelp("↑/↓ j/k", "move", "space", "pick", "c", "work/context", "p", "push", "b", "branch", "t", "task name", "/", "filter", "enter", "start", "q", "cancel"))
		}
	}
	if m.width > 0 {
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, m.width-1, "…")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// taskRows are an env's indexed repositories as a new task takes them by
// default, a mounted one with what its checkout is.
func taskRows(ctx context.Context, repos map[string]unibox.IndexedRepo) []taskRepo {
	rows := []taskRepo{}
	for _, name := range sorted(repos) {
		r := repos[name]
		row := taskRepo{name: name, repo: r, picked: r.Default != unibox.RepoOff, context: r.IsContext(), push: true, local: r.LocalPath()}
		row.preset = row.picked
		if row.local != "" {
			if top, origin, current, err := unibox.HostCheckout(ctx, row.local); err == nil && isDirectory(top) {
				// A checkout with no forge has nowhere to push to.
				row.top, row.origin, row.current, row.push = top, origin, current, origin != ""
			} else {
				row.picked = false
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// pickTask asks, at a terminal, which repositories of the env's index a new
// task takes, which of them it works in, the branch those get and whether
// it is pushed, and the task's name. In accessible mode the same are
// numbered questions, the roles as the index says them and every branch
// pushed.
func (o *options) pickTask(cmd *cobra.Command, env unibox.Env, adding *api.Task, branch string, noPush bool) (taskPickModel, error) {
	ctx := cmd.Context()
	m := taskPickModel{env: env.Name, branch: branch, rows: taskRows(ctx, env.Repos), existing: map[string]bool{}}
	if noPush {
		for i := range m.rows {
			m.rows[i].push = false
		}
	}
	if adding != nil {
		// What the task holds already is not offered again, and nothing is
		// picked until the person picks it.
		m.adding = cmp.Or(adding.Alias, adding.ID)
		m.rows = slices.DeleteFunc(m.rows, func(r taskRepo) bool {
			return slices.ContainsFunc(adding.Repositories, func(h api.Repository) bool {
				return h.Name == r.name || h.URL != "" && unibox.SourceURL(ctx, cmp.Or(r.local, r.repo.URL)) == strings.TrimSuffix(h.URL, ".git")
			})
		})
		for i := range m.rows {
			m.rows[i].picked, m.rows[i].preset = false, false
		}
		if len(m.rows) == 0 {
			return m, fmt.Errorf("task %s holds every repository env %s names; unibox repo add names another", m.adding, env.Name)
		}
	}
	if state, err := o.paths.ReadState(); err == nil {
		for _, b := range state.Sandboxes {
			for _, t := range b.Tasks {
				m.existing[t.ID] = true
				if t.Alias != "" {
					m.existing[t.Alias] = true
				}
			}
		}
	}
	if o.accessible() {
		labels, preset := []string{}, []string{}
		for _, it := range repoItems(env.Repos) {
			labels = append(labels, it.label)
			if it.picked {
				preset = append(preset, it.label)
			}
		}
		chosen, err := o.askChoices("Repositories for a new task in env "+env.Name, "", labels, preset, nil)
		if err != nil {
			return m, err
		}
		for i := range m.rows {
			m.rows[i].picked = slices.Contains(chosen, labels[i])
		}
		if m.branch, err = o.askText("Branch of the repositories worked in", "Empty keeps each its ref, its checkout's branch or the forge's default", "", nil); err != nil {
			return m, err
		}
		if adding != nil {
			return m, nil
		}
		if m.task, err = o.askText("Task name", "Empty names it after its repositories", "", func(s string) error {
			if s == "" {
				return nil
			}
			return unibox.ValidName(s)
		}); err != nil {
			return m, err
		}
		return m, nil
	}
	lipgloss.HasDarkBackground()
	res, err := tea.NewProgram(m, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(ctx)).Run()
	if err != nil {
		return m, err
	}
	done := res.(taskPickModel)
	if done.gaveUp {
		return done, errCancelled
	}
	return done, nil
}

// inputs are what a task is asked to hold from what was picked: a mounted
// directory switched to its branch, pushed when asked; a clone on its
// branch, which is made on the forge when pushed and in the sandbox alone
// otherwise.
func (m taskPickModel) inputs() []unibox.RepoInput {
	out := []unibox.RepoInput{}
	for _, r := range m.rows {
		if !r.picked {
			continue
		}
		b, own := m.branchOf(r)
		if r.local != "" {
			in := unibox.RepoInput{Source: r.local, Name: r.name, Context: r.context}
			if own {
				in.Branch, in.Push = b, r.push
			}
			out = append(out, in)
			continue
		}
		in := unibox.RepoInput{Source: r.repo.URL, Name: r.name, Branch: b, Clone: true, Context: r.context}
		if own {
			if r.push {
				in.Push = true
			} else {
				in.Fork, in.Base = true, r.repo.Ref
			}
		}
		out = append(out, in)
	}
	return out
}

func isDirectory(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// addTaskRepos adds repositories of its env's index to a task, picked on the
// same screen a new task's are; an agent working in it sees them at once.
func (o *options) addTaskRepos(cmd *cobra.Command, t api.Task) error {
	_, env, err := o.manager.Definition(unibox.Request{Task: t.ID})
	if err != nil {
		return err
	}
	if len(env.Repos) == 0 {
		return fmt.Errorf("env %s names no repositories; unibox repo add adds one, or the team's unibox.yaml under repos:", env.Name)
	}
	picked, err := o.pickTask(cmd, env, &t, "", false)
	if err != nil {
		return err
	}
	if err := o.confirmBranch(cmd, picked); err != nil {
		return err
	}
	req := unibox.Request{Task: t.ID, Repositories: picked.inputs()}
	if len(req.Repositories) == 0 {
		return nil
	}
	_, added, err := o.manager.AddRepositories(cmd.Context(), req)
	if err != nil {
		return err
	}
	for _, r := range added {
		fmt.Fprintf(o.log, "added %s to task %s at %s/%s\n", r.Name, t.ID, t.Directory, r.Name)
	}
	return nil
}

// branchClashes are the picked repositories worked in that have the task's
// branch already - on the forge, or in a mounted checkout - each asked
// once, as the task starts.
func (m taskPickModel) branchClashes(ctx context.Context) ([]string, error) {
	if m.branch == "" {
		return nil, nil
	}
	type answer struct {
		name string
		has  bool
		err  error
	}
	answers := make(chan answer)
	asked := 0
	for _, r := range m.rows {
		if _, own := m.branchOf(r); !r.picked || !own {
			continue
		}
		asked++
		go func() {
			if r.local != "" {
				has := unibox.HostHasBranch(ctx, r.top, m.branch)
				if !has && r.origin != "" {
					var err error
					has, err = unibox.RemoteHasBranch(ctx, r.origin, m.branch)
					answers <- answer{r.name, has, err}
					return
				}
				answers <- answer{r.name, has, nil}
				return
			}
			has, err := unibox.RemoteHasBranch(ctx, r.repo.URL, m.branch)
			answers <- answer{r.name, has, err}
		}()
	}
	clashes, errs := []string{}, []error{}
	for range asked {
		a := <-answers
		if a.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.name, a.err))
		}
		if a.has {
			clashes = append(clashes, a.name)
		}
	}
	slices.Sort(clashes)
	return clashes, errors.Join(errs...)
}

// confirmBranch stops a task whose branch exists already where the person
// meant to make it, unless they say to work on it there.
func (o *options) confirmBranch(cmd *cobra.Command, m taskPickModel) error {
	// A task that exists made its branch itself.
	if m.branch == "" || m.adding == "" && o.taskExists(m.taskName()) {
		return nil
	}
	var clashes []string
	err := o.waiting("looking for branch "+m.branch, func() (err error) {
		clashes, err = m.branchClashes(cmd.Context())
		return err
	})
	if err != nil {
		return fmt.Errorf("cannot tell whether branch %s exists already: %w", m.branch, err)
	}
	if len(clashes) == 0 {
		return nil
	}
	said := fmt.Sprintf("branch %s exists already in %s", m.branch, strings.Join(clashes, ", "))
	if !o.canAsk() {
		return unibox.Fail(4, "branch_exists", "%s; give another --branch, or pass --yes to work on it there", said)
	}
	if o.yes {
		return nil
	}
	ok, err := o.askYes(said+"; work on it there?", false)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s; pick another branch", said)
	}
	return nil
}
