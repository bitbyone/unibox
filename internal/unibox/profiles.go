package unibox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Cache struct {
	Host string `yaml:"host" json:"host"`
	Env  string `yaml:"env" json:"env"`
}
type GitOptions struct {
	Author   string `yaml:"author" json:"author"`
	CoAuthor *bool  `yaml:"co_author" json:"co_author,omitempty"`
	// CoAuthors says how each agent is credited, as Name <email>, over the
	// default unibox's table gives it.
	CoAuthors map[string]string `yaml:"-" json:"co_authors,omitempty"`
}

// ValidCoAuthor is Name <email>, or Name <> - a trailer is text, and an
// agent credited without an address is shown, though linked to no account.
func ValidCoAuthor(s string) bool {
	if _, _, err := AuthorParts(s); err == nil {
		return true
	}
	name, ok := strings.CutSuffix(s, " <>")
	return ok && strings.TrimSpace(name) != "" && !strings.ContainsAny(name, "<>\r\n")
}

// CoAuthor is how the profile credits an agent in a commit's trailer.
func (p Profile) CoAuthor(agent string) string {
	if c := p.Git.CoAuthors[agent]; c != "" {
		return c
	}
	a, _ := FindAgent(agent)
	return a.CoAuthor
}

type HostAccess struct {
	// Env are host variables passed in by name; Profile.EnvList reads them
	// from the profile with the variables it sets itself.
	Env     []string `yaml:"-" json:"env"`
	Caches  []Cache  `yaml:"caches" json:"caches"`
	Network `yaml:"network"`
	// Auth is how each agent signs in. It is host access: the runtime keeps
	// the logins on the host, and a command runs there to fetch a token.
	Auth map[string]AgentAuth `yaml:"-" json:"auth,omitempty"`
	// Services are the host-facing services the sandbox reaches - a git
	// forge, an Artifactory, a registry. Like Auth, the token lives on the
	// host and reaches the sandbox only as a placeholder the proxy swaps.
	Services map[string]Service `yaml:"services" json:"services,omitempty"`
}

// Network is what a sandbox may reach and what of it the host reaches.
// It is embedded with no JSON name, so its rules read as the fields they
// were before network grouped them, in the same order, and what unibox
// recorded of a profile stays comparable. It must not have a MarshalJSON:
// one would be promoted to HostAccess and Profile.
type Network struct {
	// DenyNetwork narrows the runtime's egress policy for the sandbox, and
	// wins over AllowNetwork, which widens it.
	DenyNetwork  []string `yaml:"deny" json:"deny_network"`
	AllowNetwork []string `yaml:"allow" json:"allow_network,omitempty"`
	// Ports are published on the host's loopback, by name.
	Ports map[string]Port `yaml:"ports" json:"ports,omitempty"`
}

// Service is a host-facing endpoint the sandbox authenticates to by a token
// the host holds. host is the host or hosts it speaks to; command is the
// host command that prints the token; scheme is how the Authorization header
// is formed (basic needs user). git wires the sandbox's git to the hosts and
// unibox owns the whole header; env names the variable a tool reads the
// placeholder from instead. Exactly one of git or env is set. A git
// service's host may carry a path, github.com/some-org, so that owners on
// one forge use tokens of their own: a fine-grained GitHub token covers one
// owner only.
type Service struct {
	Git  bool     `yaml:"git" json:"git,omitempty"`
	Host HostList `yaml:"host" json:"host"`
	// Scheme and User are how a git service's header is made; a service
	// without git has its token named in the profile's env instead, as
	// ${services.<name>.token}, and the tool makes its own header.
	Scheme  string `yaml:"scheme" json:"scheme,omitempty"`
	User    string `yaml:"user" json:"user,omitempty"`
	Command string `yaml:"command" json:"command"`
	// Refresh is how often sbx fetches the token again: a duration shorter
	// than a short-lived token's lifetime, or on-demand (sbx's default).
	Refresh string `yaml:"refresh" json:"refresh,omitempty"`
	// Author and CoAuthor are who commits in the repositories under a git
	// service's URLs, and whether the agent's co-author trailer goes in;
	// they win over --author, the host's git configuration and the
	// profile's git.co_author.
	Author   string `yaml:"author" json:"author,omitempty"`
	CoAuthor *bool  `yaml:"co_author" json:"co_author,omitempty"`
}

const (
	SchemeBasic  = "basic"
	SchemeBearer = "bearer"
	SchemeRaw    = "raw"
)

var servicePath = regexp.MustCompile(`^[A-Za-z0-9._~-]+(/[A-Za-z0-9._~-]+)*$`)

// splitServiceHost parts a service host into the host the proxy matches and
// the path git matches under it, without its slashes; "" when none.
func splitServiceHost(h string) (string, string) {
	host, path, _ := strings.Cut(h, "/")
	return host, strings.Trim(path, "/")
}

// ProxyHosts are the hosts the runtime's proxy and network policy know a
// service by: its hosts without their paths, each once.
func (s Service) ProxyHosts() []string {
	out := []string{}
	for _, h := range s.Host {
		host, _ := splitServiceHost(h)
		if !slices.Contains(out, host) {
			out = append(out, host)
		}
	}
	return out
}

// GitIdentity is who commits in the repository at url, by the git services
// whose URLs it is under: each field from the most specific service that
// sets it, as git picks the most specific of its http.<url> settings. ""
// and nil leave the author and the co-author trailer to the rest.
func GitIdentity(services map[string]Service, url string) (string, *bool) {
	type match struct {
		length int
		s      Service
	}
	matches := []match{}
	for _, s := range services {
		if !s.Git {
			continue
		}
		for _, u := range s.GitURLs() {
			if strings.HasPrefix(url, u) {
				matches = append(matches, match{len(u), s})
			}
		}
	}
	slices.SortStableFunc(matches, func(a, b match) int { return a.length - b.length })
	author := ""
	var coAuthor *bool
	for _, m := range matches {
		if m.s.Author != "" {
			author = m.s.Author
		}
		if m.s.CoAuthor != nil {
			coAuthor = m.s.CoAuthor
		}
	}
	return author, coAuthor
}

