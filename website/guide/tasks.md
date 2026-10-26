# Tasks

A task is one piece of work in a sandbox: one or more repositories at
`/work/<task>/<repo>` - your checkout mounted from the host, or a clone made
from the forge - with the env's agent running in your terminal.

## Running one

`run`, `task check` and `task prepare` take the same arguments: the task's
directories as positional arguments (the current directory when none is
named), and flags for where and how.

```sh
unibox run                                       # this repository, the default env's sandbox
unibox run -e dev -s dev-2 --task login          # a second sandbox of env dev
unibox run ../web ../api --branch feat/login     # two repositories, one branch
unibox run --clone --branch feat/login           # a clone from the forge, not your checkout
unibox run --shell                               # a shell in the task, no agent
unibox run -- --model gpt-5                      # arguments for the agent
```

| Command | Does |
| --- | --- |
| `unibox task check` | lists what stands in the way of a start, and changes nothing |
| `unibox task prepare` | makes the sandbox and the task ready, and prints both |
| `unibox run` | prepares when needed, then runs the agent in this terminal |

### The sandbox

`-s` (`--sandbox`) names where the task runs. One that does not exist yet is
created from `-e` (`--env`); with `-e` alone, the task goes into the env's own
sandbox. With neither, it goes into the default sandbox - `UNIBOX_SANDBOX`,
the [context](#the-context)'s, or the only one - and when there is none, into
the sandbox of the default env. An existing sandbox keeps the env it was
created from.

A sandbox is created at the first `run` that needs it, on your terminal,
where sbx may ask you to approve the env's bindings. A required arg you have
not set is asked for there too; see [Envs](./envs).

### What `task prepare` does

1. Creates the sandbox if new, or starts it, and publishes the env's
   [ports](./ports).
2. For each repository, mounts the host's checkout at `/work/<task>/<repo>`,
   or clones the branch there inside the sandbox (`--filter=blob:none`), or
   fetches and fast-forwards a clone that is there already.
3. Sets the commit author and the co-author hook, and has the agent trust the
   task's directories.

`run` then runs the agent with `sbx exec` - in the repository when the task
has one, in `/work/<task>` above them when it has several. It passes the
terminal through, exits with the agent's status, and stops the sandbox
afterwards if no session unibox started is left in it (`stop_idle`, on by
default).

## Mounted or cloned

A task mounts its directories by default: the agent works on your files and
your `.git` as they are, and you see each change at once. `--clone` has it
work on a clone of its own instead, made from the forge inside the sandbox,
and what comes back to you is what it pushes. A task with a repository given
as a URL is cloned.

The two differ in what the agent can reach and in how branches and local
changes are handled. [Mount or Clone](./mount) has the trade.

## Repositories and their URLs

unibox takes the repositories from the directories named. For a clone, each
one's URL comes from its `origin` - credentials stripped, an ssh URL turned
into https where the host is plain. The sandbox clones over https, signed in
by the token the env's kits give it ([Logins](./logins)).

`--repo` adds a repository by path or https URL, optionally as
`name=source`, and may be repeated:

```sh
unibox run --clone --repo https://github.com/acme/web --repo docs=https://github.com/acme/docs
```

## Branches

Without `--branch`, each directory's current branch is used, and a URL's
default branch on the forge (usually `main`). `--branch`
before every repository sets a shared branch; after a repository it
overrides that one alone:

```sh
unibox run --branch feat/login ../web ../api --branch main
```

With a mount, `--branch` switches your own checkout to that branch, making it
from HEAD when it does not exist; `task check` warns with `branch_switch`
first.

With a clone, the branch comes from the forge. A branch the forge does not
have is refused - push it first - unless `--push` asks unibox to run your own
push. `--push` pushes the selected local branches, creating one nobody has
from the local HEAD - or, for a repository given as a URL, from the forge's
default branch tip. It never forces, and it has no meaning for a mount.

Any branch may be worked on, the default branch too. A clone's agent pushes
to the branch it was given, so the forge's protection of that branch is what
bounds it.

### Local changes

A clone is made from the forge, so uncommitted changes and local commits do
not reach it. `task check` names them (`uncommitted`, `local_commits`), and
`run` asks whether to go on with the forge's version; `--ignore-local` says
yes in advance. With `--push`, local commits are pushed instead. A mount has
nothing to miss: it is your checkout.

## What `task check` reports

`unibox task check` answers with what it needs, each with a code, and `run`
fixes what it can:

| Code | Means |
| --- | --- |
| `runtime_unavailable` | sbx is missing, too old or not running |
| `arg_missing`, `arg_invalid` | a value of yours the env requires is not set, or does not match |
| `env_changed` | the env changed since the sandbox was created (a warning; `sandbox recreate` applies it) |
| `branch_not_pushed`, `branch_missing` | the branch is not on the forge |
| `uncommitted`, `local_commits` | local work will not reach a clone |
| `remote_check_pending` | the branch is checked inside the sandbox when it is cloned (a warning) |
| `branch_switch` | a mounted checkout will be switched to the branch (a warning) |

A missing token is not among them: `unibox login ls` and `unibox doctor` say
which the env lacks.

## Naming a task

A task is named after its first repository and a digest, unless `--task`
names it. **Task names are unique across sandboxes**, so a task alone finds
its sandbox.

An alias gives it a name of your own afterwards, taken wherever the ID is:

```sh
unibox task rename payments-3f2a9c1e refunds
unibox resume refunds
```

The ID and the directory `/work/<id>` stay, so a running agent is not
disturbed. `unibox task rename refunds --reset` takes the alias away.

## Coming back to a task

```sh
unibox task ls                  # every task in every sandbox
unibox task ls -s dev           # the tasks of one sandbox
unibox resume login             # its agent again
unibox resume login --shell     # a shell in it
unibox resume login -- --continue
```

## Growing a task

```sh
unibox task add-repo login ../api --branch feat/login
```

adds one more repository to `/work/<task>/`, also while its agent runs,
mounted or cloned as the task's first ones were. The agent sees it at once
and needs no restart, and the task's record grows, so removal checks the new
repository too.

A repository the agent clones by itself gets no co-author hook and is not in
the task's record: what must be kept belongs in `task add-repo`.

## Removing work

```sh
unibox task rm login            # a task, from its sandbox
unibox sandbox rm dev-2         # a whole sandbox, always named
```

Both check first for running sessions and for unpushed work, and refuse
rather than discard it. `--yes` accepts the questions they ask, and never
discards work either. A mounted task is unmounted first, and your directory
on the host is never deleted.

## Sandboxes

```sh
unibox sandbox ls                     # every sandbox, and its state in sbx
unibox sandbox inspect dev            # what unibox keeps about it
unibox sandbox shell dev              # a shell in it; counts as a session
unibox sandbox stop dev               # stop an idle one; its disk stays
unibox sandbox recreate dev           # create it again from the env as it is now
unibox sandbox create dev-2 -e dev    # make one without starting a task
```

The sandbox of `inspect`, `shell`, `stop` and `recreate` may be left out, and
is then the default one, as for `run`.

`unibox prune` forgets the sandboxes sbx no longer has, removed with
`sbx rm` say.

### The context

A day spent in one sandbox need not repeat `-s` on every command. The context
keeps the sandbox and env commands work in when they are not told:

```sh
unibox context use -s dev-2 -e dev   # checks both exist
unibox context                       # what commands would use, and why
unibox context clear                 # forget it
```

`UNIBOX_SANDBOX` and `UNIBOX_ENV` win over it, so one terminal can work
elsewhere without changing it; with neither set nor a context, the only
sandbox there is, or the default env, serves. Removing the context's sandbox
clears it from the context. `unibox task ls` lists every sandbox's tasks
whatever the context says, and `unibox sandbox rm` always names its sandbox.

## Commits

Commits carry your identity - `user.name` and `user.email` from the host's
git configuration of each repository, or `--author` - and a co-author line
for the agent of the session. See [Agents](./agents#co-authors).

A `commit-msg` hook does the crediting, and is rewritten before every
session. A commit you make in a shell, with no agent running, is left as it
is.
