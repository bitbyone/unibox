# Command Line

```
unibox [command] [flags]
```

## Conventions

**One rule for arguments.** What a command works on is its argument - a
sandbox (`shell`, `stop`, `recreate`, `rm`, `inspect`, `port`, `script`,
`tasks`, `create`), a task (`resume`, `add-repo`, `rename-task`, `rm-task`), a profile
(`build`, `diff`, `init`, `agent`, `network`, `service`, `instructions`) -
and a flag says where or how. `run`, `check` and `prepare` make a task, so
their arguments are its directories and the sandbox is a flag, `--sandbox`.

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

## The environments repository

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
| `-i`, `--interactive` | ask for the repository when none is configured, and the profile's name, agents and resources; with `--from`, which profile to copy and what it needs |
| `--from <source>` | copy the profile from a template: a git URL or a directory |
| `--use <profile>` | the template's profile to copy (default: the profile's name) |
| `--template <name>` | the name the template is kept under in `.templates` (default: its repository's) |

### `unibox template`

```
unibox template inspect <source> [--template <name>]
unibox template ls
unibox template update <template>
unibox template diff <profile>
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

### `unibox validate`

```
unibox validate
```

Checks the environments repository: its layout, every profile, every
instruction text.

### `unibox profiles`

```
unibox profiles
```

Lists profiles: name, agents, image state per runtime, errors, last commit.

### `unibox instructions`

```
unibox instructions [profile] [flags]
unibox instructions attach <sandbox> [instruction...]
```

Prints the instructions an agent gets: a profile's, or with `--sandbox` a
sandbox's - its profile's and the texts attached to it. `attach` replaces the
sandbox's list of attached instruction names.

| Flag | Meaning |
| --- | --- |
| `--sandbox <name>` | show them as this sandbox's agents get them, attached texts included |

## Images

### `unibox build`

```
unibox build <profile> [flags]
```

Builds, checks the declared agents and loads a profile's image.

| Flag | Meaning |
| --- | --- |
| `--rebuild` | build again even when current |
| `--runtime <name>` | the runtime to build for |

### `unibox diff`

```
unibox diff <profile> [flags]
```

Shows what changed in the profile since its image was last built, as a
unified diff.

| Flag | Meaning |
| --- | --- |
| `--runtime <name>` | the runtime of the previous build |

### `unibox prune`

```
unibox prune
```

Removes the images unibox no longer needs: older builds, and those of
profiles and sandboxes that are gone.

## Tasks

### `unibox run`, `unibox prepare`, `unibox check`

```
unibox run [directory...] [-- agent-args...] [flags]
unibox prepare [directory...] [-- agent-args...] [flags]
unibox check [directory...] [-- agent-args...] [flags]
```

`run` prepares a task and runs its agent in this terminal, or with `--shell`
a login shell in the task's directory. `prepare` makes
the sandbox and the task ready and prints both. `check` explains what a task
needs before it can start. The directories are the task's repositories; the
current directory when none is named. Arguments after `--` go to the agent.

| Flag | Meaning |
| --- | --- |
| `--profile <name>` | the profile a new sandbox is made from |
| `--sandbox <name>` | the sandbox to work in; one that does not exist yet is made from `--profile` (default: the profile's name) |
| `--agent <name>` | the coding agent, one the profile declares |
| `--shell` | `run` only: open a shell in the task's directory instead of the agent; excludes `--agent`, and a new task names the profile's first agent for a later `resume` |
| `--task <name>` | the task's name, unique across sandboxes (default: its first repository and a digest) |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone |
| `--repo <[name=]source>` | a repository path or https URL, optionally `name=source` (repeatable) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches; create one nobody has from the local HEAD, or a URL's default tip; never forces |
| `--ignore-local` | use the forge's version despite uncommitted or unpushed local changes |
| `--mount` | mount the selected directories into the sandbox instead of cloning them |
| `--instructions <names>` | named instruction texts for a new sandbox |
| `--runtime <name>` | the runtime adapter (existing sandboxes keep theirs) |
| `--script <name>` | `run` only: run one of the profile's scripts before the agent starts (repeatable, in order) |

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

### `unibox add-repo`

```
unibox add-repo <task> [directory...] [flags]
```

Clones more repositories into a prepared task, also while its agent runs:
directories as `run` takes them, or `--repo` for a URL or `name=source`.

| Flag | Meaning |
| --- | --- |
| `--branch <name>` | before all repositories, a shared branch; after one, that repository's alone |
| `--repo <[name=]source>` | a repository path or https URL (repeatable) |
| `--author "Name <email>"` | the commit author |
| `--push` | push the selected local branches (never forces) |
| `--ignore-local` | use the forge's version despite local changes |
| `--mount` | mount the selected directories instead of cloning them |

### `unibox tasks`

```
unibox tasks [sandbox]
```

Lists prepared tasks, in every sandbox or in one. An `ALIAS` column shows
once a task has one.

### `unibox rename-task`

```
unibox rename-task <task> <name>
```

Gives a task an alias, taken wherever its ID is (`resume`, `add-repo`,
`rm-task`). The task keeps its ID and its directory `/work/<id>`, so a
running session is not disturbed. A name is unique across sandboxes, as an
ID is; the task's own ID as the name takes the alias away.

### `unibox rm-task`

```
unibox rm-task <task>
```

Removes a task from its sandbox, after checking for running sessions and
unpushed work. A mounted directory is unmounted first and never deleted.

## Sandboxes

### `unibox sandboxes`

```
unibox sandboxes
unibox ls
```

Lists unibox's sandboxes and their runtime state.

### `unibox create`

```
unibox create <sandbox> [flags]
```

Creates a named sandbox without mounting a host workspace and without a task.

| Flag | Meaning |
| --- | --- |
| `--profile <name>` | the environment profile |
| `--instructions <names>` | named instruction texts to attach |
| `--runtime <name>` | the runtime adapter |

### `unibox inspect`

```
unibox inspect <sandbox>
```

Shows a sandbox's record and the runtime materials made for it.

### `unibox shell`

```
unibox shell <sandbox>
```

Opens an interactive shell in the sandbox. It counts as an active session, so
the sandbox is not stopped under it.

### `unibox stop`

```
unibox stop <sandbox>
```

Stops an idle sandbox without deleting its disk.

### `unibox recreate`

```
unibox recreate <sandbox> [flags]
```

Rebuilds the image when needed and recreates an idle sandbox, without
discarding unpushed work.

| Flag | Meaning |
| --- | --- |
| `--rebuild` | force a fresh image build |

### `unibox rm`

```
unibox rm <sandbox>
```

Removes a sandbox, after checking for running sessions and unpushed work.

### `unibox port`

```
unibox port <sandbox> [name|spec:on|off...]
```

Lists a sandbox's ports with their URLs, or switches them while it runs -
`fe:on`, `devtools:off` by the profile's names, or a spec the profile does not
name, `8080:3000:on`. The profile stays as it is, and the sandbox's next start
goes back to its defaults.

### `unibox script`

```
unibox script <sandbox> [name...]
```

Lists the scripts the sandbox's profile names under `scripts`, or runs them
in the sandbox now, in `/work`, one after another; the first to fail stops the
rest.

## Configuring a profile

### `unibox agent`

```
unibox agent login <profile> [agent...] [flags]
unibox agent co-author <profile> [agent] ["Name <email>"] [flags]
unibox agent ls <profile>
```

`login` signs a profile's agents in: tokens into the platform's store, the
profile's `auth` written. `co-author` shows or sets how the profile credits
each agent in its commits (`Name <>` for no address). `ls` lists the agents,
how each signs in and is credited.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--gh` | `login` | sign copilot in through your gh login (`gh auth token`) |
| `--renew` | `login` | ask for every token again, replacing the stored ones |
| `--runtime <name>` | `login` | the runtime that keeps the logins (default: the configured one) |
| `--reset` | `co-author` | credit the agent as unibox does by default |

### `unibox service`

```
unibox service add <profile> <name> [flags]
unibox service ls <profile>
unibox service rm <profile> <name> [flags]
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
| `--command <cmd>` | `add` | a host command printing the token, for a store unibox does not keep |
| `--gh` | `add` | sign a `github` service in through your gh login |
| `--refresh <duration>` | `add` | how often the runtime fetches the token again, e.g. `10m` |
| `--renew` | `add` | ask for the token again, replacing the stored one |
| `--keep-secret` | `rm` | keep the stored token |

### `unibox network`

```
unibox network allow <profile> <host...>
unibox network deny <profile> <host...>
unibox network rm <profile> <host|port...>
unibox network ls <profile>
unibox network port <profile> <name> <spec> [flags]
```

Allows or denies hosts and publishes ports for a profile's sandboxes; every
change is written into the profile and applied to its running sandboxes.
`allow` lets them reach hosts the runtime's policy refuses, `deny` refuses
hosts it allows, `rm` takes hosts back to the runtime's policy or ports out by
name, and `ls` lists the rules with the hosts the services need. `port`
publishes a sandbox port on the host's loopback by name; the spec is
`[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--off` | `port` | publish it only when switched on, not at start |

### `unibox secret`

```
unibox secret set <name>
unibox secret ls
unibox secret rm <name>
```

Keeps tokens in the platform's store, for profiles to name as
`secret:<name>`. `set` asks on a hidden prompt or reads stdin; `ls` lists the
names, never the values.

### `unibox var`

```
unibox var set <name> <value> [--profile <name>]
unibox var ls [--profile <name>]
unibox var rm <name> [--profile <name>]
```

Keeps your own values for what profiles ask of you (`vars`), in
`~/.config/unibox/vars.yaml`. Without `--profile` a value applies to every
profile; with it, to that profile alone, over the shared one. `ls --profile`
lists what the profile asks, each value and where it comes from, and what is
missing.

## unibox itself

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

### `unibox runtimes`

```
unibox runtimes
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
