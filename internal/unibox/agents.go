package unibox

import (
	"fmt"
	"strings"
)

type Agent struct {
	ID, Command, Instructions, CoAuthor string
	// Config is the agent's JSON configuration in the guest home, the file a
	// profile's agent_config seed is merged into; empty when the agent keeps
	// no JSON configuration unibox knows how to merge.
	Config string
	Args   []string
	// Setup runs in the guest before every session of the agent, for what
	// depends on the sandbox's credentials rather than on the profile.
	Setup string
	// Env is set for the agent's process.
	Env []string
	// Trust is merged into Config before every session, naming the
	// directories the session works in, so the agent does not stop to ask
	// whether to trust what unibox prepared.
	Trust func(dirs []string) map[string]any
	// Seed is what unibox init writes into a profile's _data/<agent>.json
	// and names in agent_config: the agent's first questions answered and
	// it left to work unasked, as the sandbox is the boundary. The user
	// edits it there; it is merged into Config before every session.
	Seed map[string]any
	// Files are the agent's further JSON configurations, by the name a
	// profile's agent_config gives them as <agent>.<name>, each with its
	// own seed.
	Files map[string]AgentFile
}

// AgentFile is one more JSON configuration of an agent in the guest home.
type AgentFile struct {
	Path string
	Seed map[string]any
}

// ConfigPath is the file in the guest home an agent_config key names:
// <agent> its main configuration, <agent>.<name> one of its Files.
func ConfigPath(key string) (Agent, string, error) {
	id, file, _ := strings.Cut(key, ".")
	a, err := FindAgent(id)
	if err != nil {
		return a, "", err
	}
	if file == "" {
		if a.Config == "" {
			return a, "", fmt.Errorf("agent %s keeps no JSON configuration unibox can merge; remove it from agent_config", id)
		}
		return a, a.Config, nil
	}
	f, ok := a.Files[file]
	if !ok {
		return a, "", fmt.Errorf("agent %s has no configuration %s unibox knows; name one of its own or %s alone", id, file, id)
	}
	return a, f.Path, nil
}

// claudeTrust is what sbx's own claude kit seeds: each directory's trust
// dialog answered, and the bypass mode unibox starts claude in accepted.
func claudeTrust(dirs []string) map[string]any {
	projects := map[string]any{}
	for _, d := range dirs {
		projects[d] = map[string]any{"hasTrustDialogAccepted": true}
	}
	return map[string]any{"bypassPermissionsModeAccepted": true, "projects": projects}
}

var Agents = []Agent{
	// The image fixes each agent's version, so none updates itself inside.
	{ID: "claude", Command: "claude", Instructions: ".claude/CLAUDE.md", CoAuthor: "Claude <noreply@anthropic.com>", Config: ".claude.json", Args: []string{"--dangerously-skip-permissions"}, Env: []string{"DISABLE_AUTOUPDATER=1"}, Trust: claudeTrust,
		Seed: map[string]any{"hasCompletedOnboarding": true, "theme": "dark", "bypassPermissionsModeAccepted": true},
		// unibox's commit hook says the co-author; claude's own byline is off.
		Files: map[string]AgentFile{"settings": {Path: ".claude/settings.json", Seed: map[string]any{"includeCoAuthoredBy": false}}}},
	{ID: "codex", Command: "codex", Instructions: ".codex/AGENTS.md", CoAuthor: "Codex <noreply@openai.com>", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}, Setup: codexSetup},
	{ID: "copilot", Command: "copilot", Instructions: ".copilot/copilot-instructions.md", CoAuthor: "Copilot <223556219+Copilot@users.noreply.github.com>", Config: ".copilot/config.json", Args: []string{"--allow-all"}, Env: []string{"COPILOT_ALLOW_ALL=true"},
		// Its settings.json, per copilot help config: unibox's commit hook
		// says the co-author, the image fixes the version, /work is trusted.
		Files: map[string]AgentFile{"settings": {Path: ".copilot/settings.json", Seed: map[string]any{
			"theme": "default", "includeCoAuthoredBy": false, "autoUpdate": false, "banner": "never", "trustedFolders": []any{"/work"},
		}}}},
	{ID: "opencode", Command: "opencode", Instructions: ".config/opencode/AGENTS.md", CoAuthor: "opencode <noreply@opencode.ai>", Config: ".config/opencode/opencode.json",
		Seed: map[string]any{"$schema": "https://opencode.ai/config.json", "autoupdate": false, "permission": map[string]any{"edit": "allow", "bash": "allow", "webfetch": "allow"}}},
}

func FindAgent(id string) (Agent, error) {
	for _, a := range Agents {
		if a.ID == id {
			return a, nil
		}
	}
	return Agent{}, fmt.Errorf("unknown agent %q; choose one listed by unibox profiles", id)
}
