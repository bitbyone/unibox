# Sandboxes

## See and use them

```sh
unibox sandbox                  # pick one: enter its tasks, s shell, p ports, t stop, c recreate, d remove
unibox sandbox ls               # every sandbox and its state
unibox sandbox shell dev        # a shell in it
unibox sandbox inspect dev      # what unibox records about it
unibox sandbox stop dev         # stop it; disk and tasks stay
```

A sandbox stops by itself when its last session ends. To keep sandboxes
running, `unibox config set stop_idle false`.

## Recreate after the env changed

When a teammate changes the env, or you change a value, the next `run` and
`unibox doctor` say the sandbox no longer matches. Nothing is applied until
you ask:

```sh
unibox sandbox recreate dev
```

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
unibox context use -s dev-big     # commands now default to dev-big
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
