# Command Line

```
unibox [command] [flags]
```

## Conventions

**Nouns, then verbs.** Commands come in groups named for what they work on -
`env`, `var`, `login`, `task`, `sandbox`, `port`, `herdr` - and the verb
follows: `unibox sandbox stop`, `unibox port publish`. Only the everyday
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
unibox init
```

Sets unibox up step by step, each shown as it goes: **sbx** (installed,
recent enough, signed in, the SSH agent decided), **Env** (an sbxenv file
added as an env), **Values** (the args it asks of you), **Logins** (the tokens
sbx lacks, asked for by sbx) and **Sandbox** (created, on your terminal). A
step already done is said and passed. See [Getting Started](/guide/).

### `unibox doctor`

```
unibox doctor [-e <env>]
```

Checks sbx, the envs, your args and logins, and the sandboxes, and says what
to fix: sbx's version (0.49.2 or newer) and `sbx diagnose`; the SSH agent
decision; that each env's files read and `sbx env plan` accepts them with
your args; every required arg and login; whether a sandbox still matches its
env, and is still in sbx; mounted directories that are gone. With `-e`, one
env alone. Exits `4` on a failure.

### `unibox env`

```
unibox env add <env> <file...> [--name <sandbox>] [--default]
unibox env ls
unibox env show [<env>]
unibox env rm <env>
```

An env names one or more sbxenv files - the team's, as sbx reads them -
merged by sbx in the order given; a later file, such as a local overlay,
overrides an earlier one. unibox never changes the files. Envs are kept in
`~/.config/unibox/config.yaml`.

`add` adds an env, or changes the files of one. `--name` names its sandbox
(default: the file's `name:`, or the env's); `--default` makes it the env new
sandboxes come from. `show` prints what the files declare - args, kits,
agent - and what you set. `rm` forgets an env; its files and sandbox stay.
See [Envs](/guide/envs).

### `unibox var`

```
unibox var ls [-e <env>]
unibox var set <name> <value> [-e <env>]
unibox var rm <name> [-e <env>]
```

Your own values for the args an env asks for - a user name, a mirror - kept
per env in `~/.config/unibox/vars.toml` and passed to `sbx env create` as an
args file. A sandbox is not created while a required one is missing. No
secret belongs here; store tokens with `unibox login`.

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
unibox run [directory...] [-- agent-args...] [flags]
```

Prepares a task and runs its agent in this terminal, or with `--shell` a
shell in the task's directory. The directories are the task's repositories;
the current directory when none is named. Arguments after `--` go to the
agent. With neither `-s` nor `-e`, the task goes into the default sandbox -
`UNIBOX_SANDBOX`, the context's, or the only one - and when there is none,
into the sandbox of the default env.

| Flag | Meaning |
| --- | --- |
| `-e`, `--env <name>` | the env to create the sandbox from, named as the env names it unless `-s` says |
| `-s`, `--sandbox <name>` | the sandbox to work in |
| `--shell` | open a shell in the task's directory instead of running an agent |
| `--task <name>` | the task's name, unique across sandboxes (default: its first repository and a digest) |
| `--mount` | mount the host's directories: the agent works on your files and `.git` as they are (the default for directories) |
| `--clone` | clone the repositories from the forge inside the sandbox instead |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone |
| `--repo <[name=]source>` | a repository path or https URL, optionally `name=source` (repeatable) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches; create one nobody has from the local HEAD, or a URL's default tip; never forces |
| `--ignore-local` | use the forge's version despite uncommitted or unpushed local changes |

### `unibox resume`

```
unibox resume <task> [-- agent-args...] [--shell]
```

Runs a prepared task's agent again, or with `--shell` a shell in its
directory, in the sandbox that holds it.

### `unibox task`

```
unibox task check [directory...] [-- agent-args...] [flags]
unibox task prepare [directory...] [-- agent-args...] [flags]
unibox task ls [-s <sandbox>]
unibox task add-repo <task> [directory...] [flags]
unibox task rename <task> [<name>] [--reset]
unibox task rm <task>
```

`check` explains what a task needs before it can start, each with a code
(see [Tasks](/guide/tasks#what-task-check-reports)), and changes nothing.
`prepare` makes the sandbox and the task ready - its directories mounted, or
cloned from the forge - and prints both. Both take the directories and flags
`run` does, except `--shell`, and choose the sandbox as it does.

`ls` lists prepared tasks, in every sandbox or with `-s` in one; it does not
take the sandbox from the context, so a forgotten context hides nothing.

`add-repo` adds repositories to a prepared task, also while its agent runs:
directories as `run` takes them, or `--repo` for a URL or `name=source`.
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
unibox sandbox create [<name>] [-e <env>] [--adopt]
unibox sandbox ls
unibox sandbox inspect [<sandbox>]
unibox sandbox shell [<sandbox>]
unibox sandbox stop [<sandbox>]
unibox sandbox recreate [<sandbox>]
unibox sandbox rm <sandbox> [--force]
```

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
unibox port set <name> --map <mapping> [--off] [-e <env>]
unibox port unset <name...> [-e <env>]
unibox port ls [-s <sandbox>]
unibox port publish [name...] [--map <mapping>...] [-s <sandbox>]
unibox port unpublish [name...] [--map <mapping>...] [-s <sandbox>]
```

A port is published on the host's loopback. `set` and `unset` change the
env's named ports, for its sandbox, and apply at once while it runs; `--off`
publishes one only when asked to, not at start. `publish` and `unpublish`
switch a running sandbox's ports alone - the env's by name, others by
`--map` - and its next start goes back to the env's. `ls` lists a sandbox's
ports and their URLs.

A mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`: `3000` takes
any free host port, `8080:3000` host port 8080. See [Ports](/guide/ports).

### `unibox prune`

```
unibox prune
```

Forgets the sandboxes sbx no longer has - removed with `sbx rm`, say.

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
team's and sbx's.

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
