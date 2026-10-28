package unibox

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

func writeGuest(file string, data []byte, mode string) string {
	return "mkdir -p " + Quote(path.Dir(file)) + "\nprintf %s " + Quote(base64.StdEncoding.EncodeToString(data)) + " | base64 -d > " + Quote(file) + "\nchmod " + mode + " " + Quote(file) + "\n"
}

func prepareScript(t api.Task) string {
	var b strings.Builder
	b.WriteString("set -eu\nexport GIT_TERMINAL_PROMPT=0\nmkdir -p " + Quote(t.Directory) + "\n")
	for _, r := range t.Repositories {
		dir := path.Join(t.Directory, r.Name)
		if r.Mounted {
			b.WriteString(mountedScript(t, r))
			continue
		}
		name, email, _ := AuthorParts(r.Author)
		fmt.Fprintf(&b, "url=%s\nbranch=%s\nbase=%s\ndir=%s\n", Quote(r.URL), Quote(r.Branch), Quote(r.Base), Quote(dir))
		// A forge that redirects the URL - GitLab's without .git - has git
		// warn on every request; the first one finds where it leads, and
		// the rest go there.
		b.WriteString(`given=$url
errs="$dir.unibox-ls-remote"
refs=$(git ls-remote --symref "$url" HEAD "refs/heads/$branch" ${base:+"refs/heads/$base"} 2>"$errs") || true
grep -v '^warning: redirecting to ' "$errs" >&2 || true
moved=$(sed -n 's/^warning: redirecting to //p' "$errs" | head -n 1)
rm -f "$errs"
[ -z "$moved" ] || url=${moved%/}
tip=$(printf '%s\n' "$refs" | awk -v ref="refs/heads/$branch" '$2 == ref {print $1}')
head=$(printf '%s\n' "$refs" | sed -n 's|^ref: refs/heads/\(.*\)	HEAD$|\1|p')
headtip=$(printf '%s\n' "$refs" | awk '$2 == "HEAD" {print $1}')
# No branch is the forge's default one, and a base of HEAD starts there.
if [ -z "$branch" ]; then
  [ -n "$head" ] || { echo "$given names no default branch; give the task a branch of it" >&2; exit 4; }
  branch=$head tip=$headtip
fi
fromhead=
if [ "$base" = HEAD ]; then
  base=$head fromhead=1
fi
# A branch the forge lacks is the task's alone when it has a base: made in
# the sandbox from it, and pushed by nobody but the agent.
basetip=
if [ -z "$tip" ] && [ -n "$base" ]; then
  basetip=$(printf '%s\n' "$refs" | awk -v ref="refs/heads/$base" '$2 == ref {print $1}')
  [ -z "$fromhead" ] || basetip=$headtip
  [ -n "$basetip" ] || { echo "$given has neither branch $branch nor $base to make it from${head:+ (its default branch is $head)}" >&2; exit 4; }
fi
[ -n "$tip" ] || [ -n "$basetip" ] || { echo "$given has no branch $branch${head:+ (its default branch is $head)}; push it first, or name a branch it has - for a repository of the env's index, its ref" >&2; exit 4; }
if [ ! -d "$dir" ]; then
  tmp="$dir.unibox-clone"
  [ ! -e "$tmp" ] || { echo 'incomplete clone remains; inspect it in unibox sandbox shell and remove it before retrying' >&2; exit 4; }
  if [ -n "$tip" ]; then
    git clone --filter=blob:none --single-branch --branch "$branch" -- "$url" "$tmp"
  else
    git clone --filter=blob:none --single-branch --branch "$base" -- "$url" "$tmp"
    git -C "$tmp" switch --quiet -c "$branch"
  fi
  mv "$tmp" "$dir"
elif [ -z "$tip" ]; then
  # The task's own branch, not on the forge: it is what the task has.
  [ -d "$dir/.git" ] || { echo 'task directory is not a clone; inspect it in unibox sandbox shell' >&2; exit 4; }
  [ "$(git -C "$dir" symbolic-ref --short HEAD)" = "$branch" ] || { echo 'task branch changed; restore it in unibox sandbox shell' >&2; exit 4; }
else
  [ -d "$dir/.git" ] || { echo 'task directory is not a clone; inspect it in unibox sandbox shell' >&2; exit 4; }
  origin=$(git -C "$dir" remote get-url origin)
  [ "$origin" = "$url" ] || [ "$origin" = "$given" ] || { echo 'task origin changed; inspect it in unibox sandbox shell' >&2; exit 4; }
  [ "$(git -C "$dir" symbolic-ref --short HEAD)" = "$branch" ] || { echo 'task branch changed; restore it in unibox sandbox shell' >&2; exit 4; }
  [ -z "$(git -C "$dir" status --porcelain --untracked-files=all)" ] || { echo 'task has local edits; finish them in unibox sandbox shell before preparing again' >&2; exit 4; }
  git -C "$dir" fetch origin "+refs/heads/$branch:refs/remotes/origin/$branch"
  git -C "$dir" merge-base --is-ancestor HEAD "refs/remotes/origin/$branch" || { echo 'task has unpushed or divergent commits; preserve them before continuing' >&2; exit 4; }
  git -C "$dir" merge --ff-only "refs/remotes/origin/$branch"
fi
`)
		fmt.Fprintf(&b, "git -C \"$dir\" config user.name %s\ngit -C \"$dir\" config user.email %s\n", Quote(name), Quote(email))
		b.WriteString("git -C \"$dir\" config push.default simple\n")
		// The hook is there either way: with crediting off it still takes
		// out the trailer an agent adds of itself, so off means none.
		credit := "on\n"
		if r.CoAuthor != nil && !*r.CoAuthor {
			credit = "off\n"
		}
		hooks := path.Join(dir, ".git", "unibox-hooks")
		b.WriteString(writeGuest(path.Join(hooks, "commit-msg"), []byte(coAuthorHook()), "700"))
		b.WriteString(writeGuest(path.Join(hooks, "credit"), []byte(credit), "600"))
		fmt.Fprintf(&b, "git -C \"$dir\" config core.hooksPath %s\n", Quote(hooks))
	}
	return b.String()
}

