# Mount or Clone

A task's repositories reach the sandbox one of two ways, and the choice is
what the agent can touch on your machine.

| | Mount (the default) | Clone (`--clone`) |
| --- | --- | --- |
| What the agent works on | your checkout, files and `.git` as they are | a clone of its own, made from the forge inside the sandbox |
| Your uncommitted changes | there | not there: commit and push them, or `--ignore-local` |
| Its changes reach you | at once, on your disk | when it pushes, by `git fetch` |
| Can it change your `.git` | yes: hooks, `core.fsmonitor`, filters | no |
| A branch the forge lacks | made from HEAD in your checkout | pushed first, with `--push` |
| Git worktrees | refused | fine |

`run`, `task prepare` and `task check` mount when every repository is a
directory of the host, and clone when one is given as a URL. `--mount` and
`--clone` say it outright; `task add-repo` follows the task.

## Why mount is the default

It is how most people work with an agent: on the checkout open in their
editor, seeing each change as it is made, with nothing to push or fetch.
A clone keeps the agent away from your machine at the price of that.

::: danger What a mount gives up
Whatever the agent writes into your `.git` - a hook, `core.fsmonitor`, a
filter - runs on your machine later, when you, your editor or your shell
prompt run git there. Mount a repository you would let the agent configure.
unibox says so once, the first time it mounts each directory, and `--clone`
is the answer when you would not.
:::

## Mounting

```sh
unibox run
unibox run ../web ../api
```

mounts each repository into the sandbox at `/work/<task>/<repo>`. The agent
works on whatever is checked out, and what it writes is on your machine at
once. The mount is live and writable, kept across a stop and start of the
sandbox, and no path of your machine appears inside.

### With a branch

```sh
unibox run --branch feat/login
```

switches your checkout to `feat/login` when the task is prepared, making the
branch from HEAD when you have none of that name. It need not be on the
forge, since nothing is cloned from there. `task check` only says it would
switch (`branch_switch`), and git refuses a switch that would overwrite
uncommitted changes. Nothing is pushed or compared with a forge, so `--push`
is refused with a mount.

### Pushing over SSH

A mounted checkout keeps its remotes as they are. One over HTTPS pushes with
the token the env's kits give the sandbox. One over SSH (`git@github.com:...`)
needs your SSH agent in the sandbox, which unibox lets in only when you said
so - `unibox init` asks, `unibox config set ssh_agent true` says it later.
With it no token is needed for git at all; see
[Your SSH agent](/reference/security#your-ssh-agent) for what it gives the
agent.

### Worktrees are refused

A git worktree cannot be mounted. Its `.git` names the main repository by
your host's path, and the main repository names the worktree the same way;
mounted elsewhere, git in the sandbox would find neither, and a
`git worktree prune` there would delete the worktree's record on the host.
Mount the repository's own checkout instead, or `--clone`. A path with a
colon in it cannot be mounted either, as sbx's mount syntax has no way to
say it.

### What unibox writes, and where

unibox writes nothing into your `.git`. The task's author and its co-author
hook reach git in the sandbox through an `includeIf "gitdir:..."` of the
sandbox's global git configuration, which also sets
`gc.worktreePruneExpire = never`: your repository's git directory records
your other worktrees, which the sandbox does not have, and a garbage
collection there would otherwise prune their records on your machine.

### A directory that moved

sbx mounts the host's directory again at every start of the sandbox, and a
start fails when one is gone. `unibox doctor` names a mounted directory that
no longer exists; remove the task, or put the directory back.

## Cloning

```sh
unibox run --clone --branch feat/login
```

clones the branch from the forge inside the sandbox, over https, signed in
by the env's token, with `--filter=blob:none`, so history comes on demand.
A branch the forge does not have is refused unless `--push` pushes yours
first; uncommitted changes and local commits are named and left behind unless
`--ignore-local` or `--push` says what to do. [Tasks](./tasks#branches) has
the detail.

What the agent does stays in the sandbox until it pushes; a `git fetch` on
your machine brings it back, objects and refs alone. The forge's branch
protection is then the boundary, as the agent may push wherever its token
may.

## Removing a task

```sh
unibox task rm <task>
```

A mounted task is unmounted first; the sandbox checks every mount point is
empty, and the task is kept - deleting nothing - when an unmount fails or a
directory is still mounted. **It never deletes your files.** It does not
check a mount for unpushed work, which is on your machine.

A cloned task is checked for unpushed work and refused while it has any.

`unibox sandbox recreate` puts both back: mounted directories are mounted
again, clones are made again from the forge.