// GitURLs are the URL prefixes a git service's header is sent to, each with
// its trailing slash so github.com/org does not also match github.com/org2.
func (s Service) GitURLs() []string {
	out := []string{}
	for _, h := range s.Host {
		host, path := splitServiceHost(h)
		u := "https://" + host + "/"
		if path != "" {
			u += path + "/"
		}
		out = append(out, u)
	}
	return out
}

// HostList is one host or a list of them: scheme and command are shared, so a
// service reaching several hosts with the same login writes them as a
// sequence rather than repeating itself.
type HostList []string

func (h *HostList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*h = HostList{n.Value}
		return nil
	case yaml.SequenceNode:
		out := make(HostList, 0, len(n.Content))
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: a service host is a name or a list of names", n.Line)
			}
			out = append(out, c.Value)
		}
		*h = out
		return nil
	}
	return fmt.Errorf("line %d: a service host is a name or a list of names", n.Line)
}

// AgentAuth is an agent's way to sign in: oauth (the provider's own login,
// kept by the runtime), api-key (a key the runtime stores), command (a
// host command the runtime runs for its key, such as gh auth token), or
// token (a host command printing a long-lived login token the agent reads
// from its environment, such as claude setup-token's). In the profile it is
// the method's name, {command: ...} or {token: ...}.
type AgentAuth struct {
	Method  string `json:"method"`
	Command string `json:"command,omitempty"`
}

const (
	AuthOAuth   = "oauth"
	AuthAPIKey  = "api-key"
	AuthCommand = "command"
	AuthToken   = "token"
)

func (a *AgentAuth) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		a.Method = n.Value
		return nil
	}
	if n.Kind != yaml.MappingNode || len(n.Content) != 2 || (n.Content[0].Value != AuthCommand && n.Content[0].Value != AuthToken) || n.Content[1].Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: an agent's auth is oauth, api-key, {command: <host command>} or {token: <host command>}", n.Line)
	}
	a.Method, a.Command = n.Content[0].Value, n.Content[1].Value
	return nil
}

// AgentSpec is what a profile says of one agent: how it signs in, how it is
// credited in commits, and the seeds of its configuration files by the name
// the agent gives each.
type AgentSpec struct {
	Auth     *AgentAuth        `yaml:"auth"`
	CoAuthor string            `yaml:"co_author"`
	Config   map[string]string `yaml:"config"`
}

// AgentList is a profile's agents in their order - which matters, sbx
// signing in by the OAuth of the first that declares one: a list of names,
// or a mapping of names to what the profile says of each.
type AgentList struct {
	IDs   []string
	Specs map[string]AgentSpec
}

func (l *AgentList) UnmarshalYAML(n *yaml.Node) error {
	l.IDs, l.Specs = []string{}, map[string]AgentSpec{}
	switch n.Kind {
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: agents is a list of names, or a mapping of names to auth, co_author and config", c.Line)
			}
			l.IDs = append(l.IDs, c.Value)
		}
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			id, body := n.Content[i].Value, n.Content[i+1]
			var spec AgentSpec
			if !(body.Kind == yaml.ScalarNode && body.Tag == "!!null") {
				if body.Kind != yaml.MappingNode {
					return fmt.Errorf("line %d: agent %s takes auth, co_author and config", body.Line, id)
				}
				// Strict, as the profile is: a misspelt field is named.
				if err := decodeStrict(body, &spec); err != nil {
					return fmt.Errorf("agent %s: %w", id, err)
				}
			}
			l.IDs = append(l.IDs, id)
			l.Specs[id] = spec
		}
		return nil
	}
	return fmt.Errorf("line %d: agents is a list of names, or a mapping of names to auth, co_author and config", n.Line)
}

// EnvList is a profile's env: a list whose items are a host variable's
// name, passed in, or NAME: value, set by the profile.
type EnvList struct {
	Names  []string
	Values map[string]string
}

func (l *EnvList) UnmarshalYAML(n *yaml.Node) error {
	l.Names, l.Values = []string{}, map[string]string{}
	bad := fmt.Errorf("line %d: env is a list of host variables by name, or NAME: value to set one", n.Line)
	if n.Kind != yaml.SequenceNode {
		return bad
	}
	for _, c := range n.Content {
		switch {
		case c.Kind == yaml.ScalarNode:
			l.Names = append(l.Names, c.Value)
		case c.Kind == yaml.MappingNode && len(c.Content) == 2 && c.Content[1].Kind == yaml.ScalarNode:
			if _, ok := l.Values[c.Content[0].Value]; ok {
				return fmt.Errorf("line %d: env sets %s twice", c.Line, c.Content[0].Value)
			}
			l.Values[c.Content[0].Value] = c.Content[1].Value
		default:
			return bad
		}
	}
	return nil
}

// ProfileDockerfile is the file a profile's image builds from.
const ProfileDockerfile = "sandbox.dockerfile"

// ProfileFormat is the form of sandbox.yaml this unibox reads. A profile
// says its own in version; one of an older form is not read until unibox
// init migrates it (migrations.go).
const ProfileFormat = 6

