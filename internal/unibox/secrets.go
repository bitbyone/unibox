package unibox

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tobola/unibox/internal/unibox/api"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var secretName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidSecretName says whether a name is one sbx stores a service's token
// under, as a kit's credentials and an env's bindings name them.
func ValidSecretName(name string) bool { return secretName.MatchString(name) }

// NeededSecret is a service an env's sandbox takes a token for, and
// whether sbx has one stored: sbx's secrets are set once and used by every
// sandbox that needs them.
type NeededSecret struct {
	Service     string `json:"service"`
	Description string `json:"description,omitempty"`
	// Optional is a credential a kit can do without.
	Optional bool `json:"optional,omitempty"`
	// Kits are the kits asking for it; empty for one only the env's
	// bindings name.
	Kits   []string `json:"kits,omitempty"`
	Stored bool     `json:"stored"`
	// Domains are the hosts the kits have the token sent to.
	Domains []string `json:"domains,omitempty"`
	// Scope is where the token is stored: global, a sandbox's name, or
	// the env or the sandbox config.yaml keeps it for, as "env dev" or
	// "sandbox dev"; Own says how those keep it, by a command or in the
	// keychain.
	Scope string `json:"scope,omitempty"`
	Own   string `json:"own,omitempty"`
	// Variable is the variable a token kept the way a subscription's is
	// stands in, for one stored so.
	Variable string `json:"variable,omitempty"`
	// Page is where the token is made, and Hint what to make there.
	Page string `json:"page,omitempty"`
	Hint string `json:"hint,omitempty"`
}

// TokenGuide says where a service's token is made - a page a browser
// opens, filled in as far as the service allows - and what to make there;
// or, for a service whose token is made more ways than one, each way.
type TokenGuide struct {
	Page string `yaml:"page"`
	Hint string `yaml:"hint"`
	// Command is what the person runs on the host for the token, such as
	// claude setup-token, which prints it; unibox only names it.
	Command string     `yaml:"command"`
	Ways    []TokenWay `yaml:"ways"`
}

// TokenWay is one way a token is made: a subscription's token a command
// prints, or an API key made on a page.
type TokenWay struct {
	Name    string `yaml:"name"`
	Page    string `yaml:"page"`
	Command string `yaml:"command"`
	Hint    string `yaml:"hint"`
	// SignIn has sbx sign the person in with their subscription, in the
	// browser, rather than ask for a token.
	SignIn bool `yaml:"sign_in"`
	// Agent says the agent signs in itself, in the sandbox, as Claude
	// does with a subscription (/login); sbx keeps the sign-in, and
	// nothing is stored now.
	Agent bool `yaml:"agent"`
	// Env, Placeholder and Hosts keep the token as a custom secret: the
	// sandbox sees Placeholder in Env, and the proxy replaces it on
	// requests to Hosts.
	Env         string   `yaml:"env"`
	Placeholder string   `yaml:"placeholder"`
	Hosts       []string `yaml:"hosts"`
}

// AllWays are the ways a guide offers, a guide of one page or command as
// one way; none when it names neither.
func (g TokenGuide) AllWays() []TokenWay {
	if len(g.Ways) > 0 {
		return g.Ways
	}
	if g.Page == "" && g.Command == "" {
		return nil
	}
	return []TokenWay{{Name: "Generate a new token", Page: g.Page, Command: g.Command, Hint: g.Hint}}
}

// githubTokenPage is GitHub's page for a fine-grained token, filled in by
// its parameters: a name, and permissions such as contents=write.
func githubTokenPage(name, description string, permissions ...string) string {
	q := url.Values{"name": {name}, "description": {description}}
	for _, p := range permissions {
		k, v, _ := strings.Cut(p, "=")
		q.Set(k, v)
	}
	return "https://github.com/settings/personal-access-tokens/new?" + q.Encode()
}

