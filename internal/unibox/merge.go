package unibox

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tobola/unibox/internal/unibox/api"
)

// mergeJSON lays the seed over the current configuration: a key the seed
// names wins, maps merge recursively, a list keeps what the agent added to
// it - a folder trusted during a session - with the seed's items joined,
// and everything else - the agent's own state, projects, paths - stays as
// it is.
func mergeJSON(current, seed map[string]any) map[string]any {
	for key, value := range seed {
		if sm, ok := value.(map[string]any); ok {
			if cm, ok := current[key].(map[string]any); ok {
				current[key] = mergeJSON(cm, sm)
				continue
			}
		}
		if sl, ok := value.([]any); ok {
			if cl, ok := current[key].([]any); ok {
				for _, item := range sl {
					if !slices.ContainsFunc(cl, func(c any) bool { return fmt.Sprint(c) == fmt.Sprint(item) }) {
						cl = append(cl, item)
					}
				}
				current[key] = cl
				continue
			}
		}
		current[key] = value
	}
	return current
}

// splitJSONComments takes the leading // lines off a configuration - copilot
// heads its config.json with them - so the rest parses as JSON and they can
// be put back as they were.
func splitJSONComments(text string) (head, body string) {
	lines := strings.SplitAfter(text, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "//") {
		i++
	}
	return strings.Join(lines[:i], ""), strings.Join(lines[i:], "")
}

// mergeAgentConfig lays a seed over the JSON file the agent's Trust goes to.
func (m *Manager) mergeAgentConfig(ctx context.Context, b api.Sandbox, a Agent, seed map[string]any) error {
	return m.mergeConfigFile(ctx, b, a.ID, a.Config, seed)
}

// mergeConfigFile lays a seed over one of an agent's configuration files in
// the guest home, JSON or TOML, writing only when something changes.
func (m *Manager) mergeConfigFile(ctx context.Context, b api.Sandbox, agent, file string, seed map[string]any) error {
	out, err := guestOutput(ctx, m.SBX, b.Name, "bash", "-c", "cat \"$HOME/"+file+"\" 2>/dev/null || true")
	if err != nil {
		return err
	}
	var merged []byte
	if strings.HasSuffix(file, ".toml") {
		merged, err = mergeTOML(out, seed)
	} else {
		merged, err = mergeJSONText(out, seed)
	}
	if err != nil {
		return fmt.Errorf("agent %s configuration in sandbox %s: %v; repair or remove ~/%s before seeding it", agent, b.Name, err, file)
	}
	if merged == nil {
		return nil
	}
	script := "set -eu\nmkdir -p \"$HOME/" + path.Dir(file) + "\"\nprintf %s " + Quote(base64.StdEncoding.EncodeToString(merged)) + " | base64 -d > \"$HOME/" + file + "\"\n"
	return runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-s"}, Input: strings.NewReader(script)}), m.Log)
}

// mergeJSONText lays a seed over a JSON configuration, keeping its leading
// // lines; nil when nothing changes.
func mergeJSONText(text string, seed map[string]any) ([]byte, error) {
	current := map[string]any{}
	head, body := splitJSONComments(text)
	if strings.TrimSpace(head) != "" {
		head = strings.TrimRight(head, "\n") + "\n"
	}
	if strings.TrimSpace(body) != "" {
		if err := json.Unmarshal([]byte(body), &current); err != nil {
			return nil, fmt.Errorf("not valid JSON")
		}
	}
	before, _ := json.Marshal(current)
	mergedMap := mergeJSON(current, seed)
	after, _ := json.Marshal(mergedMap)
	if bytes.Equal(before, after) {
		return nil, nil
	}
	merged, err := json.MarshalIndent(mergedMap, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(head), append(merged, '\n')...), nil
}

// mergeTOML lays a seed over a TOML configuration; nil when nothing
// changes. The blocks between unibox's markers - written before each
// session by the agent's own setup - are kept where they were: one before
// any table stays at the top, the rest go after.
func mergeTOML(text string, seed map[string]any) ([]byte, error) {
	var top, tail, body strings.Builder
	inBlock, table := false, false
	var block *strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# unibox:begin"):
			inBlock = true
			block = &tail
			if !table {
				block = &top
			}
			block.WriteString(line)
		case strings.HasPrefix(trimmed, "# unibox:end"):
			inBlock = false
			block.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				block.WriteString("\n")
			}
		case inBlock:
			block.WriteString(line)
		default:
			if strings.HasPrefix(trimmed, "[") {
				table = true
			}
			body.WriteString(line)
		}
	}
	current := map[string]any{}
	if err := toml.Unmarshal([]byte(body.String()), &current); err != nil {
		return nil, fmt.Errorf("not valid TOML")
	}
	before, _ := json.Marshal(current)
	mergedMap := mergeJSON(current, seed)
	after, _ := json.Marshal(mergedMap)
	if bytes.Equal(before, after) {
		return nil, nil
	}
	merged, err := toml.Marshal(mergedMap)
	if err != nil {
		return nil, err
	}
	return []byte(top.String() + string(merged) + tail.String()), nil
}
