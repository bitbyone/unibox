package unibox

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/tobola/unibox/internal/unibox/api"
)

// msb keeps no logins of its own and runs no OAuth flow: an agent signs in
// by a token or key that a host command prints when the sandbox boots - a
// secret unibox keeps in the platform's store, or the profile's own command.
var msbAuthMethods = map[string][]string{
	"claude":  {AuthToken, AuthAPIKey, AuthCommand},
	"codex":   {AuthAPIKey, AuthCommand},
	"copilot": {AuthToken, AuthAPIKey, AuthCommand},
}

// msbKeys is where an agent's key or token goes: the variable the agent
// reads, a placeholder of the shape it accepts, the hosts that get the
// value and the header it travels in. Claude's subscription token and
// copilot's are sbx's, so a sandbox looks the same in either runtime.
// Codex reads its key from its own auth.json, which codexSetup writes with
// the placeholder sbx uses, proxy-managed.
var msbKeys = map[string]map[string]msbSecret{
	"claude": {
		AuthToken:  {Env: sbxTokens["claude"].Env, Placeholder: sbxTokens["claude"].Placeholder, Hosts: sbxTokens["claude"].Hosts, Headers: []string{"Authorization"}},
		AuthAPIKey: {Env: "ANTHROPIC_API_KEY", Placeholder: "sk-ant-api03-unibox0claude0proxy0managed", Hosts: []string{"api.anthropic.com"}, Headers: []string{"x-api-key"}},
	},
	"codex": {
		AuthAPIKey: {Env: "OPENAI_API_KEY", Placeholder: "proxy-managed", Hosts: []string{"api.openai.com"}, Headers: []string{"Authorization"}},
	},
	"copilot": {
		AuthToken: {Env: sbxTokens["copilot"].Env, Placeholder: sbxTokens["copilot"].Placeholder, Hosts: sbxTokens["copilot"].Hosts, Headers: []string{"Authorization"}},
	},
}

// msbKeyName is the secret unibox keeps an agent's api-key login in.
func msbKeyName(agent string) string { return agent + "-api-key" }

// msbAgentSecret is the secret an agent signs in by. A key and a command
// both give the agent's API key; copilot's key is a GitHub token, so it
// goes where its token does.
func msbAgentSecret(agent string, a AgentAuth) (msbSecret, bool, error) {
	kinds, ok := msbKeys[agent]
	if !ok {
		return msbSecret{}, false, nil
	}
	if !slices.Contains(msbAuthMethods[agent], a.Method) {
		return msbSecret{}, false, fmt.Errorf("msb cannot sign %s in by %s; choose %s", agent, a.Method, strings.Join(msbAuthMethods[agent], ", "))
	}
	kind := a.Method
	if kind == AuthCommand {
		kind = AuthAPIKey
	}
	if _, ok := kinds[kind]; !ok {
		kind = AuthToken
	}
	sec := kinds[kind]
	sec.Hosts = slices.Clone(sec.Hosts)
	sec.Source = msbSource(sec.Env)
	command := a.Command
	if a.Method == AuthAPIKey {
		command = SecretPrefix + msbKeyName(agent)
	}
	var err error
	if sec.Command, err = ResolveCommand(command); err != nil {
		return msbSecret{}, false, err
	}
	return sec, true, nil
}

// SessionEnv tells codexSetup that codex signs in by a key the proxy puts
// in, so it writes the placeholder into codex's auth.json. The other
// agents read theirs from the sandbox's environment, where msb sets it.
func (r *MSB) SessionEnv(p Profile, agent string) []string {
	if agent != "codex" {
		return nil
	}
	if m := p.Auth[agent].Method; m == AuthAPIKey || m == AuthCommand {
		return []string{"SBX_CRED_OPENAI_MODE=apikey"}
	}
	return nil
}

func (r *MSB) AuthNeeds(ctx context.Context, sandbox string, p Profile) []api.Need {
	needs := []api.Need{}
	var store SecretStore
	for _, id := range p.Agents {
		a, ok := p.Auth[id]
		if !ok {
			continue
		}
		if !slices.Contains(msbAuthMethods[id], a.Method) {
			hint := "choose " + strings.Join(msbAuthMethods[id], ", ")
			switch {
			case id == "claude" && a.Method == AuthOAuth:
				hint = "sign claude in by token, which unibox agent login " + p.Name + " claude sets up"
			case id == "codex" && a.Method == AuthOAuth:
				hint = "sign codex in by an OpenAI API key: unibox agent login " + p.Name + " codex"
			}
			needs = append(needs, api.Need{Code: "auth_unsupported", Agent: id, Message: fmt.Sprintf("msb cannot sign %s in by %s, as it keeps no OAuth logins; %s", id, a.Method, hint)})
			continue
		}
		if a.Method != AuthAPIKey {
			// A command runs when the sandbox boots; a secret it names is
			// checked with the profile's other secrets.
			continue
		}
		if store == nil {
			var err error
			if store, err = PlatformSecrets(); err != nil {
				return append(needs, api.Need{Code: "secret_missing", Message: err.Error()})
			}
		}
		if ok, err := store.Has(ctx, msbKeyName(id)); err != nil {
			needs = append(needs, api.Need{Code: "secret_missing", Message: err.Error()})
		} else if !ok {
			needs = append(needs, api.Need{Code: "login_required", Agent: id, Message: fmt.Sprintf("%s signs in by api-key, but no key is stored; unibox agent login %s %s asks for it", id, p.Name, id)})
		}
	}
	return needs
}

// Login takes an agent's API key on the user's terminal into the platform's
// store; a token or a command has nothing to store, as its command runs
// each time the sandbox boots.
func (r *MSB) Login(ctx context.Context, agent string, a AgentAuth, in io.Reader, out, errout io.Writer) error {
	if a.Method != AuthAPIKey {
		return nil
	}
	if _, ok := msbKeys[agent]; !ok {
		return fmt.Errorf("msb has no login for agent %s", agent)
	}
	store, err := PlatformSecrets()
	if err != nil {
		return err
	}
	value, err := readHidden(in, errout, "  paste the "+agent+" API key: ")
	if err != nil {
		return err
	}
	if err = store.Set(ctx, msbKeyName(agent), value); err != nil {
		return err
	}
	fmt.Fprintf(out, "  stored secret %s in %s\n", msbKeyName(agent), store.Name())
	return nil
}

// readHidden takes a value on a hidden prompt at a terminal, else one line
// of input. It is never echoed or logged.
func readHidden(in io.Reader, out io.Writer, prompt string) ([]byte, error) {
	var value []byte
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(out, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return nil, err
		}
		value = b
	} else {
		line, err := bufio.NewReader(in).ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		value = line
	}
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return nil, fmt.Errorf("no value given; paste the key at the prompt, or pipe it on stdin")
	}
	return value, nil
}