// knownTokenGuides are the services unibox knows the page of without the
// team saying it: GitHub's, which sbx's own kits ask for.
var knownTokenGuides = map[string]TokenGuide{
	"anthropic": {Ways: []TokenWay{
		// Docker's Claude kit manages api.anthropic.com and drops any other
		// credential there, so a subscription signs in inside Claude.
		{Name: "Claude subscription", Agent: true, Hint: "Claude Code signs in with your subscription itself: run /login in it the first time it starts; sbx keeps the sign-in"},
		{Name: "API key", Hint: "an Anthropic API key"},
	}},
	"openai": {Ways: []TokenWay{
		{Name: "ChatGPT subscription", SignIn: true, Hint: "sbx signs you in with your ChatGPT account, in the browser"},
		{Name: "API key", Hint: "an OpenAI API key"},
	}},
	"copilot": {
		Page: githubTokenPage("unibox copilot", "Copilot requests from unibox's sandboxes", "user_copilot_requests=read"),
		Hint: "a fine-grained token with Copilot Requests read-only and access to no repository",
	},
	"github": {
		Page: githubTokenPage("unibox github", "git and gh in unibox's sandboxes", "contents=write", "pull_requests=write"),
		Hint: "a fine-grained token for the repositories the agent works on; leave Workflows out, so it cannot change GitHub Actions",
	},
}

// TokenGuides are where an env's tokens are made: unibox's own for the
// services it knows, under what the team's files say, a later file's over
// an earlier one's.
func TokenGuides(sbxenvFiles []string) (map[string]TokenGuide, error) {
	out := maps.Clone(knownTokenGuides)
	err := readTeamFiles(sbxenvFiles, func(name, _ string, doc teamDoc) error {
		for service, g := range doc.Secrets {
			if !ValidSecretName(service) {
				return fmt.Errorf("%s: secret %q is no service name sbx takes; use letters, digits and dashes", name, service)
			}
			for _, w := range append([]TokenWay{{Page: g.Page}}, g.Ways...) {
				if w.Page == "" {
					continue
				}
				if u, err := url.Parse(w.Page); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
					return fmt.Errorf("%s: secret %s: page %q is no web address; give one starting with https://", name, service, w.Page)
				}
			}
			for _, w := range g.Ways {
				if w.Name == "" {
					return fmt.Errorf("%s: secret %s: each of its ways needs a name", name, service)
				}
			}
			was := out[service]
			if len(g.Ways) > 0 {
				// Ways the team names replace what was said before.
				out[service] = TokenGuide{Hint: cmp.Or(g.Hint, was.Hint), Ways: g.Ways}
				continue
			}
			out[service] = TokenGuide{Page: cmp.Or(g.Page, was.Page), Hint: cmp.Or(g.Hint, was.Hint), Command: cmp.Or(g.Command, was.Command)}
		}
		return nil
	})
	return out, err
}

