# unibox

**Run coding agents with full autonomy, without giving them your machine.**

unibox starts Claude Code, Codex, Copilot CLI or opencode inside a VM
sandbox, on a fresh clone of the repository you work on, on a branch of its
own. The agent may do anything in there. It cannot see your home directory,
your SSH keys or your tokens, and it reaches the network only through an
allow list. Its work comes back the normal way: as commits pushed to the
branch.

```sh
unibox run --profile personal --repo . --branch feat/login
```

## Why

An agent you can leave alone needs to run commands without asking.
Running them on your laptop means trusting it with everything your user
can reach. Containers made by hand leak in the other direction: a mounted
worktree carries `.git`, hooks and credentials helpers, and every project
gets its own one-off setup.

unibox makes the safe way the easy one:

- **One definition per kind of work.** A *profile* describes the image,
  resources, agents, logins and network rules once. It is versioned in a
  git repository of yours and runs on any machine.
- **Nothing of the host is mounted.** A task clones from the forge inside
  the sandbox. The only way out is a push to the task's branch.
- **Logins stay on the host.** The sandbox gets placeholders. The
  runtime's proxy puts the real token on the agent's requests.
- **Several tasks per sandbox.** Each task has its own clones, branches and
  sessions, and can be resumed by name.

## Install

Requirements: Go 1.26+, git, and a runtime: [Docker
Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`), signed in with
`sbx login`.

```sh
git clone https://github.com/bitbyone/unibox && cd unibox
make install                      # into ~/.local/bin (PREFIX=... to change)
```

## Quick start

```sh
unibox config set environments ~/env-repo          # where profiles live
unibox init personal --agents claude,codex,copilot # lay out a profile
unibox approve personal                            # what it may ask of the host
unibox build personal                              # build its image
unibox run --profile personal --repo . --branch feat/x   # first task
```

`run` checks everything before it starts (`unibox check` shows the same
list): approval, image, logins, the branch on the forge, and local changes
that would not reach the sandbox. Interactively it offers to fix what it
can.

| Command | What it does |
| --- | --- |
| `run` / `prepare` | create the sandbox if needed, clone the repos into a task, run the agent (or only prepare) |
| `resume <task>` | run a prepared task's agent again; the task finds its sandbox |
| `tasks [sandbox]` | tasks of every sandbox: agent, state, sessions, repositories |
| `ls` | sandboxes and their runtime state |
| `shell <sandbox>` | an interactive shell inside |
| `recreate <sandbox>` | rebuild and recreate after a profile change; refuses while work is unpushed |
| `stop`, `rm`, `rm-task` | stop, or remove after checking for running sessions and unpushed work |
| `check`, `validate`, `inspect`, `diff` | explain, verify, show what was generated, show what changed since the build |

Every command takes `--json` and answers in a versioned format, for other
programs to build on (unagit is one).

### Tasks

```sh
unibox run --profile personal --repo ~/src/api --repo ~/src/web --branch feat/x
unibox run --sandbox personal --repo https://github.com/me/lib --branch fix/y --agent codex
unibox run ... --push            # create or push the branch first, never force
unibox resume api-3f2a91c0 -- --model gpt-6   # arguments after -- go to the agent
```

A repository is a local path, whose origin and branch are used, or an
HTTPS URL. The default branch is refused: work happens on a branch. A task
lives in `/work/<task>/<repo>` in the sandbox. Task names are unique across
sandboxes.

## The environments repository

Profiles live in a git repository of your own, shared between machines.
Its layout is checked, so a typo is an error and not a silently ignored
file:

```
env-repo/
├── sandboxes/
│   └── personal/                 one directory per profile
│       ├── sandbox.yaml          the definition (below)
│       ├── Dockerfile            the image: everything static goes here
│       ├── instructions.md       the profile's own text for its agents (optional)
│       ├── start.sh              runs once per VM boot (optional)
│       ├── _data/                files the Dockerfile or sandbox.yaml refer to
│       └── .sbx/build.args       runtime-specific build arguments
├── instructions/                 attachable texts: <name>.md
│   └── unibox.md                 how to behave in a sandbox (laid out by init)
├── _shared/                      shared by every profile's image build
│   ├── .install                  runs the named installation scripts
│   └── installations/            base, tools/{nvm,sdkman,mise}, agents/{claude,codex,copilot,opencode}
└── README.md
```

Only `sandboxes/`, `instructions/`, `README.md` and directories starting
with `_` or `.` may be at the root. A Dockerfile composes its image from
the shared scripts:

```dockerfile
RUN --mount=type=bind,source=_shared,target=/unibox /unibox/.install <<EOF
base
tools/sdkman
agents/claude
agents/codex
EOF
```

`unibox init` lays this out and never overwrites what exists.
`unibox validate` checks the whole repository.

## Configuration: one file per profile

`sandboxes/<name>/sandbox.yaml` knows no runtime. It says what the
environment is; the runtime adapter decides how.

```yaml
dockerfile: Dockerfile            # or image: <ref> built elsewhere
cpus: 4
memory: 8g
agents: [claude, codex, copilot]  # what the image has
instructions: [unibox]            # texts from instructions/, plus instructions.md
start: start.sh                   # once per boot, inside the VM
bootstrap: .agent/bootstrap.sh    # per repository, if the repository has it
agent_config:                     # JSON merged into an agent's own settings
  claude: _data/claude.json
git:
  author: user                    # commits carry your identity
  co_author: true                 # plus a Co-Authored-By trailer for the agent
env: [SPRING_PROFILES_ACTIVE]     # names; values from your environment
caches:                           # host directories, mounted read-only
  - host: ~/.gradle/caches
    env: GRADLE_RO_DEP_CACHE
auth:                             # how each agent signs in (below)
  codex: oauth
  claude: {token: security find-generic-password -s claude-code-oauth-token -w}
  copilot: {command: gh auth token}
allow_network: [jitpack.io, "*.jetbrains.team"]   # beyond the runtime's policy
deny_network: [gist.github.com]                   # deny always wins
```

An unknown field is an error. Anything a profile asks of the host
(`env`, `caches`, `auth`, network rules) is shown and has to be approved
with `unibox approve`, and again whenever it changes. A profile asking
nothing of the host needs no approval.

unibox's own settings (`unibox config get`) hold only the environments
path, the default runtime and per-runtime switches. They live in
`~/.config/unibox`; caches and generated runtime files live in
`~/.cache/unibox`.

## Agents

| Agent | Instructions file | Started as | Notes |
| --- | --- | --- | --- |
| `claude` | `~/.claude/CLAUDE.md` | `claude --dangerously-skip-permissions` | |
| `codex` | `~/.codex/AGENTS.md` | `codex --dangerously-bypass-approvals-and-sandbox` | |
| `copilot` | `~/.copilot/copilot-instructions.md` | `copilot --allow-all` | |
| `opencode` | `~/.config/opencode/AGENTS.md` | `opencode` | no sbx login declaration yet |

Before each session unibox prepares the agent so it starts straight into
work. Prepared task directories are trusted, so no trust prompt appears.
The proxy's login is written into codex's own config. Commits made in the
sandbox carry your identity and a co-author trailer naming the agent.

## Auth

Credentials never enter the sandbox and never pass through unibox. A
profile names the method; the runtime keeps the secret on the host and
asks for it on your terminal when it is missing.

| Method | Profile | Meaning | With sbx |
| --- | --- | --- | --- |
| OAuth | `oauth` | the provider's own login | codex: `sbx secret set openai --oauth`, stored once. claude: sign in inside the sandbox |
| API key | `api-key` | a key the runtime stores | `sbx secret set <service>` |
| Command | `{command: gh auth token}` | a host command prints the key when needed | `sbx secret set github --command '…'` |
| Token | `{token: <host command>}` | a long-lived login token in an env variable | claude: `claude setup-token`, kept e.g. in the keychain |

For a Claude subscription next to Codex:

```sh
claude setup-token                                            # on the host
security add-generic-password -a "$USER" -s claude-code-oauth-token -w   # paste it
unibox recreate personal                                      # stores it in sbx when asked
```

## Runtimes

**sbx (Docker Sandboxes)**, the only runtime today: a micro-VM per
sandbox, egress through a host-side proxy that enforces the network policy
and puts credentials on requests. Logins are kept in the macOS keychain.

- One agent creates the sandbox from sbx's built-in kit for that agent.
  Several agents create it from a v3 workload kit that unibox generates
  (`unibox inspect` shows it). unibox also writes the credential bindings
  sbx requires for such a kit.
- sbx signs **one agent per sandbox in by OAuth**. A profile with two
  `oauth` agents is refused. Use `token`, `command` or `api-key` for the
  others.
- The default network policy allows the common registries (Maven Central,
  Gradle, npm, PyPI, GitHub and more) and refuses the rest with a 403.
  `allow_network` and `deny_network` adjust it per sandbox.
- After a profile change, `unibox recreate <sandbox>`: network rules,
  credential declarations and custom secrets reach a sandbox only when it
  is created.

**Future runtimes** implement the same adapter interface: create, exec,
stop, build, and optionally keep logins (`Authenticator`). Candidates are a
Podman or Docker container adapter for Linux hosts without sbx, and remote
VMs. A runtime without a credential proxy would have to put tokens into
the sandbox; the profile stays the same, and the adapter states what it
can isolate (`unibox runtimes`).

## Recommendations

- **Keep the environments repository in git.** It is the whole
  environment. A new machine needs only `unibox config set environments`.
- **One profile per kind of work** (personal, a client, a stack), not per
  project. Put project specifics in the repository's own
  `bootstrap` script and instructions.
- **Prefer `command` and `token` over pasted keys.** `gh auth token` and
  the keychain never go stale and are never copied.
- **Deny what would let data out** with the token the agent holds. Gists
  are allowed by default.
- **Push often, from the agent.** A task's clone exists only in the
  sandbox. `rm`, `rm-task` and `recreate` refuse while work is unpushed.
- **Never run the default branch.** unibox refuses it. Review the agent's
  branch like anyone's pull request.

## herdr

`unibox herdr install-plugin` adds a plugin to the herdr terminal
multiplexer. It reports each sandboxed agent's pane as working, idle or
blocked, titled with its sandbox, the same way herdr shows agents running
on the host.

## More

[docs/design.md](docs/design.md) records the design, the reasons behind
it, and everything verified about sbx along the way. [AGENTS.md](AGENTS.md)
is for whoever works on unibox.
