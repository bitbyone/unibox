# Command Line

## Conventions

Commands are a noun, then a verb (`unibox sandbox stop`). `run`, `resume`,
`init`, `doctor`, `context`, `config`, `prune`, `destroy` and `version`
stand alone.

`-s`/`--sandbox` is the sandbox a command works in; `-e`/`--env` the env a
new sandbox is created from. Left out, `-s` comes from `UNIBOX_SANDBOX`,
the [context](#unibox-context), or the only sandbox; `-e` from
`UNIBOX_ENV`, the context, `default_env`, or the only env.
`unibox sandbox rm` always names its sandbox, and `unibox task ls` lists
every sandbox unless given `-s`.

A noun alone at a terminal (`unibox task`, `sandbox`, `env`, `var`,
`secret`, `network`, `port`, `context`) opens a list to act on by keys;
elsewhere it is the noun's `ls`. `↑` `↓` or `j` `k` move, `q` or `esc`
leave, `d` (or `x`) removes after `y`.

### Global flags

| Flag | Meaning |
| --- | --- |
| `--json` | versioned JSON output, no prompts; every object has `api_version` |
| `-y`, `--yes` | accept prompts; never discards work |
| `-h`, `--help` | help |
| `-v`, `--version` | the release (top level only) |

### Exit codes

| Code | Means |
| --- | --- |
| `0` | success |
| `1` | error |
| `4` | refused or not found (missing env, sandbox, arg or token; unpushed work; busy sandbox; confirmation required; `doctor` failed) |
| the agent's | `run` and `resume` exit with the agent's status |

With `--json`, a failure is `{"api_version", "code", "message"}`.

## Setting up

### `unibox init`

```
unibox init [directory|file|git-address] [--no-input] [--file <file>...] [--var <name>=<value>...]
```

Sets up a team's env repository: the current one, the path given, or a git
address cloned into `~/.local/share/unibox/repos/<host>/<path>`. Steps:
sbx (installed, signed in, SSH agent), Envs, Values, Secrets, Sandboxes.
Running it again adds only what is new.

| Flag | Meaning |
| --- | --- |
| `--no-input` | ask nothing (also without a terminal); fail naming what is missing |
| `--file <file>` | set up this sbxenv file rather than each found (repeatable) |
| `--var <name>=<value>` | a value for each env asking for it (repeatable) |

### `unibox doctor`

```
unibox doctor [-e <env>] [-i]
```

Checks sbx (installed, `sbx diagnose`, version 0.49.2 or newer, host
commands setting, SSH agent), `vars.toml`, each env (files, args,
`sbx env plan`, secrets, drift, cloned repository behind), and each
sandbox (still in sbx, mounts). Names the fix for each finding. `-e`
checks one env. `-i` then offers, problem by problem, to run the unibox
command that fixes it - `sandbox recreate` for a changed env, `secret
set`, `network init`, `env update`, `prune` - at a terminal. Exits `4` on
a failure, without `-i`.

### `unibox env`

```
unibox env
unibox env add [env] [file...] [--name <sandbox>] [--default]
unibox env ls
unibox env show [env]
unibox env update [env]
unibox env rm <env>
```

An env names one or more sbxenv files, merged by sbx in order. Kept in
`config.yaml`.

- `add` - add an env or change its files. Without a file, the one sbxenv
  file in this directory; without a name, the file's `name:`.
  `--name` sets the sandbox's name; `--default` makes it `default_env`.
- `ls` - list envs.
- `show` - what the files declare (args, kits, agent, ports) and what you set.
- `update` - fast-forward the env repositories and say what changes. A
  checkout of your own only when clean and tracking a remote branch.
- `rm` - forget an env; files and sandbox stay.

List keys: `enter` show, `space` make default, `u` update, `c` create a
sandbox, `d` forget.

### `unibox var`

```
unibox var [-e <env> | -g]
unibox var ls [-e <env> | -g]
unibox var set <name> <value> [-e <env> | -g]
unibox var rm <name> [-e <env> | -g]
```

Your values for an env's args, in `vars.toml`. `-g`/`--global` is for
every env declaring the arg; an env's own value wins. A value starting with
`secret:` is refused. List keys: `enter` set, `a` add, `d` remove.

### `unibox secret`

```
unibox secret [-e <env>]
unibox secret ls [-e <env>]
unibox secret set [service...] [-e <env>] [--all] [--command <cmd>]
```

The tokens an env needs, kept by sbx. `ls` lists them and whether sbx has
each. `set` has sbx ask for the missing ones (or those named) on this
terminal; without a terminal, one service's token may be piped in.

| Flag | Meaning |
| --- | --- |
| `--all` | ask for optional tokens too |
| `--command <cmd>` | store a command sbx runs on the host for the token; one service |

List keys: `enter` store, `c` store a command.

## Tasks

### `unibox run`

```
unibox run [directory|url] [-- agent-args...] [flags]
```

Prepares a task and runs its agent, or a shell, in this terminal. The
argument is the repository (default: the current directory); several are
named with `--repo`, which does not mix with the argument. Arguments after
`--` go to the agent.

| Flag | Meaning |
| --- | --- |
| `-e`, `--env <env>` | env to create the sandbox from |
| `-s`, `--sandbox <name>` | sandbox to work in |
| `--shell` | a shell in the task's directory instead of the agent |
| `--agent <name>` | `claude`, `codex`, `copilot` or `opencode`, when the sandbox has it (default: the task's last, else the env's) |
| `--task <name>` | the task's name, unique across sandboxes (default: first repository and a digest) |
| `--mount` | mount the host's directories (default for directories) |
| `--clone` | clone from the forge inside the sandbox |
| `--repo <[dirname=]source>` | a directory, HTTPS URL or a name of the env's index; repeat for several |
| `--defaults` | take also the index's repositories that say `default: on`, as `task new` picks them (those that say `always` come anyway) |
| `--only` | take only the repositories named, not even those that say `always` |
| `--branch <name>` | before all `--repo`: shared branch; after one: that repository's (default: current branch, or the forge's default) |
| `--author "Name <email>"` | commit author |
| `--push` | push the selected local branches first; create a missing one from local HEAD (or a URL's default tip); never forces |
| `--ignore-local` | use the forge's version despite local changes |

### `unibox resume`

```
unibox resume [task] [-- agent-args...] [--agent <name>] [--shell]
```

Runs a prepared task's agent again, or with `--shell` a shell, in its
sandbox. `--agent` picks another agent the sandbox has; the task keeps it.
Without a task at a terminal, lists tasks to pick.

### `unibox task`

```
unibox task [-s <sandbox>]
unibox task new [repo...] [-e <env>] [-s <sandbox>] [--branch <b>] [--no-push] [--task <name>] [--agent <a> | --shell]
unibox task ls [-s <sandbox>]
unibox task check [directory|url] [-- agent-args...] [flags]
unibox task prepare [directory|url] [-- agent-args...] [flags]
unibox task add-repo <task> [directory|url] [flags]
unibox task rename <task> [name] [--reset]
unibox task instructions <task> [--agent <a>] [--paths]
unibox task rm <task>
```

- `new` - a task on repositories of the env's index (`unibox repo`), a
  forge's cloned and a directory mounted. At a terminal one screen: `space`
  pick, `c` work/context, `b` the branch of those worked in, `t` the task's
  name, `p` push a row's branch, `/` filter, `enter` start; each row shows
  the branch it gets (`default` for the forge's default) and whether it is
  pushed, and the task's name stands at the top; nothing is asked of a
  forge while picking. As it starts, a branch that exists already in a
  repository worked in is asked about (`--yes` takes it), once. Named, those that say `always`
  come along; `--branch`, `--task` and `--no-push` say the rest. A context
  repository keeps its ref and is never pushed to. Then it runs as `run`
  does.
- `ls` - prepared tasks, in every sandbox or with `-s` in one.
- `instructions` - the files the task's agent (or `--agent`'s) reads its
  instructions from when a session starts, in the order it reads them,
  each once: its home's, with the kits' guidance sbx writes there, the
  session directory's and its repository's. The task's `AGENTS.md` is
  shown as the next session gets it. Read in the running sandbox; a
  stopped one shows only what unibox writes. `--paths` lists the files.
- `check` - what a task needs before it can start; changes nothing. Takes
  `run`'s flags except `--shell` and `--agent`. Codes:
  `runtime_unavailable`, `arg_missing`, `arg_invalid`, `env_changed`,
  `branch_not_pushed`, `branch_missing`, `uncommitted`, `local_commits`,
  `remote_check_pending`, `branch_switch`.
- `prepare` - set the task up (mount or clone) and print it; takes the same
  flags as `check`.
- `add-repo` - add repositories, even while the agent runs, mounted or
  cloned like the first. Flags: `--repo`, `--branch`, `--author`, `--push`,
  `--ignore-local`.
- `rename` - an alias usable wherever the ID is; the ID and `/work/<id>`
  stay. `--reset` removes the alias.
- `rm` - remove a task after checking for running sessions and unpushed
  work. Mounted directories are unmounted, never deleted.

List keys: `enter` resume, `t` new task (`task new` in the sandbox of the
one under the cursor), `a` attach another agent (`←` `→` or `h` `l`),
`r` add repositories of the env's index (picked as in `task new`, seen at
once by a running agent), `s` shell, `n` rename, `d` remove.