// NeededSecrets are the services an env needs tokens for: those its kits
// declare and those its bindings name, each with whether sbx has one,
// globally or for the sandbox alone. sandbox "" is the one the env
// creates, which a secret may already be stored for.
func (m *Manager) NeededSecrets(ctx context.Context, env Env, sandbox string) ([]NeededSecret, error) {
	byService := map[string]*NeededSecret{}
	order := []string{}
	add := func(service string) *NeededSecret {
		if l, ok := byService[service]; ok {
			return l
		}
		l := &NeededSecret{Service: service, Optional: true}
		byService[service] = l
		order = append(order, service)
		return l
	}
	if sandbox == "" {
		sandbox = env.SandboxName()
	}
	// sbx's secrets are listed while the kits are read.
	var secrets []Secret
	var secretsErr error
	listed := make(chan struct{})
	go func() {
		defer close(listed)
		secrets, secretsErr = m.SBX.Secrets(ctx)
	}()
	kits, err := m.readKits(ctx, env)
	<-listed
	if err != nil {
		return nil, err
	}
	for i, k := range env.SBX.Kits {
		for _, c := range kits[i].Credentials {
			l := add(c.Service)
			if l.Description == "" {
				l.Description = c.Description
			}
			l.Optional = l.Optional && c.Optional
			l.Kits = append(l.Kits, kitName(k.Source))
			for _, d := range c.Domains() {
				if !slices.Contains(l.Domains, d) {
					l.Domains = append(l.Domains, d)
				}
			}
		}
	}
	for _, service := range env.SBX.Bindings {
		add(service).Optional = false
	}
	if secretsErr != nil {
		return nil, fmt.Errorf("cannot list sbx's secrets: %w", secretsErr)
	}
	guides, err := TokenGuides(env.Config.Files)
	if err != nil {
		return nil, err
	}
	out := []NeededSecret{}
	for _, service := range order {
		l := *byService[service]
		l.Page, l.Hint = guides[service].Page, guides[service].Hint
		for _, s := range secrets {
			if s.Scope != "global" && (sandbox == "" || s.Scope != sandbox) {
				continue
			}
			if s.Type != "custom" && s.Name == service {
				l.Stored, l.Scope = true, s.Scope
			}
		}
		// The env's own, which unibox has sbx keep for each of its
		// sandboxes, stands before a global one, and the sandbox's own
		// before the env's.
		if c, ok := env.Config.Secrets[service]; ok && c.Command != "" {
			l.Stored, l.Scope, l.Own = true, "env "+env.Name, ownText(c)
		}
		if c, ok := m.Config.Sandboxes[sandbox].Secrets[service]; ok && c.Command != "" {
			l.Stored, l.Scope, l.Own = true, "sandbox "+sandbox, ownText(c)
		}
		// A token kept the way a subscription's is, in a variable, stands
		// for the service's.
		for _, w := range guides[service].AllWays() {
			for _, s := range secrets {
				if !l.Stored && w.Env != "" && s.Type == "custom" && s.Env == w.Env && (s.Scope == "global" || s.Scope == sandbox) {
					l.Stored, l.Scope, l.Variable = true, s.Scope+", "+w.Name, s.Env
				}
			}
		}
		out = append(out, l)
	}
	slices.SortStableFunc(out, func(a, b NeededSecret) int {
		switch {
		case !a.Optional && b.Optional:
			return -1
		case a.Optional && !b.Optional:
			return 1
		}
		return 0
	})
	return out, nil
}

// readKits reads an env's kits as sbx sees them, each with the args the
// env gives it, which may be the person's. sbx reads them one at a time,
// a kit of a registry for seconds, so what it said is kept: in memory for
// a list drawn again, and on disk for the next command - for a kit of
// files until one of them changes, for a registry's until a day passes or,
// pinned by digest, for good.
func (m *Manager) readKits(ctx context.Context, env Env) ([]Kit, error) {
	values, _ := m.Paths.LoadEnvValues()
	kits := make([]Kit, len(env.SBX.Kits))
	errs := make([]error, len(env.SBX.Kits))
	var wg sync.WaitGroup
	for i, k := range env.SBX.Kits {
		ref := k.Source
		if k.Dir != "" {
			ref = k.Dir
		}
		args := env.SBX.KitArgs(k, values.ArgsFor(env))
		key := ref
		for _, name := range sortedKeys(args) {
			key += "\x00" + name + "=" + args[name]
		}
		if kit, ok := m.kits.Load(key); ok {
			kits[i] = kit.(Kit)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			file, fresh := m.kitCacheFile(key, k.Dir)
			if file != "" {
				if b, err := os.ReadFile(file); err == nil && fresh(file) && json.Unmarshal(b, &kits[i]) == nil {
					m.kits.Store(key, kits[i])
					return
				}
			}
			if kits[i], errs[i] = m.SBX.KitInspect(ctx, ref, args); errs[i] != nil {
				return
			}
			m.kits.Store(key, kits[i])
			if b, err := json.Marshal(kits[i]); err == nil && file != "" && os.MkdirAll(filepath.Dir(file), 0o700) == nil {
				_ = os.WriteFile(file, b, 0o600)
			}
		}()
	}
	wg.Wait()
	return kits, errors.Join(errs...)
}

