package unibox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// A profile path names a value in sandbox.yaml by its keys, joined by dots:
// resources.cpus, agents.claude.args. A key holding a dot, or an item of a
// list by its index, goes in brackets: agents.claude.config[settings.json],
// env[0]. Any field can so be read and written by a program, as an editor
// would; what the value must be is the profile's own validation, applied
// before anything is kept. Empty brackets at the end, env[], add an item to
// the end of a list.

// ParseProfilePath splits a path into its keys.
func ParseProfilePath(path string) ([]string, error) {
	keys := []string{}
	rest := path
	for rest != "" {
		switch {
		case rest[0] == '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("path %q: a bracket holds a key or an index, as config[settings.json] or env[0]", path)
			}
			if end == 1 && len(rest) > 2 {
				return nil, fmt.Errorf("path %q: empty brackets add to a list, so they come last", path)
			}
			keys = append(keys, rest[1:end])
			rest = rest[end+1:]
			if strings.HasPrefix(rest, ".") {
				rest = rest[1:]
				if rest == "" {
					return nil, fmt.Errorf("path %q ends in a dot", path)
				}
			} else if rest != "" && rest[0] != '[' {
				return nil, fmt.Errorf("path %q: a dot or a bracket follows a bracket", path)
			}
		default:
			end := strings.IndexAny(rest, ".[")
			if end == 0 {
				return nil, fmt.Errorf("path %q has an empty key", path)
			}
			if end < 0 {
				end = len(rest)
			}
			keys = append(keys, rest[:end])
			rest = rest[end:]
			if strings.HasPrefix(rest, ".") {
				rest = rest[1:]
				if rest == "" {
					return nil, fmt.Errorf("path %q ends in a dot", path)
				}
			}
		}
	}
	if len(keys) == 0 || keys[0] == "" {
		return nil, fmt.Errorf("no path given; name a field, as resources.cpus")
	}
	switch keys[0] {
	case "version":
		return nil, fmt.Errorf("version is the profile's format, which unibox keeps itself")
	}
	return keys, nil
}

// ParseProfileValue reads a value as YAML: 4, true, a string, [a, b] or
// {key: value}.
func ParseProfileValue(value string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(value), &doc); err != nil {
		return nil, fmt.Errorf("value %q is not YAML: %w", value, err)
	}
	if len(doc.Content) != 1 {
		return nil, fmt.Errorf("no value given; to take a field out use unset")
	}
	return doc.Content[0], nil
}

