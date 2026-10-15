# unibox

A wrapper over sandbox runtimes for coding agents: one CLI and one
profile format, with an adapter per runtime. Today: Docker Sandboxes
(`sbx`). Using the runtime directly still works.

## How it works

```
 environments repo (git)            host                         sandbox (VM)
 ┌──────────────────────┐   ┌──────────────────────┐   ┌──────────────────────────┐
 │ sandboxes/<profile>/ │   │ unibox               │   │ /work/<task>/<repo>      │
 │   sandbox.yaml       │──▶│  profile → adapter   │──▶│   fresh clone, own branch│
 │   Dockerfile         │   │                      │   │                          │
 │ instructions/*.md    │   │ runtime (sbx)        │   │ claude / codex / copilot │
 │ _shared/             │   │  logins, proxy,      │◀──│   commits + pushes       │
 └──────────────────────┘   │  network policy      │   └──────────────────────────┘
                            └──────────────────────┘             │
                                      │ real tokens added        ▼
                                      └──────────────────────▶ forge, model APIs
```

- A **profile** describes an environment: image, resources, agents, how
  they sign in, what network they may reach.
- A **sandbox** is a VM created from a profile. You name it.
- A **task** is one piece of work in a sandbox: fresh clones of one or more
  repositories, on a branch, with an agent running in your terminal.

No host directory is mounted; work leaves as pushes to the task's branch.
Credentials stay on the host; the runtime's proxy adds them to requests.

## Install

Needs Go 1.26+, git, and `sbx` signed in (`sbx login`).

```sh
git clone https://github.com/bitbyone/unibox && cd unibox
make install            # → ~/.local/bin/unibox
```

## Use

```sh
unibox init --dir ~/my-envs                       # lay out your environments repo and use it
unibox init web --agents claude,codex             # add profile "web" (or: unibox init -i)
unibox agent login web                            # sign its agents in
unibox service add web github                     # sign its git in to a forge
unibox network allow web jitpack.io               # reach a host the policy refuses, applied live
unibox build web                                  # build its image now (run builds when needed)

# create sandbox "web-1" from profile "web", clone this repo, run claude
unibox run --profile web --new-sandbox web-1 --repo . --branch feat/login --agent claude

unibox tasks                                      # all tasks in all sandboxes
unibox resume <task>                              # continue a task later
unibox add-repo <task> --repo ../api --branch feat/login  # clone one more, even while it runs
unibox shell web-1                                # shell inside the sandbox
unibox recreate web-1                             # recreate now (a start does when the profile needs it)
```

`unibox check` (same arguments as `run`) lists anything missing before a
start: an image to build, logins and secrets, a branch the forge does not
have, and what changed in the profile. `run` fixes what it can. Every command accepts `--json`.

## Environments repository

`unibox init <profile>` on a profile that exists completes it to what init
writes today - its agents' seeds and `config` entries, how each is credited,
the layout - and changes nothing it already says; its agents
stay its own.

A git repository of yours holding the profiles. `unibox init` creates:

```
my-envs/
├── sandboxes/
│   └── web/
│       ├── sandbox.yaml       profile definition
│       ├── Dockerfile         the image
│       ├── instructions.md    extra instructions for the agents (optional)
│       └── _data/             files the profile refers to (optional)
├── instructions/              reusable instruction texts, attached by name
├── _shared/installations/     install scripts Dockerfiles compose from
└── README.md
```

## Profile

`sandboxes/<profile>/sandbox.yaml`. It does not mention the runtime.
`version` is the profile's format; a unibox reads one format (`unibox
--version` shows it) and refuses another, saying to make the profile again
with `unibox init` - there is no migration.

```yaml
version: 1
dockerfile: Dockerfile
cpus: 4
memory: 8g
agents:                              # or a list: [claude, codex]
  claude:
    auth: {token: secret:claude}     # unibox agent login
    co_author: Claude <noreply@anthropic.com>
    config:                          # seeds merged before every session
      .claude.json: _data/claude/claude.json
      settings.json: _data/claude/settings.json
  codex:
    auth: oauth                      # provider login kept by the runtime
    config:
      config.toml: _data/codex/config.toml
