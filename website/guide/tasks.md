# Tasks

A task is your repositories in a sandbox, at `/work/<task>/<repo>`, with an
agent working in them.

## Start a task

```sh
cd ~/code/api
unibox run                          # this repository, the default sandbox
unibox run ~/code/api               # the same, by path
unibox run --task login             # name the task (default: repo name and a digest)
unibox run --branch feat/login      # switch your checkout to feat/login first
unibox run --agent codex            # another agent the sandbox has
unibox run --shell                  # a shell in the task, no agent
unibox run -- --model opus          # everything after -- goes to the agent
```

The sandbox is created on first use. If the env asks for a value you have
not set, `run` asks for it.

Several repositories in one task:

```sh
unibox run --repo ~/code/api --repo ~/code/web
unibox run --branch feat/login --repo ~/code/api --repo ~/code/web --branch main
```

A `--branch` before every `--repo` applies to all of them; one after a
`--repo` applies to that repository alone. The agent starts in
`/work/<task>`, above the repositories.

## Repositories by name

An env can name the repositories its tasks use - the team's under
`repos:` in its `unibox.yaml` files, yours in `config.yaml` - so a task is
started by picking them rather than typing addresses:

```sh
unibox task new                     # pick them on one screen
unibox task new payments adrs --branch feat/pricing   # or name them
unibox run adrs                     # a name works wherever a repository does
unibox repo                         # the index: / filter, space default, a add
```

```yaml
repos:
  adrs:
    url: https://gitlab.example.com/arch/adrs.git
    ref: main
    default: always               # every new task takes it
    description: Architecture decisions
  spec: {url: https://gitlab.example.com/biz/spec.git, default: on, context: true}
  payments: https://gitlab.example.com/x/payments.git
  web: ~/code/web                 # a directory of yours: mounted, not cloned
```

`default: always` puts the repository into every new task of the env -
cloned beside your mounted checkout in `unibox run` too - so the agent
always has the decisions and specifications at hand. `on` has it picked
to begin with in `task new`, `off` (the default) only when picked. `ref`
is the branch a task works on, else the forge's default. A forge address
is cloned; a directory of your machine (`/…`, `~/…`) is mounted, as
`run` mounts one.

A repository is worked in, or read: `context: true` - unsaid, true for
one that says `always` - keeps it on its `ref`, and no branch of a task's
is made or pushed there.

`task new` shows them on one screen: `space` picks, `c` switches work and
context, `b` types the branch the ones worked in get, `t` the task's name,
`p` whether a row's branch is pushed, `/` filters, `enter` starts. Each row
shows where it comes from (clone or mount), the branch it gets - `default`
for the forge's default branch when the index names none - and whether it
is pushed; nothing is asked of a forge while you pick. As the task
starts, unibox looks for the branch once, with git: where it exists
already, it asks whether to work on it there. The task's name - the
one typed, or the one it will be given - stands at the top, with
`(exists; it is resumed)` when a task has it. A generated name follows the
first repository you picked by hand and work in, not one picked to begin
with or only read, so tasks on the same decisions stay apart. In
`unibox task`, `r` adds repositories of the index to a task the same way,
seen at once by an agent working in it. A new branch that is pushed
is made on the forge from its default branch, or pushed from your
checkout; one that is not is made in the sandbox's clone, or your
checkout, alone. Named, `task new` takes `--branch`, `--task` and
`--no-push`.

Yours win over the team's: `unibox repo add name url|directory [--ref]
[--default] [--context true|false] [--every-env]`, `unibox repo default
name always|on|off|-` and `unibox repo rm name` write them.

## Mount or clone

By default a task mounts your checkout. `--clone`, or a repository given as
an HTTPS URL, clones it from the forge inside the sandbox instead.

| | Mount (default) | Clone (`--clone`) |
| --- | --- | --- |
| The agent works on | your checkout, as it is | its own clone of the forge's branch |
| You see its changes | at once, on disk | after it pushes and you fetch |
| Uncommitted local work | visible to the agent | not there |
| It can change your `.git` (hooks, config) | yes | no |

```sh
unibox run --clone --branch feat/login
unibox run --clone --push --branch feat/login    # push your local branch first
unibox run --clone --ignore-local                # use the forge's version, leave local changes
unibox run https://github.com/acme/api           # a URL is always cloned
unibox run --repo api=https://github.com/acme/api --repo docs=https://github.com/acme/docs
```

With a clone, the branch must exist on the forge: `--push` pushes yours
first (never forced). `run` warns about uncommitted or unpushed local work
the clone will not see, and asks whether to go on.

::: warning A mount gives the agent your .git
Hooks and config the agent writes into a mounted `.git` run on your machine
the next time you or your editor run git there. Use `--clone` for a
repository you would not let the agent configure.
:::

## Check before starting

```sh
unibox task check --clone --branch feat/login
```

`task check` takes the same arguments as `run`, changes nothing, and lists
what is in the way: a missing value, a branch not on the forge, local work
a clone would miss, an env that changed since the sandbox was made.
`unibox task prepare` sets the task up without starting an agent.

## Come back to a task

```sh
unibox task                      # pick a task: enter resumes, t starts a new one, a attaches another agent, r adds repositories, s a shell
unibox task ls                   # every task, in every sandbox
unibox task ls -s dev            # one sandbox's tasks
unibox resume login              # its agent again
unibox resume login --agent claude
unibox resume login --shell
unibox resume login -- --continue
```

## Change a task

```sh
unibox task rename api-3f2a9c1e login     # a name of your own
unibox task rename login --reset          # back to its ID
unibox task add-repo login ~/code/web     # add a repository, even while the agent runs
```

## Remove a task

```sh
unibox task rm login
```

It refuses while a session is running in the task or a clone has unpushed
work. A mounted directory is unmounted, never deleted.

## Commits

Commits use your `user.name` and `user.email` from the repository's git
config, or `--author "Name <email>"`. Each commit made during an agent
session gets a `Co-authored-by` line for that agent; commits you make in
a shell do not.