## Sandboxes

### `unibox sandbox`

```
unibox                      # alone at a terminal: the same list
unibox sandbox
unibox sandbox ls
unibox sandbox create [name] [-e <env>] [--cpus <n>] [--memory <size>] [--adopt]
unibox sandbox inspect [sandbox]
unibox sandbox shell [sandbox]
unibox sandbox stop [sandbox]
unibox sandbox recreate [sandbox]
unibox sandbox resources [sandbox] [--cpus <n>] [--memory <size>]
unibox sandbox rm <sandbox> [--force]
```

- `ls` - unibox's sandboxes and their state in sbx.
- `create` - a sandbox from an env, without a task. `--cpus` (count, `0`
  for all, or a share like `50%`) and `--memory` (size like `16g`, or a
  share) set its own resources. `--adopt` takes over a sandbox made with
  sbx; unibox never recreates it and removes it only with `--force`.
- `inspect` - what unibox records about it.
- `shell` - an interactive shell; counts as a session.
- `stop` - stop an idle sandbox; the disk stays.
- `recreate` - remove and create again from the env and your values as
  they are now, after checking for sessions and unpushed work. Tasks come
  back; anything else in the sandbox is lost.
- `resources` - show or set its CPUs and memory, over the env's; `-`
  restores the env's. Kept in `vars.toml`, applied at the next recreate.
