# Unibox

A thin layer over Docker Sandboxes (`sbx`) for running coding agents on
your work. The team describes its sandboxes in its own repository, as
sbx's env files (`*.sbxenv.yaml`) and kits; unibox adds what each person
needs on top: their own values for the env's args, secrets kept by sbx,
tasks - your checkouts mounted, or fresh clones - with an agent in your
terminal, named ports, a doctor that says what to fix, and herdr's status
of the agents. The isolation, the credential proxy and the network policy
are sbx's. Using sbx directly still works.

Documentation: **https://bitbyone.github.io/unibox/**

## How it works

```
 team repo (git)                    host                          sandbox (VM)
 ┌──────────────────────────┐   ┌──────────────────────────┐   ┌──────────────────────┐
 │ sandboxes/dev.sbxenv.yaml│──▶│ unibox                   │──▶│ /work/<task>/<repo>  │
 │ kits/...                 │   │  env + your args         │   │   your checkout,     │
 └──────────────────────────┘   │  tasks, ports, doctor    │   │   mounted or cloned  │
                                │                          │   │                      │
 ~/.config/unibox/              │ sbx                      │   │ the kit's agent      │
   config.yaml  vars.toml  ────▶│  env create, secrets,    │◀──│   claude / codex /   │
                                │  proxy, network policy   │   │   copilot / opencode │
                                └──────────────────────────┘   └──────────────────────┘
```

- An **env** names a team's sbxenv files, merged in order, that a sandbox
  is created from. It is the team's; your values for its args are yours.
- A **sandbox** is the VM sbx creates from an env, named as the env names
  it.
- A **task** is one piece of work in a sandbox: one or more of your
  directories mounted at `/work/<task>/<repo>`, or cloned there from the
  forge, with the kit's agent running in your terminal.

Tokens stay in sbx's store on the host; the sandbox sees placeholders the
proxy replaces. Unibox never reads them.

## Install

Needs git and Docker Sandboxes 0.50.0 or later; `unibox init` signs you in
to sbx. Runs on Linux, macOS and Windows.

With Homebrew, from the tap in this repository; it installs the release's
binary for your system, so it needs no Go:

```sh
brew tap bitbyone/unibox https://github.com/bitbyone/unibox
brew trust bitbyone/unibox     # Homebrew loads a third-party tap only once trusted
brew install unibox
```

On Windows, with Scoop, from the bucket in this repository:

```powershell
scoop bucket add unibox https://github.com/bitbyone/unibox
scoop install unibox
```

Or download the archive for your system from the
[releases](https://github.com/bitbyone/unibox/releases): one binary, no
dependencies, with `checksums.txt` beside it. Or from a checkout, with Go
1.26+:

```sh
git clone https://github.com/bitbyone/unibox && cd unibox
make install            # → ~/.local/bin/unibox
make uninstall          # removes it again
```

`unibox --version` prints the release it was built from, `dev (<commit>)`
for a build that is not one; `unibox version --json` adds the API version.

## Use

The quickest start: in your team's repository of sbxenv files, `init`
finds every `*.sbxenv.yaml`, lets you pick, and walks you through it - sbx,
the envs, your args, secrets, the team's ports, the sandboxes - one step at
a time. Given a git address, it clones the repository for you first.

```sh
cd ~/team/agent-sandboxes && unibox init
unibox init git@github.com:team/agent-sandboxes.git   # cloned into ~/.local/share/unibox/repos
unibox env update                                       # later: update that clone, say what it changes
unibox doctor                                         # what a run needs, and what to fix
```

`init --no-input` asks nothing - for a script - and takes values from
`--var name=value`.

Each step also has its command:

```sh
unibox env add dev ~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml
unibox var set -e dev artifactoryUser tobola    # a value the env asks of you
unibox secret ls -e dev                          # the tokens it needs, and which sbx has
unibox secret set -e dev                         # have sbx store the missing ones
unibox sandbox create -e dev                    # or let run create it
unibox                                          # alone at a terminal: the sandbox list

cd ~/src/payments
unibox run --branch feat/refunds                # mount this checkout, run the agent
unibox run --clone --repo ../api --repo .       # fresh clones from the forge instead

unibox task                                     # pick a task: resume, another agent, shell, rename, remove
unibox task ls                                  # every task in every sandbox
unibox resume <task>                            # continue later
unibox resume <task> --shell                    # a shell beside its agent
unibox resume <task> --agent codex              # another agent the env has
unibox task add-repo <task> ../web              # one more, even while it runs
unibox task instructions <task>                 # what its agent is told at a session's start
unibox task rm <task>                           # unmounts; never deletes your files
unibox sandbox recreate dev                     # after the team changed the env
```

Commands are a noun, then a verb; the everyday ones - `run`, `resume`,
`init` - stand alone. The env a command works in is `-e`, the sandbox
`-s`; left out, they come from `UNIBOX_ENV` and `UNIBOX_SANDBOX`, then the
context (`unibox context use`), then there being only one. `unibox task
check` (same arguments as `run`) says what is missing before a start;
`run` asks for what it can. Every command accepts `--json`.

## Envs

An env is a list of sbxenv files, kept in `~/.config/unibox/config.yaml`;
`init` makes one of each file you pick:

```sh
unibox env add dev team/sandboxes/dev.sbxenv.yaml            # the team's
unibox env add dev team/sandboxes/dev.sbxenv.yaml mine.sbxenv.yaml --name dev-mine
unibox env show dev
```

sbx merges the files in order, so a file of your own after the team's
adds a kit or a mount without changing theirs. The sandbox is named as
the file names it, or `--name`. When the files, a kit or your args change
after a sandbox was made, `doctor` and the next `run` say so, and
`doctor -i` offers to recreate it;
`unibox sandbox recreate` applies it, keeping the tasks: mounts are made
again, clones cloned again. A sandbox made with sbx itself can be taken
over with `unibox sandbox create --adopt <name> -e <env>`.

## Your values

An env declares the args a person gives (`args:` in the sbxenv, required
or with a default). Your values are in `~/.config/unibox/vars.toml`,
per env, and go to sbx in a private file (`--env-args-file`) that is
removed straight after:

```sh
unibox var ls -e dev
unibox var set -e dev artifactoryUser tobola
```

`run` and `sandbox create` ask for a required one that is missing.

## Secrets

The env's kits and bindings name the tokens its sandboxes use.
`unibox secret ls` shows each, whether sbx has it and what it is for;
`unibox secret set` has sbx ask for the missing ones, or stores the one piped in. Before sbx
asks, it offers to open the page where a new token is made - GitHub's for
`github` and `copilot`, others where the team names them - and
`--generate` opens it straight away. `--command` gives sbx a host command that
prints the token - a keychain, a vault - which sbx runs itself. Secrets
are stored globally: sbx removes a sandbox's own with the sandbox.

## Tasks

`unibox task new` starts a task on repositories the env names - the
team's under `repos:` in its `unibox.yaml` files, yours in `config.yaml`
(`unibox repo add`), a forge's cloned and a directory mounted - picked on
one screen that shows each one's branch and whether it is pushed,
and the task's name; context repositories keep their `ref`. One that says `default: always` comes with every new
task of the env, cloned beside your mounted checkout in `run` too: the
architecture decisions and specifications an agent should always have.
A name from that index works wherever a repository does (`unibox run
adrs`); `unibox task new .` opens the same screen with your checkout
mounted among them.

By default `run` mounts the directories you name (the current one when
none) into the sandbox at `/work/<task>/<name>`: the agent works on your
files and your `.git` as they are, and what it writes there - a git hook
included - is on your machine at once. Unibox says so the first time a
directory is mounted. `--branch` switches your checkout first, making the
branch from HEAD when you have none of that name. A git worktree cannot
be mounted; mount the repository's own checkout.

`--clone` clones from the forge inside the sandbox instead, on a branch
of the task's, and nothing of yours is mounted; `--push` pushes local
branches first. Commits carry your git identity and a co-author line for
the session's agent.

The agent is the one the env's kit starts (claude, codex, copilot,
opencode); unibox runs it with `sbx exec` in the task's directory and
trusts that directory in the agent's settings before each session.
Arguments after `--` go to it.

