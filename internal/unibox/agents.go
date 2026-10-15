package unibox

import (
	"fmt"
	"strings"
)

type Agent struct {
	ID, Command, Instructions, CoAuthor string
	// Config is the JSON file Trust is merged into.
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
	// Configs are the agent's configuration files unibox knows how to merge
	// a seed into, by the name the agent gives each. A profile names them
	// under the agent's config, and init seeds them from here: the agent's
	// first questions answered and it left to work unasked, as the sandbox
	// is the boundary. The user edits the seeds in _data.
	Configs []AgentFile
}

// AgentFile is one configuration file of an agent in the guest home, JSON
// or, by its name, TOML.
type AgentFile struct {
	Name, Path string
	Seed       map[string]any
}

// TOML says whether the file is TOML rather than JSON.
func (f AgentFile) TOML() bool { return strings.HasSuffix(f.Name, ".toml") }

// SeedPath is where init keeps a file's seed in a profile: in the agent's
// own directory, under the file's own name.
func (f AgentFile) SeedPath(agent string) string {
	return "_data/" + agent + "/" + f.Name
}

// ConfigFile is the file of an agent a profile's config names.
func ConfigFile(agent, name string) (Agent, AgentFile, error) {
	a, err := FindAgent(agent)
	if err != nil {
		return a, AgentFile{}, err
	}
	for _, f := range a.Configs {
		if f.Name == name {
			return a, f, nil
		}
	}
	names := []string{}
	for _, f := range a.Configs {
		names = append(names, f.Name)
	}
	if len(names) == 0 {
		return a, AgentFile{}, fmt.Errorf("agent %s has no configuration unibox can seed", agent)
	}
	return a, AgentFile{}, fmt.Errorf("agent %s has no configuration %s unibox can seed; it knows %s", agent, name, strings.Join(names, ", "))
}

// agentConfigKey is how a profile's AgentConfig keys one file of an agent.
func agentConfigKey(agent, name string) string { return agent + "/" + name }

func splitAgentConfigKey(key string) (string, string) {
	agent, name, _ := strings.Cut(key, "/")
	return agent, name
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
	// The image fixes each agent's version, so none updates itself inside,
	// and unibox's commit hook says the co-author, so no agent's own byline.
	{ID: "claude", Command: "claude", Instructions: ".claude/CLAUDE.md", CoAuthor: "Claude <noreply@anthropic.com>", Config: ".claude.json", Args: []string{"--dangerously-skip-permissions"}, Env: []string{"DISABLE_AUTOUPDATER=1"}, Trust: claudeTrust,
		Configs: []AgentFile{
			{Name: ".claude.json", Path: ".claude.json", Seed: map[string]any{"hasCompletedOnboarding": true, "theme": "dark", "bypassPermissionsModeAccepted": true}},
			{Name: "settings.json", Path: ".claude/settings.json", Seed: map[string]any{"includeCoAuthoredBy": false}},
		}},
	// codex works unasked - the VM is the boundary - so one started by hand
	// in unibox shell is as free as one unibox runs.
	{ID: "codex", Command: "codex", Instructions: ".codex/AGENTS.md", CoAuthor: "Codex <noreply@openai.com>", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}, Setup: codexSetup,
		Configs: []AgentFile{
			{Name: "config.toml", Path: ".codex/config.toml", Seed: map[string]any{"approval_policy": "never", "sandbox_mode": "danger-full-access"}},
		}},
	// Its settings.json, per copilot help config; config.json is the state
	// it keeps itself, seeded only by a profile that wants to.
	{ID: "copilot", Command: "copilot", Instructions: ".copilot/copilot-instructions.md", CoAuthor: "Copilot <Copilot@users.noreply.github.com>", Args: []string{"--allow-all"}, Env: []string{"COPILOT_ALLOW_ALL=true"},
		Configs: []AgentFile{
			{Name: "settings.json", Path: ".copilot/settings.json", Seed: map[string]any{
				"theme": "default", "includeCoAuthoredBy": false, "autoUpdate": false, "banner": "never", "trustedFolders": []any{"/work"},
			}},
			{Name: "config.json", Path: ".copilot/config.json"},
		}},
	// --auto approves what the seed does not name either, as the other
	// agents' flags do; the sandbox is the boundary.
	{ID: "opencode", Command: "opencode", Instructions: ".config/opencode/AGENTS.md", CoAuthor: "opencode <noreply@opencode.ai>", Args: []string{"--auto"},
		Configs: []AgentFile{
			{Name: "opencode.json", Path: ".config/opencode/opencode.json", Seed: map[string]any{"$schema": "https://opencode.ai/config.json", "autoupdate": false, "permission": map[string]any{"edit": "allow", "bash": "allow", "webfetch": "allow"}}},
		}},
}

func FindAgent(id string) (Agent, error) {
	for _, a := range Agents {
		if a.ID == id {
			return a, nil
		}
	}
	return Agent{}, fmt.Errorf("unknown agent %q; choose one listed by unibox profiles", id)
}