- `rm` - remove after checking for sessions and unpushed work; `--force`
  discards them, and removes an adopted sandbox.

List keys: `enter` tasks, `s` shell, `p` ports, `i` inspect, `r`
resources (then offers a recreate), `t` stop, `c` recreate, `d` remove.

### `unibox port`

```
unibox port [-s <sandbox>]
unibox port ls [-s <sandbox>]
unibox port on [name[=host_port]...] [--map <mapping>...] [-s <sandbox>]
unibox port off [name[=host_port]...] [--map <mapping>...] [-s <sandbox>]
unibox port default <name> on|off|- [-e <env>]
```

Publishes a running sandbox's ports on the host's loopback until it stops;
the next start goes back to the named ports in the files (see
[Ports](/guide/ports#named-ports)). `name=HOST_PORT` uses another host
port; `--map` (repeatable) a mapping no file names, as
`[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. `ls` lists ports, where
each is named, and URLs. List keys: `←`/`h` off, `→`/`l` on, `space`
switch, `m` on at a host port typed there (as `name=HOST_PORT`), `d` on
or off at each start, for you (as `port default`), `enter` or `q` leave.
`default` writes whether a named port is on at start into your
`config.yaml` under the env; `-` takes it back to the team's.

### `unibox network`

```
unibox network [-s <sandbox>]
unibox network init [balanced|allow-all|deny-all]
unibox network ls [-s <sandbox>] [--all]
unibox network allow <host...> [-s <sandbox>] [--protocol tcp|udp|tcp,udp]
unibox network deny <host...> [-s <sandbox>] [--protocol tcp|udp|tcp,udp]
unibox network rm <host...> [-s <sandbox>]
unibox network export [-s <sandbox>]
```

`init` chooses, once, the policy every sandbox starts from, which sbx
creates no sandbox without; at a terminal it asks. The rest are a
sandbox's own network rules, kept in `config.yaml`, applied again after
each create, removed with the sandbox. `ls` lists your rules, rules added
with sbx, and hosts refused lately; `--all` adds the env's and sbx's
defaults. `deny` wins over the env. `rm` removes your rule or an sbx one
for the sandbox. A host is `name`, `*.name`, `**.name` or `name:port`;
`--protocol` defaults to `tcp`. `export` writes the rules the env's kits
lack, and the hosts refused lately commented out, as a kit's
`network-policy@2` entry; `--json` gives them as `allow`, `deny` and
`refused`. List keys: `a` allow, `n` deny, `d` remove, `x` export.

### `unibox repo`

```
unibox repo [-e <env>]
unibox repo ls [-e <env>]
unibox repo add <name> <url|directory> [--ref <branch>] [--default always|on|off] [--context true|false] [--description <text>] [-e <env> | --every-env]
unibox repo default <name> always|on|off|- [-e <env>]
unibox repo rm <name> [-e <env> | --every-env]
```

The repositories an env's tasks are given by name: the team's under
`repos:` in its `unibox.yaml` files, yours in `config.yaml` for every env
or one, over the team's. A forge address is cloned, a directory of this
machine mounted. `always` has every new task take one, cloned beside what
it mounts; `on` has `task new` pick it to begin with. `context` - unsaid,
true for `always` - is one a task reads: it keeps its ref and is never
pushed to. A name
works wherever `run`, `prepare`, `check` and `add-repo` take a repository.
List keys: `/` filter, `n` new task with it, `space` off/on/always, `a`
add, `d` remove yours.

### `unibox prune`

```
unibox prune
```

Forgets sandboxes sbx no longer has and unmounts directories no task uses.

### `unibox destroy`

```
unibox destroy [--force] [--all]
```

Removes every sandbox unibox created, and their tasks, after checking for
running sessions and unpushed work; any found stops it unless `--force`.
Mounted directories are never deleted; adopted sandboxes are only
forgotten. `--all` also removes the herdr plugin and unibox's state,
configuration and values. Tokens in sbx, env repositories and clones
stay.

## unibox itself

### `unibox context`

```
unibox context
unibox context use [-s <sandbox>] [-e <env>]
unibox context clear
```

Shows, sets or clears the default sandbox and env. List keys: `enter`
use, `c` clear.

### `unibox config`

```
unibox config get [key]
unibox config set <key> <value>
unibox config unset <key>
```

| Key | Meaning |
| --- | --- |
| `default_env` | env a sandbox is created from when none is named |
| `stop_idle` | stop a sandbox when its last session ends (default `true`) |
| `ssh_agent` | may sandboxes use your SSH agent; sbx is set to match ([Security](./security#your-ssh-agent)) |
| `editor_ssh` | may editors and apps reach a sandbox as `ssh <sandbox>.sbx`; sbx is set to match and its daemon restarted |
| `remember_host_commands` | sbx's `env.rememberHostCommands`: approving an env's host commands covers later runs (default `false`) |

### `unibox herdr`

```
unibox herdr status
unibox herdr install-plugin
unibox herdr uninstall-plugin
```

The [herdr](/guide/herdr) plugin: whether it is installed, install it
(asks unless `--yes`), remove it and its observer.

### `unibox version`

```
unibox version [--json]
```

Prints the release, such as `0.2`, or `dev (<commit>)` for a non-release
build. `--json` adds `api_version`.

### `unibox completion`

```
unibox completion bash|zsh|fish|powershell
```

Prints a shell completion script.
