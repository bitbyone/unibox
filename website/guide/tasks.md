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
unibox task                      # pick a task: enter resumes, a another agent, s a shell
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