// kitCacheFile is where what sbx said of a kit is kept, and whether what
// is kept there still holds. A kit of files is named by the files' names,
// sizes and times, so a change to one is a file not yet written.
func (m *Manager) kitCacheFile(key, dir string) (string, func(string) bool) {
	always := func(string) bool { return true }
	h := sha256.New()
	h.Write([]byte(key))
	fresh := always
	switch {
	case dir != "":
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "\x00%s %d %d", path, info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			return "", always
		}
	case !strings.Contains(key, "@sha256:"):
		fresh = func(file string) bool {
			info, err := os.Stat(file)
			return err == nil && time.Since(info.ModTime()) < 24*time.Hour
		}
	}
	return filepath.Join(m.Paths.Cache, "kits", hex.EncodeToString(h.Sum(nil))+".json"), fresh
}

// SecretClash is a service envs need a token for at different hosts. sbx
// keeps one token a service, so whichever is stored is sent to the hosts of
// every env that names the service: one team's token to another's forge.
type SecretClash struct {
	Service string
	// Hosts are each env's hosts for the service.
	Hosts map[string][]string
}

// SecretClashes are the services the envs' kits send to different hosts,
// by name; an env whose kits name no host for a service says nothing.
func SecretClashes(needed map[string][]NeededSecret) []SecretClash {
	hosts := map[string]map[string][]string{}
	for env, secrets := range needed {
		for _, s := range secrets {
			if len(s.Domains) == 0 {
				continue
			}
			if hosts[s.Service] == nil {
				hosts[s.Service] = map[string][]string{}
			}
			hosts[s.Service][env] = slices.Sorted(slices.Values(s.Domains))
		}
	}
	out := []SecretClash{}
	for _, service := range slices.Sorted(maps.Keys(hosts)) {
		seen := map[string]bool{}
		for _, h := range hosts[service] {
			seen[strings.Join(h, ",")] = true
		}
		if len(seen) > 1 {
			out = append(out, SecretClash{Service: service, Hosts: hosts[service]})
		}
	}
	return out
}

// ownText says how a token of an env's or a sandbox's own is kept.
func ownText(s EnvSecret) string {
	if s.Keychain != "" {
		return "keychain"
	}
	return "command"
}

// SecretScope is where a token of a service is kept: the global one sbx
// keeps for every sandbox, with neither said; an env's own, for each of
// its sandboxes; or one sandbox's own, over its env's.
type SecretScope struct{ Env, Sandbox string }

func (s SecretScope) String() string {
	switch {
	case s.Sandbox != "":
		return "sandbox " + s.Sandbox
	case s.Env != "":
		return "env " + s.Env
	}
	return "global"
}

// ownSecrets are the tokens a sandbox of env takes as its own: its env's,
// and its own over them.
func (c Config) ownSecrets(env Env, sandbox string) map[string]EnvSecret {
	out := maps.Clone(env.Config.Secrets)
	if out == nil {
		out = map[string]EnvSecret{}
	}
	maps.Copy(out, c.Sandboxes[sandbox].Secrets)
	return out
}

// OwnSecret is the token of a service a scope keeps, if it keeps one.
func (c Config) OwnSecret(s SecretScope, service string) (EnvSecret, bool) {
	var own map[string]EnvSecret
	switch {
	case s.Sandbox != "":
		own = c.Sandboxes[s.Sandbox].Secrets
	case s.Env != "":
		own = c.Envs[s.Env].Secrets
	}
	v, ok := own[service]
	return v, ok
}

