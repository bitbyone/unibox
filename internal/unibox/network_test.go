package unibox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// liveNetwork is the fake runtime with sbx's ability to change a running
// sandbox's network rules.
type liveNetwork struct {
	*fakeRuntime
	mu      sync.Mutex
	changes []NetworkChange
}

func (l *liveNetwork) UpdateNetwork(_ context.Context, _ string, c NetworkChange, _ io.Writer) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changes = append(l.changes, c)
	return nil
}

func TestANetworkChangeIsAppliedLiveWhereTheRuntimeCan(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	live := &liveNetwork{fakeRuntime: f}
	m.Runtimes = map[string]Runtime{"fake": live}
	ctx := context.Background()
	file := filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude, codex]\nnetwork: {allow: [jitpack.io]}\n")
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	creates := f.creates
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude, codex]\nnetwork: {allow: [maven.google.com], deny: [gist.github.com]}\n")
	needRecreate, err := m.ApplyNetwork(ctx, "node")
	if err != nil || len(needRecreate) != 0 {
		t.Fatalf("%v %v", needRecreate, err)
	}
	if len(live.changes) != 1 {
		t.Fatalf("%+v", live.changes)
	}
	c := live.changes[0]
	if !slices.Equal(c.AddAllow, []string{"maven.google.com"}) || !slices.Equal(c.RemoveAllow, []string{"jitpack.io"}) || !slices.Equal(c.AddDeny, []string{"gist.github.com"}) {
		t.Fatalf("%+v", c)
	}
	// The next start finds the sandbox as it is: no recreation for rules.
	b, _ := m.sandbox("dev")
	r, _ := m.Runtime("fake")
	p, _ := m.profile("node")
	if err := m.ensure(ctx, &b, r, p, false, false); err != nil {
		t.Fatal(err)
	}
	if f.creates != creates || len(live.changes) != 1 {
		t.Fatalf("a change of the rules recreated the sandbox (%d creates) or was applied twice (%d)", f.creates-creates, len(live.changes))
	}
}

func TestANetworkChangeRecreatesWhereTheRuntimeCannot(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	file := filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml")
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	creates := f.creates
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude, codex]\nresources: {cpus: 2, memory: 1g}\nnetwork: {allow: [jitpack.io]}\n")
	needRecreate, err := m.ApplyNetwork(ctx, "node")
	if err != nil || !slices.Equal(needRecreate, []string{"dev"}) {
		t.Fatalf("%v %v", needRecreate, err)
	}
	b, _ := m.sandbox("dev")
	r, _ := m.Runtime("fake")
	p, _ := m.profile("node")
	if err := m.ensure(ctx, &b, r, p, false, false); err != nil {
		t.Fatal(err)
	}
	if f.creates != creates+1 {
		t.Fatal("rules the runtime cannot change live did not recreate the sandbox")
	}
	if log := m.Log.(interface{ String() string }).String(); !strings.Contains(log, "recreating sandbox dev: allow_network\n") {
		t.Fatalf("the recreation did not say why:\n%q", log)
	}
}

func TestSBXChangesOnlyTheRulesItIsToldOf(t *testing.T) {
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte(`#!/bin/sh
echo "$*" >> `+Quote(calls)+`
if [ "$1 $2" = "policy ls" ]; then
  echo '{"rules":[
    {"id":"r1","sandbox_id":"unibox-dev","resource_type":"network","decision":"allow","resources":["jitpack.io"]},
    {"id":"r2","sandbox_id":"unibox-dev","resource_type":"network","decision":"allow","resources":["mine.example"]},
    {"id":"r3","sandbox_id":"unibox-other","resource_type":"network","decision":"allow","resources":["jitpack.io"]},
    {"id":"r4","sandbox_id":"unibox-dev","resource_type":"network","decision":"deny","resources":["gist.github.com"]}]}'
fi
`))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{}
	err := r.UpdateNetwork(context.Background(), "unibox-dev", NetworkChange{RemoveAllow: []string{"jitpack.io"}, RemoveDeny: []string{"gist.github.com"}, AddDeny: []string{"jitpack.io"}, AddAllow: []string{"maven.google.com"}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(calls)
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"policy ls --json",
		"policy rm network --id r1 --sandbox unibox-dev --force",
		"policy rm network --id r4 --sandbox unibox-dev --force",
		"policy deny network --sandbox unibox-dev jitpack.io",
		"policy allow network --sandbox unibox-dev maven.google.com",
	}
	// The removals may come in either order; both before the additions.
	slices.Sort(got[1:3])
	slices.Sort(want[1:3])
	if !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}