// coAuthorHook makes a commit's co-author unibox's to say. The session
// names its agent (UNIBOX_AGENT, as unibox's table calls it) and its
// co-author (UNIBOX_CO_AUTHOR); the hook takes out any co-author
// trailer of either name - agents add their own, copilot always, in their
// own case and address - and, unless the clone's credit file says off,
// adds the agent's. A commit made by hand in unibox sandbox shell, with no agent
// running, is left as it is.
func coAuthorHook() string {
	return `#!/bin/sh
set -eu
[ -n "${UNIBOX_AGENT:-}" ] || exit 0
co="${UNIBOX_CO_AUTHOR:-}"
names="$UNIBOX_AGENT"
[ -z "$co" ] || names="$names|${co%% <*}"
awk -v names="$names" '
BEGIN { n = split(tolower(names), name, "|") }
{
  line = tolower($0)
  for (i = 1; i <= n; i++) if (index(line, "co-authored-by: " name[i] " <") == 1) next
  # Blank lines wait for what follows, so none is left at the end where a
  # taken-out trailer was.
  if ($0 ~ /^[ \t]*$/) { blank = blank $0 "\n"; next }
  printf "%s", blank
  blank = ""
  print
}' "$1" > "$1.unibox"
cat "$1.unibox" > "$1"
rm -f "$1.unibox"
credit=$(cat "$(dirname "$0")/credit" 2>/dev/null || echo on)
[ "$credit" = on ] && [ -n "$co" ] || exit 0
git interpret-trailers --in-place --trailer "Co-Authored-By: $co" "$1"
`
}

// A disconnected host process is not proof that its guest process ended.
const guestIdleScript = `set -eu
for f in /var/lib/unibox/runs/*; do
  [ -f "$f" ] || continue
  read -r pid born < "$f"
  now=$(awk '{print $22}' "/proc/$pid/stat" 2>/dev/null || true)
  if [ -n "$now" ] && [ "$now" = "$born" ]; then
    echo 'a guest session is still running; inspect it with unibox sandbox shell' >&2
    exit 4
  fi
done
`