// SetOwnSecret keeps in config.yaml the command a scope's token is read
// by, over one it had - taking out of the keychain an item unibox stored
// for it that the new one does not read - and has sbx keep it for the
// sandboxes it is for. env is the env whose sandboxes those are.
func (m *Manager) SetOwnSecret(ctx context.Context, s SecretScope, env, service string, v EnvSecret) error {
	if !ValidSecretName(service) {
		return fmt.Errorf("%q is no service name sbx takes; use letters, digits and dashes", service)
	}
	if strings.TrimSpace(v.Command) == "" {
		return fmt.Errorf("say the command that prints the %s token", service)
	}
	was, _ := m.Config.OwnSecret(s, service)
	if err := m.updateOwnSecrets(s, func(own map[string]EnvSecret) error { own[service] = v; return nil }); err != nil {
		return err
	}
	if was.Keychain != "" && was.Keychain != v.Keychain {
		if err := m.Paths.KeychainRemove(ctx, was.Keychain); err != nil {
			return err
		}
	}
	return m.SyncOwnSecrets(ctx, env, s.Sandbox)
}

// RemoveOwnSecret has a scope take its token of a service from the scope
// over it again: a sandbox its env's or the global one, an env the global
// one. What unibox stored in the keychain for it goes too.
func (m *Manager) RemoveOwnSecret(ctx context.Context, s SecretScope, env, service string) error {
	was, ok := m.Config.OwnSecret(s, service)
	if !ok {
		return fmt.Errorf("%s keeps no %s token of its own; unibox secret ls says where each comes from", s, service)
	}
	if err := m.updateOwnSecrets(s, func(own map[string]EnvSecret) error { delete(own, service); return nil }); err != nil {
		return err
	}
	if was.Keychain != "" {
		if err := m.Paths.KeychainRemove(ctx, was.Keychain); err != nil {
			return err
		}
	}
	return m.SyncOwnSecrets(ctx, env, s.Sandbox)
}

// forgetSecrets drops the tokens kept for a sandbox that is removed, and
// what unibox stored in the keychain for them.
func (m *Manager) forgetSecrets(ctx context.Context, sandbox string) error {
	own := m.Config.Sandboxes[sandbox].Secrets
	if len(own) == 0 {
		return nil
	}
	if err := m.updateOwnSecrets(SecretScope{Sandbox: sandbox}, func(o map[string]EnvSecret) error { clear(o); return nil }); err != nil {
		return err
	}
	for _, service := range slices.Sorted(maps.Keys(own)) {
		if own[service].Keychain != "" {
			if err := m.Paths.KeychainRemove(ctx, own[service].Keychain); err != nil {
				return err
			}
		}
	}
	return nil
}

// updateOwnSecrets changes a scope's tokens in config.yaml.
func (m *Manager) updateOwnSecrets(s SecretScope, change func(map[string]EnvSecret) error) error {
	c, err := m.Paths.LoadConfig()
	if err != nil {
		return err
	}
	var own map[string]EnvSecret
	switch {
	case s.Sandbox != "":
		own = c.Sandboxes[s.Sandbox].Secrets
	case s.Env != "":
		ec, ok := c.Envs[s.Env]
		if !ok {
			return Fail(4, "env_not_found", "there is no env %s; unibox env ls lists them", s.Env)
		}
		own = ec.Secrets
	default:
		return fmt.Errorf("a global token is sbx's; unibox secret set stores it")
	}
	own = maps.Clone(own)
	if own == nil {
		own = map[string]EnvSecret{}
	}
	if err := change(own); err != nil {
		return err
	}
	if len(own) == 0 {
		own = nil
	}
	if s.Sandbox != "" {
		c.Sandboxes = maps.Clone(c.Sandboxes)
		if c.Sandboxes == nil {
			c.Sandboxes = map[string]SandboxConfig{}
		}
		box := c.Sandboxes[s.Sandbox]
		box.Secrets = own
		if box.empty() {
			delete(c.Sandboxes, s.Sandbox)
		} else {
			c.Sandboxes[s.Sandbox] = box
		}
	} else {
		c.Envs = maps.Clone(c.Envs)
		ec := c.Envs[s.Env]
		ec.Secrets = own
		c.Envs[s.Env] = ec
	}
	if err := m.Paths.SaveConfig(c); err != nil {
		return err
	}
	m.Config = c
	return nil
}

