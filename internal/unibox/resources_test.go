package unibox

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAResourceIsASizeACountOrAShareOfTheHost(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, value string
		ok          bool
	}{
		{"cpus", "4", true}, {"cpus", "0", true}, {"cpus", "50%", true}, {"cpus", "100%", true},
		{"cpus", "101%", false}, {"cpus", "0%", false}, {"cpus", "2g", false}, {"cpus", "", false},
		{"memory", "16g", true}, {"memory", "2048m", true}, {"memory", "75%", true}, {"memory", "100%", true},
		{"memory", "0", false}, {"memory", "16gb", false}, {"memory", "120%", false},
		{"disk", "10g", false},
	} {
		if err := ValidResource(c.name, c.value); (err == nil) != c.ok {
			t.Errorf("%s=%q: %v", c.name, c.value, err)
		}
	}
}

func TestSharesResolveOnTheHostAndMemoryStaysWithinWhatSbxGives(t *testing.T) {
	t.Parallel()
	host := Host{CPUs: 10, Memory: 64 << 30}
	for _, c := range []struct {
		in, out Resources
		capped  bool
	}{
		{Resources{CPUs: "100%", Memory: "75%"}, Resources{CPUs: "0", Memory: "49152m"}, false},
		{Resources{CPUs: "25%", Memory: "50%"}, Resources{CPUs: "3", Memory: "32768m"}, false},
		{Resources{CPUs: "1%"}, Resources{CPUs: "1"}, false},
		{Resources{CPUs: "6", Memory: "16g"}, Resources{CPUs: "6", Memory: "16g"}, false},
		{Resources{Memory: "100%"}, Resources{Memory: "49152m"}, true},
		{Resources{Memory: "60g"}, Resources{Memory: "49152m"}, true},
	} {
		var log bytes.Buffer
		got, err := c.in.Resolve(host, &log)
		if err != nil || got != c.out || (log.Len() > 0) != c.capped {
			t.Errorf("%+v: %v %+v %q", c.in, err, got, log.String())
		}
	}
	if _, err := (Resources{Memory: "50%"}).Resolve(Host{CPUs: 4}, io.Discard); err == nil {
		t.Fatal("a share resolved without the host's memory")
	}
	if got, err := (Resources{Memory: "8g"}).Resolve(Host{CPUs: 4}, io.Discard); err != nil || got.Memory != "8g" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestArgsAndResourcesForEveryEnvAreKeptAndAnEnvsOwnWin(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	v := EnvValues{
		Args:         map[string]string{"user": "everyone", "unused": "x"},
		Envs:         map[string]map[string]string{"dev": {"token": "mine"}},
		Resources:    Resources{CPUs: "100%", Memory: "75%"},
		EnvResources: map[string]Resources{"dev": {Memory: "8g"}, "ci": {CPUs: "2"}},
	}
	if err := m.Paths.SaveEnvValues(v); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.Paths.VarsFile())
	if strings.Contains(string(data), "[envs]\n") || strings.Contains(string(data), "[envs.dev]\n") {
		t.Fatalf("tables holding only others were written:\n%s", data)
	}
	got, err := m.Paths.LoadEnvValues()
	if err != nil {
		t.Fatal(err)
	}
	if got.Args["user"] != "everyone" || got.Envs["dev"]["token"] != "mine" || got.Resources != v.Resources || got.EnvResources["dev"] != v.EnvResources["dev"] || got.EnvResources["ci"] != v.EnvResources["ci"] {
		t.Fatalf("%+v", got)
	}
	if r := got.ResourcesFor("dev", ""); r != (Resources{CPUs: "100%", Memory: "8g"}) {
		t.Fatalf("%+v", r)
	}
	if r := got.ResourcesFor("other", ""); r != v.Resources {
		t.Fatalf("%+v", r)
	}
	env := Env{Name: "dev"}
	env.SBX.Args = map[string]EnvArg{"user": {}, "token": {}}
	got.Envs["dev"]["user"] = "me"
	args := got.ArgsFor(env)
	if len(args) != 2 || args["user"] != "me" || args["token"] != "mine" {
		t.Fatalf("%v", args)
	}

	write(t, m.Paths.VarsFile(), "[resources]\nmemory = \"lots\"\n")
	if _, err := m.Paths.LoadEnvValues(); err == nil || !strings.Contains(err.Error(), "resources") {
		t.Fatalf("%v", err)
	}
	write(t, m.Paths.VarsFile(), "[envs.dev.resources]\ncpus = 4\n")
	if _, err := m.Paths.LoadEnvValues(); err == nil || !strings.Contains(err.Error(), "in quotes") {
		t.Fatalf("%v", err)
	}
}

