package unibox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fakeMSB is a stand-in msb that records its arguments, its environment and
// the configuration it is given, and answers list, image ls and inspect.
func fakeMSB(t *testing.T) (r *MSB, argv, env, conf string) {
	t.Helper()
	dir := t.TempDir()
	argv, env, conf = filepath.Join(dir, "argv"), filepath.Join(dir, "env"), filepath.Join(dir, "conf")
	script := `#!/bin/sh
printf '%s\n' "$@" >> ` + Quote(argv) + `
env > ` + Quote(env) + `
while [ $# -gt 0 ]; do
  case "$1" in
    --conf) cp "$2" ` + Quote(conf) + `; shift ;;
    list) printf '%s\n' '[{"name":"unibox-dev","status":"Stopped","image":"unibox/dev:abc"}]' ;;
    ls) printf '%s\n' '[{"reference":"unibox/dev:abc","digest":"sha256:1","created_at":"2026-10-17T08:27:20+00:00"},{"reference":"python:3","digest":"sha256:2"}]' ;;
    inspect) printf '%s\n' '{"config":{"network":{"ports":[{"guest_port":3000,"host_bind":"127.0.0.1","host_port":3001,"protocol":"Tcp"}]}}}' ;;
  esac
  shift
done
`
	mustWriteExecutable(t, filepath.Join(dir, "msb"), []byte(script))
	return &MSB{Cache: filepath.Join(dir, "cache"), Binary: filepath.Join(dir, "msb"), DNS: fakeDNS()}, argv, env, conf
}

// fakeDNS is a host whose VPN resolves git.corp.example, privately, through
// a resolver of its own, 10.1.1.53, which answers public names too; the
// global resolver, 100.100.100.100, does not know the name.
func fakeDNS() *hostDNS {
	answers := map[string]map[string][]net.IP{
		"10.1.1.53":       {"git.corp.example": {net.ParseIP("10.0.0.7")}, msbDNSProbe: {net.ParseIP("140.82.121.3")}},
		"10.9.9.9":        {"git.corp.example": {net.ParseIP("10.0.0.7")}},
		"100.100.100.100": {msbDNSProbe: {net.ParseIP("140.82.121.3")}},
	}
	return &hostDNS{
		lookup: func(ctx context.Context, name string) ([]net.IP, error) {
			switch name {
			case "git.corp.example":
				return []net.IP{net.ParseIP("10.0.0.7")}, nil
			case "gone.corp.example":
				return nil, fmt.Errorf("no such host")
			}
			return []net.IP{net.ParseIP("203.0.113.5")}, nil
		},
		query: func(ctx context.Context, server, name string) ([]net.IP, error) {
			if ips, ok := answers[server][name]; ok {
				return ips, nil
			}
			return nil, fmt.Errorf("no answer")
		},
		resolvers: func(ctx context.Context) ([]string, []string, error) {
			return []string{"100.100.100.100"}, []string{"100.100.100.100", "10.9.9.9", "10.1.1.53"}, nil
		},
	}
}

func TestMSBResolvesAVPNsPrivateHostsThroughTheVPNsResolver(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("msb does the split itself where resolv.conf has a stub resolver")
	}
	servers, allow, notes := fakeDNS().msbDNSFor(context.Background(), []string{"git.corp.example", "github.com", "*.corp.example", "gone.corp.example", "10.0.0.0/8"})
	// 10.9.9.9 knows the name but not public ones, so msb, which falls
	// through only on no answer, could not use it.
	if !slices.Equal(servers, []string{"10.1.1.53", "100.100.100.100"}) || !slices.Equal(allow, []string{"10.0.0.7"}) {
		t.Fatalf("servers %v allow %v", servers, allow)
	}
	if len(notes) != 2 || !strings.Contains(notes[0], "through 10.1.1.53") || !strings.Contains(notes[1], "gone.corp.example does not resolve") {
		t.Fatalf("notes %q", notes)
	}
	global, all := scutilResolvers("DNS configuration\n\nresolver #1\n  nameserver[0] : 100.100.100.100\n  nameserver[1] : fd7a:115c:a1e0::53\n\nresolver #2\n  domain : local\n\nresolver #3\n  nameserver[0] : 10.132.64.20\n\nDNS configuration (for scoped queries)\n\nresolver #1\n  nameserver[0] : 100.100.100.100\n")
	if !slices.Equal(global, []string{"100.100.100.100", "fd7a:115c:a1e0::53"}) || !slices.Equal(all, []string{"100.100.100.100", "fd7a:115c:a1e0::53", "10.132.64.20"}) {
		t.Fatalf("global %v all %v", global, all)
	}
}