type Profile struct {
	Version int `yaml:"version" json:"-"`
	// Template is where the profile was copied from, <template>/<profile>:
	// its install scripts and instruction texts are in .templates/<template>,
	// between the user's _shared and unibox's .shared.
	Template string `yaml:"template" json:"-"`
	// Dockerfile is sandbox.dockerfile when the profile has it, which a
	// profile no longer names; empty when it names a built image instead.
	Dockerfile string `yaml:"-" json:"dockerfile"`
	Image      string `yaml:"image" json:"image"`
	Resources  `yaml:"resources"`
	HostAccess `yaml:",inline"`
	Agents     []string `yaml:"-" json:"agents"`
	// AgentList is how agents reads in YAML; LoadProfile spreads it into
	// Agents, Auth, Git.CoAuthors and AgentConfig.
	AgentList AgentList `yaml:"agents" json:"-"`
	Start     string    `yaml:"start" json:"start"`
	// Scripts run in a sandbox only when asked for, by name: before an
	// agent's session (--script) or at once (unibox script).
	Scripts map[string]string `yaml:"scripts" json:"scripts,omitempty"`
	// Bootstrap is always empty: format 3 ran a script the worked-on
	// repository had to carry, which none did, and format 4 has none. It
	// stays in JSON so the settings recorded of a sandbox before then
	// compare equal, and do not recreate it.
	Bootstrap    string   `yaml:"-" json:"bootstrap"`
	Instructions []string `yaml:"instructions" json:"instructions"`
	// EnvList is how env reads in YAML: host variables by name, and
	// variables the profile sets to a value that may refer to a var or a
	// service's token. LoadProfile spreads it into Env and EnvValues.
	EnvList   EnvList           `yaml:"env" json:"-"`
	EnvValues map[string]string `yaml:"-" json:"-"`
	// BuildArgSpec are build arguments for every runtime, whose values may
	// refer to a var.
	BuildArgSpec map[string]string `yaml:"build_args" json:"-"`
	// Vars are what each person sets for themselves, by a name of its own
	// with what each is for. The values live in the user's vars.yaml, never
	// in the profile, and reach a sandbox only where the profile refers to
	// them as ${vars.<name>}. VarValues are the values found; SetEnv and
	// BuildArgValues are env and build_args with every reference filled.
	Vars           map[string]string       `yaml:"vars" json:"-"`
	VarValues      map[string]string       `yaml:"-" json:"-"`
	SetEnv         map[string]string       `yaml:"-" json:"-"`
	BuildArgValues map[string]string       `yaml:"-" json:"-"`
	AgentConfig    map[string]string       `yaml:"-" json:"agent_config,omitempty"`
	Git            GitOptions              `yaml:"git" json:"git"`
	Name           string                  `yaml:"-" json:"-"`
	Dir            string                  `yaml:"-" json:"-"`
	Root           string                  `yaml:"-" json:"-"`
	Revision       string                  `yaml:"-" json:"-"`
	Files          map[string]SnapshotFile `yaml:"-" json:"-"`
}
type SnapshotFile struct {
	Data []byte `json:"data"`
	Mode uint32 `json:"mode"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// toolEnvName is a variable a tool reads, whose name is the tool's: Gradle
// reads a project property from ORG_GRADLE_PROJECT_<property>, in the
// property's own case.
var toolEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// A network rule is a host, a domain wildcard, an IP or a CIDR prefix, with
// an optional port: one pattern, never a list or anything with spaces.
var networkPattern = regexp.MustCompile(`^[A-Za-z0-9.*?\[\]!:/_-]+$`)
var sensitiveEnv = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|PASSPHRASE|CREDENTIAL|API_KEY|PRIVATE_KEY|SSH_AUTH|GIT_CONFIG|DOCKER_HOST|DOCKER_CONTEXT|^LD_|^DYLD_)`)

// validCommand is one host command, or a reference to a secret unibox keeps.
func validCommand(c string) bool {
	if name, ok := SecretRef(c); ok {
		return ValidName(name) == nil
	}
	return strings.TrimSpace(c) != "" && !strings.ContainsAny(c, "\n\r")
}

func safeRelative(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.Clean(path) != ".." && !strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator))
}
func inside(root, path string) bool {
	r, err := filepath.Rel(root, path)
	return err == nil && (r == "." || safeRelative(r))
}

func readInside(root, path string) ([]byte, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !inside(root, real) {
		return nil, fmt.Errorf("%s leaves the environments repository; move it inside", path)
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, fmt.Errorf("%s must be a regular file smaller than 16 MiB", path)
	}
	return os.ReadFile(real)
}