## Ports

Ports are named in files, by hand, and published on the host's loopback
at every start unless they say `default: off`; yours go under `ports:` in
`config.yaml`, the mapping `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`.
`unibox port ls -s <sandbox>` lists them with their URLs; `port on` and
`port off` switch them while it runs, `port on vite=15173` on another host
port.

A team names the ports everyone uses beside its sbxenv file, in a file sbx
never reads - `dev.unibox.yaml` for `dev.sbxenv.yaml` - so nobody without
unibox is limited by it:

```yaml
ports:
  vite: {port: '5173:5173', description: the dev server, default: on}
  storybook: {port: '6006', description: Storybook}
```

Each person changes them for themselves in `config.yaml`, which wins over
the team's file.

## Network

```sh
unibox network init                      # once: the policy every sandbox starts from
unibox network ls -s dev                 # your rules, and the hosts refused lately
unibox network allow jitpack.io -s dev   # kept by unibox, for this sandbox
unibox network export -s dev             # what the env lacks, as a kit's network policy
```

The rules are sbx's network policy; a rule the team needs belongs in a
kit.

## Doctor

`unibox doctor` checks sbx (installed, its own diagnosis, a version unibox
supports, `env.rememberHostCommands`, the SSH agent), each env (its files,
your args, `sbx env plan`, the secrets, a change since its sandbox was
made, a clone of unibox's behind its remote) and the sandboxes (gone from
sbx, a mounted directory gone from the host, a mount a running sandbox
lacks or no task has), each with the unibox command that fixes it;
`unibox prune` clears what sbx no longer has and the mounts no task has.
You never need sbx's own CLI, though it keeps working beside unibox.

## The SSH agent

sbx forwards your SSH agent by default, which lets an agent sign with
every key you have; while it does, unibox refuses to run until you say
whether sandboxes may use it (`unibox config set ssh_agent false`, or
`true`), and sets sbx to match. `init` asks, recommending no.

## Editors over SSH

sbx can serve each sandbox to editors and desktop apps (VS Code, Cursor,
Orca, T3 Code) as `ssh <sandbox>.sbx`. `init` asks; `unibox config set
editor_ssh true` turns it on later: unibox sets sbx's `feature.ssh`,
restarts its daemon, which serves SSH only when started with it, and has
`sbx setup ssh` add the host entry to your SSH client's configuration.
Open a task's directory under `/work/<task>/<repo>`. An editor's session
is not unibox's, so `unibox config set stop_idle false` keeps a sandbox
running for it.

## Coming from unibox 1

Profiles, templates, image builds, msb and the commands over them are
gone: a team's sbxenv files and kits say all of that now. Unibox 2 sets
the old state aside as `state.v1.json` and names the old sandboxes once;
remove them with `sbx rm` when their work is safe.

## More

- [docs/design.md](docs/design.md): design decisions and what was verified about sbx
- [AGENTS.md](AGENTS.md): notes for contributors