func TestMSBCreatesFromAConfigurationAndHandsSecretsOnlyToMSB(t *testing.T) {
	t.Parallel()
	r, argv, env, conf := fakeMSB(t)
	cache := t.TempDir()
	spec := Spec{Name: "unibox-dev", Image: "unibox/dev:abc", Agents: []string{"claude", "copilot"}, Resources: Resources{CPUs: "2", Memory: "4g"},
		Access: HostAccess{
			Caches:  []Cache{{Host: cache, Env: "NPM_CACHE"}},
			Network: Network{AllowNetwork: []string{"repo.example.com:8443", "*.corp.example"}, DenyNetwork: []string{"gist.github.com"}, Ports: map[string]Port{"fe": {Spec: "3000:3000", Default: true}, "api": {Spec: "8080", Default: true}, "dbg": {Spec: "9229", Default: false}}},
			Auth:    map[string]AgentAuth{"claude": {Method: AuthToken, Command: "printf %s-%s claude value"}, "copilot": {Method: AuthToken, Command: "printf %s-%s copilot value"}},
			Services: map[string]Service{
				"gitlab": {Git: true, Host: HostList{"gitlab.example.com"}, User: "oauth2", Command: "printf %s-%s gitlab value"},
				"jfrog":  {Host: HostList{"jfrog.example.com"}, Command: "printf %s-%s jfrog value"},
				"corp":   {Git: true, Host: HostList{"git.corp.example"}, User: "oauth2", Command: "printf %s-%s corp value"},
			},
		}}
	if err := r.Create(context.Background(), spec, io.Discard); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argv)
	environment, _ := os.ReadFile(env)
	stored, _ := os.ReadFile(filepath.Join(r.Cache, "unibox-dev", "secrets.json"))
	written, _ := os.ReadFile(conf)
	// Each value reaches msb's environment, under the variable the
	// configuration names, and nothing unibox writes or passes as an
	// argument.
	for _, v := range []string{"claude-value", "copilot-value", "jfrog-value"} {
		if strings.Contains(string(args)+string(stored)+string(written), v) {
			t.Errorf("value %s outside msb's environment", v)
		}
	}
	for _, want := range []string{"UNIBOX_MSB_CLAUDE_CODE_OAUTH_TOKEN=claude-value", "UNIBOX_MSB_COPILOT_GITHUB_TOKEN=copilot-value", "UNIBOX_MSB_UNIBOX_SVC_JFROG=jfrog-value", "UNIBOX_MSB_UNIBOX_SVC_GITLAB=Basic "} {
		if !strings.Contains(string(environment), want) {
			t.Errorf("msb's environment lacks %s", want)
		}
	}
	if !strings.HasPrefix(string(args), "create\n--name\nunibox-dev\n--conf\n") {
		t.Fatalf("argv %s", args)
	}
	var c struct {
		Image      map[string]string
		PullPolicy string `yaml:"pull_policy"`
		CPUs       int
		Memory     string
		Env        map[string]string
		Mounts     []map[string]any
		Network    struct {
			Allow, Deny, Ports []string
			TLS                map[string]bool
			DNS                map[string][]string
		}
		Secrets map[string]struct {
			Value        map[string]string
			Placeholder  string
			Allow        []string
			Substitution map[string]any
		}
	}
	if err := yaml.Unmarshal(written, &c); err != nil {
		t.Fatal(err)
	}
	if c.Image["oci"] != "unibox/dev:abc" || c.PullPolicy != "never" || c.CPUs != 2 || c.Memory != "4096M" || c.Env["NPM_CACHE"] != cache || len(c.Mounts) != 1 || c.Mounts[0]["target"] != cache || c.Mounts[0]["readonly"] != true {
		t.Fatalf("configuration %s", written)
	}
	for _, want := range []string{"dns", "api.anthropic.com:tcp:443", "*.githubcopilot.com:tcp:443", "repo.example.com:tcp:8443", "*.corp.example", "gitlab.example.com", "jfrog.example.com", "*.digicert.com:tcp:80"} {
		if !slices.Contains(c.Network.Allow, want) {
			t.Errorf("allow lacks %s", want)
		}
	}
	if runtime.GOOS == "darwin" && (!slices.Equal(c.Network.DNS["nameservers"], []string{"10.1.1.53", "100.100.100.100"}) || !slices.Contains(c.Network.Allow, "10.0.0.7")) {
		t.Errorf("the VPN's host does not resolve: %+v", c.Network)
	}
	if !slices.Equal(c.Network.Deny, []string{"gist.github.com"}) || !c.Network.TLS["enabled"] {
		t.Errorf("network %+v", c.Network)
	}
	// A port without a host port gets a free one; one off by default is
	// not published.
	if len(c.Network.Ports) != 2 || !slices.Contains(c.Network.Ports, "127.0.0.1:3000:3000/tcp") || strings.Contains(strings.Join(c.Network.Ports, ","), ":9229/") {
		t.Errorf("ports %v", c.Network.Ports)
	}
	claude := c.Secrets["CLAUDE_CODE_OAUTH_TOKEN"]
	if claude.Value["$msb_env"] != "UNIBOX_MSB_CLAUDE_CODE_OAUTH_TOKEN" || claude.Placeholder != sbxTokens["claude"].Placeholder || !slices.Equal(claude.Allow, []string{"api.anthropic.com"}) {
		t.Errorf("claude secret %+v", claude)
	}
	if git := c.Secrets["UNIBOX_SVC_GITLAB"]; git.Placeholder != ServicePlaceholder("gitlab", spec.Access.Services["gitlab"]) || git.Substitution["header_fields"] == nil {
		t.Errorf("git service secret %+v", git)
	}
	if jfrog := c.Secrets["UNIBOX_SVC_JFROG"]; jfrog.Substitution["header_fields"] != nil {
		t.Errorf("an env service's token is held to headers unibox does not know: %+v", jfrog)
	}
}

