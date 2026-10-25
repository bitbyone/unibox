package unibox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheTeamsPortsAreReadBesideTheSbxenvFilesAndThePersonsWin(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"/r/dev.sbxenv.yaml": "/r/dev.unibox.yaml",
		"/r/dev.sbxenv.yml":  "/r/dev.unibox.yaml",
		"/r/base.yaml":       "/r/base.unibox.yaml",
	} {
		if got := TeamFile(in); got != filepath.FromSlash(want) {
			t.Errorf("%s: %s", in, got)
		}
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	base := write("base.sbxenv.yaml", "agent: shell\n")
	dev := write("dev.sbxenv.yaml", "agent: shell\n")
	alone := write("alone.sbxenv.yaml", "agent: shell\n")
	write("base.unibox.yaml", "ports:\n  web: {port: 3000, description: The web app, default: on}\n  db: {port: 5432}\n")
	write("dev.unibox.yaml", "ports:\n  web: {port: '8000:3000', description: The dev server}\n")
	team, err := ReadTeamPorts([]string{base, dev, alone})
	if err != nil {
		t.Fatal(err)
	}
	if len(team) != 2 || team["web"] != (TeamPort{Spec: "8000:3000", Description: "The dev server"}) || team["db"] != (TeamPort{Spec: "5432"}) {
		t.Fatalf("%+v", team)
	}
	ports, err := envPorts(EnvConfig{Files: []string{base, dev}, Ports: map[string]Port{"db": {Default: true}, "web": {Spec: "8001:3000"}, "mine": {Spec: "9000"}}})
	if err != nil {
		t.Fatal(err)
	}
	if ports["db"] != (Port{Spec: "5432", Default: true, Team: true}) || ports["web"] != (Port{Spec: "8001:3000", Description: "The dev server", Team: true}) || ports["mine"] != (Port{Spec: "9000"}) {
		t.Fatalf("%+v", ports)
	}
	if _, err := envPorts(EnvConfig{Files: []string{alone}, Ports: map[string]Port{"db": {Default: true}}}); err == nil || !strings.Contains(err.Error(), "name no such port") {
		t.Fatalf("%v", err)
	}
	for text, want := range map[string]string{
		"ports:\n  web: {port: nope}\n":                 "port web",
		"ports:\n  Web: {port: 3000}\n":                 "port name",
		"ports:\n  web: {description: x}\n":             "says no port",
		"ports:\n  web: {port: 3000, url: x}\n":         "not url",
		"ports:\n  web: {port: 3000, default: maybe}\n": "on or off",
		"ports:\n  web: 3000\n":                         "a team's port is",
		"port:\n  web: {port: 3000}\n":                  "port not found",
	} {
		write("bad.unibox.yaml", text)
		_, err := ReadTeamPorts([]string{filepath.Join(dir, "bad.sbxenv.yaml")})
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "bad.unibox.yaml") {
			t.Errorf("%q: %v", text, err)
		}
	}
	// What a person keeps for the team's port is no more than they changed.
	b, err := Port{Default: false}.MarshalYAML()
	if err != nil || b.(map[string]string)["default"] != "off" || len(b.(map[string]string)) != 1 {
		t.Fatalf("%v %v", b, err)
	}
}