func LoadProfile(root, name string) (Profile, error) {
	p := Profile{Name: name}
	if err := ValidName(name); err != nil {
		return p, err
	}
	if root == "" {
		return p, fmt.Errorf("no environments repository is configured; start with unibox init --dir <path>, or unibox init -i to be asked")
	}
	abs, err := ExpandPath(root)
	if err != nil {
		return p, err
	}
	p.Root, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return p, err
	}
	if err = CheckLayout(p.Root); err != nil {
		return p, err
	}
	p.Dir = filepath.Join(p.Root, "sandboxes", name)
	b, err := readInside(p.Root, filepath.Join(p.Dir, "sandbox.yaml"))
	if err != nil {
		return p, err
	}
	if err = checkFormat(name, b); err != nil {
		return p, err
	}
	if err = DecodeYAML(b, &p); err != nil {
		return p, fmt.Errorf("profile %s: %w", name, err)
	}
	p.Env, p.EnvValues = p.EnvList.Names, p.EnvList.Values
	p.Agents = p.AgentList.IDs
	for _, id := range p.AgentList.IDs {
		spec := p.AgentList.Specs[id]
		if spec.Auth != nil {
			if p.Auth == nil {
				p.Auth = map[string]AgentAuth{}
			}
			p.Auth[id] = *spec.Auth
		}
		if spec.CoAuthor != "" {
			if p.Git.CoAuthors == nil {
				p.Git.CoAuthors = map[string]string{}
			}
			p.Git.CoAuthors[id] = spec.CoAuthor
		}
		for file, seed := range spec.Config {
			if p.AgentConfig == nil {
				p.AgentConfig = map[string]string{}
			}
			p.AgentConfig[agentConfigKey(id, file)] = seed
		}
	}
	if err = p.Resources.validate(); err != nil {
		return p, fmt.Errorf("profile %s: %w", name, err)
	}
	if len(p.Agents) == 0 {
		return p, fmt.Errorf("profile %s has no agents; declare the agents installed in its image", name)
	}
	seen := map[string]bool{}
	for _, id := range p.Agents {
		if _, err := FindAgent(id); err != nil {
			return p, err
		}
		if seen[id] {
			return p, fmt.Errorf("duplicate agent %s", id)
		}
		seen[id] = true
	}
	for _, path := range append([]string{p.Start}, sortedValues(p.Scripts)...) {
		if path != "" && !safeRelative(path) {
			return p, fmt.Errorf("profile %s: script and dockerfile paths must stay inside their directory", name)
		}
	}
	for _, rules := range [][]string{p.AllowNetwork, p.DenyNetwork} {
		for _, h := range rules {
			if !networkPattern.MatchString(h) || h == "*" {
				return p, fmt.Errorf("profile %s: network rule %q must be one host, wildcard domain (*.example.com), IP or CIDR, optionally with :port", name, h)
			}
		}
	}
	// sbx refuses a deny for a host an allow names, and a service's host has
	// to be reachable.
	for _, h := range p.DenyNetwork {
		if slices.Contains(p.AllowNetwork, h) {
			return p, fmt.Errorf("profile %s: %s is both allowed and denied; keep one", name, h)
		}
		for svc, s := range p.Services {
			if slices.Contains(s.ProxyHosts(), h) {
				return p, fmt.Errorf("profile %s: %s is denied, but service %s needs it", name, h, svc)
			}
		}
	}
	if err = validatePorts(p.Ports); err != nil {
		return p, fmt.Errorf("profile %s: %w", name, err)
	}
	for id, a := range p.Auth {
		if !seen[id] {
			return p, fmt.Errorf("profile %s: auth names %s, which is not among its agents", name, id)
		}
		switch {
		case a.Method == AuthOAuth || a.Method == AuthAPIKey:
		case (a.Method == AuthCommand || a.Method == AuthToken) && validCommand(a.Command):
		default:
			return p, fmt.Errorf("profile %s: auth of %s must be oauth, api-key, {command: <one host command>} or {token: <one host command>}, a command being secret:<name> for one unibox keeps", name, id)
		}
	}
	gitURLs := map[string]string{}
	for svc, s := range p.Services {
		if err := ValidName(svc); err != nil {
			return p, fmt.Errorf("profile %s: service name %q: %w", name, svc, err)
		}
		if len(s.Host) == 0 {
			return p, fmt.Errorf("profile %s: service %s needs a host it authenticates to", name, svc)
		}
		for _, h := range s.Host {
			host, path := splitServiceHost(h)
			if !networkPattern.MatchString(host) || host == "*" {
				return p, fmt.Errorf("profile %s: service %s host %q must be one host, wildcard domain (*.example.com) or IP, optionally with :port", name, svc, h)
			}
			if s.Git && strings.ContainsAny(host, "*?[]!") {
				return p, fmt.Errorf("profile %s: service %s is a git service; its host %q must be exact, since git sends the header by URL", name, svc, h)
			}
			if strings.Contains(h, "/") && (!s.Git || !servicePath.MatchString(path) || slices.Contains(strings.Split(path, "/"), "..") || slices.Contains(strings.Split(path, "/"), ".")) {
				return p, fmt.Errorf("profile %s: service %s host %q: only a git service takes a path, such as github.com/some-org", name, svc, h)
			}
		}
		if s.Git {
			for _, u := range s.GitURLs() {
				if other, ok := gitURLs[u]; ok {
					return p, fmt.Errorf("profile %s: services %s and %s both sign git in at %s; give one of them another host or path", name, other, svc, u)
				}
				gitURLs[u] = svc
			}
		}
		if !validCommand(s.Command) {
			return p, fmt.Errorf("profile %s: service %s needs command: one host command that prints its token, or secret:<name> for one unibox keeps", name, svc)
		}
		if !s.Git && (s.Scheme != "" || s.User != "") {
			return p, fmt.Errorf("profile %s: service %s sets scheme or user, which only a git service has; a tool makes its own header from ${services.%s.token} in env", name, svc, svc)
		}
		scheme := s.Scheme
		if scheme == "" {
			scheme = SchemeBasic
		}
		switch scheme {
		case SchemeBasic, SchemeBearer, SchemeRaw:
		default:
			return p, fmt.Errorf("profile %s: service %s scheme must be basic, bearer or raw", name, svc)
		}
		if s.Git && scheme == SchemeBasic && strings.TrimSpace(s.User) == "" {
			return p, fmt.Errorf("profile %s: service %s uses basic auth; give it the user to pair with the token", name, svc)
		}
		if s.Git && scheme != SchemeBasic && strings.TrimSpace(s.User) != "" {
			return p, fmt.Errorf("profile %s: service %s sets user, which only the basic scheme uses", name, svc)
		}
		if s.Refresh != "" && s.Refresh != "on-demand" {
			if d, err := time.ParseDuration(s.Refresh); err != nil || d < time.Minute {
				return p, fmt.Errorf("profile %s: service %s refresh must be on-demand or a duration of a minute or more, such as 10m", name, svc)
			}
		}
		if (s.Author != "" || s.CoAuthor != nil) && !s.Git {
			return p, fmt.Errorf("profile %s: service %s sets author or co_author, which only a git service has", name, svc)
		}
		for _, field := range []string{s.User, s.Author} {
			if err := p.checkRefs("service "+svc, field, false); err != nil {
				return p, err
			}
		}
		if s.Author != "" && !hasRefs(s.Author) {
			if _, _, err := AuthorParts(s.Author); err != nil {
				return p, fmt.Errorf("profile %s: service %s author must be 'Name <email>'", name, svc)
			}
		}
	}
	if p.Git.Author != "" && p.Git.Author != "user" {
		return p, fmt.Errorf("profile %s: git.author must be user", name)
	}
	for id, c := range p.Git.CoAuthors {
		if !seen[id] {
			return p, fmt.Errorf("profile %s: git.co_authors names %s, which is not among its agents", name, id)
		}
		if !ValidCoAuthor(c) {
			return p, fmt.Errorf("profile %s: co_author of %s must be Name <email>, or Name <> for none", name, id)
		}
	}
	for _, key := range sortedKeys(p.Vars) {
		if err := ValidVarName(key); err != nil {
			return p, fmt.Errorf("profile %s: var %w", name, err)
		}
	}
	for _, key := range sortedKeys(p.EnvValues) {
		token := strings.HasPrefix(strings.TrimSpace(p.EnvValues[key]), "${services.")
		if !toolEnvName.MatchString(key) || sensitiveEnv.MatchString(key) && !token || key == "HOME" || key == "PATH" || strings.HasPrefix(key, "UNIBOX_") || slices.Contains(p.Env, key) {
			return p, fmt.Errorf("profile %s: env %s is reserved, passed from the host already, or names a secret that is not a service's token", name, key)
		}
		if err := p.checkRefs("env "+key, p.EnvValues[key], true); err != nil {
			return p, err
		}
	}
	for _, key := range sortedKeys(p.BuildArgSpec) {
		if !toolEnvName.MatchString(key) || sensitiveEnv.MatchString(key) || key == "UNIBOX_RUNTIME" {
			return p, fmt.Errorf("profile %s: build argument %s is reserved or names a secret, which a build argument would leave in the image", name, key)
		}
		if err := p.checkRefs("build argument "+key, p.BuildArgSpec[key], false); err != nil {
			return p, err
		}
	}
	for _, key := range p.Env {
		if !toolEnvName.MatchString(key) || sensitiveEnv.MatchString(key) || key == "HOME" || key == "PATH" {
			return p, fmt.Errorf("profile %s: environment name %q is reserved or sensitive; configure credentials through the runtime", name, key)
		}
	}
	for i, c := range p.Caches {
		if !envName.MatchString(c.Env) || sensitiveEnv.MatchString(c.Env) || c.Env == "HOME" || c.Env == "PATH" {
			return p, fmt.Errorf("profile %s: invalid cache environment name %q", name, c.Env)
		}
		h, err := ExpandPath(c.Host)
		if err != nil {
			return p, err
		}
		h, err = filepath.EvalSymlinks(h)
		if err != nil {
			return p, fmt.Errorf("cache %s is unavailable; create it or remove it from the profile", c.Host)
		}
		info, err := os.Stat(h)
		if err != nil || !info.IsDir() {
			return p, fmt.Errorf("cache %s must be a directory", h)
		}
		if strings.ContainsAny(h, ":\n\r") {
			return p, fmt.Errorf("cache path contains an unsupported character")
		}
		p.Caches[i].Host = h
	}
	p.Files = map[string]SnapshotFile{}
	// The profile's root is held to its shape too: the definition, its
	// Dockerfile, start script and instructions.md, hidden runtime
	// directories - and custom files behind an underscore, _data by
	// convention, where the Dockerfile and sandbox.yaml reference them.
	known := map[string]bool{"sandbox.yaml": true, ProfileDockerfile: true, "instructions.md": true}
	for _, ref := range append([]string{p.Start}, sortedValues(p.Scripts)...) {
		if ref != "" && !strings.Contains(ref, "/") {
			known[ref] = true
		}
	}
	shape, err := os.ReadDir(p.Dir)
	if err != nil {
		return p, err
	}
	for _, e := range shape {
		n := e.Name()
		if strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || known[n] {
			continue
		}
		if n == "Dockerfile" {
			return p, fmt.Errorf("profile %s: its image builds from sandbox.dockerfile; rename Dockerfile to it (unibox init %s does)", name, name)
		}
		return p, fmt.Errorf("profile %s: unexpected %s; a profile's root holds sandbox.yaml, sandbox.dockerfile, its start script, scripts and instructions.md - custom files go under _data and are referenced from there", name, n)
	}
	snapshot := func(dir, prefix string) error {
		return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("profile contains symlink %s; use a regular file in its build context", path)
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("profile contains non-regular file %s", path)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			p.Files[prefix+filepath.ToSlash(rel)] = SnapshotFile{b, uint32(info.Mode().Perm())}
			return nil
		})
	}
	if err = snapshot(p.Dir, ""); err != nil {
		return p, err
	}
	// .shared, unibox's, and _shared, the user's, are part of every
	// profile's build context, so a Dockerfile runs their installation
	// scripts; a change of them outdates every image.
	for _, shared := range []string{".shared", "_shared"} {
		if _, err := os.Stat(filepath.Join(p.Dir, shared)); err == nil {
			return p, fmt.Errorf("profile %s has a %s directory of its own; %s lives at the repository root", name, shared, shared)
		}
		if info, err := os.Stat(filepath.Join(p.Root, shared)); err == nil && info.IsDir() {
			if err = snapshot(filepath.Join(p.Root, shared), shared+"/"); err != nil {
				return p, err
			}
		}
	}
	// A profile copied from a template sees that template's shared files
	// too, and no other template's.
	if p.Template != "" {
		t := p.TemplateName()
		if ValidName(t) != nil {
			return p, fmt.Errorf("profile %s: template %q is <template>/<profile>, as unibox init --from writes it", name, p.Template)
		}
		dir := filepath.Join(p.Root, ".templates", t)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return p, fmt.Errorf("profile %s was copied from template %s, which .templates no longer has; run unibox template update %s, or take template: out of the profile", name, t, t)
		}
		for _, sub := range []string{"@install", "@instructions"} {
			if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
				continue
			}
			if err = snapshot(filepath.Join(dir, sub), ".templates/"+t+"/"+sub+"/"); err != nil {
				return p, err
			}
		}
	}
	if p.Start != "" {
		if _, ok := p.Files[filepath.ToSlash(p.Start)]; !ok {
			return p, fmt.Errorf("profile %s: missing %s", name, p.Start)
		}
	}
	// The image builds from sandbox.dockerfile, or is one built already.
	_, builds := p.Files[ProfileDockerfile]
	switch {
	case builds && p.Image != "":
		return p, fmt.Errorf("profile %s has sandbox.dockerfile and names image %s; keep one", name, p.Image)
	case builds:
		p.Dockerfile = ProfileDockerfile
	case p.Image == "":
		return p, fmt.Errorf("profile %s has no image: add sandbox.dockerfile beside its sandbox.yaml, or name a built one with image: <reference>", name)
	}
	for _, script := range sortedKeys(p.Scripts) {
		if !portName.MatchString(script) {
			return p, fmt.Errorf("profile %s: script name %q must be lower case letters, digits and dashes, starting with a letter", name, script)
		}
		if p.Scripts[script] == "" {
			return p, fmt.Errorf("profile %s: script %s names no file", name, script)
		}
		if _, ok := p.Files[filepath.ToSlash(p.Scripts[script])]; !ok {
			return p, fmt.Errorf("profile %s: script %s: missing %s", name, script, p.Scripts[script])
		}
	}
	for _, rt := range []string{"sbx", "msb", "podman"} {
		entries, _ := os.ReadDir(filepath.Join(p.Dir, "."+rt))
		for _, e := range entries {
			if e.IsDir() || e.Name() != "build.args" {
				return p, fmt.Errorf("unknown runtime file .%s/%s; only build.args is supported", rt, e.Name())
			}
		}
		if _, err := os.Stat(filepath.Join(p.Dir, rt)); err == nil {
			return p, fmt.Errorf("profile %s: runtime directories are hidden; rename %s to .%s", name, rt, rt)
		}
	}
	for _, entry := range p.Instructions {
		path, err := p.instructionFile(entry)
		if err != nil {
			return p, err
		}
		if _, err := readInside(p.Root, path); err != nil {
			return p, fmt.Errorf("profile instructions: %w", err)
		}
	}
	for entry, file := range p.AgentConfig {
		agent, fileName := splitAgentConfigKey(entry)
		_, f, err := ConfigFile(agent, fileName)
		if err != nil {
			return p, fmt.Errorf("profile %s: %w", name, err)
		}
		data, err := readInside(p.Root, filepath.Join(p.Dir, file))
		if err != nil {
			return p, fmt.Errorf("profile %s: config of %s: %w", name, agent, err)
		}
		if _, err = parseSeed(f, data); err != nil {
			return p, fmt.Errorf("profile %s: config %s of %s: %v", name, file, agent, err)
		}
	}
	install, err := p.InstallContext()
	if err != nil {
		return p, err
	}
	p.Revision = digest(struct {
		Files             map[string]SnapshotFile
		Dockerfile, Image string
		Agents            []string
		Install           map[string]SnapshotFile
	}{p.BuildFiles(), p.Dockerfile, p.Image, p.Agents, install})
	return p, nil
}

