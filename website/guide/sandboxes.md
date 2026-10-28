# Sandboxes

A sandbox is sbx's: unibox creates it with `sbx env create` from the
env's files and your values, and keeps a record of what it was made
from. What unibox adds is knowing when the sandbox no longer matches its
env, bringing tasks back after it is made again, resources given as a
share of the machine, and defaults so that most commands need no name.

## See and use them

```sh
unibox sandbox                  # the same as a list
unibox sandbox ls               # every sandbox and its state
unibox sandbox shell dev        # a shell in it
unibox sandbox inspect dev      # what unibox records about it
unibox sandbox stop dev         # stop it; disk and tasks stay
```

A sandbox stops by itself when its last session ends. To keep sandboxes
running, `unibox config set stop_idle false`.

```term
$ unibox sandbox ls
SANDBOX  ENV  AGENT   STATE    TASKS  SESSIONS
dev      dev  claude  running  2      1
dev-big  dev  claude  stopped  0      0
```

## Recreate after the env changed

The sandbox keeps a fingerprint of the env's files, kits and your values.
When a teammate changes the env, or you change a value, the next start and
`unibox doctor` say so. Nothing is applied until you ask:

```term ~/code/api
$ unibox run
env dev changed since sandbox dev was created; unibox sandbox recreate dev applies it
[dim]...[/]

$ unibox sandbox recreate dev
[blue]?[/] [b]Recreate sandbox dev and discard what it holds outside its tasks?[/]  Yes
removing sandbox dev to create it again from env dev
creating sandbox dev from env dev
[dim]... sbx shows its plan and asks to approve it ...[/]
mounted /Users/jdoe/code/api at /work/login/api in sandbox dev again
sandbox recreated
```

With `--yes` sbx applies the env's plan without asking; with
`unibox config set remember_host_commands true`, sbx's own setting, it
does not ask again for host commands it was shown before.

This removes the sandbox and creates it again from the env as it is now.
Tasks come back: mounts are mounted again, clones are cloned again from the
forge. Anything else in the sandbox, such as installed packages or Docker
images, is lost. It refuses while a session runs or a clone has unpushed
work.

## CPUs and memory

```sh
unibox sandbox resources dev                       # what it gets, and from where
unibox sandbox resources dev --cpus 50% --memory 16g
unibox sandbox resources dev --memory -            # back to the env's
unibox sandbox recreate dev                        # applies the change
```

```term
$ unibox sandbox resources dev --memory 16g
cpus    100% (the team's)
memory  16g (its own)
unibox sandbox recreate dev applies it
```

`--cpus` is a count, `0` for all, or a share such as `50%`. `--memory` is a
size such as `16g` or a share. sbx caps a sandbox at 75% of the host's
memory. For every env, or one env, set `[resources]` or
`[envs.<env>.resources]` in `vars.toml` (see
[Files](/reference/files#vars-toml)). The env's team may give a default in
its `unibox.yaml` files; yours win over it, and `unibox sandbox resources`
says where each value comes from.

## A second sandbox

```sh
unibox sandbox create dev-big -e dev --cpus 8 --memory 50%
unibox run -s dev-big
```

`-s` names the sandbox a command works in, `-e` the env a new one is
created from.

## Stop typing -s

```sh
unibox context use -s dev-big     # commands now work in dev-big, and its env
unibox context                    # what commands would use, and why
unibox context clear
```

`UNIBOX_SANDBOX` and `UNIBOX_ENV` override the context for one shell.
Without either, a command uses the only sandbox or env there is.
`unibox sandbox rm` and `unibox task ls` never take the default.

## Remove

```sh
unibox sandbox rm dev-big           # refuses while sessions run or a clone has unpushed work
unibox sandbox rm dev-big --force   # discards them
unibox prune                        # forget sandboxes removed with sbx directly
unibox destroy                      # every sandbox and task unibox made
```

Your mounted directories are never deleted. `unibox destroy --all` also
removes the herdr plugin and unibox's configuration, state and values; your
tokens in sbx and the env repository stay.

## A sandbox made with sbx

If you created a sandbox with `sbx env create` yourself, `run` will not
reuse its name. Adopt it instead:

```sh
unibox sandbox create --adopt dev -e dev
```

unibox then adds tasks to it, but never recreates it, and removes it only
with `--force`.
