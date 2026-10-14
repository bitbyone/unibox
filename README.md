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
unibox config set environments ~/my-envs          # your environments repo
unibox init web --agents claude,codex             # create profile "web"
unibox approve web                                # accept what it asks of the host
unibox build web                                  # build its image

# create sandbox "web-1" from profile "web", clone this repo, run claude
unibox run --profile web --new-sandbox web-1 --repo . --branch feat/login --agent claude

unibox tasks                                      # all tasks in all sandboxes
unibox resume <task>                              # continue a task later
unibox add-repo <task> --repo ../api --branch feat/login  # clone one more, even while it runs
unibox shell web-1                                # shell inside the sandbox
unibox recreate web-1                             # apply profile changes
```

`unibox check` (same arguments as `run`) lists anything missing before a
start: approval, image, logins, a branch the forge does not have. `run`
offers to fix these. Every command accepts `--json`.

## Environments repository

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

```yaml
dockerfile: Dockerfile
cpus: 4
memory: 8g
agents: [claude, codex, copilot]
instructions: [unibox]               # from instructions/
auth:
  codex: oauth                       # provider login kept by the runtime
  copilot: {token: security find-generic-password -s unibox-copilot-token -w}
allow_network: [jitpack.io]          # hosts to allow
deny_network: [gist.github.com]      # hosts to block
```

Unknown fields are errors. Anything that touches the host (environment
variables, mounted caches, logins, network rules) needs `unibox approve`,
and again whenever it changes.

## Agents

`claude`, `codex`, `copilot`, `opencode`, started with their permission
prompts off. Commits carry your git identity and a co-author line for the
agent.

## Auth

The runtime stores secrets on the host; a missing one is asked for in your
terminal on `run`. unibox never sees the value.

| `auth` value | Meaning |
| --- | --- |
| `oauth` | the provider's own login |
| `api-key` | an API key |
| `{command: <cmd>}` | a host command that prints the key, e.g. `gh auth token` |
| `{token: <cmd>}` | a host command that prints a long-lived login token, e.g. one from `claude setup-token`, or a GitHub token for copilot alone |

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
