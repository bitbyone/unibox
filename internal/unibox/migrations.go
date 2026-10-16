package unibox

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// migrations bring a profile from the format of their index to the next
// one, on its YAML so its comments and order survive. Each is best effort:
// what it cannot carry over it leaves out and says so. A migration is frozen
// once released - it describes two past formats, not today's agents - so a
// later change to the agent table does not alter what it does.
var migrations = []func(profile *yaml.Node) (moves []seedMove, notes []string){
	0: migrateFrom0,
	1: migrateFrom1,
	2: migrateFrom2,
	3: migrateFrom3,
}

// seedMove is a file of a profile a migration renames, both relative to
// the profile's directory.
type seedMove struct{ from, to string }

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
	var moves []seedMove
	for v := from; v < ProfileFormat; v++ {
		m, n := migrations[v](profile)
		moves, notes = append(moves, m...), append(notes, n...)
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
	dir := filepath.Dir(file)
	var moved []seedMove
	undo := func() {
		for _, m := range slices.Backward(moved) {
			_ = os.Rename(filepath.Join(dir, m.to), filepath.Join(dir, m.from))
		}
	}
	for _, m := range moves {
		to := filepath.Join(dir, m.to)
		if _, statErr := os.Stat(to); statErr == nil {
			notes = append(notes, fmt.Sprintf("%s already there, so %s was left as it was", m.to, m.from))
			continue
		}
		if err = os.MkdirAll(filepath.Dir(to), 0o755); err == nil {
			err = os.Rename(filepath.Join(dir, m.from), to)
		}
		if err != nil {
			undo()
			return from, notes, err
		}
		moved = append(moved, m)
	}
	if err = AtomicWrite(file, out, info.Mode().Perm()); err != nil {
		undo()
		return from, notes, err
	}
	if _, err = LoadProfile(root, name); err != nil {
		undo()
		if restore := AtomicWrite(file, before, info.Mode().Perm()); restore != nil {
			return from, notes, fmt.Errorf("%w; and the profile could not be restored: %v", err, restore)
		}
		return from, notes, fmt.Errorf("profile %s of format %d did not migrate to format %d, so it was left as it was; fix what this says in the old form and run init again, or make it again: %w", name, from, ProfileFormat, err)
	}
	return from, notes, nil
}

// migrateFrom0 moves what format 0 said of agents in three sections - auth,
// git.co_authors and agent_config keyed <agent> or <agent>.settings - under
// each agent, as format 1 has it. A seed where format 0's init put it,
// _data/<key>.json, moves to format 1's place, _data/<agent>/<file name>.
func migrateFrom0(profile *yaml.Node) (moves []seedMove, notes []string) {
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
		if value.Kind == yaml.ScalarNode && value.Value == "_data/"+key+".json" {
			to := "_data/" + agent + "/" + file
			moves = append(moves, seedMove{value.Value, to})
			value = scalarNode(to)
		}
		setMapValue(childMap(agentNode(profile, agent), "config"), file, value)
		return true
	})
	deleteMapKey(profile, "agent_config")
	dropTemplate0(profile)
	return moves, notes
}

// dropTemplate0 takes out the commented-out sections format 0's init wrote
// under a profile, in each of its wordings: from "# How each agent signs
// in:" to its last line, "# deny_network: ...". Format 1 keeps such hints
// in a footer it rewrites, so the old ones would only repeat it, wrongly.
func dropTemplate0(n *yaml.Node) {
	for _, c := range []*string{&n.HeadComment, &n.LineComment, &n.FootComment} {
		lines := strings.Split(*c, "\n")
		start := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "# How each agent signs in:") })
		if start < 0 {
			continue
		}
		end := len(lines)
		for i := start; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "# deny_network:") {
				end = i + 1
				break
			}
		}
		*c = strings.TrimSpace(strings.Join(append(lines[:start:start], lines[end:]...), "\n"))
	}
	for _, child := range n.Content {
		dropTemplate0(child)
	}
}

// migrateFrom1 groups format 1's top-level cpus and memory under resources,
// on one line; their comments go to resources.
func migrateFrom1(profile *yaml.Node) (moves []seedMove, notes []string) {
	resources := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
	var head, line []string
	for i := 0; i+1 < len(profile.Content); i += 2 {
		k, v := profile.Content[i], profile.Content[i+1]
		if k.Value != "cpus" && k.Value != "memory" {
			continue
		}
		for _, c := range []string{k.HeadComment, v.HeadComment} {
			if c != "" {
				head = append(head, c)
			}
		}
		for _, c := range []string{k.LineComment, v.LineComment, k.FootComment, v.FootComment} {
			if c != "" {
				line = append(line, strings.TrimPrefix(c, "# "))
			}
		}
		resources.Content = append(resources.Content, scalarNode(k.Value), &yaml.Node{Kind: v.Kind, Tag: v.Tag, Value: v.Value, Style: v.Style})
	}
	if len(resources.Content) == 0 {
		return nil, nil
	}
	deleteMapKey(profile, "cpus")
	deleteMapKey(profile, "memory")
	setMapValue(profile, "resources", resources)
	key := profile.Content[len(profile.Content)-2]
	key.HeadComment = strings.Join(head, "\n")
	if len(line) > 0 {
		resources.LineComment = "# " + strings.Join(line, "; ")
	}
	return nil, nil
}

// format2NetworkComment is what format 2's layout wrote above its network
// rules; it does not travel into network, which has its own.
const format2NetworkComment = "# Hosts beyond the runtime's policy, and hosts refused; applied live -\n# unibox network allow|deny."

// migrateFrom2 moves format 2's allow_network and deny_network under
// network, as allow and deny, with the user's own comments.
func migrateFrom2(profile *yaml.Node) (moves []seedMove, notes []string) {
	var pairs []*yaml.Node
	for _, old := range []string{"allow_network", "deny_network"} {
		for i := 0; i+1 < len(profile.Content); i += 2 {
			k := profile.Content[i]
			if k.Value != old {
				continue
			}
			if k.HeadComment == format2NetworkComment {
				k.HeadComment = ""
			}
			k.Value = strings.TrimSuffix(old, "_network")
			pairs = append(pairs, k, profile.Content[i+1])
		}
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	deleteMapKey(profile, "allow")
	deleteMapKey(profile, "deny")
	network := childMap(profile, "network")
	network.Content = append(network.Content, pairs...)
	return nil, nil
}

// migrateFrom3 drops bootstrap: a script the worked-on repository had to
// carry, which format 4 no longer runs.
func migrateFrom3(profile *yaml.Node) (moves []seedMove, notes []string) {
	if v := mapValue(profile, "bootstrap"); v != nil {
		deleteMapKey(profile, "bootstrap")
		notes = append(notes, "bootstrap "+v.Value+" left out: unibox no longer runs a script from the worked-on repositories")
	}
	return nil, notes
}