func TestMSBStartsWithSecretsAndRefusesAnEmptyOne(t *testing.T) {
	t.Parallel()
	r, argv, env, _ := fakeMSB(t)
	dir := filepath.Join(r.Cache, "unibox-dev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	list, _ := json.Marshal([]msbSecret{{Env: "TOKEN", Source: "UNIBOX_MSB_TOKEN", Command: "printf fresh-value"}})
	write(t, filepath.Join(dir, "secrets.json"), string(list))
	if err := r.Start(context.Background(), "unibox-dev", io.Discard); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argv)
	environment, _ := os.ReadFile(env)
	if string(args) != "start\nunibox-dev\n" || !strings.Contains(string(environment), "UNIBOX_MSB_TOKEN=fresh-value") {
		t.Fatalf("argv %q", args)
	}
	list, _ = json.Marshal([]msbSecret{{Env: "TOKEN", Source: "UNIBOX_MSB_TOKEN", Command: "true"}})
	write(t, filepath.Join(dir, "secrets.json"), string(list))
	if err := r.Start(context.Background(), "unibox-dev", io.Discard); err == nil || !strings.Contains(err.Error(), "TOKEN printed nothing") {
		t.Fatalf("an empty token was accepted: %v", err)
	}
}

func TestMSBRunsCommandsAndReadsItsInventory(t *testing.T) {
	t.Parallel()
	r, argv, _, _ := fakeMSB(t)
	ctx := context.Background()
	if err := r.Exec(ctx, "unibox-dev", Command{Args: []string{"sh", "-c", "true"}, User: "root", Directory: "/work"}).Run(); err != nil {
		t.Fatal(err)
	}
	if err := r.Exec(ctx, "unibox-dev", Command{Args: []string{"bash"}, Interactive: true}).Run(); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argv)
	want := "exec\n--stream\n--no-stdin\n--workdir\n/work\n--user\nroot\nunibox-dev\n--\nsh\n-c\ntrue\nexec\n--tty\nunibox-dev\n--\nbash\n"
	if string(args) != want {
		t.Fatalf("argv %q", args)
	}
	boxes, err := r.List(ctx)
	if err != nil || len(boxes) != 1 || boxes[0].Name != "unibox-dev" || boxes[0].State != "stopped" {
		t.Fatalf("%+v %v", boxes, err)
	}
	images, err := r.Images(ctx)
	if err != nil || len(images) != 2 || images[0].Reference != "unibox/dev:abc" {
		t.Fatalf("%+v %v", images, err)
	}
	ports, err := r.PublishedPorts(ctx, "unibox-dev")
	if err != nil || len(ports) != 1 || ports[0].HostPort != 3001 || ports[0].SandboxPort != 3000 || ports[0].Protocol != "tcp4" {
		t.Fatalf("%+v %v", ports, err)
	}
}

