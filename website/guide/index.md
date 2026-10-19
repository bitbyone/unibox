# Getting Started

unibox is a command-line wrapper over Docker Sandboxes (`sbx`) for running
coding agents -
claude, codex, copilot and opencode - on your machine during local
development. It gives both one profile format, kept in a git repository of
yours, and sets up what the runtime needs. The runtime makes the sandbox, a
micro-VM, and keeps your tokens out of it; the agent clones your
repositories inside, commits and pushes to a branch, and you pull.

An experimental adapter for microsandbox (`msb`) shows that the same
profile runs on another runtime; it is used only when asked for. Using the
runtime directly still works.

## Install unibox

### Homebrew

The tap lives in unibox's own repository. Homebrew builds the tagged release
on your machine from source; it needs Go only while it builds, and nothing
compiled is downloaded.

```sh
brew tap bitbyone/unibox https://github.com/bitbyone/unibox
brew trust bitbyone/unibox
brew install unibox
```

A recent Homebrew loads a third-party tap's formulae only once you trust
it; `brew trust --formula bitbyone/unibox/unibox` trusts this one formula
alone. `brew upgrade unibox` picks up a new release.

### From a checkout

```sh
git clone https://github.com/bitbyone/unibox
cd unibox
make install     # -> ~/.local/bin/unibox
```

`unibox --version` (or `unibox version`) prints the release it was built
from, for example `0.1`. A build that is not a release calls itself `dev`,
with its commit: `dev (<commit>)`. `unibox version --json` adds the profile
format it reads and the version of its JSON contract.

### Requirements

- **macOS** (verified on Apple Silicon) or **Linux**
- **Docker Sandboxes** (`sbx`), signed in with `sbx login`. The
  experimental msb runtime needs nothing more: unibox installs it into
  `~/.microsandbox` the first time it is used (Apple silicon Macs, Linux
  with KVM)
- **Docker**, for building the profiles' images
- **git**
- **Go 1.26** or newer, only to build unibox from a checkout

`unibox runtimes` shows which runtimes are installed and what each isolates.

## Your environments repository

Profiles live in a git repository of yours, the
[environments repository](./environments-repository). The first command lays
it out and makes it unibox's configured one:

```sh
unibox init --dir ~/my-envs
```

Then add a profile. `--agents` names the agents its image installs (copilot
when you name none); `--cpus` and `--memory` take a fixed amount or a share of
the host's:

```sh
unibox init web --agents claude,codex --cpus 4 --memory 50%
```

`unibox init -i` asks for all of it instead. You now have
`sandboxes/web/sandbox.yaml` and `sandboxes/web/sandbox.dockerfile`, ready to
edit. Commit them as you would any repository; unibox reads the working tree,
so what you see in your editor is what runs.

::: tip
`unibox validate` checks the whole repository - its layout, every profile and
every instruction text - and `unibox profiles` lists the profiles with their
agents and the state of their images.
:::

## Sign it in

An agent signs in through the runtime; unibox writes how it does into the
profile. A git forge is a **service** the sandbox pushes to with a token of
its own:

```sh
unibox agent login web             # sign its agents in
unibox service add web github      # a token for git on github.com
```

Tokens are asked for on a hidden prompt and kept in the platform's store - the
login keychain on macOS, the Secret Service on Linux. They never enter a
sandbox. [Agents](./agents) and [Services and Secrets](./services-and-secrets)
say more.

## Your first run

From inside a repository whose branch is on the forge:

```sh
cd ~/src/web
unibox run --profile web --sandbox web-1 --branch feat/login --agent claude
```

This builds the image when it needs to, creates sandbox `web-1` from profile
`web`, clones the repository at `feat/login` into `/work/<task>/web` inside it,
and runs claude in your terminal. When claude ends, the sandbox is stopped if
no other agent of unibox's is left in it; its disk, caches and tasks stay.

The agent pushes to `feat/login`. You pull as you would a colleague's commits.

```sh
unibox tasks                # every task, in every sandbox
unibox resume <task>        # continue it later
unibox shell web-1          # a shell inside the sandbox
unibox sandboxes            # every sandbox (also: unibox ls)
```

::: warning Protect your branches
The agent can push to any branch its token may push to. Protect `main` and
release branches on the forge, and give the sandbox a narrow token.
:::

## Before a start

`unibox check` takes the same arguments as `run` and lists anything in the
way: an image to build, logins and secrets, a branch the forge does not have,
and what changed in the profile. `run` fixes what it can.

```sh
unibox check --profile web --sandbox web-1 --branch feat/login --agent claude
```

Every command accepts `--json`, and with `--json` or `--yes` none asks a
question.

## Where next

- [What unibox Is](./why): what it does, what it leaves to the runtime, and the threat model
- [Concepts](./concepts): environments repository, profile, sandbox, task, runtime
- [Profiles](./profiles): what `sandbox.yaml` says
- [Tasks](./tasks): branches, several repositories, resuming
- [Command Line](/reference/cli): every command and flag