// syncSecrets has sbx keep, as the sandbox's own, each token its env's
// config.yaml or its own names a command for, and forget those it no
// longer names. A
// recreate takes a sandbox's own tokens away, and a person may change a
// command by hand, so each is set again when sbx lacks it for the sandbox
// or the command is not the one set last.
func (m *Manager) syncSecrets(ctx context.Context, b *api.Sandbox, env Env) error {
	want := m.Config.ownSecrets(env, b.Name)
	if len(want) == 0 && len(b.Secrets) == 0 {
		return nil
	}
	have, err := m.SBX.Secrets(ctx)
	if err != nil {
		return err
	}
	kept := func(service string) bool {
		return slices.ContainsFunc(have, func(s Secret) bool { return s.Type != "custom" && s.Name == service && s.Scope == b.Name })
	}
	for _, service := range slices.Sorted(maps.Keys(want)) {
		command := want[service].Command
		if kept(service) && b.Secrets[service] == command {
			continue
		}
		if err := m.SBX.SecretSetSandbox(ctx, service, b.Name, command); err != nil {
			return err
		}
		if b.Secrets == nil {
			b.Secrets = map[string]string{}
		}
		b.Secrets[service] = command
	}
	for _, service := range slices.Sorted(maps.Keys(b.Secrets)) {
		if _, still := want[service]; still {
			continue
		}
		if kept(service) {
			if err := m.SBX.SecretRemoveSandbox(ctx, service, b.Name); err != nil {
				return err
			}
		}
		delete(b.Secrets, service)
	}
	return nil
}