// BuildFiles is the image's inputs: the profile's files without what only
// unibox reads - the definition, the automatic instructions.md, the agent
// configuration seeds. Those change without the image changing.
//
// A script is read when it runs, so it is not one either.
func (p Profile) BuildFiles() map[string]SnapshotFile {
	skip := map[string]bool{"sandbox.yaml": true, "instructions.md": true}
	for _, file := range p.AgentConfig {
		skip[filepath.ToSlash(file)] = true
	}
	for _, file := range p.Scripts {
		skip[filepath.ToSlash(file)] = true
	}
	out := make(map[string]SnapshotFile, len(p.Files))
	for name, file := range p.Files {
		// The shared instruction texts go to the agent, not into the image.
		if strings.HasPrefix(name, ".shared/@instructions/") || strings.HasPrefix(name, "_shared/@instructions/") || strings.HasPrefix(name, ".templates/") && strings.Contains(name, "/@instructions/") || strings.HasSuffix(name, "/source.yaml") && strings.HasPrefix(name, ".templates/") {
			continue
		}
		if !skip[name] {
			out[name] = file
		}
	}
	return out
}

// InstallContext is the build context a Dockerfile copies @install from
// (COPY --from=unibox / /): the runner at /usr/local/bin/@install and the
// installation scripts at /usr/local/lib/unibox/install, unibox's from
// .shared/@install with the user's of _shared/@install over them, so a name
// of the user's wins.
func (p Profile) InstallContext() (map[string]SnapshotFile, error) {
	runner, err := templatesFS.ReadFile("templates/build/@install")
	if err != nil {
		return nil, err
	}
	out := map[string]SnapshotFile{"usr/local/bin/@install": {runner, 0o755}}
	// Later wins: the template's over unibox's, the user's over both.
	froms := []string{".shared/@install/"}
	if p.Template != "" {
		froms = append(froms, ".templates/"+p.TemplateName()+"/@install/")
	}
	for _, from := range append(froms, "_shared/@install/") {
		for name, file := range p.Files {
			if rest, ok := strings.CutPrefix(name, from); ok {
				out["usr/local/lib/unibox/install/"+rest] = file
			}
		}
	}
	return out, nil
}

