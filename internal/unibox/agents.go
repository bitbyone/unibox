package unibox

import (
	"path"
	"slices"
	"strings"
)

// Agent is what unibox knows of a coding agent a kit runs, beyond how the
// kit starts it: what to answer before a session so it does not stop to
// ask, and how its commits credit it.
type Agent struct {
	ID, Command, CoAuthor string
	// Args are what sbx's own kit for the agent starts it with, used when
	// no kit of the env names the agent but its sandbox has it.
	Args []string
	// Config is the JSON file in the guest home Trust is merged into.
	Config string
	// Setup runs in the guest before every session of the agent.
	Setup string
	// Env is set for the agent's process.
	Env []string

	// Trust is merged into Config before every session, naming the
	// directories the session works in, so the agent does not stop to ask
	// whether to trust what unibox prepared.
	Trust func(dirs []string) map[string]any

	// Resume starts the agent, from the command it is started with, on the
	// last conversation it had in the session's directory, and HadSession
	// is a guest shell test, of $dir, for whether it had one there: an
	// agent asked to continue where it has none fails rather than starting
	// afresh. Without both, every session starts afresh.
	Resume     func(command []string) []string
	HadSession string
}

// claudeHadSession finds claude's conversations of a directory where it
// keeps them, the directory's path with every other character a dash.
const claudeHadSession = `ls "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects/$(printf %s "$dir" | sed 's/[^A-Za-z0-9]/-/g')/"*.jsonl >/dev/null 2>&1`

// codexHadSession finds a codex session recorded with the directory as
// its working directory, which codex resume --last picks among.
const codexHadSession = `grep -rqsF --include='rollout-*.jsonl' "\"cwd\":\"$dir\"" "${CODEX_HOME:-$HOME/.codex}/sessions"`

// copilotHadSession finds the session copilot last had in the directory:
// its --continue takes the last one anywhere, which in a sandbox of
// several tasks is another task's. A session's workspace.yaml keeps the
// directory it began in, its events.jsonl is written to as it goes on, so
// the newest of those is the last; the test leaves its id in $session.
const copilotHadSession = `{ session= newest=
for w in "${COPILOT_HOME:-$HOME/.copilot}"/session-state/*/workspace.yaml; do
grep -qxF "cwd: $dir" "$w" 2>/dev/null || continue
e=${w%/workspace.yaml}/events.jsonl; [ -e "$e" ] || e=$w
if [ -z "$newest" ] || [ "$e" -nt "$newest" ]; then newest=$e; session=${w%/workspace.yaml}; session=${session##*/}; fi
done
[ -n "$session" ]; }`

// sessionArg stands in an argument of Resume for $session, the session a
// HadSession test found, which the guest's shell puts in its place. An
// argument cannot hold a NUL, so no argument a person gives is taken for it.
const sessionArg = "\x00session\x00"

// claudeTrust is what sbx's own claude kit seeds: each directory's trust
// dialog answered, and the bypass mode its kit starts claude in accepted.
func claudeTrust(dirs []string) map[string]any {
	projects := map[string]any{}
	for _, d := range dirs {
		projects[d] = map[string]any{"hasTrustDialogAccepted": true}
	}
	// unibox runs claude bypassing permissions, so its offer to make auto
	// mode the default would change nothing a session does.
	return map[string]any{"bypassPermissionsModeAccepted": true, "hasSeenAutoDefaultNudge": true, "projects": projects}
}

// copilotTrust names the directories in copilot's trusted_folders, a list
// the merge adds to.
func copilotTrust(dirs []string) map[string]any {
	folders := []any{}
	for _, d := range dirs {
		folders = append(folders, d)
	}
	return map[string]any{"trusted_folders": folders}
}

var Agents = []Agent{
	// The kit's image fixes each agent's version, so none updates itself,
	// and unibox's commit hook says the co-author, so no agent's own byline.
	{ID: "claude", Command: "claude", Args: []string{"--dangerously-skip-permissions"}, CoAuthor: "Claude <noreply@anthropic.com>", Config: ".claude.json", Env: []string{"DISABLE_AUTOUPDATER=1"}, Trust: claudeTrust,
		Resume: func(c []string) []string { return append(slices.Clone(c), "--continue") }, HadSession: claudeHadSession},
	{ID: "codex", Command: "codex", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}, CoAuthor: "Codex <noreply@openai.com>", Setup: codexSetup,
		// resume takes the options codex itself does, after it.
		Resume: func(c []string) []string { return append([]string{c[0], "resume", "--last"}, c[1:]...) }, HadSession: codexHadSession},
	{ID: "copilot", Command: "copilot", Args: []string{"--yolo"}, CoAuthor: "Copilot <Copilot@users.noreply.github.com>", Config: ".copilot/config.json", Trust: copilotTrust,
		Resume: func(c []string) []string { return append(slices.Clone(c), "--resume="+sessionArg) }, HadSession: copilotHadSession},
	{ID: "opencode", Command: "opencode", CoAuthor: "opencode <noreply@opencode.ai>"},
}

// AgentFor is the agent a kit's entrypoint starts, by the command's name;
// an agent unibox does not know is still run, with nothing prepared.
func AgentFor(entrypoint []string) Agent {
	if len(entrypoint) == 0 {
		return Agent{}
	}
	command := path.Base(entrypoint[0])
	for _, a := range Agents {
		if a.Command == command {
			return a
		}
	}
	return Agent{ID: strings.ToLower(command), Command: command}
}

// FindAgent is the agent of an ID unibox recorded.
func FindAgent(id string) Agent {
	for _, a := range Agents {
		if a.ID == id {
			return a
		}
	}
	return Agent{ID: id, Command: id}
}
