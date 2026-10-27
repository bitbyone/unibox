package unibox

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

// instructionsMark opens a task's AGENTS.md, so unibox knows the file as
// its own: it writes it again before each session and takes it away when
// nothing is to be said.
const instructionsMark = "<!-- Written by unibox for this task before each session; edits are lost. -->"

// TaskInstructions are what a task's agents are told of it: its
// repositories, each read or worked in and on which branch, then the env's
// instructions and each repository's from the index. "" for a task of one
// repository with none, whose session starts in it.
func TaskInstructions(t api.Task, env Env) string {
	repoText := map[string]string{}
	for _, r := range t.Repositories {
		if text := strings.TrimSpace(env.Repos[r.Name].Instructions); text != "" {
			repoText[r.Name] = text
		}
	}
	if len(env.Instructions) == 0 && len(repoText) == 0 && len(t.Repositories) < 2 {
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

// sessionDir is where a task's session starts: above its repositories when
// it is told of them, else in its one repository.
func sessionDir(t api.Task, instructions string) string {
	if len(t.Repositories) == 1 && instructions == "" {
		return path.Join(t.Directory, t.Repositories[0].Name)
	}
	return t.Directory
}

// agentInstructionFiles are, for each agent unibox knows, the files it
// reads its instructions from as its own documentation says, in the order
// it reads them, as a script over emit (a file), tree (the files of a
// directory's tree matching a name), up and down (the directories between
// two, from the first or the last), $cwd, the session's directory, and
// $root, its git repository's top.
var agentInstructionFiles = map[string]string{
	"claude": `emit "$HOME/.claude/CLAUDE.md"; tree "$HOME/.claude/rules" '*.md'
for d in $(down "$cwd" /); do emit "$d/CLAUDE.md"; emit "$d/.claude/CLAUDE.md"; emit "$d/CLAUDE.local.md"; done
tree "$cwd/.claude/rules" '*.md'
`,
	"codex": `h=${CODEX_HOME:-$HOME/.codex}; if [ -f "$h/AGENTS.override.md" ]; then emit "$h/AGENTS.override.md"; else emit "$h/AGENTS.md"; fi
for d in $(down "$cwd" "${root:-$cwd}"); do if [ -f "$d/AGENTS.override.md" ]; then emit "$d/AGENTS.override.md"; else emit "$d/AGENTS.md"; fi; done
`,
	"copilot": `emit "$HOME/.copilot/copilot-instructions.md"; tree "$HOME/.copilot/instructions" '*.instructions.md'
for d in $(printf '%s\n' "$root" "$cwd" $(printf '%s' "$COPILOT_CUSTOM_INSTRUCTIONS_DIRS" | tr ',:' '  ') | awk 'NF && !seen[$0]++'); do
  emit "$d/CLAUDE.md"; emit "$d/GEMINI.md"; emit "$d/AGENTS.md"; tree "$d/.claude/rules" '*.md'
  tree "$d/.github/instructions" '*.instructions.md'; emit "$d/.github/copilot-instructions.md"
done
`,
	"opencode": `if [ -f "$HOME/.config/opencode/AGENTS.md" ]; then emit "$HOME/.config/opencode/AGENTS.md"; else emit "$HOME/.claude/CLAUDE.md"; fi
for d in $(up "$cwd" "${root:-/}"); do
  if [ -f "$d/AGENTS.md" ] || [ "$d/AGENTS.md" = "$U/AGENTS.md" ]; then emit "$d/AGENTS.md"; break; fi
  if [ -f "$d/CLAUDE.md" ]; then emit "$d/CLAUDE.md"; break; fi
done
`,
}

// instructionsReadScript prints, NUL-separated, the path, link target,
// whether unibox writes it and content of each file the agent reads, the
// same file once. The files unibox writes are named in $U even before it
// has written them; ones it would take away are left out.
func instructionsReadScript(agent string, t api.Task, writes bool) string {
	u := ""
	if writes {
		u = t.Directory
	}
	return fmt.Sprintf(`cwd=$(pwd -P) U=%s G=%s MARK=%s
G=$(cd "$G" 2>/dev/null && pwd -P || printf '%%s' "$G")
[ -n "$U" ] && { [ -w "$U" ] && U=$G || U=; }
root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null || true)
seen=
up() { d=$1; while :; do printf '%%s\n' "$d"; [ "$d" = "$2" ] || [ "$d" = / ] && break; d=$(dirname "$d"); done; }
down() { up "$@" | awk '{a[NR]=$0} END {for (i = NR; i; i--) print a[i]}'; }
emit() {
  f=$1 link= own=
  case "$f" in //*) f=${f#/};; esac
  if [ -n "$U" ] && { [ "$f" = "$U/AGENTS.md" ] || [ "$f" = "$U/CLAUDE.md" ]; }; then
    real=$U/AGENTS.md own=1; [ "$f" = "$U/CLAUDE.md" ] && link=AGENTS.md
  else
    [ -f "$f" ] || return 0
    real=$(readlink -f "$f"); [ -L "$f" ] && link=$(readlink "$f")
    if [ -z "$U" ] && [ "$real" = "$G/AGENTS.md" ] && head -n 1 "$real" | grep -qF "$MARK"; then return 0; fi
  fi
  case " $seen " in *" $real "*) return 0;; esac
  seen="$seen $real"
  printf '%%s\0%%s\0%%s\0' "$f" "$link" "$own"
  [ -n "$own" ] || cat "$real"
  printf '\0'
}
tree() { [ -d "$1" ] || return 0; while IFS= read -r f; do emit "$f"; done < <(find "$1" -type f -name "$2" 2>/dev/null | sort); }
%s`, Quote(u), Quote(t.Directory), Quote(instructionsMark), agentInstructionFiles[agent])
}

// AgentInstructions are what a task's agent is told when its next session
// starts: the files it reads, as they are in its sandbox, with what unibox
// writes before the session in place of what it wrote before. A sandbox
// not running is not started; only what unibox writes is told then.
func (m *Manager) AgentInstructions(ctx context.Context, task, agent string) (api.Instructions, error) {
	id := m.taskID(task)
	name, err := m.taskSandbox(id, false)
	if err != nil {
		return api.Instructions{}, err
	}
	b, err := m.sandbox(name)
	if err != nil {
		return api.Instructions{}, err
	}
	i := slices.IndexFunc(b.Tasks, func(t api.Task) bool { return t.ID == id })
	if i < 0 {
		return api.Instructions{}, fmt.Errorf("task %s does not exist; unibox task ls lists them", task)
	}
	t := b.Tasks[i]
	env, err := m.Config.LoadEnv(b.Env)
	if err != nil {
		return api.Instructions{}, err
	}
	text := TaskInstructions(t, env)
	agent = cmp.Or(agent, t.Agent)
	_, known := agentInstructionFiles[agent]
	out := api.Instructions{APIVersion: api.Version, Sandbox: b.Name, Task: cmp.Or(t.Alias, t.ID), Agent: agent, Directory: sessionDir(t, text), Known: known, Files: []api.InstructionFile{}}
	own := api.InstructionFile{Path: path.Join(t.Directory, "AGENTS.md"), Unibox: true, Content: text}
	box, exists, err := m.findBox(ctx, b)
	if err != nil {
		return api.Instructions{}, err
	}
	if !exists || box.State != "running" || !known {
		if text != "" {
			out.Files = append(out.Files, own)
		}
		return out, nil
	}
	var stdout, stderr strings.Builder
	cmd := m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", instructionsReadScript(agent, t, text != "")}, Directory: out.Directory, Output: &stdout, Error: &stderr})
	if err := cmd.Run(); err != nil {
		return api.Instructions{}, fmt.Errorf("reading %s's instructions in sandbox %s failed; check the sandbox with unibox doctor: %s", agent, b.Name, strings.TrimSpace(stderr.String()))
	}
	fields := strings.Split(stdout.String(), "\x00")
	for j := 0; j+3 < len(fields); j += 4 {
		f := api.InstructionFile{Path: fields[j], Link: fields[j+1], Unibox: fields[j+2] != "", Content: fields[j+3]}
		if f.Unibox {
			f.Content = text
		}
		out.Files = append(out.Files, f)
	}
	out.Read = true
	return out, nil
}
