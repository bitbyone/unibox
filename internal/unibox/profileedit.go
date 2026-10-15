package unibox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EditProfile changes a profile's sandbox.yaml as a node tree, so the user's
// comments and order stay, and keeps the change only if the profile still
// loads - a command that writes the profile never leaves it broken.
func (m *Manager) EditProfile(name string, edit func(doc *yaml.Node) error) error {
	p, err := m.profile(name)
	if err != nil {
		return err
	}
	file := filepath.Join(p.Dir, "sandbox.yaml")
	before, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(before, &doc); err != nil {
		return fmt.Errorf("profile %s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("profile %s is not a YAML mapping; repair sandbox.yaml by hand", name)
	}
	if err = edit(doc.Content[0]); err != nil {
		return err
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err = enc.Encode(&doc); err != nil {
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if err = AtomicWrite(file, out.Bytes(), info.Mode().Perm()); err != nil {
		return err
	}
	if _, err = LoadProfile(m.Config.Environments, name); err != nil {
		if restore := AtomicWrite(file, before, info.Mode().Perm()); restore != nil {
			return fmt.Errorf("%w; and the profile could not be restored: %v", err, restore)
		}
		return fmt.Errorf("the change would break profile %s, so it was not kept: %w", name, err)
	}
	// The command that made the change has said what it did; the next start
	// need not tell it again.
	return m.RecordAccess(name)
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue replaces a key's value in place, keeping its comments, or
// appends the key at the end.
func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			old := m.Content[i+1]
			value.LineComment = old.LineComment
			if value.HeadComment == "" {
				value.HeadComment = old.HeadComment
			}
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, scalarNode(key), value)
}

func deleteMapKey(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}

// childMap is the mapping under key, made when missing - a null value, as
// an emptied `services:` leaves, counts as missing.
func childMap(m *yaml.Node, key string) *yaml.Node {
	if v := mapValue(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapValue(m, key, v)
	return v
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// SetAgentAuth writes how an agent signs in: the method's name, or
// {command: ...} and {token: ...} on one line, as the scaffold writes them.
func SetAgentAuth(profile *yaml.Node, agent string, a AgentAuth) {
	auth := childMap(profile, "auth")
	if a.Method == AuthCommand || a.Method == AuthToken {
		setMapValue(auth, agent, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle, Content: []*yaml.Node{scalarNode(a.Method), scalarNode(a.Command)}})
		return
	}
	setMapValue(auth, agent, scalarNode(a.Method))
}

// SetService writes a service's entry, its fields in the order the
// documentation shows them and only those it sets.
func SetService(profile *yaml.Node, name string, s Service) {
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(key string, value *yaml.Node) { entry.Content = append(entry.Content, scalarNode(key), value) }
	if s.Git {
		add("git", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	if len(s.Host) == 1 {
		add("host", scalarNode(s.Host[0]))
	} else {
		hosts := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
		for _, h := range s.Host {
			hosts.Content = append(hosts.Content, scalarNode(h))
		}
		add("host", hosts)
	}
	for _, f := range []struct{ key, value string }{{"scheme", s.Scheme}, {"user", s.User}, {"env", s.Env}, {"command", s.Command}, {"refresh", s.Refresh}, {"author", s.Author}} {
		if f.value != "" {
			add(f.key, scalarNode(f.value))
		}
	}
	if s.CoAuthor != nil {
		add("co_author", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(*s.CoAuthor)})
	}
	setMapValue(childMap(profile, "services"), name, entry)
}

// RemoveService takes a service's entry out, and the services key with the
// last one.
func RemoveService(profile *yaml.Node, name string) bool {
	services := mapValue(profile, "services")
	if services == nil || services.Kind != yaml.MappingNode || !deleteMapKey(services, name) {
		return false
	}
	if len(services.Content) == 0 {
		deleteMapKey(profile, "services")
	}
	return true
}

// SetStringList writes a list of the profile, on one line as the scaffold
// writes them, and takes the key out when it is empty.
func SetStringList(profile *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		deleteMapKey(profile, key)
		return
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	if old := mapValue(profile, key); old != nil && old.Kind == yaml.SequenceNode {
		list.Style = old.Style
	}
	for _, v := range values {
		list.Content = append(list.Content, scalarNode(v))
	}
	setMapValue(profile, key, list)
}

// SetCoAuthor writes how an agent is credited under git.co_authors; an
// empty value takes it out, and the maps with the last one.
func SetCoAuthor(profile *yaml.Node, agent, coAuthor string) {
	if coAuthor != "" {
		setMapValue(childMap(childMap(profile, "git"), "co_authors"), agent, scalarNode(coAuthor))
		return
	}
	git := mapValue(profile, "git")
	if git == nil || git.Kind != yaml.MappingNode {
		return
	}
	authors := mapValue(git, "co_authors")
	if authors == nil || authors.Kind != yaml.MappingNode {
		return
	}
	deleteMapKey(authors, agent)
	if len(authors.Content) == 0 {
		deleteMapKey(git, "co_authors")
	}
	if len(git.Content) == 0 {
		deleteMapKey(profile, "git")
	}
}
