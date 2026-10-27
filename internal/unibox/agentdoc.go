package unibox

import (
	"cmp"
	"fmt"
	"path"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

// instructionsMark opens a task's AGENTS.md, so unibox knows the file as
// its own: it writes it again before each session and takes it away when
// nothing is to be said.
const instructionsMark = "<!-- Written by unibox for this task before each session; edits are lost. -->"

// TaskInstructions are what a task's agents are told of it: its
// repositories, each read or worked in and on which branch, then the env's
// instructions and each repository's from the index. "" when neither the
// env nor a repository of the task has any.
func TaskInstructions(t api.Task, env Env) string {
	repoText := map[string]string{}
	for _, r := range t.Repositories {
		if text := strings.TrimSpace(env.Repos[r.Name].Instructions); text != "" {
			repoText[r.Name] = text
		}
	}
	if len(env.Instructions) == 0 && len(repoText) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n# Task %s\n\nIts repositories, each in a directory of its own here:\n\n", instructionsMark, cmp.Or(t.Alias, t.ID))
	for _, r := range t.Repositories {
		branch := cmp.Or(r.Branch, "the forge's default branch")
		switch {
		case r.Context:
			fmt.Fprintf(&b, "- `%s/` - read it, on %s; change and commit nothing there.\n", r.Name, branch)
		case r.Mounted:
			fmt.Fprintf(&b, "- `%s/` - work in it, on %s; it is a checkout of the person's machine.\n", r.Name, branch)
		default:
			fmt.Fprintf(&b, "- `%s/` - work in it, on %s.\n", r.Name, branch)
		}
	}
	if len(env.Instructions) > 0 {
		fmt.Fprintf(&b, "\n## Env %s\n\n", env.Name)
		for _, text := range env.Instructions {
			b.WriteString(strings.TrimSpace(text) + "\n\n")
		}
	}
	for _, r := range t.Repositories {
		if text, ok := repoText[r.Name]; ok {
			fmt.Fprintf(&b, "\n## %s\n\n%s\n", r.Name, text)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// instructionsScript writes a task's AGENTS.md, CLAUDE.md a link to it, or
// with nothing to say takes away those unibox wrote. A directory the agent
// cannot write in, as sbx makes a mount's parents, is said and left.
func instructionsScript(t api.Task, text string) string {
	file, link := path.Join(t.Directory, "AGENTS.md"), path.Join(t.Directory, "CLAUDE.md")
	if text == "" {
		return fmt.Sprintf("if head -n 1 %s 2>/dev/null | grep -qF %s; then rm -f %s %s; fi\n", Quote(file), Quote(instructionsMark), Quote(file), Quote(link))
	}
	return fmt.Sprintf("if [ -w %s ]; then\n%sln -sfn AGENTS.md %s\nelse\necho %s >&2\nfi\n",
		Quote(t.Directory), writeGuest(file, []byte(text), "644"), Quote(link),
		Quote("unibox: "+t.Directory+" is not the agent's to write in; the task's instructions are left out"))
}
