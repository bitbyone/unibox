package unibox

import (
	"encoding/base64"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

func writeGuest(file string, data []byte, mode string) string {
	return "mkdir -p " + Quote(path.Dir(file)) + "\nprintf %s " + Quote(base64.StdEncoding.EncodeToString(data)) + " | base64 -d > " + Quote(file) + "\nchmod " + mode + " " + Quote(file) + "\n"
}

func initializeScript(p Profile, instructions string) string {
	s := "set -eu\n" + gitServicesScript(p)
	// Every clone's co-author hook is rewritten before each session, so one
	// prepared by an older unibox - with an agent fixed in it - follows the
	// session's agent too.
	s += "for d in /work/*/*/.git/unibox-hooks; do\n  [ -d \"$d\" ] || continue\n  printf %s " + Quote(base64.StdEncoding.EncodeToString([]byte(coAuthorHook()))) + " | base64 -d > \"$d/commit-msg\"\n  chmod 700 \"$d/commit-msg\"\ndone\n"
	for _, id := range p.Agents {
		a, _ := FindAgent(id)
		s += "mkdir -p \"$HOME/" + path.Dir(a.Instructions) + "\"\nprintf %s " + Quote(base64.StdEncoding.EncodeToString([]byte(instructions))) + " | base64 -d > \"$HOME/" + a.Instructions + "\"\n"
	}
	if p.Start != "" {
		s += writeGuest("/var/lib/unibox/start.sh", p.Files[p.Start].Data, "700")
	}
	s += "boot=$(cat /proc/sys/kernel/random/boot_id)\nprevious=$(cat /var/lib/unibox/started 2>/dev/null || true)\nif [ \"$boot\" != \"$previous\" ]; then\n"
	if p.Start != "" {
		s += "  /bin/sh /var/lib/unibox/start.sh\n"
	}
	s += "  printf '%s\\n' \"$boot\" > /var/lib/unibox/started\nfi\n"
	return s
}

// gitServicesScript wires the sandbox's git to its git services, for every
// clone in it - prepared by unibox or made by the agent - so it is part of
// the sandbox's setup, not a task's.
func gitServicesScript(p Profile) string {
	var b strings.Builder
	// A git service sends its placeholder as the Authorization header; the
	// proxy swaps the host's whole credential in. git's own helper cannot
	// carry it - git base64-encodes user:password, hiding the placeholder -
	// so it goes in verbatim through http.extraHeader. git sends every
	// extraHeader whose URL matches, from the least specific URL to the
	// most, so each list starts with an empty value, which resets it: a
	// path's header replaces its host's instead of joining it.
	for name, s := range p.Services {
		if !s.Git {
			continue
		}
		header := "Authorization: " + ServicePlaceholder(name, s)
		for _, u := range s.GitURLs() {
			key := Quote("http." + u + ".extraHeader")
			fmt.Fprintf(&b, "git config --global --unset-all %s || true\ngit config --global --add %s ''\ngit config --global --add %s %s\n", key, key, key, Quote(header))
		}
	}
	b.WriteString(identityScript(p))
	return b.String()
}

func prepareScript(t api.Task, p Profile, a Agent) string {
	var b strings.Builder
	b.WriteString("set -eu\nexport GIT_TERMINAL_PROMPT=0\nmkdir -p " + Quote(t.Directory) + "\n")
	for _, r := range t.Repositories {
		dir := path.Join(t.Directory, r.Name)
		name, email, _ := AuthorParts(r.Author)
		fmt.Fprintf(&b, "url=%s\nbranch=%s\ndir=%s\n", Quote(r.URL), Quote(r.Branch), Quote(dir))
		b.WriteString(`refs=$(git ls-remote --symref "$url" HEAD "refs/heads/$branch")
default=$(printf '%s\n' "$refs" | awk '$1 == "ref:" && $3 == "HEAD" {sub("refs/heads/", "", $2); print $2}')
tip=$(printf '%s\n' "$refs" | awk -v ref="refs/heads/$branch" '$2 == ref {print $1}')
[ -n "$default" ] || { echo 'cannot determine the default branch; check the forge HEAD' >&2; exit 4; }
[ "$branch" != "$default" ] || { echo 'default branch refused; make a work branch first' >&2; exit 4; }
[ -n "$tip" ] || { echo 'branch does not exist on the forge; push it first' >&2; exit 4; }
if [ ! -d "$dir" ]; then
  tmp="$dir.unibox-clone"
  [ ! -e "$tmp" ] || { echo 'incomplete clone remains; inspect it in unibox shell and remove it before retrying' >&2; exit 4; }
  git clone --filter=blob:none --single-branch --branch "$branch" -- "$url" "$tmp"
  mv "$tmp" "$dir"
else
  [ -d "$dir/.git" ] || { echo 'task directory is not a clone; inspect it in unibox shell' >&2; exit 4; }
  [ "$(git -C "$dir" remote get-url origin)" = "$url" ] || { echo 'task origin changed; inspect it in unibox shell' >&2; exit 4; }
  [ "$(git -C "$dir" symbolic-ref --short HEAD)" = "$branch" ] || { echo 'task branch changed; restore it in unibox shell' >&2; exit 4; }
  [ -z "$(git -C "$dir" status --porcelain --untracked-files=all)" ] || { echo 'task has local edits; finish them in unibox shell before preparing again' >&2; exit 4; }
  git -C "$dir" fetch origin "+refs/heads/$branch:refs/remotes/origin/$branch"
  git -C "$dir" merge-base --is-ancestor HEAD "refs/remotes/origin/$branch" || { echo 'task has unpushed or divergent commits; preserve them before continuing' >&2; exit 4; }
  git -C "$dir" merge --ff-only "refs/remotes/origin/$branch"
fi
`)
		fmt.Fprintf(&b, "git -C \"$dir\" config user.name %s\ngit -C \"$dir\" config user.email %s\n", Quote(name), Quote(email))
		b.WriteString("git -C \"$dir\" config push.default simple\n")
		coAuthor := p.Git.CoAuthor
		if r.CoAuthor != nil {
			coAuthor = r.CoAuthor
		}
		// The hook is there either way: with crediting off it still takes
		// out the trailer an agent adds of itself, so off means none.
		credit := "on\n"
		if coAuthor != nil && !*coAuthor {
			credit = "off\n"
		}
		hooks := path.Join(dir, ".git", "unibox-hooks")
		b.WriteString(writeGuest(path.Join(hooks, "commit-msg"), []byte(coAuthorHook()), "700"))
		b.WriteString(writeGuest(path.Join(hooks, "credit"), []byte(credit), "600"))
		fmt.Fprintf(&b, "git -C \"$dir\" config core.hooksPath %s\n", Quote(hooks))
		if p.Bootstrap != "" {
			fmt.Fprintf(&b, "if [ -f %s ]; then (cd \"$dir\" && /bin/sh %s); fi\n", Quote(path.Join(dir, p.Bootstrap)), Quote(p.Bootstrap))
		}
	}
	return b.String()
}

// identityScript gives a clone the agent makes itself the author its git
// service names, as the clones unibox prepares have. Each service's identity
// is a file of its own, included when a repository's remote is under the
// service's URL; git applies the includes in order, so they are listed from
// the least specific URL to the most and the most specific wins. A clone
// unibox prepared keeps its own user.name and user.email, which outrank
// anything global. The index is rewritten every time, so a service that
// left the profile leaves the sandbox's identities too.
func identityScript(p Profile) string {
	type entry struct{ url, file string }
	entries := []entry{}
	var b strings.Builder
	// Without identities only the old files go; an include of a missing
	// file is ignored by git, and the global config is left alone.
	b.WriteString("ids=\"$HOME/.config/unibox/identities\"\nrm -rf \"$ids\"\n")
	names := []string{}
	for name, s := range p.Services {
		if s.Git && s.Author != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return b.String()
	}
	slices.Sort(names)
	b.WriteString("mkdir -p \"$ids\"\n: > \"$ids/index.gitconfig\"\n")
	for _, name := range names {
		s := p.Services[name]
		who, email, _ := AuthorParts(s.Author)
		file := "\"$ids/" + name + ".gitconfig\""
		fmt.Fprintf(&b, "git config -f %s user.name %s\ngit config -f %s user.email %s\n", file, Quote(who), file, Quote(email))
		for _, u := range s.GitURLs() {
			entries = append(entries, entry{u, file})
		}
	}
	slices.SortStableFunc(entries, func(a, b entry) int {
		if len(a.url) != len(b.url) {
			return len(a.url) - len(b.url)
		}
		return strings.Compare(a.url, b.url)
	})
	for _, e := range entries {
		fmt.Fprintf(&b, "git config -f \"$ids/index.gitconfig\" --add %s %s\n", Quote("includeIf.hasconfig:remote.*.url:"+e.url+"**.path"), e.file)
	}
	b.WriteString("git config --global --replace-all include.path \"$ids/index.gitconfig\" 'unibox/identities/index'\n")
	return b.String()
}

// coAuthorHook makes a commit's co-author unibox's to say. The session
// names its agent (UNIBOX_AGENT, as unibox's table calls it) and how the
// profile credits it (UNIBOX_CO_AUTHOR); the hook takes out any co-author
// trailer of either name - agents add their own, copilot always, in their
// own case and address - and, unless the clone's credit file says off,
// adds the profile's. A commit made by hand in unibox shell, with no agent
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
    echo 'a guest session is still running; inspect it with unibox shell' >&2
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
	b.WriteString("*) echo \"${entry##*/}: outside the task's repositories; preserve it before removing\" >&2; exit 4 ;;\nesac\ndone\nfi\necho '  nothing outside the repositories'\n")
	return b.String()
}

