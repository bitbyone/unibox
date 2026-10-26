# Command Line

```
unibox [command] [flags]
```

## Conventions

**Nouns, then verbs.** Commands come in groups named for what they work on -
`env`, `var`, `login`, `task`, `sandbox`, `port`, `herdr` - and the verb
follows: `unibox sandbox stop`, `unibox port on`. Only the everyday
commands stand alone: `run`, `resume`, `init`, `doctor`, `context`, `config`,
`prune`, `destroy` and `version`.

**The argument is what the command acts on.** One name - a task, a sandbox,
an env, a port - or a list of the same kind, such as directories. The env a
command works in is `-e`/`--env`, the sandbox `-s`/`--sandbox`, so the same
flag means the same thing everywhere.

**Where a command works when not told.** Left out, `-s` comes from
`UNIBOX_SANDBOX`, then from the [context](#unibox-context), then from there
being only one sandbox; `-e` from `UNIBOX_ENV`, the context, the
configuration's `default_env`, then from there being only one env. A command
that cannot tell says so and names the choices. Two exceptions keep a mistake
cheap: `unibox sandbox rm` always names its sandbox, and `unibox task ls`
lists every sandbox's tasks unless given `-s`.

**Global flags**, on every command:

| Flag | Meaning |
| --- | --- |
| `--json` | write versioned JSON, without prompting |
| `-y`, `--yes` | accept operation prompts; never discards work |
| `-h`, `--help` | help for the command |
| `-v`, `--version` | the release unibox was built from, as `unibox version` (top level only) |

Every list has `--json`; a long operation prints its progress as lines, for a
log. Called with `--json` or `--yes`, unibox never asks: what stands in the
way is an answer, not a prompt. Every JSON object carries `api_version`.

**Exit codes**:

| Code | Means |
| --- | --- |
| `0` | success |
| `1` | an error |
| `4` | refused or not found: an env or sandbox unibox does not have, unpushed work, a busy sandbox, a sandbox not unibox's, host access, a confirmation required, an arg missing or invalid, a failed `doctor` |
| the agent's | after a session, `run` and `resume` exit with the agent's status |

Under `--json` a failure is `{"api_version", "code", "message"}`. Errors are
lower case and say what to do: `sbx is not signed in - run sbx login`.

## Setting up

### `unibox init`

```
unibox init [<directory>|<file>|<git-address>] [--no-input] [--file <file>...] [--var <name>=<value>...]
```

Sets unibox up for a team's repository of sbxenv files: the current one
(the whole repository, from anywhere in it), the one given, or one cloned
from a git address into `~/.local/share/unibox/repos/<host>/<path>` with
your own git. It finds every `*.sbxenv.yaml` there and offers them to pick,
then goes step by step, each shown as it goes: **sbx** (installed, recent
enough, signed in, the SSH agent decided), **Envs** (each file picked
added as an env, and the default one), **Values** (the args they ask of
you), **Logins** (the tokens sbx lacks, asked for by sbx), **Ports** (the
team's ports switched on or off) and **Sandboxes** (created, on your
terminal). A step already done is said and passed; running it again adds
what is new.

| Flag | Meaning |
| --- | --- |
| `--no-input` | ask nothing, as without a terminal; fail naming what is missing and the flag that gives it |
| `--file <file>` | set up this sbxenv file rather than each found (repeatable) |
| `--var <name>=<value>` | a value of yours for each env asking for it (repeatable) |

See [Getting Started](/guide/#set-it-up).

### `unibox doctor`

```
unibox doctor [-e <env>]
```

Checks sbx, the envs, your args and logins, and the sandboxes, and says what
to fix: sbx's version (0.49.2 or newer) and `sbx diagnose`; the SSH agent
decision; that each env's files read and `sbx env plan` accepts them with
your args; every required arg and login; whether a sandbox still matches its
env, and is still in sbx; mounted directories that are gone, and mounts a
running sandbox has that no task does; a clone of unibox's that is behind
its remote. Each finding names the unibox command that fixes it. With `-e`,
one env alone. Exits `4` on a failure.

### `unibox env`

```
unibox env add [<env>] [<file>...] [--name <sandbox>] [--default]
unibox env ls
unibox env show [<env>]
unibox env update [<env>]
unibox env rm <env>
```

An env names one or more sbxenv files - the team's, as sbx reads them -
merged by sbx in the order given; a later file, such as a local overlay,
overrides an earlier one. unibox never changes the files. Envs are kept in
`~/.config/unibox/config.yaml`.

`add` adds an env, or changes the files of one; without a file it takes the
one sbxenv file in the current directory, and without a name the name that
file gives its sandbox. `--name` names its sandbox (default: the file's
`name:`, or the env's); `--default` makes it the env new sandboxes come
from. `show` prints what the files declare - args, kits, agent, the team's
ports - and what you set. `update` brings the repositories unibox init cloned
up to their remote, by a fast-forward alone, and says what that changes; a
checkout of your own is never pulled. `rm` forgets an env; its files and
sandbox stay.
See [Envs](/guide/envs).

### `unibox var`

```
unibox var ls [-e <env> | -g]
unibox var set <name> <value> [-e <env> | -g]
unibox var rm <name> [-e <env> | -g]
```

Your own values for the args an env asks for - a user name, a mirror - kept
in `~/.config/unibox/vars.toml`, with `-g` for every env that declares the
arg, or for one env, which wins; passed to `sbx env create` as an args
file. A sandbox is not created while a required one is missing. No secret
belongs here; store tokens with `unibox login`. The CPUs and memory your
sandboxes get are written in the same file by hand; see
[files](./files#vars-toml).

### `unibox login`

```
unibox login ls [-e <env>]
unibox login set [service...] [-e <env>] [--all] [--command <cmd>]
```

The tokens an env's sandboxes use, kept by sbx. `ls` lists the services the
env's kits and bindings take a token for, and whether sbx has each. `set` has
sbx ask for the missing ones on this terminal, or those named - the value
goes from the keyboard to sbx, never through unibox - or with `--command`
stores a command sbx runs on the host for one, a keychain read say. Away from
a terminal, one service's token may be piped in.

| Flag | Meaning |
| --- | --- |
| `--all` | ask for the optional tokens too |
| `--command <cmd>` | a command sbx runs on the host for the token, instead of asking for it; one service |

See [Logins](/guide/logins).

## Tasks

### `unibox run`

```
unibox run [directory|url] [-- agent-args...] [flags]
```

Prepares a task and runs its agent in this terminal, or with `--shell` a
shell in the task's directory. The argument is the task's repository, a
directory or a URL - the current directory when none is named; several are
named with `--repo` each, which does not mix with the argument. Arguments after `--` go to the
agent. With neither `-s` nor `-e`, the task goes into the default sandbox -
`UNIBOX_SANDBOX`, the context's, or the only one - and when there is none,
into the sandbox of the default env.

| Flag | Meaning |
| --- | --- |
| `-e`, `--env <name>` | the env to create the sandbox from, named as the env names it unless `-s` says |
| `-s`, `--sandbox <name>` | the sandbox to work in |
| `--shell` | open a shell in the task's directory instead of running an agent |
| `--agent <name>` | the agent to run - `claude`, `codex`, `copilot`, `opencode` - when the sandbox has it; the task remembers it (default: the task's last, else the env's) |
| `--task <name>` | the task's name, unique across sandboxes (default: its first repository and a digest) |
| `--mount` | mount the host's directories: the agent works on your files and `.git` as they are (the default for directories) |
| `--clone` | clone the repositories from the forge inside the sandbox instead |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone (default: a directory's current branch, a URL's default branch on the forge) |
| `--repo <[dirname=]source>` | a repository, directory or https URL, repeated for several; `dirname=` names its directory in `/work/<task>/` (default: the path's or URL's last part) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches; create one nobody has from the local HEAD, or a URL's default tip; never forces |
| `--ignore-local` | use the forge's version despite uncommitted or unpushed local changes |

### `unibox resume`

```
unibox resume [<task>] [-- agent-args...] [--shell | --agent <name>]
```

Runs a prepared task's agent again, or with `--shell` a shell in its
directory, in the sandbox that holds it. `--agent` runs another agent the
sandbox has, and the task keeps it. Without a task at a terminal it lists
the tasks, as `unibox task` does.

### `unibox task`

```
unibox task [-s <sandbox>]
unibox task check [directory|url] [-- agent-args...] [flags]
unibox task prepare [directory|url] [-- agent-args...] [flags]
unibox task ls [-s <sandbox>]
unibox task add-repo <task> [directory|url] [flags]
unibox task rename <task> [<name>] [--reset]
unibox task rm <task>
```

Alone at a terminal, `unibox task` lists the tasks to act on one by a
key; elsewhere it is `task ls`:

| Key | What it does |
| --- | --- |
| `↑` `↓`, `j` `k` | move |
| `enter`, `r` | resume the task with its agent |
| `a` | resume it with another agent the sandbox has, picked with `←` `→` or `h` `l` |
| `s` | a shell in the task's directory |
| `n` | name it, as `task rename` |
| `d` | remove it, as `task rm`, after `y` |
| `q`, `esc` | leave |

`check` explains what a task needs before it can start, each with a code
(see [Tasks](/guide/tasks#what-task-check-reports)), and changes nothing.
`prepare` makes the sandbox and the task ready - its directories mounted, or
cloned from the forge - and prints both. Both take the repository and flags
`run` does, except `--shell`, and choose the sandbox as it does.

`ls` lists prepared tasks, in every sandbox or with `-s` in one; it does not
take the sandbox from the context, so a forgotten context hides nothing.

`add-repo` adds repositories to a prepared task, also while its agent runs:
one directory or URL as `run` takes it, or several with `--repo`.
They are mounted or cloned as the task's first ones were. It takes
`--branch`, `--repo`, `--author`, `--push` and `--ignore-local` as `run`
does.

`rename` gives a task a name taken wherever its ID is (`resume`,
`task add-repo`, `task rm`). The task keeps its ID and its directory
`/work/<id>`, so a running session is not disturbed. A name is unique across
sandboxes, as an ID is; `--reset` takes it away.

`rm` removes a task from its sandbox, after checking for running sessions and
unpushed work. A mounted directory is unmounted first and never deleted.

## Sandboxes

### `unibox sandbox`

```
unibox sandbox
unibox sandbox create [<name>] [-e <env>] [--adopt]
unibox sandbox ls
unibox sandbox inspect [<sandbox>]
unibox sandbox shell [<sandbox>]
unibox sandbox stop [<sandbox>]
unibox sandbox recreate [<sandbox>]
unibox sandbox rm <sandbox> [--force]
```

Alone at a terminal, `unibox sandbox` lists the sandboxes to act on one
by a key; elsewhere it is `sandbox ls`:

| Key | What it does |
| --- | --- |
| `↑` `↓`, `j` `k` | move |
| `enter` | its tasks, as `unibox task -s` |
| `s` | a shell, as `sandbox shell` |
| `p` | its ports, as `unibox port -s` |
| `i` | what unibox keeps of it, as `sandbox inspect` |
| `t` | stop it |
| `c` | recreate it, after `y` |
| `d` | remove it, after `y`; running sessions and unpushed work still stop it |
| `q`, `esc` | leave |

`create` creates a sandbox from an env, named as the env names it unless a
name is given. `--adopt` takes over a sandbox sbx already has, made from the
env with sbx itself: unibox then adds tasks to it, never recreates it, and
removes it only when forced.

`ls` lists unibox's sandboxes and their state in sbx; `inspect` shows what
unibox keeps about one. `shell` opens an interactive shell, which counts as
an active session. `stop` stops an idle sandbox without deleting its disk.

`recreate` removes a sandbox and creates it again from its env's files and
your args as they are now, after checking for running sessions and unpushed
work. Its tasks come back: mounted directories are mounted again, clones are
made again from the forge. What the sandbox held outside its tasks is lost.

`rm` removes a sandbox after checking for running sessions and unpushed
work; `--force` removes it with them, or removes one unibox adopted.

The sandbox of `inspect`, `shell`, `stop` and `recreate` defaults as `-s`
does; `rm` always names it.

### `unibox port`

```
unibox port [-s <sandbox>]
unibox port on [name[=host_port]...] [--map <mapping>...] [-s <sandbox>]
unibox port off [name...] [--map <mapping>...] [-s <sandbox>]
unibox port ls [-s <sandbox>]
```

A port is published on the host's loopback. The ports are named in files,
by hand - the repository's `unibox.yaml`, the `.unibox.yaml` beside the
env's sbxenv file, and your `config.yaml` - each saying whether a sandbox
starts with it published; see [Ports](/guide/ports#which-wins) for the
order. `on` and `off` switch a running sandbox's ports for as long as it
runs - by name, `name=HOST_PORT` for a named port on another host port, or
`--map` for one no file names - and its next start goes back to the
files. They are refused on a stopped sandbox. `ls` lists a sandbox's
ports, where each comes from, and their URLs. Alone at a terminal,
`unibox port` lists a running sandbox's ports to switch where they
stand: `↑` `↓` or `j` `k` to move, `←` or `h` off, `→` or `l` on, `space`
to switch, `enter` or `q` to leave; elsewhere it is `port ls`.

A mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`: `3000` takes
any free host port, `8080:3000` host port 8080. See [Ports](/guide/ports).

### `unibox network`

```
unibox network ls [-s <sandbox>] [--global]
unibox network allow <host...> [-s <sandbox>] [--global]
unibox network deny <host...> [-s <sandbox>] [--global]
unibox network rm <host...> [-s <sandbox>] [--global]
unibox network log [-s <sandbox>] [--limit <n>]
```

The hosts a sandbox may reach, as sbx's network policy says. A rule is
for the sandbox unless `--global` makes it every sandbox's; `ls --global`
lists every rule. `rm` takes away a rule you added; the defaults' and the
kits' are not to be removed. `log` shows the latest requests and what the
policy decided. See [Ports](/guide/ports#what-a-sandbox-may-reach).

### `unibox prune`

```
unibox prune
```

Forgets the sandboxes sbx no longer has - removed with sbx itself, say -
and unmounts from running sandboxes the directories no task of theirs
mounts any more. A sandbox busy with another unibox command is passed and
named.

### `unibox destroy`

```
unibox destroy [--force] [--all]
```

Removes every sandbox unibox created, and so every task, after checking each
for running sessions and unpushed work: one that has either stops it, and
nothing is removed, unless `--force` discards them. A mounted directory of
the host is never deleted, and a sandbox unibox adopted is only forgotten.

`--all` goes on to the herdr plugin and unibox's state, configuration and
your args. The env files, the kits and your logins in sbx stay: they are the
team's and sbx's; so do the repositories init cloned, in
`~/.local/share/unibox/repos`.

## unibox itself

### `unibox context`

```
unibox context
unibox context use [-s <sandbox>] [-e <env>]
unibox context clear
```

Commands take the sandbox they work in as `-s` and the env a new one is
created from as `-e`. Left out, it comes from `UNIBOX_SANDBOX` or
`UNIBOX_ENV`, then from the context set here, then from there being only one
(for an env, the configuration's `default_env` before that). `context` alone
shows what commands would use, and why; `use` sets the sandbox, the env or
both, checking they exist; `clear` forgets the context.

### `unibox config`

```
unibox config get [<key>]
unibox config set <key> <value>
unibox config unset <key>
```

unibox's own configuration. `get` without a key prints all of it.

| Key | Meaning |
| --- | --- |
| `default_env` | the env a sandbox is created from when none is named |
| `stop_idle` | whether a sandbox is stopped when its last session ends (default `true`) |
| `ssh_agent` | whether sandboxes may use your SSH agent; sbx is set to match |
| `remember_host_commands` | whether approving an env's host commands covers their later runs too, rather than being asked each time; sbx's own setting, `env.rememberHostCommands` (default `false`) |

`unset` forgets a setting, back to its default. See
[Your SSH agent](/reference/security#your-ssh-agent) for `ssh_agent`.

### `unibox herdr`

```
unibox herdr status
unibox herdr install-plugin
unibox herdr uninstall-plugin
```

`status` shows whether herdr and the plugin are installed;
`install-plugin` installs the plugin that shows sandboxed agents' status in
herdr, asking first unless `--yes`; `uninstall-plugin` removes it and its
background observer. See [herdr](/guide/herdr).

### `unibox version`

```
unibox version [--json]
```

Prints the release unibox was built from - the `vX.Y` tag without its `v`,
set when it is built - as `unibox --version` does. A build that is not a
release prints `dev (<commit>)`, with `-dirty` when the tree had changes.
With `--json` it adds the version of its JSON contract:

```json
{
  "api_version": 2,
  "version": "2.0"
}
```

### `unibox completion`

```
unibox completion [bash|zsh|fish|powershell]
```

Generates the shell completion script.
