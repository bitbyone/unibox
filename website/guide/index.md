# Getting Started

unibox is a thin extension of Docker Sandboxes (`sbx`) for running coding
agents - claude, codex, copilot or opencode - on your machine during local
development. sbx makes the sandbox, a micro-VM, and keeps your tokens out of
it; your team describes the sandbox in its own `*.sbxenv.yaml` files and
kits. unibox adds what sbx leaves to you: your own values for the args those
files ask for, the logins they need, tasks that put your repositories in front
of an agent, named ports, and a doctor that says what is missing or changed.

unibox is never a condition. The same files work with plain
`sbx env create`, for anyone who would rather not use it.

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

### Scoop, on Windows

This repository is a Scoop bucket as well; Scoop installs the release's
Windows archive, checked against its SHA-256.

```powershell
scoop bucket add unibox https://github.com/bitbyone/unibox
scoop install unibox
```

`scoop update unibox` picks up a new release.

### A release archive

Every release on [GitHub](https://github.com/bitbyone/unibox/releases)
has an archive for Linux, macOS and Windows, on amd64 and arm64, each with
one binary that needs nothing else, and `checksums.txt` with their SHA-256.
Put `unibox` (`unibox.exe`) anywhere on your `PATH`.

### From a checkout

```sh
git clone https://github.com/bitbyone/unibox
cd unibox
make install     # -> ~/.local/bin/unibox
```

`unibox --version` (or `unibox version`) prints the release it was built
from, for example `2.0`. A build that is not a release calls itself `dev`,
with its commit: `dev (<commit>)`. `unibox version --json` adds the version
of its JSON contract.

### Requirements

- **macOS** (verified on Apple Silicon), **Linux** or **Windows**
- **Docker Sandboxes** (`sbx`) **0.49.2** or newer, signed in (`unibox
  init` runs `sbx login` for you)
- **An sbxenv file**, usually your team's, in a repository of sbxenv files
  and kits
- **git**
- **Go 1.26** or newer, only to build unibox from a checkout

## Set it up

In your team's repository of sbxenv files, or anywhere in it:

```sh
cd ~/team/agent-sandboxes
unibox init
```

init finds every `*.sbxenv.yaml` in the repository and offers them to
pick, several at once; each becomes an env, named as the file names its
sandbox. It then walks you through five steps at the terminal. Each runs the
command it stands for, so what the walk does and what the commands do never
differ, and a step already done is only said. Running it again adds what is
new - a file the team added, an arg it now asks for - and skips the rest.

| Step | What it does |
| --- | --- |
| sbx | checks sbx is installed and signed in (running `sbx login` for you), and asks whether sandboxes may use your SSH agent |
| Envs | names each sbxenv file you picked as an env, as `unibox env add`, and which is the default when there are several |
| Values | asks for the args the files require and you have not set, as `unibox var set` |
| Logins | lists the tokens the envs' kits need and has sbx ask for the missing ones, as `unibox login set` |
| Sandboxes | creates the envs' sandboxes, as `unibox sandbox create`; sbx shows what it makes and asks you to approve it |

You need no checkout of the repository first: given its git address, init
clones it with your own git into
`~/.local/share/unibox/repos/<host>/<path>` and sets it up from there.

```sh
unibox init git@github.com:team/agent-sandboxes.git
unibox env update        # later: brings the clone up to date, and says what that changes
```

`unibox env update` updates only the clones unibox made; a checkout of your own
is yours to pull. `unibox doctor` says when such a clone is behind.

Without a terminal, or with `--no-input`, init asks nothing: it sets up
every file found, or those named with `--file`, takes your values from
`--var name=value`, and fails naming what is still missing and the flag
that gives it. Tokens are never given on the command line.

```sh
unibox init --no-input --file sandboxes/dev.sbxenv.yaml --var user=jdoe
```

The same by hand:

```sh
unibox env add dev ~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml --default
unibox var set -e dev user jdoe        # an arg the file asks of each person
unibox login set -e dev                 # sbx asks for each missing token
unibox sandbox create -e dev
unibox doctor                           # what is still missing, and what to run
```

[Envs](./envs) and [Logins](./logins) say more.

## Your first run

From inside a repository:

```sh
cd ~/src/web
unibox run
```

This creates the env's sandbox when it does not exist yet, mounts the
repository into it at `/work/<task>/web`, and runs the agent the env's kit
runs, in your terminal. The agent works on your files and your `.git` as they
are. When it ends, the sandbox is stopped if no other session of unibox's is
left in it; its disk and tasks stay.

To keep the agent out of your checkout, give it a clone from the forge
instead:

```sh
unibox run --clone --branch feat/login
```

It then pushes to `feat/login`, and you pull as you would a colleague's
commits. [Mount or Clone](./mount) weighs the two.

```sh
unibox task                    # pick a task and act on it by a key
unibox task ls                 # every task, in every sandbox
unibox resume <task>           # continue it later
unibox resume <task> --shell   # a shell in the task's directory
unibox sandbox ls              # every sandbox
```

Commands are a noun, then a verb, and take the env a sandbox is created from
as `-e` and the sandbox they work in as `-s`. To leave those out, set a
context once:

```sh
unibox context use -s dev -e dev
```

`UNIBOX_SANDBOX` and `UNIBOX_ENV` win over the context; with neither, a
sandbox or env that is the only one serves, and for an env, `default_env`
before that. [Tasks](./tasks#the-context) says more.

::: warning Protect your branches
An agent that can push can push to any branch its token may push to.
Protect `main` and release branches on the forge, and give the sandbox a
narrow token.
:::

## Before a start

`unibox task check` takes the same arguments as `run` and lists anything in
the way - an arg not set, a branch the forge does not have, local work a
clone would not see, the env changed since its sandbox was made - without
changing anything. `run` fixes what it can, and asks for a missing arg at
the terminal.

Every command accepts `--json`, and with `--json` or `--yes` none asks a
question.

## Coming from unibox 1

unibox 2 has no profiles, `sandbox.yaml`, environments repository, image
builds or templates: the team's sbxenv files and kits say all of that now,
network rules are sbx's policy (`unibox network`) and tokens sbx's
secrets (`unibox login`). The first
command of unibox 2 sets the old state aside as `state.v1.json` and names
the sandboxes of unibox 1, which it no longer manages; remove them with
`sbx rm` once their work is safe.

## Where next

- [What unibox Is](./why): what it adds to sbx, and the threat model
- [Concepts](./concepts): env, sandbox, task, login, agent
- [Envs](./envs): the sbxenv files and your values for them
- [Tasks](./tasks): branches, several repositories, resuming
- [Command Line](/reference/cli): every command and flag
