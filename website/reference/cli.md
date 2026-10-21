# Command Line

```
unibox [command] [flags]
```

## Conventions

**Nouns, then verbs.** Commands come in groups named for what they work on -
`task`, `sandbox`, `profile`, `runtime`, `network`, `agent`, `service`,
`script`, `instructions`, `template`, `secret`, `var`, `herdr` - and the verb
follows: `unibox sandbox stop`, `unibox network port publish`. Only the
everyday commands stand alone: `run`, `resume`, `init`, `context`, `config`,
`prune`, `destroy` and `version`.

**The argument is what the command acts on.** One name - a task, a sandbox,
a service, a host's rule - or a list of the same kind, such as directories or
hosts. The profile a command works in is `-p`/`--profile`, the sandbox
`-s`/`--sandbox`, so the same flag means the same thing everywhere.

**Where a command works when not told.** Left out, `-p` and `-s` come from
`UNIBOX_PROFILE` and `UNIBOX_SANDBOX`, then from the
[context](#unibox-context), then from there being only one. A command that
cannot tell says so and names the choices. Three exceptions keep a mistake
cheap: `unibox sandbox rm` always names its sandbox, `unibox task ls` lists
every sandbox's tasks unless given `-s`, and `unibox var` without `-p` is the
value every profile shares.

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
| `1` | any other error |
| `3` | an image must be built first |
| `4` | refused: unpushed work, a busy sandbox, a confirmation required, host access |
| the agent's | after a session, `run` and `resume` exit with the agent's status |

Under `--json` a failure is `{"api_version", "code", "message"}`. Errors are
lower case and say what to do: `sbx is not signed in - run sbx login`.

## Tasks

### `unibox run`

```
unibox run [directory...] [-- agent-args...] [flags]
```

Prepares a task and runs its agent in this terminal, or with `--shell` a
login shell in the task's directory. The directories are the task's
repositories; the current directory when none is named. Arguments after `--`
go to the agent. With neither `-s` nor `-p`, the task goes into the default
sandbox - `UNIBOX_SANDBOX`, the context's, or the only one - and when there is
none, into a new sandbox of the default profile.

| Flag | Meaning |
| --- | --- |
| `-p`, `--profile <name>` | the profile a new sandbox is made from |
| `-s`, `--sandbox <name>` | the sandbox to work in; one that does not exist yet is made from `-p` |
| `--agent <name>` | the coding agent, one the profile declares |
| `--shell` | open a shell in the task's directory instead of the agent; excludes `--agent`, and a new task names the profile's first agent for a later `resume` |
| `--task <name>` | the task's name, unique across sandboxes (default: its first repository and a digest) |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone |
| `--repo <[name=]source>` | a repository path or https URL, optionally `name=source` (repeatable) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches; create one nobody has from the local HEAD, or a URL's default tip; never forces |
| `--ignore-local` | use the forge's version despite uncommitted or unpushed local changes |
| `--mount` | mount the selected directories into the sandbox instead of cloning them |
| `--instructions <names>` | named instruction texts for a new sandbox |
| `--runtime <name>` | the runtime adapter (existing sandboxes keep theirs) |
| `--script <name>` | run one of the profile's scripts before the agent starts (repeatable, in order) |

### `unibox resume`

```
unibox resume <task> [-- agent-args...] [flags]
```

Runs a prepared task's agent again, or a shell in its directory, in the
sandbox that holds it.

| Flag | Meaning |
| --- | --- |
| `--agent <name>` | a coding agent to run instead of the task's own |
| `--shell` | open a shell in the task's directory instead of the agent; excludes `--agent` |
| `--script <name>` | run one of the profile's scripts before the agent starts (repeatable, in order) |

### `unibox task`

```
unibox task prepare [directory...] [-- agent-args...] [flags]
unibox task check [directory...] [-- agent-args...] [flags]
unibox task ls [-s <sandbox>]
unibox task add-repo <task> [directory...] [flags]
unibox task rename <task> <name>
unibox task rename <task> --reset
unibox task rm <task>
```

`prepare` makes the sandbox and the task ready and prints both; `check`
explains what a task needs before it can start. Both take the directories and
flags `run` does, except `--shell` and `--script`, and choose the sandbox as it
does.

`ls` lists prepared tasks, in every sandbox or with `-s` in one; it does not
take the sandbox from the context, so a forgotten context hides nothing. An
`ALIAS` column shows once a task has one.

`add-repo` clones more repositories into a prepared task, also while its
agent runs: directories as `run` takes them, or `--repo` for a URL or
`name=source`.

| Flag | Meaning |
| --- | --- |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone |
| `--repo <[name=]source>` | a repository path or https URL (repeatable) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches (never forces) |
| `--ignore-local` | use the forge's version despite local changes |
| `--mount` | mount the selected directories instead of cloning them |

`rename` gives a task an alias, taken wherever its ID is (`resume`,
`task add-repo`, `task rm`). The task keeps its ID and its directory
`/work/<id>`, so a running session is not disturbed. A name is unique across
sandboxes, as an ID is; `--reset` takes the alias away.

`rm` removes a task from its sandbox, after checking for running sessions and
unpushed work. A mounted directory is unmounted first and never deleted.

## Sandboxes

### `unibox sandbox`

```
unibox sandbox ls
unibox sandbox create <name> [-p <profile>] [flags]
unibox sandbox inspect [sandbox]
unibox sandbox shell [sandbox]
unibox sandbox stop [sandbox]
unibox sandbox recreate [sandbox] [--rebuild]
unibox sandbox rm <sandbox>
```

`ls` lists unibox's sandboxes and their runtime state. `create` makes a named
sandbox of a profile without mounting a host workspace and without a task.
`inspect` shows a sandbox's record and the runtime materials made for it.
`shell` opens an interactive shell in it, which counts as an active session,
so the sandbox is not stopped under it. `stop` stops an idle sandbox without
deleting its disk. `recreate` rebuilds the image when needed and recreates an
idle sandbox, without discarding unpushed work.

`rm` removes a sandbox, after checking for running sessions and unpushed
work. It always names the sandbox rather than taking the default, and when
that sandbox is the context's, the context forgets it.

The sandbox of `inspect`, `shell`, `stop` and `recreate` is optional: left
out, it is the default, as the [conventions](#conventions) say.

| Flag | Command | Meaning |
| --- | --- | --- |
| `-p`, `--profile <name>` | `create` | the environment profile |
| `--instructions <names>` | `create` | named instruction texts to attach |
| `--runtime <name>` | `create` | the runtime adapter |
| `--rebuild` | `recreate` | force a fresh image build |

### `unibox script`

```
unibox script ls [-s <sandbox>]
unibox script run <name...> [-s <sandbox>]
```

`ls` lists the scripts the sandbox's profile names under `scripts`; `run`
runs them in the sandbox now, in `/work`, one after another; the first to
fail stops the rest. `run` and `resume` take `--script` to run them before
the agent starts instead.

### `unibox instructions`

```
unibox instructions [-p <profile> | -s <sandbox>]
unibox instructions attach [instruction...] -s <sandbox>
```

Prints the instructions an agent gets: a profile's, or with `-s` a
sandbox's - its profile's and the texts attached to it. `attach` replaces the
sandbox's list of attached instruction names; with none it clears the list.

## Profiles

### `unibox init`

```
unibox init [profile] [flags]
```

Lays out the environments repository, and adds a profile to it. On a profile
that exists it completes it to what `init` writes today, and migrates an
older format. With `--from` it copies the profile from a
[template](/guide/templates) instead, and walks you through what it needs.

| Flag | Meaning |
| --- | --- |
| `--dir <path>` | the environments repository to lay out and configure (default: the configured one) |
| `--agents <list>` | agents the profile declares and installs (default `copilot`) |
| `--cpus <n\|n%>` | CPUs for the profile's sandboxes, or a share of the host's (default `4`) |
| `--memory <size\|n%>` | memory for the profile's sandboxes, `8g` or a share like `50%` (default `8g`) |
| `-i`, `--interactive` | ask for the repository when none is configured, and the profile's name, agents and resources; with `--from`, which profile to copy and what it needs; at the end, whether to sign the agents in now, as `agent login`. Without sbx installed it says so before anything is written and offers to go on with the experimental msb instead, which then becomes `default_runtime`. A form at the terminal - choices, an answer of your own, each checked as given; with `ACCESSIBLE=1`, numbered lines for a screen reader |
| `--from <source>` | copy the profile from a template: a git URL or a directory |
| `--use <profile>` | the template's profile to copy (default: the profile's name) |
| `--template <name>` | the name the template is kept under in `.templates` (default: its repository's) |

### `unibox profile`

```
unibox profile ls
unibox profile show [profile]
unibox profile set <path> <value> [-p <profile>]
unibox profile unset <path> [-p <profile>]
unibox profile build [profile] [flags]
unibox profile diff [profile] [flags]
unibox profile validate
```

`ls` lists profiles: name, agents, image state per runtime, errors, last
commit. `build` builds a profile's image, checks the declared agents and
loads it. `diff` shows what changed in the profile since its image was last
built, as a unified diff. `validate` checks the environments repository: its
layout, every profile, every instruction text. The profile of `show`,
`build` and `diff` is optional: left out, it is the default.

`show` prints the profile's `sandbox.yaml`; with `--json` the same document,
its keys as in the file and in the file's order, under `document`. `set` and
`unset` change any field of it, so a program configures a profile through
unibox alone. A path is the field's keys joined by dots; a key holding a dot,
or a list's item by its index, goes in brackets, as in jq; empty brackets
at the end add an item to a list. A value is YAML:

```sh
unibox profile set resources.cpus 6
unibox profile set agents.claude.args '[--model, opus]'
unibox profile set agents.claude.config[settings.json] _data/claude/settings.json
unibox profile set network.ports.web "{port: '3000', default: off}"
unibox profile set env[0] SPRING_PROFILES_ACTIVE
unibox profile set env[] BUILD_KIND
unibox profile set agents[] codex
unibox profile unset agents.claude.args
```

What is written is read back as unibox reads a profile - the field must
exist, its value be of the right form, a file it names be there - and kept
only then. `unset` takes out the mappings it leaves empty too; `version` is
unibox's own. `validate` checks the whole repository.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--rebuild` | `build` | build again even when current |
| `--runtime <name>` | `build` | the runtime to build for |
| `--runtime <name>` | `diff` | the runtime of the previous build |

### `unibox prune`

```
unibox prune
```

Removes the images unibox no longer needs: older builds, and those of
profiles and sandboxes that are gone.

### `unibox destroy`

```
unibox destroy [--all] [--secrets] [--msb] [--force]
```

Removes every sandbox, and so every task, and sandboxes named `unibox-*` the
state no longer knows. Every sandbox is checked first for running sessions
and unpushed work; one with either stops it before anything is removed,
unless `--force` discards them. A mounted host directory is never deleted.

| Flag | Also removes |
| --- | --- |
| `--all` | unibox's images; in sbx, the custom secrets carrying unibox's placeholders, a secret read by unibox's lookup and the domain approvals it added to `credentials.yaml` - a login or secret made with sbx stays; the herdr plugin; `~/.config/unibox` (state, configuration, `vars.toml`) and `~/.cache/unibox`. The environments repository stays where it is; only the configuration naming it goes |
| `--secrets` | the tokens unibox keeps in the platform's store (`unibox secret ls`) |
| `--msb` | the microsandbox runtime in `~/.microsandbox` |
| `--force` | sandboxes with running sessions or unpushed work, discarding it |

It asks first; `--yes` answers.

### `unibox template`

```
unibox template inspect <source> [--template <name>]
unibox template ls
unibox template update <template>
unibox template diff [-p <profile>]
```

`inspect` fetches a template - a git URL or a directory - and tells what it
holds without writing anything: each profile's agents, vars, services, tokens
and variables, its shared files, and how it stands to the repository; with
`--json` for a UI. A git template is cloned into a temporary directory, which
goes when the command ends. `ls` lists the templates the repository keeps, where each comes from and the
profiles copied from it. `update` fetches a template again and merges it into its profiles three ways,
from each as it was copied, with its shared files - all of it after asking, or
nothing when you and the template changed the same place, which it leaves to
resolve under `.templates/<template>/conflicts/`. `diff` shows how a
profile differs from the template profile it was copied from.

### `unibox agent`

```
unibox agent ls [-p <profile>]
unibox agent login [agent...] [-p <profile>] [flags]
unibox agent co-author [agent] [--as "Name <email>" | --reset] [-p <profile>]
```

`login` signs a profile's agents in: tokens into the platform's store, the
profile's `auth` written. A token comes on a hidden prompt or stdin, never as
an argument; `-i` walks through each sign-in - pages opened, `claude
setup-token` run, gh offered for copilot, the choice kept in `vars.toml`. `co-author` shows how the profile credits each
agent in its commits, or with `--as` sets it (`Name <>` for no address). `ls`
lists the agents, how each signs in, is credited and the arguments it starts with.

| Flag | Command | Meaning |
| --- | --- | --- |
| `-i`, `--interactive` | `login` | walk through each sign-in, with choices and the pages opened in the browser |
| `--gh` | `login` | sign copilot in through your gh login (`gh auth token`), kept in `vars.toml` |
| `--command <cmd>` | `login` | a host command printing one agent's token, kept in `vars.toml`; the profile names `secret:<agent>` |
| `--in-profile` | `login` | write `--command` or `--gh` into the profile, versioned with the repository |
| `--renew` | `login` | ask for every token again, replacing the stored ones |
| `--runtime <name>` | `login` | the runtime that keeps the logins (default: the configured one) |
| `--as "Name <email>"` | `co-author` | how to credit the agent |
| `--reset` | `co-author` | credit the agent as unibox does by default |

### `unibox service`

```
unibox service add <name> [-p <profile>] [flags]
unibox service ls [-p <profile>]
unibox service rm <name> [-p <profile>] [flags]
```

`add` stores a service's token and writes its entry into the profile. `ls`
lists the profile's services and whether their tokens are stored. `rm` takes a
service out of the profile, and its token unless another profile names it.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--kind <kind>` | `add` | `github`, `gitlab`, `artifactory` or `generic` (default: from the name) |
| `--host <host>` | `add` | a host, or `host/path` for one owner on a git forge (repeatable) |
| `--git` | `add` | wire the sandbox's git to the hosts (generic services) |
| `--env <name>` | `add` | the variable a tool reads the placeholder from: written into the profile's `env` as `NAME: ${services.<name>}` |
| `--scheme <s>` | `add` | a git service's header: `basic`, `bearer` or `raw` |
| `--user <name>` | `add` | the user a git forge pairs with the token (basic scheme) |
| `--author "Name <email>"` | `add` | who commits under the service (`--author` on a run wins) |
| `--no-co-author` | `add` | no `Co-Authored-By` trailer for the agent in its repositories |
| `--command <cmd>` | `add` | a host command printing the token, for a store unibox does not keep, kept in `vars.toml`; the profile names `secret:<name>` |
| `--secret <name>` | `add` | a token stored with `unibox secret set`, by its name, instead of asking for one |
| `--gh` | `add` | sign a `github` service in through your gh login, kept in `vars.toml`; the profile names `secret:<name>` |
| `--in-profile` | `add` | write `--command` or `--gh` into the profile, versioned with the repository, instead of `vars.toml` |
| `--refresh <duration>` | `add` | how often the runtime fetches the token again, e.g. `10m` |
| `--renew` | `add` | ask for the token again, replacing the stored one |
| `--keep-secret` | `rm` | keep the stored token |

## Network

### `unibox network`

```
unibox network allow <host...> [-p <profile>]
unibox network deny <host...> [-p <profile>]
unibox network rm <host...> [-p <profile>]
unibox network ls [-p <profile>]
```

Allows or denies hosts for a profile's sandboxes; every change is written
into the profile and applied to its running sandboxes. `allow` lets them
reach hosts the runtime's policy refuses, `deny` refuses hosts it allows,
`rm` takes hosts back to the runtime's policy, and `ls` lists the rules with
the hosts the services need. Ports are listed by `network port ls`.

### `unibox network port`

```
unibox network port ls [-s <sandbox> | -p <profile>]
unibox network port set <name> --map <mapping> [--off] [-p <profile>]
unibox network port unset <name...> [-p <profile>]
unibox network port publish [name...] [--map <mapping>...] [-s <sandbox>]
unibox network port unpublish [name...] [--map <mapping>...] [-s <sandbox>]
```

A port is published on the host's loopback. A mapping is
`[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`: `3000` takes any free host
port, `8080:3000` host port 8080.

`set` and `unset` change the profile, for every sandbox of it, and apply at
once to the running ones: `set` publishes a sandbox port by name at every
start, or with `--off` only when asked to; `unset` takes ports out.
`publish` and `unpublish` switch a running sandbox's ports alone - the
profile's by name, others by `--map` - and its next start goes back to the
profile's. `ls` lists a sandbox's ports with their URLs, or with `-p` the
ports the profile declares.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--map <mapping>` | `set` | the mapping the name stands for |
| `--off` | `set` | publish it only when asked to, not at start |
| `--map <mapping>` | `publish`, `unpublish` | a mapping the profile does not name (repeatable) |

## Secrets and values

### `unibox secret`

```
unibox secret set <name>
unibox secret ls
unibox secret rm <name>
unibox secret source [command | --reset] [--name <secret>] [-p <profile>]
```

Keeps tokens in the platform's store, for profiles to name as
`secret:<name>`. `set` asks on a hidden prompt or reads stdin; `ls` lists the
names, never the values. `source` shows or sets the command of yours that
reads `secret:<name>` instead of the store, `{name}` standing for the name -
for every profile, or with `-p` one profile's over it; `--name` sets one
secret's own command over it, as `gh auth token` for copilot; `--reset`
goes back to the store. It is kept in your `vars.toml`. `source` does not take the
profile from the context, as `var` does not.

### `unibox var`

```
unibox var set <name> <value> [-p <profile>]
unibox var ls [-p <profile>]
unibox var rm <name> [-p <profile>]
```

Keeps your own values for what profiles ask of you (`vars`), in
`~/.config/unibox/vars.toml`. Without `-p` a value applies to every profile;
with it, to that profile alone, over the shared one. `var` does not take the
profile from the context, so a value is never made one profile's by
accident. `ls -p` lists what the profile asks, each value and where it comes
from, and what is missing.

## unibox itself

### `unibox context`

```
unibox context
unibox context use [-s <sandbox>] [-p <profile>]
unibox context clear
```

Keeps the sandbox and profile commands work in when they are not told, so a
day spent in one sandbox needs no `-s`. `use` sets either or both, after
checking they exist; `context` alone shows what commands would use and why -
the context, `UNIBOX_SANDBOX` or `UNIBOX_PROFILE`, or there being only one;
`clear` forgets it. The context is kept in `config.yaml` as
`context: {sandbox, profile}`. The environment variables win over it, so a
terminal can work elsewhere without changing it.

### `unibox config`

```
unibox config get [key]
unibox config set <key> <value>
unibox config unset <key>
```

Reads or changes unibox's own configuration. The keys:

| Key | Value |
| --- | --- |
| `environments` | the environments repository's path |
| `default_runtime` | the runtime used when none is named |
| `runtimes.<runtime>.on` | `true` or `false`: whether the runtime is used |
| `runtimes.<runtime>.stop_idle` | `true` or `false`: stop a sandbox when its last agent ends |

`unset` forgets a setting, back to its default.

### `unibox runtime`

```
unibox runtime ls
```

Shows each runtime: supported or experimental, which is the default,
whether it can be used here, and what it isolates with. `--json` adds its
capabilities.

### `unibox herdr`

```
unibox herdr install-plugin
unibox herdr uninstall-plugin
unibox herdr status
```

Installs and operates the [herdr](/guide/herdr) status plugin.
`install-plugin` asks first unless `--yes`.

### `unibox version`

```
unibox version [--json]
```

Prints the release unibox was built from - the `vX.Y` tag without its `v`,
set when it is built - as `unibox --version` does. A build that is not a
release prints `dev (<commit>)`, with `-dirty` when the tree had changes.
With `--json` it adds the profile format it reads and the version of its
JSON contract:

```json
{
  "api_version": 1,
  "profile_format": 5,
  "version": "0.1"
}
```

### `unibox completion`

```
unibox completion [bash|zsh|fish|powershell]
```

Generates the shell completion script.