// safeToRemoveScript checks a task's clones for work only the sandbox has.
// It says what it checked, one line a repository, rather than letting git
// narrate: the fetch that learns every branch on the forge would otherwise
// list each of them as a new branch, which reads as if something changed.
func safeToRemoveScript(t api.Task) string {
	var b strings.Builder
	b.WriteString("set -eu\nexport GIT_TERMINAL_PROMPT=0\n")
	fmt.Fprintf(&b, "echo %s\n", Quote("checking task "+t.ID+" for work not on the forge"))
	for _, r := range t.Repositories {
		dir := path.Join(t.Directory, r.Name)
		if r.Mounted {
			fmt.Fprintf(&b, "echo %s\n", Quote("  "+r.Name+": mounted from "+r.HostPath+"; its work is on the host"))
			continue
		}
		fmt.Fprintf(&b, "dir=%s\nname=%s\n", Quote(dir), Quote(r.Name))
		b.WriteString(`if [ -d "$dir" ]; then
  [ -d "$dir/.git" ] || { echo "$name: an incomplete clone; inspect it before removing" >&2; exit 4; }
  [ -z "$(git -C "$dir" status --porcelain --untracked-files=all)" ] || { echo "$name: uncommitted files; commit and push before removing" >&2; exit 4; }
  git -C "$dir" fetch -q origin '+refs/heads/*:refs/remotes/origin/*' || { echo "$name: cannot fetch from the forge to compare; check its access and retry" >&2; exit 4; }
  [ -z "$(git -C "$dir" rev-list --branches HEAD --not --remotes=origin)" ] || { echo "$name: unpushed commits; push them before removing" >&2; exit 4; }
  [ -z "$(git -C "$dir" stash list)" ] || { echo "$name: stashes; preserve them before removing" >&2; exit 4; }
  echo "  $name: clean, every commit on the forge, no stashes"
else
  echo "  $name: not cloned, nothing to keep"
fi
`)
	}
	// Files outside the registered clones have no forge copy either.
	b.WriteString("if [ -d " + Quote(t.Directory) + " ]; then\nfor entry in " + Quote(t.Directory) + "/* " + Quote(t.Directory) + "/.[!.]* " + Quote(t.Directory) + "/..?*; do\n[ -e \"$entry\" ] || continue\ncase \"$entry\" in\n")
	for _, r := range t.Repositories {
		b.WriteString(Quote(path.Join(t.Directory, r.Name)) + " ) ;;\n")
	}
	// What unibox writes before each session is its own, written again by
	// the next; one the agent replaced is not.
	b.WriteString(Quote(path.Join(t.Directory, "AGENTS.md")) + " ) head -n 1 \"$entry\" | grep -qF " + Quote(instructionsMark) + " || { echo 'AGENTS.md: not the one unibox writes; preserve it before removing' >&2; exit 4; } ;;\n")
	b.WriteString(Quote(path.Join(t.Directory, "CLAUDE.md")) + " ) [ \"$(readlink -- \"$entry\")\" = AGENTS.md ] || { echo 'CLAUDE.md: not the link unibox makes; preserve it before removing' >&2; exit 4; } ;;\n")
	b.WriteString("*) echo \"${entry##*/}: outside the task's repositories; preserve it before removing\" >&2; exit 4 ;;\nesac\ndone\nfi\necho '  nothing outside the repositories'\n")
	return b.String()
}

// codexSetup trusts every task directory and repository under /work in
// codex's config.toml, so codex does not ask whether to trust what unibox
// prepared. What unibox writes sits between markers, rewritten each time;
// the rest - the kit's and the user's - stays, and a directory it names
// already is left to it.
const codexSetup = `home="${CODEX_HOME:-$HOME/.codex}"
work="${UNIBOX_WORK:-/work}"
mkdir -p "$home"
cfg="$home/config.toml"
touch "$cfg"
awk '
/^# unibox:begin/ { skip = 1; next }
/^# unibox:end/ { skip = 0; next }
skip { next }
{ print }' "$cfg" > "$cfg.unibox"
{
  cat "$cfg.unibox"
  for d in "$work"/* "$work"/*/*; do
    [ -d "$d" ] || continue
    case "$d" in *.unibox-clone) continue ;; esac
    grep -qxF "[projects.\"$d\"]" "$cfg.unibox" && continue
    printf '[projects."%s"]\ntrust_level = "trusted"\n' "$d"
  done > "$cfg.tail"
  if [ -s "$cfg.tail" ]; then
    echo '# unibox:begin'
    cat "$cfg.tail"
    echo '# unibox:end'
  fi
} > "$cfg.new"
cat "$cfg.new" > "$cfg"
rm -f "$cfg.new" "$cfg.unibox" "$cfg.tail"
`