instructions: [unibox]               # from instructions/
allow_network: [jitpack.io]          # hosts to allow
deny_network: [gist.github.com]      # hosts to block
```

Unknown fields are errors. The profile is yours and nothing asks you to
approve it: when what it asks of the host changes (environment variables,
mounted caches, logins, services, network rules), the next start says what
changed and what it leads to - an image built, a sandbox recreated, or rules
applied to the running sandbox live.

## Agents

`claude`, `codex`, `copilot`, `opencode`, started with their permission
prompts off - the sandbox is the boundary. `unibox init` writes each one's
first answers into the profile's `_data/<agent>/<file>`, named under the
agent's `config` by the file's own name and merged into the agent's
configuration before every session, JSON or TOML: claude's onboarding and
theme done, copilot trusting `/work` with its default theme, no banner and
no updates (its settings.json), codex working unasked (`approval_policy =
"never"`, `sandbox_mode = "danger-full-access"` in its config.toml), opencode
allowed to edit, run and fetch. No agent updates itself; the image fixes its
version.
Edit the seeds as you like. Commits carry your git identity and one
co-author line for the agent of the session that commits - a task resumed
with another agent credits that one, a commit you make in `unibox shell`
none. unibox alone says it: claude's and copilot's own bylines are off in
their seeds (`includeCoAuthoredBy: false`), and the hook takes out the trailer an agent
adds of itself - copilot always does - before adding the profile's, or none
where `co_author: false`. `unibox agent co-author <profile> <agent>
"Name <email>"` says how an agent is credited (its `co_author`, which init
writes out with the default); `Name <>` credits it with no address, linked
to no account, and `--reset` takes it back to the default. `unibox agent ls
<profile>` shows each agent's sign-in and credit.

## Auth

`unibox agent login <profile> [agent...]` signs agents in and writes their `auth`
into the profile: codex through the runtime's ChatGPT login in the browser,
claude through `claude setup-token`, copilot through a fine-grained GitHub
token with Copilot Requests alone. A token is asked for on a hidden prompt
and kept in the platform's store - the login keychain on macOS, the Secret
Service on Linux - where the profile names it as `secret:<name>`. It never
enters a sandbox: the runtime reads it on the host and the sandbox sees a
placeholder its proxy swaps. `unibox secret set|ls|rm` keeps any other.

Copilot can sign in through your gh login instead (`--gh`, or the question
unibox asks at a terminal when gh is signed in): `copilot: {token: gh auth
token}`. It is your whole gh login, every repository and scope it has, but
as a token login it reaches only Copilot's hosts, not github.com.

A missing `token` or `command` login is stored in the runtime on `run`
without asking; an OAuth login or a key is asked for in your terminal.

| `auth` value | Meaning |
| --- | --- |
| `oauth` | the provider's own login |
| `api-key` | an API key |
| `{command: <cmd>}` | a host command that prints the key, e.g. `gh auth token` |
| `{token: <cmd>}` | a host command that prints a long-lived login token, e.g. one from `claude setup-token`, or a GitHub token for copilot alone |

`<cmd>` is `secret:<name>` for a secret unibox keeps, or any host command -
`vault kv get -field=token ...`, `op read ...` - for a store it does not.

## Services

`unibox service add <profile> <name>` signs the sandbox in to a git forge or
another host service: it shows where the token is made, asks for it, keeps it
in the platform's store and writes the `services` entry. It knows `github`,
`gitlab` and `artifactory` (from the name, or `--kind`), anything else is
`--kind generic` with `--host`, `--git` or `--env`. `--host github.com/<owner>`
gives one owner on a forge a token of its own; `--author` and `--no-co-author`
say who commits; `--command` takes a store unibox does not keep; `--gh` signs a `github`
service in through your gh login, which reaches every owner it can at once. `unibox
service ls <profile>` shows each and whether its token is stored, `unibox
service rm` takes one out.

The profile stays yours: what these commands write is ordinary YAML, and a
hand edit is as valid. Each write lays it out the same way: the sections it
has in a fixed order, each under a line on what it does and which command
changes it, then, commented out, only the sections it does not have yet -
recomputed every time, so nothing unibox writes lands under examples of
itself. Your own comment over a section stays.

## Network

`unibox network allow|deny <profile> <host...>` lets a profile's sandboxes
reach hosts the runtime's policy refuses, or refuses ones it allows; `rm`
takes hosts back to the policy and `ls` lists the rules with the hosts the
services need. The change is written into the profile and applied to its
running sandboxes at once - sbx changes rules live, so nothing is recreated.

Sign copilot in by `token`, not `command`: `command` stores sbx's built-in
`github` secret, and with it sbx signs every request to github.com in itself -
over the git `services` of the profile. `unibox check` warns when it is stored.

## Runtimes

| Runtime | Status | Isolation |
| --- | --- | --- |
| `sbx` (Docker Sandboxes) | supported | micro-VM, egress proxy, credentials kept on the host |
| Podman / Docker | planned | container |

Adding a runtime means implementing one Go interface (create, exec, stop,
build, and optionally store logins). Profiles do not change.
`unibox runtimes` shows what is installed.

sbx limits that show through: only one agent per sandbox can use `oauth`,
and changes to network rules or logins apply when a sandbox is recreated.

## More

- [docs/design.md](docs/design.md): design decisions and what was verified about sbx
- [AGENTS.md](AGENTS.md): notes for contributors
