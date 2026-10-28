# Tasks

sbx gives a sandbox one workspace. unibox puts any number of tasks into
one sandbox instead: a task is a set of repositories at
`/work/<task>/<repo>`, the branch they are worked on, and the agent
working there. A task is kept until it is removed, and resumed by name.

## Start a task

```sh
cd ~/code/api
unibox run                          # this checkout, the default sandbox
unibox run ~/code/api               # the same, by path
unibox run --task login             # a name of yours (default: its first repository and a digest)
unibox run --branch feat/login      # switch the checkout to feat/login first, made from HEAD if missing
unibox run --agent codex            # another agent the sandbox has
unibox run --shell                  # a shell in the task instead of an agent
unibox run -- --model opus          # what follows -- goes to the agent
```

The sandbox is created on first use. A value the env asks for and you have
not given is asked for at the terminal.

Several repositories in one task:

```sh
unibox run --repo ~/code/api --repo ~/code/web
unibox run --branch feat/login --repo ~/code/api --repo ~/code/web --branch main
unibox run --repo . --repo adrs     # a name of the env's repository index
```

A `--branch` before every `--repo` is the task's branch; one after a
`--repo` is that repository's alone. With several repositories the agent
starts in `/work/<task>`, above them, where
[`AGENTS.md`](./instructions) tells it what each one is.

The repositories the [index](./repos) says every task takes come along;
`--defaults` adds those it says `on`, `--only` takes nothing but what is
named.

## Mount or clone

A directory is mounted; an address, or any repository with `--clone`, is
cloned from the forge inside the sandbox.

| | Mount (default for directories) | Clone (`--clone`, or an address) |
| --- | --- | --- |
| The agent works on | your checkout as it is | its own clone of the forge's branch |
| You see its changes | at once, on disk | after it pushes and you fetch |
| Uncommitted local work | visible to the agent | not there |
| It can change your `.git` (hooks, config) | yes | no |

```sh
unibox run --clone --branch feat/login
unibox run --clone --push --branch feat/login    # push your local branch first, never forced
unibox run --clone --ignore-local                # take the forge's version despite local changes
unibox run https://github.com/acme/api           # an address is always cloned
unibox run --repo api=https://github.com/acme/api --repo docs=https://github.com/acme/docs
```

A clone needs its branch on the forge, or made there: `--push` pushes
yours first. `run` names uncommitted or unpushed work the clone would not
see and asks whether to go on.

::: warning A mount gives the agent your .git
A hook or setting the agent writes into a mounted `.git` runs on your
machine the next time you or your editor run git there. unibox says so the
first time it mounts a directory. Use `--clone` for a repository you would
not let the agent configure.
:::

## Check before starting

`task check` takes the same arguments as `run`, changes nothing, and lists
what is in the way: a missing value or token, a branch not on the forge,
local work a clone would miss, an env changed since the sandbox was made.

```term ~/code/api
$ unibox task check --clone --branch feat/login
uncommitted: api local edits will not reach the sandbox; commit and push them or use --ignore-local
branch_not_pushed: api push the selected branch first (or pass --push)
```

`unibox task prepare` sets a task up without starting an agent; with
`--json` both are for scripts.

## Come back to a task

```sh
unibox task                      # the same as a list
unibox task ls                   # every task in every sandbox
unibox resume login              # its agent again
unibox resume login --shell      # a shell beside it, also while the agent runs
unibox resume login -- --continue
```

A task keeps its repositories, branches and agent; `run` with the same
repositories on the same branches resumes the task it made before.

## Another agent on the same task

`--agent` resumes a task with another agent the sandbox has; the task
keeps it from then on. The task list offers the same:

```term
$ unibox task
[blue]Tasks[/]

  TASK   SANDBOX  AGENT   STATE  SESSIONS  REPOSITORIES
[blue]>[/] [b]login  dev      claude  ready  1         api@feat/login, adrs@main[/]

Resume login with  [b]claude[/]  [blue]codex[/]
[key]←/→ h/l[/] agent · [key]enter[/] resume · [key]esc[/] back
```

Both may work in the task at once: each session is its own process in the
same directories.

## Add a repository while the agent works

```sh
unibox task add-repo login ~/code/web     # a directory, an address or an index name
```

The task list offers the index's repositories the task does not hold,
on the screen [`task new`](./repos#task-new) uses:

```term
$ unibox task
[dim]... r on login ...[/]
[blue]Add repositories to task login[/]

  branch  [dim]none: each its ref, its checkout's branch or the forge's default[/]

     REPO      ROLE     FROM   BRANCH        PUSH  ABOUT
[blue]>[/] [ok]✓[/]  [b]web       work     clone  default             The web front end[/]
  [dim]•[/]  payments  work     clone  [dim]default[/]             Payments service
  [dim]•[/]  spec      [dim]context[/]  clone  [dim]default[/]             Product specification

[dim]1 picked[/]
[key]↑/↓ j/k[/] move · [key]space[/] pick · [key]c[/] work/context · [key]p[/] push · [key]b[/] branch · [key]/[/] filter · [key]enter[/] add them · [key]q[/] cancel
adding 1 repositories to task login
added web to task login at /work/login/web
```

A running agent sees the new directory at once; the next session's
`AGENTS.md` lists it.

## Rename and remove

```sh
unibox task rename api-3f2a9c1e login     # a name taken wherever its ID is
unibox task rename login --reset          # back to its ID
unibox task rm login
```

```term
$ unibox task rm pricing
  payments: clean, every commit on the forge, no stashes
  spec: clean, every commit on the forge, no stashes
  nothing outside the repositories
removed task pricing-3f2a9c1e
```

Removal refuses while the task's own session runs, or while a clone holds
uncommitted files, unpushed commits or stashes; `--force` discards them.
Sessions of other tasks in the sandbox do not stand in the way. A mounted
directory is unmounted and checked to be gone from the sandbox, never
deleted.

## Commits

Commits use your `user.name` and `user.email` from the repository's git
configuration, or `--author "Name <email>"`. Each commit made during an
agent session gets a `Co-authored-by` line for that agent; commits made
in a shell do not.
