package unibox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TeamFile is the file of unibox's own beside an sbxenv file, in which a
// team names the ports of the env: dev.sbxenv.yaml has dev.unibox.yaml.
// sbx does not read it, so it limits nobody who uses sbx alone.
func TeamFile(sbxenv string) string {
	dir, base := filepath.Split(sbxenv)
	for _, ext := range []string{".sbxenv.yaml", ".sbxenv.yml", ".yaml", ".yml"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	return filepath.Join(dir, base+".unibox.yaml")
}

// TeamPort is a port a team names: Spec as a person's port says it, a
// description of what listens there, and whether a sandbox starts with
// it published, off unless the team says on.
type TeamPort struct {
	Spec        string
	Description string
	Default     bool
}

func (p *TeamPort) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a team's port is {port: 5173, description: ..., default: off}", n.Line)
	}
	*p = TeamPort{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: %s is one value", v.Line, k.Value)
		}
		switch k.Value {
		case "port":
			p.Spec = v.Value
		case "description":
			p.Description = v.Value
		case "default":
			on, ok := onOff(v.Value)
			if !ok {
				return fmt.Errorf("line %d: a port's default is on or off", v.Line)
			}
			p.Default = on
		default:
			return fmt.Errorf("line %d: a team's port says port, description and default, not %s", k.Line, k.Value)
		}
	}
	if p.Spec == "" {
		return fmt.Errorf("line %d: the port says no port, such as port: 5173", n.Line)
	}
	return nil
}

// ReadTeamPorts reads the team files beside an env's sbxenv files, a later
// file's port over an earlier one's as sbx merges the files themselves.
// A file that is not there names no ports.
func ReadTeamPorts(sbxenvFiles []string) (map[string]TeamPort, error) {
	out := map[string]TeamPort{}
	for _, f := range sbxenvFiles {
		name := TeamFile(f)
		b, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var doc struct {
			Ports map[string]TeamPort `yaml:"ports"`
		}
		d := yaml.NewDecoder(bytes.NewReader(b))
		d.KnownFields(true)
		if err := d.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for k, v := range doc.Ports {
			if !portName.MatchString(k) {
				return nil, fmt.Errorf("%s: port name %q must be lower case letters, digits and dashes, starting with a letter", name, k)
			}
			if _, err := ParsePortSpec(v.Spec); err != nil {
				return nil, fmt.Errorf("%s: port %s: %w", name, k, err)
			}
			out[k] = v
		}
	}
	return out, nil
}

// envPorts are an env's ports as a sandbox gets them: the team's, each
// as the person changed it, and the person's own. A person's port with
// no spec only switches the team's port of its name on or off.
func envPorts(ec EnvConfig) (map[string]Port, error) {
	team, err := ReadTeamPorts(ec.Files)
	if err != nil {
		return nil, err
	}
	out := map[string]Port{}
	for k, v := range team {
		out[k] = Port{Spec: v.Spec, Default: v.Default, Description: v.Description, Team: true}
	}
	for k, v := range ec.Ports {
		p, isTeam := out[k]
		if v.Spec == "" && !isTeam {
			return nil, fmt.Errorf("port %s says only whether it is on, and the env's files name no such port; unibox port unset %s takes it away", k, k)
		}
		if v.Spec != "" {
			p.Spec = v.Spec
		}
		p.Default = v.Default
		out[k] = p
	}
	if err := validatePorts(out); err != nil {
		return nil, err
	}
	return out, nil
}
