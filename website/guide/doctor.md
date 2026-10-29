# Doctor

`unibox doctor` checks what a run needs, end to end, and names for each
problem the command that fixes it. It asks sbx for its own diagnosis and
adds what only unibox knows: your values, the tokens the env's kits need,
whether each sandbox still matches its env, mounts whose directory is
gone, and whether the env's repository is behind.

```sh
unibox doctor             # everything
unibox doctor -e dev      # one env
unibox doctor -i          # and offer to run each fix
```

```term
$ unibox doctor
[b]sbx[/]
  [ok]✓[/] installed              found
  [ok]✓[/] Daemon                 healthy
  [ok]✓[/] Authentication         authenticated
  [ok]✓[/] Global network policy  initialized
  [ok]✓[/] for unibox             0.50.0, new enough
  [ok]✓[/] SSH agent              not forwarded, as you chose

[b]Env dev[/]
  [ok]✓[/] files                  /Users/jdoe/.local/share/unibox/repos/github.com/acme/agent-sandboxes/sandboxes/dev.sbxenv.yaml
  [ok]✓[/] args                   given
  [ok]✓[/] plan                   1 to add · 0 to change · 0 to destroy · 6 need approval
  [ok]✓[/] secret anthropic       stored (global)
  [err]✗[/] secret github          the token git pushes with
                           [dim]→ unibox secret set -e dev github[/]
  [warn]![/] sandbox dev            the env changed since the sandbox was created
                           [dim]→ unibox sandbox recreate dev[/]

[err]1 to fix[/]
```

With `-i` doctor asks, one problem at a time, whether to run its fix, and
says what a fix gives up:

```term
$ unibox doctor -i
[dim]...[/]
[blue]?[/] [b]Env dev, sandbox dev: the env changed since the sandbox was created. Run unibox sandbox recreate dev? It discards what the sandbox holds outside its tasks.[/]  Yes
```

## Common problems

**`sbx is not installed`** - install
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/), then run
`unibox init`.

**`sbx forwards your SSH agent, which lets agents sign with every SSH key
you have`** - decide once:

```sh
unibox config set ssh_agent false   # recommended
unibox config set ssh_agent true    # needed for git over SSH in a mounted checkout
```

**`env dev needs registryUser`** - `unibox var set -e dev registryUser jdoe`.

**A token is missing, or the agent says it is not signed in** -
`unibox secret ls`, then `unibox secret set`. Unibox cannot see whether a
stored token has expired; store it again.

**`env dev changed since sandbox dev was created`** - read the change in
the env's repository, then `unibox sandbox recreate dev`.

**`sbx has a sandbox dev that unibox did not create`** - adopt it with
`unibox sandbox create --adopt dev -e dev`, or name another with `-s`.

**`... is a git worktree, which cannot be mounted`** - run in the main
checkout, or add `--clone`. A path containing `:` cannot be mounted
either.

**A branch is not on the forge** (with `--clone`) - push it, or add
`--push`.

**`sandbox dev has no claude`** - the env does not install that agent.
Pick one it has with `--agent`, or ask the env's maintainers.

**`task ... may contain unpushed work`** - push it from the task
(`unibox resume <task> --shell`), or use `--force` to discard it.

**The sandbox fails to start after you moved a repository** - a mounted
directory is gone. `unibox doctor` names the task; `unibox task rm <task>`.

**A host is blocked** - `unibox network ls` lists refused hosts;
[allow it](./network).

**A sandbox was removed with sbx directly** - `unibox prune`.

## Scripts

Every command takes `--json` (versioned output, no prompts) and `--yes`
(accept prompts; never discards work). Exit code `4` means refused or not
found; `run` and `resume` exit with the agent's status.

## From unibox 0.1

0.2 drops 0.1's profiles. On its first run it moves the old state to
`~/.config/unibox/state.v1.json` and names the old sandboxes; remove them
with `sbx rm` once their work is safe.
