# Keeping the Env Current

The team changes its repository; each person's sandbox follows when they
choose. Unibox pulls the repository and says what the change means for
you.

```sh
unibox env update          # every env repository
unibox env update dev      # the one dev comes from
```

```term
$ unibox env update
/Users/jdoe/.local/share/unibox/repos/github.com/acme/agent-sandboxes: updated
  env dev now asks for npmUser: unibox var set -e dev npmUser <value>
  env dev changed for sandbox dev: unibox sandbox recreate dev
unibox doctor checks the secrets they need
```

`env update` fast-forwards the env repository and says what that changes
for you: a value the env now asks for, a sandbox to recreate, a new sbxenv
file. A repository `unibox init` cloned is always updated; a checkout of
your own only when it has no uncommitted changes and its branch tracks a
remote one.

Then act on what it says:

```sh
unibox var set user jdoe      # a new value the env asks for
unibox secret set             # a new token
unibox sandbox recreate dev   # apply the changed env to the sandbox
unibox init                   # add a new sbxenv file as an env
```

`unibox doctor` says when a cloned env repository is behind its remote.

## Your values

```sh
unibox var ls                 # what the env asks for, and what you set
unibox var set user jdoe      # for the default env
unibox var set -g user jdoe   # for every env that asks for it
unibox var rm user
unibox var                    # the same as a list
```

Values are kept in `~/.config/unibox/vars.toml`. An env's own value wins
over a `-g` one. A changed value reaches the sandbox when you recreate it.
Never put a token in a value; `vars.toml` refuses one that starts with
`secret:`.

## Envs

```sh
unibox env ls                 # the envs and their files
unibox env show dev           # what dev's files declare, and what you set
unibox config set default_env dev
unibox env rm dev             # forget it; files and sandbox stay
unibox env                    # the same as a list
```