func sessionScript(id string, args []string) string {
	s := "set -u\nmkdir -p /var/lib/unibox/runs\nborn=$(awk '{print $22}' /proc/$$/stat)\nprintf '%s %s\\n' \"$$\" \"$born\" > " + Quote("/var/lib/unibox/runs/"+id) + "\ntrap " + Quote("rm -f "+Quote("/var/lib/unibox/runs/"+id)) + " EXIT\n"
	for _, arg := range args {
		s += Quote(arg) + " "
	}
	return s + "\nexit $?\n"
}

// mountedDir is where unibox keeps, in the guest, what a mounted repository
// of a task needs of git: its configuration and commit hook, outside the
// host's .git, which unibox does not write.
func mountedDir(t api.Task, r api.Repository) string {
	return "/var/lib/unibox/mounted/" + t.ID + "/" + r.Name
}

// mountedScript checks a mounted directory is in its place in the task,
// where the agent works, and gives git in the guest the author and
// co-author hook for it through an includeIf of the guest's global
// configuration - so commits made in the sandbox are the task's, and the
// host's .git/config is left as it is.
func mountedScript(t api.Task, r api.Repository) string {
	var b strings.Builder
	dir := path.Join(t.Directory, r.Name)
	fmt.Fprintf(&b, "dir=%s\n", Quote(dir))
	b.WriteString(`[ -d "$dir" ] || { echo "$dir is not mounted in the sandbox; prepare the task again" >&2; exit 4; }
`)
	own := mountedDir(t, r)
	// The repository's git directory records its worktrees on the host,
	// which the guest does not have; git's gc would take them for gone and
	// prune their records on the host.
	config := "[gc]\n\tworktreePruneExpire = never\n"
	if r.Author != "" {
		name, email, _ := AuthorParts(r.Author)
		credit := "on\n"
		if r.CoAuthor != nil && !*r.CoAuthor {
			credit = "off\n"
		}
		config += fmt.Sprintf("[user]\n\tname = %s\n\temail = %s\n[core]\n\thooksPath = %s\n", name, email, own+"/hooks")
		b.WriteString(writeGuest(own+"/hooks/commit-msg", []byte(coAuthorHook()), "700"))
		b.WriteString(writeGuest(own+"/hooks/credit", []byte(credit), "600"))
	}
	b.WriteString(writeGuest(own+"/gitconfig", []byte(config), "600"))
	key := "includeIf.gitdir:" + dir + "/.git/.path"
	fmt.Fprintf(&b, "git config --global --unset-all %s >/dev/null 2>&1 || true\ngit config --global --add %s %s\n", Quote(key), Quote(key), Quote(own+"/gitconfig"))
	return b.String()
}

// unmountedScript takes out of the guest what mountedScript put there for
// a task's mounted repositories - git's include - and, run after the
// runtime unmounted them, refuses to go on while a mount point still holds
// anything: the task's directory is deleted next, and what a mount holds
// is the host's.
func unmountedScript(t api.Task) string {
	var b strings.Builder
	for _, r := range t.Repositories {
		if !r.Mounted {
			continue
		}
		dir := path.Join(t.Directory, r.Name)
		key := "includeIf.gitdir:" + dir + "/.git/.path"
		fmt.Fprintf(&b, "git config --global --unset-all %s >/dev/null 2>&1 || true\n", Quote(key))
		fmt.Fprintf(&b, "dir=%s\n", Quote(dir))
		b.WriteString(`if [ -d "$dir" ] && [ -n "$(ls -A "$dir" 2>/dev/null)" ]; then echo "$dir is still mounted; the task is kept so nothing of the host is deleted" >&2; exit 4; fi
`)
	}
	if b.Len() > 0 {
		b.WriteString("rm -rf " + Quote("/var/lib/unibox/mounted/"+t.ID) + "\n")
	}
	return b.String()
}

// setupScript readies a sandbox for unibox's tasks, as root: the kit's
// image need not have the directories unibox keeps tasks and runs in. Not
// recursive, as what is mounted under /work is the host's.
const setupScript = `set -eu
mkdir -p /work /var/lib/unibox/runs /var/lib/unibox/mounted
chown agent:agent /work /var/lib/unibox /var/lib/unibox/runs /var/lib/unibox/mounted
`
