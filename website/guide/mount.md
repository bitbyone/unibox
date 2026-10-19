# Mounting a Directory

By default nothing of yours is mounted into a sandbox: the agent works on
fresh clones from the forge. `--mount` is the exception, and it is your call.

```sh
unibox run -s dev --agent copilot --mount
```

mounts the current repository into the running sandbox `dev` at
`/work/<task>/<name>` instead of cloning it. The agent works on your files and
your `.git` as they are, on whatever is checked out, and what it writes there
is on your machine at once.

`--mount` is taken by `run`, `task prepare`, `task check` and `task add-repo`, and applies to
every directory the command selects.

::: danger Use it knowingly
A mount gives up what the clones protect. Whatever the agent writes into your
`.git` - a hook, `core.fsmonitor`, a filter - runs on your machine later,
when you or your editor run git there. Mount a repository you would let the
agent configure.
:::

## With a branch

```sh
unibox run -s dev --agent claude --mount --branch feat/login
```

switches your checkout to `feat/login` when the task is prepared, making the
branch from HEAD when you have none of that name. It need not be on the forge,
since nothing is cloned from there. `task check` only says it would switch
(`branch_switch`), and git refuses a switch that would overwrite uncommitted
changes.

Nothing is cloned, pushed or compared with a forge, so `--push` is refused
with `--mount`.

## Worktrees are refused

A git worktree cannot be mounted. Its `.git` names the main repository by
your host's path, and the main repository names the worktree the same way;
mounted elsewhere, git in the sandbox would find neither, and a
`git worktree prune` there would delete the worktree's record on the host.
Mount the repository's own checkout instead.

## What unibox writes, and where

unibox writes nothing into your `.git`. The task's author and its co-author
hook reach git in the sandbox through an `includeIf "gitdir:..."` of the
sandbox's global git configuration, which also sets
`gc.worktreePruneExpire = never`: your repository's git directory records your
other worktrees, which the sandbox does not have, and a garbage collection
there would otherwise prune their records on your machine.

## Removing a mounted task

```sh
unibox task rm <task>
```

unmounts first, has the sandbox check every mount point is empty, and keeps
the task - deleting nothing - when an unmount fails or a directory is still
mounted. **It never deletes your files.** It does not check for unpushed
work, which is on your machine.

A recreated sandbox has no mounts; preparing its tasks again mounts them
again.

## How it works underneath

`--mount` needs sbx: msb cannot add a mount to a sandbox it has made, so
unibox refuses it there.

sbx mounts into a running sandbox. Verified on a throwaway sandbox: the mount
is live and writable, kept across a stop, taken out by an unmount that leaves
an empty mount point, and removing the sandbox leaves the host's files alone.
The directory is mounted at its place in the task, so no path of your machine
appears inside.