// codexSetup does for a generated kit what sbx's own codex kit does: codex
// signs in only through a file of its own, so the proxy's login has to be
// written there. SBX_CRED_OPENAI_MODE says which login sbx wired: oauth
// (ChatGPT, through a provider that sends the proxy's placeholder to
// chatgpt.com), apikey, or none. In none a placeholder would reach OpenAI
// as it is, so it is taken away and codex asks. What unibox writes into
// config.toml sits between markers, rewritten each time; the rest is the
// user's. Top-level keys go before the first table, or TOML would read
// them as the table's. codex is left to work unasked - approval_policy
// never, sandbox_mode danger-full-access - as the VM is the boundary, so a
// codex started by hand in unibox shell is as free as one unibox runs.
// Every task directory and repository under /work is
// trusted, so codex does not ask whether to trust what unibox prepared; a
// directory the user's own part already names is left to it.
const codexSetup = `home="${CODEX_HOME:-$HOME/.codex}"
work="${UNIBOX_WORK:-/work}"
mode="${SBX_CRED_OPENAI_MODE:-none}"
mkdir -p "$home"
cfg="$home/config.toml"
touch "$cfg"
awk -v mode="$mode" '
/^# unibox:begin/ { skip = 1; next }
/^# unibox:end/ { skip = 0; next }
skip { next }
/^[ \t]*\[/ { table = 1 }
!table && /^[ \t]*(approval_policy|sandbox_mode)[ \t]*=/ { next }
!table && (mode == "oauth" || mode == "apikey") && /^[ \t]*forced_login_method[ \t]*=/ { next }
!table && mode == "oauth" && /^[ \t]*model_provider[ \t]*=/ { next }
{ print }' "$cfg" > "$cfg.unibox"
{
  echo '# unibox:begin - rewritten before every session; the sandbox is the boundary'
  echo 'approval_policy = "never"'
  echo 'sandbox_mode = "danger-full-access"'
  case "$mode" in
    oauth|apikey)
      echo 'forced_login_method = "api"'
      [ "$mode" = oauth ] && echo 'model_provider = "sandboxd"'
      ;;
  esac
  echo '# unibox:end'
  cat "$cfg.unibox"
  {
    if [ "$mode" = oauth ]; then
      printf '%s\n' '[model_providers.sandboxd]' 'name = "Sandbox Proxy"' 'base_url = "https://chatgpt.com/backend-api/codex"' 'experimental_bearer_token = "oai-oat01-proxy-managed"' 'requires_openai_auth = false'
    fi
    for d in "$work"/* "$work"/*/*; do
      [ -d "$d" ] || continue
      case "$d" in *.unibox-clone) continue ;; esac
      grep -qxF "[projects.\"$d\"]" "$cfg.unibox" && continue
      printf '[projects."%s"]\ntrust_level = "trusted"\n' "$d"
    done
  } > "$cfg.tail"
  if [ -s "$cfg.tail" ]; then
    echo '# unibox:begin'
    cat "$cfg.tail"
    echo '# unibox:end'
  fi
} > "$cfg.new"
cat "$cfg.new" > "$cfg"
rm -f "$cfg.new" "$cfg.unibox" "$cfg.tail"
case "$mode" in
  oauth|apikey)
    (umask 077; printf '{\n  "OPENAI_API_KEY": "proxy-managed"\n}\n' > "$home/auth.json")
    ;;
  *)
    if grep -q '"proxy-managed"' "$home/auth.json" 2>/dev/null; then rm -f "$home/auth.json"; fi
    ;;
esac
`

func sessionScript(id string, args []string) string {
	s := "set -u\nmkdir -p /var/lib/unibox/runs\nborn=$(awk '{print $22}' /proc/$$/stat)\nprintf '%s %s\\n' \"$$\" \"$born\" > " + Quote("/var/lib/unibox/runs/"+id) + "\ntrap " + Quote("rm -f "+Quote("/var/lib/unibox/runs/"+id)) + " EXIT\n"
	for _, arg := range args {
		s += Quote(arg) + " "
	}
	return s + "\nexit $?\n"
}
