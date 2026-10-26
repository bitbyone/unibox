# Troubleshooting

```sh
unibox doctor             # everything
unibox doctor -e dev      # one env
```

`doctor` checks that sbx is installed, signed in and 0.49.2 or newer; your
SSH agent setting; that each env's files read and sbx accepts them with
your values; every required value and token; whether each sandbox still
matches its env and still exists; mounted directories that are gone; and
whether a cloned env repository is behind. Each problem comes with the
command that fixes it.

## Common errors

**`sbx is not installed`** - install
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/), then run
`unibox init`.

**`sbx forwards your SSH agent, which lets agents sign with every SSH key
you have`** - decide once:

```sh
unibox config set ssh_agent false   # recommended
unibox config set ssh_agent true    # needed for git over SSH in a mounted checkout
```

**`env dev needs user`** - set the value: `unibox var set -e dev user
jdoe`.

**A token is missing, or the agent says it is not signed in** -
`unibox secret ls`, then `unibox secret set`. unibox cannot see whether a
stored token has expired; store it again.

**`env dev changed since sandbox dev was created`** - read the change in
the env repository, then `unibox sandbox recreate dev`.

**`sbx has a sandbox dev that unibox did not create`** - adopt it with
`unibox sandbox create --adopt dev -e dev`, or pick another name with
`-s`.

**`... is a git worktree, which cannot be mounted`** - run in the main
checkout, or add `--clone`. A path containing `:` cannot be mounted
either.

**A branch is not on the forge** (with `--clone`) - push it, or add
`--push`.

**`sandbox dev has no claude`** - the env does not install that agent.
Pick one it does with `--agent`, or ask the env's maintainers.

**`task ... may contain unpushed work`** - push it from the task
(`unibox resume <task> --shell`), or use `--force` to discard it.

**The sandbox fails to start after you moved a repository** - a mounted
directory is gone. `unibox doctor` names the task; `unibox task rm <task>`.

**A host is blocked** - `unibox network ls` lists refused hosts;
[allow it](./network).

**A sandbox was removed with sbx directly** - `unibox prune`.

## Upgrading from unibox 0.1

0.2 drops 0.1's profiles. On its first run it moves the old state to
`~/.config/unibox/state.v1.json` and names the old sandboxes; remove them
with `sbx rm` once their work is safe.

## Scripts

Every command takes `--json` (versioned output, no prompts) and `--yes`
(accept prompts; never discards work). Exit code `4` means refused or not
found; `run` and `resume` exit with the agent's status.
