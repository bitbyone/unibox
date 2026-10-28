package unibox

import (
	"path"
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
}

// claudeTrust is what sbx's own claude kit seeds: each directory's trust
// dialog answered, and the bypass mode its kit starts claude in accepted.
func claudeTrust(dirs []string) map[string]any {
	projects := map[string]any{}
	for _, d := range dirs {
		projects[d] = map[string]any{"hasTrustDialogAccepted": true}
	}
	return map[string]any{"bypassPermissionsModeAccepted": true, "projects": projects}
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
	{ID: "claude", Command: "claude", Args: []string{"--dangerously-skip-permissions"}, CoAuthor: "Claude <noreply@anthropic.com>", Config: ".claude.json", Env: []string{"DISABLE_AUTOUPDATER=1"}, Trust: claudeTrust},
	{ID: "codex", Command: "codex", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}, CoAuthor: "Codex <noreply@openai.com>", Setup: codexSetup},
	{ID: "copilot", Command: "copilot", Args: []string{"--yolo"}, CoAuthor: "Copilot <Copilot@users.noreply.github.com>", Config: ".copilot/config.json", Trust: copilotTrust},
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
