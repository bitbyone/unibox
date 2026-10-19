# unibox

A command-line wrapper over sandbox runtimes for running coding agents
during local development: one profile format and the setup around it, with
an adapter per runtime. The supported runtime is Docker Sandboxes (`sbx`);
microsandbox (`msb`) is experimental, there to show that a profile carries
over to another runtime unchanged. Others may follow. The isolation, the credential proxy and the network policy are
the runtime's; unibox configures them. Using the runtime directly still
works.

Documentation: **https://bitbyone.github.io/unibox/**

## How it works

```
 environments repo (git)            host                         sandbox (VM)
 ┌──────────────────────┐   ┌──────────────────────┐   ┌──────────────────────────┐
 │ sandboxes/<profile>/ │   │ unibox               │   │ /work/<task>/<repo>      │
 │   sandbox.yaml       │──▶│  profile → adapter   │──▶│   fresh clone, own branch│
 │   sandbox.dockerfile │   │                      │   │                          │
 │ _shared/  .shared/   │   │ runtime (sbx)        │   │ claude / codex / copilot │
 │   @install @instr.   │   │  logins, proxy,      │◀──│   commits + pushes       │
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

Needs git, Docker, and `sbx` signed in (`sbx login`). The experimental msb
runtime needs nothing more: unibox installs microsandbox into
`~/.microsandbox` the first time it is used (Apple silicon Macs and Linux
with KVM).

With Homebrew, from the tap in this repository; it builds the release on
your machine from its tagged commit:

```sh
brew tap bitbyone/unibox https://github.com/bitbyone/unibox
brew trust bitbyone/unibox     # Homebrew loads a third-party tap only once trusted
brew install unibox
```

Or from a checkout, with Go 1.26+:

```sh
git clone https://github.com/bitbyone/unibox && cd unibox
make install            # → ~/.local/bin/unibox
```

`unibox --version` (or `unibox version`) prints the release it was built
from, `dev (<commit>)` for a build that is not one; `unibox version --json`
adds the profile format and the API version it speaks.

## Use

```sh
unibox init --dir ~/my-envs                       # lay out your environments repo and use it
unibox init web --agents claude,codex             # add profile "web" (or: unibox init -i)
unibox agent login web                            # sign its agents in
unibox service add web github                     # sign its git in to a forge
unibox network allow web jitpack.io               # reach a host the policy refuses, applied live
unibox network port web fe 3000:3000              # publish a port on localhost at every start
unibox build web                                  # build its image now (run builds when needed)

# create sandbox "web-1" from profile "web", clone this repo, run claude
unibox run --profile web --sandbox web-1 --branch feat/login --agent claude

# or work on this directory in place: mounted into sandbox "dev", not cloned
unibox run --sandbox dev --agent copilot --mount

unibox sandboxes                                  # all sandboxes (also: ls)
unibox tasks                                      # all tasks in all sandboxes
unibox resume <task>                              # continue a task later
unibox resume <task> --shell                      # a shell in the task's repository instead
unibox add-repo <task> ../api --branch feat/login  # clone one more, even while it runs
unibox shell web-1                                # shell inside the sandbox
unibox recreate web-1                             # recreate now (a start does when the profile needs it)
```

`unibox check` (same arguments as `run`) lists anything missing before a
start: an image to build, logins and secrets, a branch the forge does not
have, and what changed in the profile. `run` fixes what it can. Every command accepts `--json`.

`--mount` (on `run`, `prepare` and `add-repo`) mounts the selected
directories into a running sandbox, at `/work/<task>/<name>`, instead of
cloning them: the agent works on your files and your `.git` as they are,
on whatever is checked out, and what it writes there - a git hook
included - is on your machine at once. `--branch` with it switches your
checkout to that branch first, making it from HEAD when you have none of
that name; it need not be on the forge. Use it knowingly; without it
nothing of yours is mounted. A git worktree cannot be mounted (its `.git`
points into the main repository by path); mount the repository's own
checkout. `rm-task` unmounts first and never deletes your files.

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
│       ├── sandbox.dockerfile the image (or image: in sandbox.yaml)
│       ├── instructions.md    extra instructions for the agents (optional)
│       └── _data/             files the profile refers to (optional)
├── _shared/                   yours
│   ├── @install/custom.sh     installation scripts, named in a Dockerfile
│   └── @instructions/custom.md  instruction texts, named in a profile
├── .shared/                   unibox's, kept at its own version
│   ├── @install/              base, agents/claude, tools/nvm, ...
│   └── @instructions/unibox.md
└── README.md
```

