# Agents

unibox knows four agents: `claude`, `codex`, `copilot` and `opencode`. A
profile declares the ones its image installs; each is started with its
permission prompts off - the sandbox is the boundary - signs in through the
runtime, and is credited in the commits it makes.

## Declaring agents

```yaml
agents: [claude, codex]
```

or, with what the profile says of each:

```yaml
agents:
  claude:
    auth: {token: secret:claude}
    co_author: Claude <noreply@anthropic.com>
    config:
      .claude.json: _data/claude/.claude.json
      settings.json: _data/claude/settings.json
  codex:
    auth: oauth
    config:
      config.toml: _data/codex/config.toml
```

The image must install every declared agent; the [build checks
it](./images#the-agents-check). `unibox init <profile> --agents claude,codex`
writes both the declaration and the Dockerfile's `@install` line.

When a profile offers several agents, `run` needs `--agent`; `resume` can
run the task with any of the profile's agents.

## How each one runs

| Agent | Started as | Its instructions file |
| --- | --- | --- |
| `claude` | `claude --dangerously-skip-permissions`, with `DISABLE_AUTOUPDATER=1` | `~/.claude/CLAUDE.md` |
| `codex` | `codex --dangerously-bypass-approvals-and-sandbox` | `~/.codex/AGENTS.md` |
| `copilot` | `copilot --allow-all`, with `COPILOT_ALLOW_ALL=true` | `~/.copilot/copilot-instructions.md` |
| `opencode` | `opencode --auto` | `~/.config/opencode/AGENTS.md` |

Arguments after `--` on `run` or `resume` go to the agent:

```sh
unibox run -p web --agent claude -- --model opus
```

No agent stops to ask whether to trust what unibox prepared: before every
session codex gets `trust_level = "trusted"` for `/work`'s task directories in
its `config.toml`, claude `hasTrustDialogAccepted` for `/work`, the task and
its repositories in `~/.claude.json`, and copilot reads `COPILOT_ALLOW_ALL` as
trust for every folder.

No agent updates itself; the image fixes its version. A new version is a
rebuild.

## Signing in

```sh
unibox agent login -p web              # every agent of profile web
unibox agent login claude -p web       # one
unibox agent login --renew -p web      # ask for every token again
unibox agent ls -p web                 # how each signs in and is credited
```

`agent login` signs agents in and writes their `auth` into the profile:

- **codex** - through the runtime's ChatGPT login, in the browser, with sbx;
  with msb, which keeps no OAuth logins, through an OpenAI API key;
- **claude** - through `claude setup-token` on your terminal, then a hidden
  prompt for the long-lived token it prints;
- **copilot** - through a fine-grained GitHub token with the **Copilot
  Requests** permission alone: unibox opens the page where it is made, says
  what to grant, then asks for it on a hidden prompt.

A token is kept in the platform's store - the login keychain on macOS, the
Secret Service on Linux - where the profile names it as `secret:<name>`. It
never enters a sandbox: the runtime reads it on the host, and the sandbox
sees a placeholder the proxy swaps.

### `auth` values

| `auth` | Meaning |
| --- | --- |
| `oauth` | the provider's own login |
| `api-key` | an API key |
| `{command: <cmd>}` | a host command that prints the key, e.g. `gh auth token` |
| `{token: <cmd>}` | a host command that prints a long-lived login token, e.g. one from `claude setup-token`, or a GitHub token for copilot alone |

`<cmd>` is `secret:<name>` for a secret unibox keeps, or any host command for
a store unibox does not keep - `vault kv get -field=token ...`,
`op read ...`. unibox turns `secret:<name>` into the platform's own command,
so a profile is shared between macOS and Linux.

A missing `token` or `command` login is stored in the runtime on `run`
without asking - its source is in the profile you wrote. An OAuth login or a
key is asked for in your terminal by the runtime itself. Either way the value
never passes through unibox.

::: warning One OAuth login per sandbox
The runtime signs one agent of a sandbox in by OAuth, so two `oauth` agents in
one profile are refused. That is why claude beside codex signs in by `token`:
claude's subscription goes through a long-lived token from
`claude setup-token`, and codex keeps its ChatGPT `oauth`.
:::

::: info msb
msb runs no OAuth flow: `oauth` is refused there, so claude signs in by
`token` and codex by `api-key` or `command`. Nothing is stored in msb; each
login's command runs when msb boots the sandbox, and again before each
session, so a new token reaches a running sandbox.
:::

### Copilot through your gh login

```sh
unibox agent login copilot --gh -p web
```

writes `copilot: {token: gh auth token}`. At a terminal where gh is signed in,
unibox asks whether you want this. It is your whole gh login - every
repository and scope it has - but as a token login it reaches only Copilot's
hosts, not github.com, so git on github.com is still left to the profile's
[services](./services-and-secrets).

::: tip
Sign copilot in by `token`, not `command`. `command` stores sbx's built-in
`github` secret, and with it sbx signs every request to github.com itself -
over the git services of the profile. `unibox task check` warns when it is stored.
:::

## Co-authors

Commits carry your git identity and one co-author line for the agent of the
session that commits. A task resumed with another agent credits that one; a
commit you make in `unibox sandbox shell` credits none.

unibox alone says it. claude's and copilot's own bylines are off in their
seeds (`includeCoAuthoredBy: false`), and a `commit-msg` hook in each clone
takes out a trailer an agent adds of itself - copilot always does - before
adding the profile's, or none where `co_author: false`.

```sh
unibox agent co-author -p web                                    # how each is credited
unibox agent co-author claude --as "Claude <noreply@anthropic.com>" -p web
unibox agent co-author codex --as "Codex <>" -p web              # no address, linked to no account
unibox agent co-author codex --reset -p web                      # back to the default
```

The defaults:

| Agent | Credited as |
| --- | --- |
| `claude` | `Claude <noreply@anthropic.com>` |
| `codex` | `Codex <noreply@openai.com>` |
| `copilot` | `Copilot <Copilot@users.noreply.github.com>` |
| `opencode` | `opencode <noreply@opencode.ai>` |

`init` writes the default out into each agent's `co_author`, so the profile
shows it. It is read per session, so changing it recreates nothing.

## Configuration seeds

`unibox init` writes each agent's first answers into the profile's
`_data/<agent>/<file>` and names them under the agent's `config`, by the
file's own name. Before every session they are merged into the agent's own
files in the sandbox, JSON or TOML: the seed's keys win, the agent's own state
stays, and a list keeps what the agent added.

| Agent | Files | What the seed sets |
| --- | --- | --- |
| `claude` | `.claude.json`, `settings.json` | onboarding done, a theme, bypass-permissions accepted; no byline of its own |
| `codex` | `config.toml` | `approval_policy = "never"`, `sandbox_mode = "danger-full-access"` |
| `copilot` | `settings.json` (`config.json` is known, not seeded) | trusts `/work`, default theme, no banner, no updates, no byline |
| `opencode` | `opencode.json` | allowed to edit, run and fetch; no autoupdate |

Edit the seeds as you like. Only files unibox knows may be named, and the
merge leaves what unibox writes between its own markers alone - codex's
login routing and its trust of `/work`, for example. Seeds are not image
inputs: changing one rebuilds nothing.

So a codex started by hand in `unibox sandbox shell` is as free as one unibox runs.
