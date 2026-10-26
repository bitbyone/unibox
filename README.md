# unibox

A thin layer over Docker Sandboxes (`sbx`) for running coding agents on
your work. The team describes its sandboxes in its own repository, as
sbx's env files (`*.sbxenv.yaml`) and kits; unibox adds what each person
needs on top: their own values for the env's args, logins kept by sbx,
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
proxy replaces. unibox never reads them.

## Install

Needs git and Docker Sandboxes 0.49.2 or later; `unibox init` signs you in
to sbx. Runs on Linux, macOS and Windows.

With Homebrew, from the tap in this repository; it builds the release on
your machine from its tagged commit:

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
```

`unibox --version` prints the release it was built from, `dev (<commit>)`
for a build that is not one; `unibox version --json` adds the API version.

## Use

The quickest start: in your team's repository of sbxenv files, `init`
finds every `*.sbxenv.yaml`, lets you pick, and walks you through it - sbx,
the envs, your args, logins, the team's ports, the sandboxes - one step at
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
unibox login ls -e dev                          # the tokens it needs, and which sbx has
unibox login set -e dev                         # have sbx store the missing ones
unibox sandbox create -e dev                    # or let run create it

cd ~/src/payments
unibox run --branch feat/refunds                # mount this checkout, run the agent
unibox run --clone --repo ../api --repo .       # fresh clones from the forge instead

unibox task ls                                  # every task in every sandbox
unibox resume <task>                            # continue later
unibox resume <task> --shell                    # a shell beside its agent
unibox resume <task> --agent codex              # another agent the env has
unibox task add-repo <task> ../web              # one more, even while it runs
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
after a sandbox was made, `doctor` and the next `run` say so;
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

## Logins

The env's kits and bindings name the tokens its sandboxes use.
`unibox login ls` shows each, whether sbx has it and what it is for;
`unibox login set` has sbx ask for the missing ones, or stores the one piped in. `--command` gives sbx a host command that
prints the token - a keychain, a vault - which sbx runs itself. Secrets
are stored globally: sbx removes a sandbox's own with the sandbox.

## Tasks

By default `run` mounts the directories you name (the current one when
none) into the sandbox at `/work/<task>/<name>`: the agent works on your
files and your `.git` as they are, and what it writes there - a git hook
included - is on your machine at once. unibox says so the first time a
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

`unibox port set fe --map 3000:3000 -e dev` publishes a sandbox port on
the host's loopback by name, at every start unless `--off`; the mapping
is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. Ports are yours, kept
with the env in `config.yaml`. `unibox port ls -s <sandbox>` lists them
with their URLs, `port publish` and `port unpublish` switch them while it
runs.

A team names the ports everyone uses beside its sbxenv file, in a file sbx
never reads - `dev.unibox.yaml` for `dev.sbxenv.yaml` - so nobody without
unibox is limited by it:

```yaml
ports:
  vite: {port: '5173:5173', description: the dev server, default: on}
  storybook: {port: '6006', description: Storybook}
```

Each person switches them with `unibox port on|off <name>`; their choice
wins over the team's default.

## Network

```sh
unibox network ls -s dev                 # the hosts the sandbox may reach
unibox network allow jitpack.io -s dev   # --global for every sandbox
unibox network log -s dev                # what was asked, and what was decided
```

The rules are sbx's network policy; a rule the team needs belongs in a
kit.

## Doctor

`unibox doctor` checks sbx (installed, its own diagnosis, a version unibox
supports, `env.rememberHostCommands`, the SSH agent), each env (its files,
your args, `sbx env plan`, the logins, a change since its sandbox was
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

## Coming from unibox 1

Profiles, templates, image builds, msb and the commands over them are
gone: a team's sbxenv files and kits say all of that now. unibox 2 sets
the old state aside as `state.v1.json` and names the old sandboxes once;
remove them with `sbx rm` when their work is safe.

## More

- [docs/design.md](docs/design.md): design decisions and what was verified about sbx
- [AGENTS.md](AGENTS.md): notes for contributors
