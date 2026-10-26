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
	if len(team) != 2 || team["web"] != (TeamPort{Spec: "8000:3000", Description: "The dev server", From: FromEnv}) || team["db"] != (TeamPort{Spec: "5432", From: FromEnv}) {
		t.Fatalf("%+v", team)
	}
	ports, err := envPorts(EnvConfig{Files: []string{base, dev}, Ports: map[string]Port{"db": {Default: true}, "web": {Spec: "8001:3000"}, "mine": {Spec: "9000"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ports["db"] != (Port{Spec: "5432", Default: true, Team: true, From: FromEnv}) || ports["web"] != (Port{Spec: "8001:3000", Description: "The dev server", Team: true, From: FromEnv}) || ports["mine"] != (Port{Spec: "9000", From: FromYours}) {
		t.Fatalf("%+v", ports)
	}
	if _, err := envPorts(EnvConfig{Files: []string{alone}, Ports: map[string]Port{"db": {Default: true}}}, nil); err == nil || !strings.Contains(err.Error(), "names no such port") {
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

func TestPortsAreLayeredFromTheRepositoryToThePersonsEnv(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) string {
		t.Helper()
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	dev := write("sandboxes/dev.sbxenv.yaml", "agent: shell\n")
	ci := write("sandboxes/ci.sbxenv.yaml", "agent: shell\n")
	if got := RepoFile(dev); got != filepath.Join(repo, "unibox.yaml") {
		t.Fatalf("%s", got)
	}
	write("unibox.yaml", "ports:\n  spring: {port: 8080, description: Spring}\n  vite: {port: 5173}\n  devtools: {port: 9222}\n")
	write("sandboxes/dev.unibox.yaml", "ports:\n  vite: {port: '5173:5173', default: on}\n")

	team, err := ReadTeamPorts([]string{dev})
	if err != nil {
		t.Fatal(err)
	}
	// The env's file wins and keeps the repository's description and origin.
	if team["spring"] != (TeamPort{Spec: "8080", Description: "Spring", From: FromRepo}) || team["vite"] != (TeamPort{Spec: "5173:5173", Default: true, From: FromRepo}) {
		t.Fatalf("%+v", team)
	}
	global := map[string]Port{"spring": {Default: true}, "devtools": {Spec: "19222:9222", Default: true}, "absent": {Default: true}, "extra": {Spec: "7000", Default: true}}
	ports, err := envPorts(EnvConfig{Files: []string{dev}, Ports: map[string]Port{"devtools": {Default: false}}}, global)
	if err != nil {
		t.Fatal(err)
	}
	if !ports["spring"].Default || ports["devtools"] != (Port{Spec: "19222:9222", Team: true, From: FromRepo}) || ports["extra"] != (Port{Spec: "7000", Default: true, From: FromYoursAll}) {
		t.Fatalf("%+v", ports)
	}
	if _, ok := ports["absent"]; ok {
		t.Fatal("a switch for every env named a port the env does not have")
	}
	// Another env of the repository has the repository's ports, off.
	ports, err = envPorts(EnvConfig{Files: []string{ci}}, nil)
	if err != nil || len(ports) != 3 || ports["vite"].Default || ports["vite"].Spec != "5173" {
		t.Fatalf("%v %+v", err, ports)
	}
}
