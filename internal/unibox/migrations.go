package unibox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// migrations bring a profile from the format of their index to the next
// one, on its YAML so its comments and order survive. Each is best effort:
// what it cannot carry over it leaves out and says so. A migration is frozen
// once released - it describes two past formats, not today's agents - so a
// later change to the agent table does not alter what it does.
var migrations = []func(profile *yaml.Node) (notes []string){
	0: migrateFrom0,
}

// profileVersion is the format a profile's text says it is of: 0 without a
// version, -1 when the version is not a number.
func profileVersion(text []byte) int {
	var doc yaml.Node
	if yaml.Unmarshal(text, &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return 0
	}
	v := mapValue(doc.Content[0], "version")
	if v == nil {
		return 0
	}
	n, err := strconv.Atoi(v.Value)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// migrateProfile brings a profile of an older format up to ProfileFormat,
// one format at a time, and writes it back laid out as init lays one out.
// It returns what it could not carry over. A result that does not load is
// not kept; the profile stays as it was.
func migrateProfile(root, name string) (from int, notes []string, err error) {
	file := filepath.Join(root, "sandboxes", name, "sandbox.yaml")
	before, err := os.ReadFile(file)
	if err != nil {
		return 0, nil, err
	}
	from = profileVersion(before)
	if from == ProfileFormat || from > ProfileFormat {
		return from, nil, nil
	}
	if from < 0 || from >= len(migrations) {
		return from, nil, formatError(fmt.Sprintf("profile %s has a version unibox cannot migrate from; make it again - move sandboxes/%s aside, run unibox init %s, and carry its settings over", name, name, name))
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(before, &doc); err != nil {
		return from, nil, fmt.Errorf("profile %s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return from, nil, fmt.Errorf("profile %s is not a YAML mapping; repair sandbox.yaml by hand", name)
	}
	profile := doc.Content[0]
	for v := from; v < ProfileFormat; v++ {
		notes = append(notes, migrations[v](profile)...)
	}
	setMapValue(profile, "version", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(ProfileFormat)})
	ArrangeProfile(&doc, name)
	out, err := encodeProfile(&doc)
	if err != nil {
		return from, notes, err
	}
	info, err := os.Stat(file)
	if err != nil {
		return from, notes, err
	}
	if err = AtomicWrite(file, out, info.Mode().Perm()); err != nil {
		return from, notes, err
	}
	if _, err = LoadProfile(root, name); err != nil {
		if restore := AtomicWrite(file, before, info.Mode().Perm()); restore != nil {
			return from, notes, fmt.Errorf("%w; and the profile could not be restored: %v", err, restore)
		}
		return from, notes, fmt.Errorf("profile %s of format %d did not migrate to format %d, so it was left as it was; fix what this says in the old form and run init again, or make it again: %w", name, from, ProfileFormat, err)
	}
	return from, notes, nil
}

// migrateFrom0 moves what format 0 said of agents in three sections - auth,
// git.co_authors and agent_config keyed <agent> or <agent>.settings - under
// each agent, as format 1 has it.
func migrateFrom0(profile *yaml.Node) (notes []string) {
	declared := map[string]bool{}
	if agents := mapValue(profile, "agents"); agents != nil {
		switch agents.Kind {
		case yaml.SequenceNode:
			for _, c := range agents.Content {
				declared[c.Value] = true
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(agents.Content); i += 2 {
				declared[agents.Content[i].Value] = true
			}
		}
	}
	// each calls put for every agent entry of a section's mapping.
	each := func(section string, m *yaml.Node, put func(agent string, value *yaml.Node) bool) {
		if m == nil || m.Kind != yaml.MappingNode {
			if m != nil {
				notes = append(notes, fmt.Sprintf("%s is not a mapping; left out", section))
			}
			return
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			key, value := m.Content[i].Value, m.Content[i+1]
			if !put(key, value) {
				notes = append(notes, fmt.Sprintf("%s %s left out", section, key))
			}
		}
	}
	each("auth", mapValue(profile, "auth"), func(agent string, value *yaml.Node) bool {
		if !declared[agent] {
			return false
		}
		setMapValue(agentNode(profile, agent), "auth", value)
		return true
	})
	deleteMapKey(profile, "auth")
	if git := mapValue(profile, "git"); git != nil && git.Kind == yaml.MappingNode {
		each("git.co_authors", mapValue(git, "co_authors"), func(agent string, value *yaml.Node) bool {
			if !declared[agent] {
				return false
			}
			setMapValue(agentNode(profile, agent), "co_author", value)
			return true
		})
		deleteMapKey(git, "co_authors")
		if len(git.Content) == 0 {
			deleteMapKey(profile, "git")
		}
	}
	// The files format 0 knew, by its keys, under their format 1 names.
	files := map[string]string{
		"claude": ".claude.json", "claude.settings": "settings.json",
		"copilot": "config.json", "copilot.settings": "settings.json",
		"opencode": "opencode.json",
	}
	each("agent_config", mapValue(profile, "agent_config"), func(key string, value *yaml.Node) bool {
		file, ok := files[key]
		agent, _, _ := strings.Cut(key, ".")
		if !ok || !declared[agent] {
			return false
		}
		setMapValue(childMap(agentNode(profile, agent), "config"), file, value)
		return true
	})
	deleteMapKey(profile, "agent_config")
	return notes
}