func TestResourcesReachSbxInAFileGivenLastAndAChangeIsTold(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{Env: "dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	// None set: no file, and the fingerprint is the one a sandbox made
	// before resources had.
	if f.overlay != "" || len(f.called("env create dev ")[0]) == 0 || strings.Contains(f.called("env create dev ")[0], "resources-") {
		t.Fatalf("%q %v", f.overlay, f.calls)
	}
	if want, _ := mustEnv(t, m).SBX.Fingerprint(map[string]string{}, Resources{}); b.Fingerprint != want {
		t.Fatal("the fingerprint changed with no resources set")
	}

	if err := m.Paths.SaveEnvValues(EnvValues{EnvResources: map[string]Resources{"dev": {CPUs: "2", Memory: "4g"}}}); err != nil {
		t.Fatal(err)
	}
	check, err := m.Check(ctx, Request{Env: "dev"})
	if err != nil || len(check.Needs) != 1 || check.Needs[0].Code != "env_changed" {
		t.Fatalf("%v %+v", err, check)
	}
	if err := m.Recreate(ctx, "dev"); err != nil {
		t.Fatal(err)
	}
	calls := f.called("env create dev ")
	last := strings.Fields(calls[len(calls)-1])
	if !strings.Contains(filepath.Base(last[len(last)-1]), "resources-") {
		t.Fatalf("the resources file is not last: %v", last)
	}
	if !strings.Contains(f.overlay, "sandboxOptions:\n  cpus: 2\n  memory: \"4g\"\n") {
		t.Fatalf("%q", f.overlay)
	}
	if _, err := os.Stat(last[len(last)-1]); !os.IsNotExist(err) {
		t.Fatal("the resources file stayed")
	}
	if check, _ = m.Check(ctx, Request{Env: "dev"}); len(check.Needs) != 0 {
		t.Fatalf("%+v", check.Needs)
	}
}

func TestASandboxsOwnResourcesWinOverItsEnvsAndGoWithIt(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	if err := m.Paths.SaveEnvValues(EnvValues{EnvResources: map[string]Resources{"dev": {CPUs: "2", Memory: "4g"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	if err := m.Paths.SetSandboxResources("big", Resources{Memory: "8g"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{Env: "dev", Sandbox: "big"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.overlay, "sandboxOptions:\n  cpus: 2\n  memory: \"8g\"\n") {
		t.Fatalf("%q", f.overlay)
	}
	if check, _ := m.Check(ctx, Request{Env: "dev"}); len(check.Needs) != 0 {
		t.Fatalf("the env's sandbox took another's resources: %+v", check.Needs)
	}
	if check, _ := m.Check(ctx, Request{Env: "dev", Sandbox: "big"}); len(check.Needs) != 0 {
		t.Fatalf("%+v", check.Needs)
	}
	if _, err := m.SetHostRules(ctx, "big", "allow", "udp", []string{"stun.example.com"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Paths.LoadConfig(); len(c.Sandboxes["big"].Network) != 1 || c.Sandboxes["big"].Network[0].Proto() != "udp" {
		t.Fatalf("%+v", c.Sandboxes)
	}
	if err := m.Remove(ctx, "big", "", false); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Paths.LoadEnvValues(); len(v.SandboxResources) != 0 || v.EnvResources["dev"].CPUs != "2" {
		t.Fatalf("%+v", v)
	}
	if c, _ := m.Paths.LoadConfig(); len(c.Sandboxes) != 0 {
		t.Fatalf("the sandbox's network rules stayed: %+v", c.Sandboxes)
	}
}