func TestMSBRules(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"github.com":            "github.com",
		"**.github.com:443":     "*.github.com:tcp:443",
		"*.corp.example":        "*.corp.example",
		"crl*.digicert.com:80":  "*.digicert.com:tcp:80",
		"10.0.0.0/8":            "10.0.0.0/8",
		"repo.example.com:8443": "repo.example.com:tcp:8443",
	} {
		if got := msbRule(in); got != want {
			t.Errorf("msbRule(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMSBAuthKeepsNoOAuth(t *testing.T) {
	t.Parallel()
	r := &MSB{}
	p := Profile{Name: "dev", Agents: []string{"claude", "codex", "copilot"}}
	p.Auth = map[string]AgentAuth{"claude": {Method: AuthOAuth}, "codex": {Method: AuthOAuth}, "copilot": {Method: AuthToken, Command: "printf x"}}
	needs := r.AuthNeeds(context.Background(), "unibox-dev", p)
	if len(needs) != 2 || needs[0].Code != "auth_unsupported" || needs[1].Agent != "codex" || !strings.Contains(needs[1].Message, "API key") {
		t.Fatalf("%+v", needs)
	}
	if _, _, err := msbAgentSecret("codex", AgentAuth{Method: AuthOAuth}); err == nil {
		t.Fatal("codex was signed in by OAuth")
	}
	sec, ok, err := msbAgentSecret("codex", AgentAuth{Method: AuthCommand, Command: "printf key"})
	if err != nil || !ok || sec.Env != "OPENAI_API_KEY" || sec.Placeholder != "proxy-managed" {
		t.Fatalf("%+v %v %v", sec, ok, err)
	}
	p.Auth["codex"] = AgentAuth{Method: AuthCommand, Command: "printf key"}
	if env := r.SessionEnv(p, "codex"); !slices.Equal(env, []string{"SBX_CRED_OPENAI_MODE=apikey"}) {
		t.Fatalf("codex session %v", env)
	}
}

func TestMSBIsUsedOnlyWhenAskedFor(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := &Manager{Runtimes: map[string]Runtime{"sbx": &SBX{}, "msb": &MSB{Binary: "/bin/true"}}}
	if got := m.defaultRuntime(); got != "" {
		t.Fatalf("without sbx the experimental runtime was chosen: %q", got)
	}
	if _, err := m.Runtime(""); err == nil || !strings.Contains(err.Error(), "default_runtime msb") {
		t.Fatalf("%v", err)
	}
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\n"))
	if got := m.defaultRuntime(); got != "sbx" {
		t.Fatalf("with sbx the runtime is %q", got)
	}
	m.Config.DefaultRuntime = "msb"
	if r, err := m.Runtime(""); err != nil || r.Name() != "msb" {
		t.Fatalf("the configured runtime was not used: %v", err)
	}
}