// TemplateName is the template the profile was copied from, "" for none.
func (p Profile) TemplateName() string {
	name, _, _ := strings.Cut(p.Template, "/")
	return name
}

// TemplateProfile is the template's profile this one was copied from.
func (p Profile) TemplateProfile() string {
	_, profile, _ := strings.Cut(p.Template, "/")
	return profile
}

// OAuthAgent is the agent the profile signs in by OAuth; the first in the
// profile's order when it names several.
func (p Profile) OAuthAgent() string {
	for _, id := range p.Agents {
		if p.Auth[id].Method == AuthOAuth {
			return id
		}
	}
	return ""
}

func (p Profile) ApprovalKey() string { return digest([]string{p.Root, p.Name}) }
func (p Profile) AccessHash() string  { return digest(p.HostAccess) }

// HostFacing says whether the profile asks for anything of the host - an
// environment value, a cache mount, a network rule, a login. A change of
// that is what a start tells the user.
func (p Profile) HostFacing() bool {
	return len(p.Env)+len(p.Caches)+len(p.DenyNetwork)+len(p.AllowNetwork)+len(p.Ports)+len(p.Auth)+len(p.Services) > 0
}

// SecretRefs are the secrets unibox keeps that the profile names, each once.
func (p Profile) SecretRefs() []string {
	out := []string{}
	add := func(c string) {
		if name, ok := SecretRef(c); ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, a := range p.Auth {
		add(a.Command)
	}
	for _, s := range p.Services {
		add(s.Command)
	}
	slices.Sort(out)
	return out
}

// ServicePlaceholder is the string a service's token is hidden behind inside
// the sandbox: distinctive and non-secret, so the proxy finds it in a header
// and swaps the host's value in. It is deterministic in the service's name
// and hosts, so provisioning and the git wiring derive the same string
// without asking the runtime.
func ServicePlaceholder(name string, s Service) string {
	sum := sha256.Sum256([]byte(name + "\x00" + strings.Join(s.Host, ",")))
	return "unibox-svc-" + name + "-" + hex.EncodeToString(sum[:4])
}

// ServiceEnv is the variable a runtime is given a service's placeholder
// in, as it wants one: UNIBOX_SVC_<NAME>, upper case as sbx requires. Git
// reads a git service's from its config, and the profile's env names the
// tool's own variable for any other, which unibox sets in each session.
func ServiceEnv(name string, s Service) string {
	return "UNIBOX_SVC_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// serviceSecretCommand is the host command whose output sbx stores for the
// service. For a git service unibox owns the whole Authorization header, so
// it is composed here; for an env service the tool builds the header and
// only the raw token is stored. $( ) strips the token's trailing newline.
func serviceSecretCommand(s Service) string {
	switch {
	case s.Git && s.Scheme == SchemeBearer:
		return fmt.Sprintf(`printf 'Bearer %%s' "$(%s)"`, s.Command)
	case s.Git && s.Scheme != SchemeRaw:
		return fmt.Sprintf(`printf 'Basic %%s' "$(printf '%%s:%%s' %s "$(%s)" | base64 | tr -d '\n')"`, Quote(s.User), s.Command)
	default:
		return fmt.Sprintf(`printf '%%s' "$(%s)"`, s.Command)
	}
}
func (p Profile) BuildArgs(runtime string) ([]string, error) {
	args := []string{"UNIBOX_RUNTIME=" + runtime}
	// The profile's own go first, so a runtime's file says the last word.
	for _, k := range sortedKeys(p.BuildArgValues) {
		args = append(args, k+"="+p.BuildArgValues[k])
	}
	f, ok := p.Files["."+runtime+"/build.args"]
	if !ok {
		return args, nil
	}
	for _, line := range strings.Split(string(f.Data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if !ok || !envName.MatchString(k) || sensitiveEnv.MatchString(k) || k == "UNIBOX_RUNTIME" {
			return nil, fmt.Errorf("invalid build argument %q; use NAME=value without credentials", k)
		}
		args = append(args, line)
	}
	return args, nil
}

// instructionFile resolves one instructions entry of a profile: a bare name
// is a shared text, the user's in _shared/@instructions before unibox's in
// .shared/@instructions; anything with a path or an extension is a markdown
// file named relative to sandbox.yaml, anywhere in the repository and
// confined to it when read.
func (p Profile) instructionFile(entry string) (string, error) {
	if ValidName(entry) == nil {
		return sharedText(p.Root, p.TemplateName(), entry)
	}
	if filepath.IsAbs(entry) || !strings.HasSuffix(entry, ".md") {
		return "", fmt.Errorf("instruction %q is neither a name of _shared/@instructions or .shared/@instructions nor a relative .md path", entry)
	}
	return filepath.Join(p.Dir, entry), nil
}

// sharedText is the file of a shared instruction text: the user's when
// _shared/@instructions has it, else the profile's template's, else
// unibox's.
func sharedText(root, template, name string) (string, error) {
	dirs := []string{"_shared"}
	if template != "" {
		dirs = append(dirs, filepath.Join(".templates", template))
	}
	for _, dir := range append(dirs, ".shared") {
		file := filepath.Join(root, dir, "@instructions", name+".md")
		if _, err := os.Lstat(file); err == nil {
			return file, nil
		}
	}
	return "", fmt.Errorf("no instruction text %s in _shared/@instructions or .shared/@instructions", name)
}

// InstructionText lays the texts together: the profile's entries, its own
// instructions.md when it has one, then the sandbox's attached names - each
// file once. Nothing is fixed in the binary; the environments repository
// owns every word the agent gets.
func (p Profile) InstructionText(attached []string) (string, error) {
	entries := append([]string{}, p.Instructions...)
	if _, ok := p.Files["instructions.md"]; ok {
		entries = append(entries, "instructions.md")
	}
	paths := []string{}
	for _, entry := range entries {
		path, err := p.instructionFile(entry)
		if err != nil {
			return "", err
		}
		paths = append(paths, path)
	}
	for _, name := range attached {
		if err := ValidName(name); err != nil {
			return "", fmt.Errorf("attached instructions are names of shared texts: %w", err)
		}
		path, err := sharedText(p.Root, p.TemplateName(), name)
		if err != nil {
			return "", err
		}
		paths = append(paths, path)
	}
	parts := []string{}
	used := map[string]bool{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if used[path] {
			continue
		}
		used[path] = true
		b, err := readInside(p.Root, path)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "\n\n") + "\n", nil
}

// CheckLayout holds the environments repository's opinionated parts to
// their shape - profiles under sandboxes/, shared texts as markdown under
// _shared/@instructions and .shared/@instructions - so a misplaced file is
// named instead of quietly ignored. Anything else in the repository is the
// user's to arrange; a profile names such a file by a relative path.
func CheckLayout(root string) error {
	if _, err := os.Stat(filepath.Join(root, "_shared", ".install")); err == nil {
		return fmt.Errorf("the environments repository is laid out as an older unibox did, with _shared/.install; run unibox init to move it to .shared and _shared/@install")
	}
	profiles, err := os.ReadDir(filepath.Join(root, "sandboxes"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range profiles {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() {
			return fmt.Errorf("sandboxes/%s is not a profile directory; every entry of sandboxes/ is a directory with a sandbox.yaml", e.Name())
		}
		if _, err := os.Stat(filepath.Join(root, "sandboxes", e.Name(), "sandbox.yaml")); err != nil {
			return fmt.Errorf("sandboxes/%s has no sandbox.yaml; every profile directory needs one", e.Name())
		}
	}
	for _, dir := range []string{"_shared/@instructions", ".shared/@instructions"} {
		texts, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, e := range texts {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				return fmt.Errorf("%s/%s is not a markdown file; %s holds attachable .md texts alone", dir, e.Name(), dir)
			}
			if e.Name() == "instructions.md" {
				return fmt.Errorf("%s/instructions.md collides with the profiles' automatic instructions.md; give the text a name of its own", dir)
			}
		}
	}
	return nil
}

func ProfileNames(root string) ([]string, error) {
	if root == "" {
		return []string{}, nil
	}
	root, err := ExpandPath(root)
	if err != nil {
		return nil, err
	}
	if err = CheckLayout(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "sandboxes"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, "sandboxes", e.Name(), "sandbox.yaml")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// decodeStrict decodes a node as DecodeYAML decodes a document: a field the
// type lacks is an error naming it.
func decodeStrict(n *yaml.Node, v any) error {
	b, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	return DecodeYAML(b, v)
}

// checkFormat reads a profile's version before anything else of it, so a
// profile of another form is named as such rather than for a field it has
// or lacks.
func checkFormat(name string, text []byte) error {
	var doc yaml.Node
	version := ""
	if yaml.Unmarshal(text, &doc) == nil && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		if v := mapValue(doc.Content[0], "version"); v != nil {
			version = v.Value
		}
	}
	if version == fmt.Sprint(ProfileFormat) {
		return nil
	}
	if n, err := strconv.Atoi(version); err == nil && n > ProfileFormat {
		return formatError(fmt.Sprintf("profile %s is of format %d, newer than the %d this unibox reads; upgrade unibox", name, n, ProfileFormat))
	}
	if version == "" {
		version = "none"
	}
	return formatError(fmt.Sprintf("profile %s is of format %s and this unibox reads format %d; run unibox init %s to migrate it", name, version, ProfileFormat, name))
}

// formatError says a profile is of another format, which no repair of a
// field fixes.
type formatError string

func (e formatError) Error() string { return string(e) }

// parseSeed reads a seed as its file is written: TOML or JSON, an object.
func parseSeed(f AgentFile, data []byte) (map[string]any, error) {
	seed := map[string]any{}
	if f.TOML() {
		if err := toml.Unmarshal(data, &seed); err != nil {
			return nil, fmt.Errorf("not TOML: %v", err)
		}
		return seed, nil
	}
	if err := json.Unmarshal(data, &seed); err != nil {
		return nil, fmt.Errorf("not a JSON object: %v", err)
	}
	return seed, nil
}

func sortedValues(m map[string]string) []string {
	out := []string{}
	for _, k := range sortedKeys(m) {
		out = append(out, m[k])
	}
	return out
}

// knownScripts says which of the names the profile has no script for.
func (p Profile) knownScripts(names []string) error {
	for _, n := range names {
		if _, ok := p.Scripts[n]; !ok {
			if len(p.Scripts) == 0 {
				return fmt.Errorf("profile %s has no scripts; name them under scripts in its sandbox.yaml", p.Name)
			}
			return fmt.Errorf("profile %s has no script %s; it has %s", p.Name, n, strings.Join(sortedKeys(p.Scripts), ", "))
		}
	}
	return nil
}