The `@` directories are what a profile names things from: a Dockerfile
runs installation scripts by name, and a profile's `instructions` names
texts (`unibox` is `@instructions/unibox.md`):

```dockerfile
COPY --from=unibox / /
RUN @install base tools/nvm agents/claude custom
```

`COPY --from=unibox` puts the `@install` command and the scripts into the
image - unibox hands them to the build as a build context of that name -
and `agents/claude` runs `@install/agents/claude.sh`. The names can also
come one per line in a heredoc (`RUN @install <<EOF`). Yours in `_shared` win over
unibox's in `.shared` of the same name, so overriding one of its scripts
is putting yours there. `.shared` is unibox's alone: `init`, `check`,
`build` and a start rewrite it to this unibox's version when it differs,
take out anything else in it, and say what they changed - an edit there is
lost. `init` writes `_shared` once, with an example of each kind; from
then on it is yours, as is everything a profile holds and anything else
in the repository - a text in a directory of your own is named by its
path from `sandbox.yaml` (`../../notes/style.md`).

## Profile

`sandboxes/<profile>/sandbox.yaml`. It does not mention the runtime.
`version` is the profile's format; a unibox reads one format (`unibox
version --json` names it) and refuses another. `unibox init <profile>` migrates an older
one, format by format, best effort: what it cannot carry over it names.

```yaml
version: 6
resources: {cpus: 4, memory: 50%}    # fixed, or a share of the host's
agents:                              # or a list: [claude, codex]
  claude:
    auth: {token: secret:claude}     # unibox agent login
    co_author: Claude <noreply@anthropic.com>
    config:                          # seeds merged before every session
      .claude.json: _data/claude/.claude.json
      settings.json: _data/claude/settings.json
  codex:
    auth: oauth                      # provider login kept by the runtime
    config:
      config.toml: _data/codex/config.toml
instructions: [unibox]               # _shared/@instructions, then .shared/@instructions
start: start.sh                      # in the VM, at every boot of it
scripts:                             # run only when asked for, by name
  update-skills: update-skills.sh
network:
  allow: [jitpack.io]                # hosts to allow
  deny: [gist.github.com]            # hosts to block
  ports:                             # published on the host's loopback, by name
    fe: '3000:3000'                  # at every start
    devtools: {port: '9333:9222', default: off}  # only when switched on
```

Unknown fields are errors. The profile is yours and nothing asks you to
approve it: when what it asks of the host changes (environment variables,
mounted caches, logins, services, network rules), the next start says what
changed and what it leads to - an image built, a sandbox recreated, or rules
applied to the running sandbox live.

## Your own values

A profile a team shares holds nothing personal. What each person sets for
themselves - a user name, a commit identity - it declares under `vars`, by a
name of its own with what it is for, and refers to it where it is needed:

```yaml
vars:
  artifactory-user: your Artifactory user name
env:                                     # every session's variables, and start's
  - BUILD_MODE                           # from the host
  - ORG_GRADLE_PROJECT_artifactoryUser2N: ${vars.artifactory-user}
  - ORG_GRADLE_PROJECT_artifactoryPassword2N: ${services.artifactory-2n}   # a service's placeholder
build_args:
  NPM_MIRROR: ${vars.npm-mirror}         # stays in the image's history: never a secret