// SetProfilePath puts a value at a path, making the mappings on the way.
func SetProfilePath(profile *yaml.Node, keys []string, value *yaml.Node) error {
	node, keys := profile, keys
	if len(keys) == 2 && keys[0] == "agents" && keys[1] == "" {
		// agents[] codex declares one more agent, whichever form agents has.
		if value.Kind != yaml.ScalarNode {
			return fmt.Errorf("agents[] takes an agent's name")
		}
		if agents := mapValue(profile, "agents"); agents != nil && agents.Kind == yaml.MappingNode {
			setMapValue(agents, value.Value, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			return nil
		}
	} else if keys[0] == "agents" && len(keys) > 1 {
		// A list of agents' names becomes a mapping, as for auth, keeping
		// the order sbx signs in by.
		body := agentNode(profile, keys[1])
		if len(keys) == 2 {
			setMapValue(mapValue(profile, "agents"), keys[1], value)
			return nil
		}
		node, keys = body, keys[2:]
	}
	for i, key := range keys {
		last := i == len(keys)-1
		switch node.Kind {
		case yaml.MappingNode:
			if key == "" {
				return fmt.Errorf("%s is a mapping, not a list; name the key in it", strings.Join(keys[:i], "."))
			}
			if last {
				setMapValue(node, key, value)
				return nil
			}
			child := mapValue(node, key)
			if child == nil {
				child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				if keys[i+1] == "" {
					child = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
				}
				setMapValue(node, key, child)
			}
			node = child
		case yaml.SequenceNode:
			if key == "" {
				node.Content = append(node.Content, value)
				return nil
			}
			n, err := index(node, key)
			if err != nil {
				return err
			}
			if last {
				node.Content[n] = value
				return nil
			}
			node = node.Content[n]
		default:
			return fmt.Errorf("%s is a single value, with no %s in it", strings.Join(keys[:i], "."), key)
		}
	}
	return nil
}

// UnsetProfilePath takes the value at a path out, and the mappings it
// leaves empty, but never an agent the profile declares.
func UnsetProfilePath(profile *yaml.Node, keys []string) error {
	if keys[len(keys)-1] == "" {
		return fmt.Errorf("empty brackets add to a list; take an item out by its index, as env[0]")
	}
	chain := []*yaml.Node{profile}
	node := profile
	for i, key := range keys {
		var child *yaml.Node
		switch node.Kind {
		case yaml.MappingNode:
			child = mapValue(node, key)
		case yaml.SequenceNode:
			if n, err := index(node, key); err == nil {
				child = node.Content[n]
			} else if i == 1 && keys[0] == "agents" {
				// agents: [claude, codex] - an agent by its name.
				for _, c := range node.Content {
					if c.Value == key {
						child = c
					}
				}
			} else {
				return err
			}
		}
		if child == nil {
			return fmt.Errorf("the profile sets no %s", strings.Join(keys[:i+1], "."))
		}
		chain = append(chain, child)
		node = child
	}
	for i := len(keys) - 1; i >= 0; i-- {
		remove(chain[i], keys[i])
		// An emptied mapping or list goes too, but an agent's empty body
		// still declares the agent.
		if i == 0 || len(chain[i].Content) > 0 || keys[0] == "agents" && i == 2 {
			break
		}
	}
	// Agents nothing is said of are a list of names again.
	if agents := mapValue(profile, "agents"); keys[0] == "agents" && agents != nil && agents.Kind == yaml.MappingNode {
		names := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, HeadComment: agents.HeadComment, LineComment: agents.LineComment}
		for i := 0; i+1 < len(agents.Content); i += 2 {
			body := agents.Content[i+1]
			if body.Kind == yaml.MappingNode && len(body.Content) > 0 || body.Kind == yaml.ScalarNode && body.Tag != "!!null" {
				return nil
			}
			names.Content = append(names.Content, scalarNode(agents.Content[i].Value))
		}
		setMapValue(profile, "agents", names)
	}
	return nil
}

func remove(parent *yaml.Node, key string) {
	switch parent.Kind {
	case yaml.MappingNode:
		deleteMapKey(parent, key)
	case yaml.SequenceNode:
		if n, err := index(parent, key); err == nil {
			parent.Content = append(parent.Content[:n], parent.Content[n+1:]...)
			return
		}
		for n, c := range parent.Content {
			if c.Value == key {
				parent.Content = append(parent.Content[:n], parent.Content[n+1:]...)
				return
			}
		}
	}
}

func index(list *yaml.Node, key string) (int, error) {
	n, err := strconv.Atoi(key)
	if err != nil || n < 0 || n >= len(list.Content) {
		return 0, fmt.Errorf("%s is no index of a list of %d; items go by [0] to [%d]", key, len(list.Content), len(list.Content)-1)
	}
	return n, nil
}

// ProfileDocument is a profile as its sandbox.yaml says it, in JSON with the
// keys in the file's order - which matters for agents.
func (m *Manager) ProfileDocument(name string) (json.RawMessage, error) {
	p, err := m.profile(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(p.Dir, "sandbox.yaml"))
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("profile %s: %w", name, err)
	}
	if len(doc.Content) == 0 {
		return json.RawMessage("{}"), nil
	}
	return nodeJSON(doc.Content[0])
}

func nodeJSON(n *yaml.Node) (json.RawMessage, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return nodeJSON(n.Alias)
	case yaml.MappingNode:
		var b bytes.Buffer
		b.WriteByte('{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				b.WriteByte(',')
			}
			key, _ := json.Marshal(n.Content[i].Value)
			b.Write(key)
			b.WriteByte(':')
			value, err := nodeJSON(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			b.Write(value)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	case yaml.SequenceNode:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				b.WriteByte(',')
			}
			value, err := nodeJSON(c)
			if err != nil {
				return nil, err
			}
			b.Write(value)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