// SyncOwnSecrets applies an env's own tokens and its sandboxes' to every
// sandbox unibox made from it that sbx still has, or to the one named, as
// config.yaml says them now.
func (m *Manager) SyncOwnSecrets(ctx context.Context, envName, sandbox string) error {
	env, err := m.Config.LoadEnv(envName)
	if err != nil {
		return err
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(state.Sandboxes)) {
		b := state.Sandboxes[name]
		if b.Env != envName || sandbox != "" && name != sandbox || b.State == "missing" || b.State == "creating" {
			continue
		}
		guard, err := m.guard(name)
		if err != nil {
			return err
		}
		err = m.syncSecrets(ctx, &b, env)
		if err == nil {
			err = m.saveBox(b)
		}
		guard.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// SecretUse is a token kept somewhere, or one a sandbox needs that is kept
// nowhere, with the sandboxes that take it.
type SecretUse struct {
	Service string `json:"service"`
	// Scope is where it is kept: global, "env <name>" or "sandbox <name>"
	// for one config.yaml keeps, "sbx sandbox <name>" for one sbx keeps
	// for a sandbox that unibox did not give it, "" for one kept nowhere.
	Scope string `json:"scope"`
	// Kept says how: stored by sbx, a variable of sbx's a subscription's
	// token is in, in the keychain or read by a command.
	Kept string `json:"kept"`
	// UsedBy are the sandboxes that take this one, those an env would make
	// marked "(not made)"; Needed whether any of them must have it.
	UsedBy []string `json:"used_by"`
	Needed bool     `json:"needed,omitempty"`
}

// SecretOverview is every token kept, global, an env's or a sandbox's,
// and every one needed and kept nowhere, with the sandboxes that take
// each: those unibox has, and the one each env without one would make.
// An env whose files cannot be read is named in skipped.
func (m *Manager) SecretOverview(ctx context.Context) (out []SecretUse, skipped []string, err error) {
	have, err := m.SBX.Secrets(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot list sbx's secrets: %w", err)
	}
	state, err := m.Paths.ReadState()
	if err != nil {
		return nil, nil, err
	}
	rows := map[string]*SecretUse{}
	order := []string{}
	row := func(scope, service, kept string) *SecretUse {
		key := scope + "\x00" + service + "\x00" + kept
		if r, ok := rows[key]; ok {
			return r
		}
		r := &SecretUse{Service: service, Scope: scope, Kept: kept, UsedBy: []string{}}
		rows[key] = r
		order = append(order, key)
		return r
	}
	ours := map[string]bool{}
	for _, b := range state.Sandboxes {
		for service := range b.Secrets {
			ours[b.Name+"\x00"+service] = true
		}
	}
	for _, s := range have {
		switch {
		case s.Type == "custom" && s.Scope == "global":
			row("global", s.Env, "variable")
		case s.Type == "custom":
			row("sbx sandbox "+s.Scope, s.Env, "variable")
		case s.Scope == "global":
			row("global", s.Name, "sbx")
		case !ours[s.Scope+"\x00"+s.Name]:
			// One unibox has sbx keep is the env's or sandbox's below.
			row("sbx sandbox "+s.Scope, s.Name, "sbx")
		}
	}
	for _, name := range sortedKeys(m.Config.Envs) {
		for _, service := range sortedKeys(m.Config.Envs[name].Secrets) {
			row("env "+name, service, ownText(m.Config.Envs[name].Secrets[service]))
		}
	}
	for _, name := range sortedKeys(m.Config.Sandboxes) {
		for _, service := range sortedKeys(m.Config.Sandboxes[name].Secrets) {
			row("sandbox "+name, service, ownText(m.Config.Sandboxes[name].Secrets[service]))
		}
	}
	// Which each sandbox takes is what it would be told it has.
	boxes := map[string][]string{}
	for _, name := range sortedKeys(state.Sandboxes) {
		b := state.Sandboxes[name]
		if b.Env != "" {
			boxes[b.Env] = append(boxes[b.Env], name)
		}
	}
	for _, envName := range sortedKeys(m.Config.Envs) {
		env, err := m.Config.LoadEnv(envName)
		if err != nil {
			skipped = append(skipped, envName)
			continue
		}
		names, made := boxes[envName], true
		if len(names) == 0 {
			names, made = []string{env.SandboxName()}, false
		}
		for _, sandbox := range names {
			needed, err := m.NeededSecrets(ctx, env, sandbox)
			if err != nil {
				skipped = append(skipped, envName)
				break
			}
			who := sandbox
			if !made {
				who += " (not made)"
			}
			for _, l := range needed {
				var r *SecretUse
				switch {
				case !l.Stored:
					r = row("", l.Service, "missing")
				case l.Variable != "":
					scope, _, _ := strings.Cut(l.Scope, ", ")
					if scope != "global" {
						scope = "sbx sandbox " + scope
					}
					r = row(scope, l.Variable, "variable")
				case l.Own != "":
					r = row(l.Scope, l.Service, l.Own)
				case l.Scope == "global":
					r = row("global", l.Service, "sbx")
				default:
					r = row("sbx sandbox "+l.Scope, l.Service, "sbx")
				}
				if !slices.Contains(r.UsedBy, who) {
					r.UsedBy = append(r.UsedBy, who)
				}
				r.Needed = r.Needed || !l.Optional
			}
		}
	}
	for _, key := range order {
		out = append(out, *rows[key])
	}
	// By service, then from the widest scope to the narrowest.
	rank := func(scope string) int {
		switch {
		case scope == "global":
			return 0
		case strings.HasPrefix(scope, "env "):
			return 1
		case scope == "":
			return 4
		case strings.HasPrefix(scope, "sbx "):
			return 3
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b SecretUse) int {
		return cmp.Or(cmp.Compare(a.Service, b.Service), cmp.Compare(rank(a.Scope), rank(b.Scope)), cmp.Compare(a.Scope, b.Scope))
	})
	return out, slices.Compact(skipped), nil
}