```

Each person keeps the values in `~/.config/unibox/vars.yaml` (laid out by
`unibox init`); a run or a build stops while one it refers to is missing:

```sh
unibox var set artifactory-user tobola                                    # every profile
unibox var set gitlab-2n-author 'Jan Tobola <tobola@2n.com>' --profile dev  # one profile, over it
unibox var ls --profile dev
```

A service's `user` and `author` may refer to a var too. Tokens stay in the
platform's store as `secret:<name>`.

## Scripts

`start` runs in the VM at every boot of it. Anything else runs only when
asked for: a profile names its scripts under `scripts`, and `unibox run` or
`unibox resume` with `--script <name>` (repeatable) runs them before the
agent starts, in its directory, while `unibox script <sandbox> <name>...`
runs them now, in `/work`; `unibox script <sandbox>` lists them. What a
script does is yours - `npx -y skills update -g -y`, a database reset.
It runs as the agent's user with `UNIBOX_SANDBOX`, `UNIBOX_SCRIPT` and,
before a session, `UNIBOX_TASK` and `UNIBOX_TASK_DIR`; its `#!` line is
honoured, `sh` without one. The first to fail stops the rest and the
agent. A script is read when it runs, so changing it builds and recreates
nothing.

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
into the profile: codex through the runtime's ChatGPT login in the browser
(sbx) or an OpenAI API key (msb, which keeps no OAuth logins),
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
`--kind generic` with `--host`, and `--git` for a forge or `--env <NAME>` for
a tool, which writes `NAME: ${services.<name>}` into the profile's `env` so the
tool finds the placeholder there. `--host github.com/<owner>`
gives one owner on a forge a token of its own; `--author` and `--no-co-author`
say who commits; `--secret <name>` names a token stored before with `unibox
secret set <name>` instead of asking for one; `--command` takes a store unibox does not keep; `--gh` signs a `github`
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
running sandboxes at once - sbx changes rules live, so nothing is recreated;
an msb sandbox is recreated for it, after unibox asks.

`unibox network port <profile> <name> <spec> [--off]` publishes a sandbox
port on the host's loopback by name; the spec is
`[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, `3000` taking any free
host port. A port is published when a sandbox starts unless `--off`, and
running sandboxes get a change at once; `unibox network rm <profile>
<name>` takes it out. `unibox port <sandbox>` lists a sandbox's ports with
their URLs, and `unibox port <sandbox> fe:off devtools:on 8080:5173:on`
switches them while it runs, by name or by a spec the profile does not
name, without touching the profile: the next start goes back to the
profile's defaults. A port published with sbx's own command is left
alone.

Sign copilot in by `token`, not `command`: `command` stores sbx's built-in
`github` secret, and with it sbx signs every request to github.com in itself -
over the git `services` of the profile. `unibox check` warns when it is stored.

## Runtimes

| Runtime | Status | Isolation |
| --- | --- | --- |
| `sbx` (Docker Sandboxes) | supported, the default | micro-VM, egress proxy, credentials kept on the host |
| `msb` (microsandbox) | experimental, used only when asked for | micro-VM, egress proxy, credentials kept on the host |
| Podman / Docker | planned | container |

Adding a runtime means implementing one Go interface (create, exec, stop,
build, and optionally store logins). Profiles do not change.
`unibox runtimes` shows what is installed and which runtime is the default.
`unibox config set default_runtime <name>` sets the default for new
sandboxes, `--runtime <name>` asks for one sandbox, and a sandbox keeps the
runtime it was made in.

An image is tagged with its profile's revision (`unibox/dev:1508ba9b...`),
so unibox tells a current one from an outdated one. A build removes the
profile's older images, and making a sandbox the older images sbx made of
its kit; a sandbox does not need the image it was made from once it
exists. `unibox prune` removes the rest unibox no longer needs: the images
of profiles and sandboxes that are gone.

sbx limits that show through: only one agent per sandbox can use `oauth`,
and a login or service added after a sandbox was made reaches it when the
sandbox is recreated (sbx binds a token's placeholder at creation). Network
rules and ports change live.

msb is experimental: it shows that a profile runs unchanged on another
runtime, and is not meant for daily use yet. Its limits that show through: no `oauth` logins (claude by `token`, codex
by an API key), and network rules, ports and logins are set when a sandbox
is made, so a change of them recreates it; `--mount` is refused. A host
behind a VPN with a resolver of its own resolves when the VPN is connected
as the sandbox is made. Its tokens
are fetched each time a sandbox boots and again before each session, so a
rotated token reaches a running sandbox without a restart. A sandbox may
reach what sbx's default policy allows - AI services, package registries,
forges, container registries - and what the profile adds.

## More

- [docs/design.md](docs/design.md): design decisions and what was verified about sbx
- [AGENTS.md](AGENTS.md): notes for contributors
