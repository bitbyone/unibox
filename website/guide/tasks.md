# Tasks

A task is one piece of work in a sandbox: fresh clones of one or more
repositories at `/work/<task>/<repo>`, on a branch, with an agent running in
your terminal.

## Running one

`run`, `check` and `prepare` take the same arguments: the task's directories
as positional arguments (the current directory when none is named), and flags
for where and how.

```sh
unibox run --profile web --agent claude                       # this repository, sandbox "web"
unibox run --profile web --sandbox web-1 --branch feat/login  # a sandbox of its own
unibox run --sandbox web-1 ../web ../api --branch feat/login  # two repositories, one branch
```

| Command | Does |
| --- | --- |
| `unibox check` | lists what stands in the way of a start, and changes nothing |
| `unibox prepare` | makes the sandbox and the task ready, and prints both |
| `unibox run` | prepares when needed, then runs the agent in this terminal |

### The sandbox

`--sandbox` names where the task runs. One that does not exist yet is made
from `--profile`; without `--sandbox`, the sandbox is named for the profile.
An existing sandbox keeps its profile and runtime. `--runtime` picks the
runtime for a new one; `--instructions` attaches named instruction texts to a
new one.

### What `prepare` does

1. Creates the sandbox if new, or starts it, and runs the profile's `start`
   if the VM booted since it last ran.
2. For each repository, clones the branch into `/work/<task>/<repo>`
   (`--filter=blob:none`), or fetches and fast-forwards when it is there
   already.
3. Sets the commit author and the co-author hook in each clone, writes the
   agent's instructions into the sandbox's home, and merges the agent's
   configuration seeds.

`run` then runs the agent as its child - in the repository, `/work/<task>/<repo>`,
when the task has one, in `/work/<task>` above them when it has several: it passes the
terminal, signals and window size through, exits with the agent's status,
and stops the sandbox afterwards if no agent unibox started is left in it.

## Repositories and their URLs

unibox takes the repositories from the directories named, and each one's
clone URL from its `origin` - credentials stripped, an ssh URL turned into
https where the host is plain. The sandbox has no SSH key; it always clones
over https, signed in by the profile's [services](./services-and-secrets).

`--repo` adds a repository by path or https URL, optionally as
`name=source`, and may be repeated:

```sh
unibox run --sandbox web-1 --repo https://github.com/acme/web --repo docs=https://github.com/acme/docs
```

## Branches

Without `--branch`, each repository's current branch is used; a detached HEAD
needs `--branch`. `--branch` before every repository sets a shared branch;
after a repository it overrides that one alone:

```sh
unibox run --sandbox web-1 --branch feat/login ../web ../api --branch main
```

Any branch may be worked on, the default branch too. The agent pushes to the
branch it was given, so the forge's protection of that branch is what bounds
it.

### A branch the forge does not have

unibox never pushes on its own. A branch the forge does not have is refused -
push it first - unless `--push` asks unibox to run your own push:

```sh
unibox run --profile web --branch feat/new --push
```

`--push` pushes the selected local branches, creating one nobody has from the
local HEAD - or, for a repository given as a URL, from the forge's default
branch tip. It never forces.

### Local changes

The sandbox clones from the forge, so uncommitted changes and local commits
do not reach it. `check` names them (`uncommitted`, `local_commits`), and
`run` asks whether to go on with the forge's version; `--ignore-local` says yes
in advance. With `--push`, local commits are pushed instead.

## What `check` reports

`unibox check` answers with what it needs, each with a code, and `run` fixes
what it can:

| Code | Means |
| --- | --- |
| `image_required` | the image must be built |
| `login_required` | an agent's login is not stored |
| `secret_missing` | a secret the profile names is not stored |
| `branch_not_pushed`, `branch_missing` | the branch is not on the forge |
| `uncommitted`, `local_commits` | local work will not reach the sandbox |
| `branch_switch` | with `--mount`, your checkout will be switched |
| `profile_changed` | the profile asks the host for something new (a warning) |
| `service_overridden` | sbx's built-in `github` secret takes github.com over a service (a warning) |

## Naming a task

A task is named after its first repository and a digest, unless `--task`
names it. **Task names are unique across sandboxes**, so a task alone finds
its sandbox.

```sh
unibox run --profile web --task login-flow
```

## Coming back to a task

```sh
unibox tasks                         # every task in every sandbox
unibox tasks web-1                   # the tasks of one sandbox
unibox resume login-flow             # its agent again
unibox resume login-flow --agent codex
unibox resume login-flow --script update-skills
```

A task resumed with another agent credits that agent in its commits.

## Growing a task

```sh
unibox add-repo login-flow ../api --branch feat/login
```

clones one more repository into `/work/<task>/`, also while its agent runs.
The agent sees it at once and needs no restart, and the task's record grows,
so removal checks the new clone for unpushed work too. `add-repo` takes
directories, `--repo`, `--branch`, `--author`, `--push`, `--ignore-local` and
`--mount` as `run` does.

A repository the agent clones by itself gets its service's author, but no
co-author hook, and is not in the task's record: what must be kept belongs in
`add-repo`.

## Removing work

```sh
unibox rm-task login-flow     # a task, from its sandbox
unibox rm web-1               # a whole sandbox
```

Both check first for running sessions and for unpushed work, and refuse
rather than discard it. `--yes` accepts the questions they ask, and never
discards work either.

## Sandboxes

```sh
unibox sandboxes               # every sandbox (also: unibox ls)
unibox inspect web-1           # its record, and what was made for it
unibox shell web-1             # a shell in it; counts as a session
unibox stop web-1              # stop an idle one; its disk stays
unibox recreate web-1          # rebuild when needed and recreate an idle one
unibox create web-2 --profile web   # make one without starting a task
```

`recreate` checks for unpushed work and running sessions before it does
anything, and makes the sandbox again with the profile's current settings. A
start does the same on its own when the profile needs it, and says why.

## Commits

Commits carry your identity - `user.name` and `user.email` from the host's git
configuration of each repository, or a service's `author`, or `--author` -
and a co-author line for the agent of the session. See
[Agents](./agents#co-authors).

A `commit-msg` hook in each clone does the crediting, and is rewritten before
every session. A commit you make by hand in `unibox shell`, with no agent
running, is left as it is.
