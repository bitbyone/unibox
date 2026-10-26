# Envs

An env is a name for the sbxenv files a sandbox is created from. The files
are your team's, as sbx reads them; unibox never changes, generates or merges
them with anything of its own. It reads the little it needs - the args they
declare, their kits, their bindings and their name - and leaves every other
field to sbx.

## An sbxenv file

What a team keeps in its repository, beside its kits, looks like this:

```yaml
schemaVersion: "1"
name: team-dev                  # the sandbox's name
agent: sbx-kit-copilot          # the workload kit, which runs the agent
args:
  user:
    required: true
    description: Your user name on the registry
  memory:
    default: "8g"
    pattern: "([0-9]+[mg])?"
kits:
  - docker.io/docker/sbx-kit-copilot:1.0.86
  - ../kits/git
  - source: ../kits/registry
    args:
      user: ${{ env.args.user }}
bindings:
  copilot: {apiKey: {domains: [api.githubcopilot.com]}}
```

sbx's documentation is the reference for the format. Everything not shown
here - lifecycle hooks, resources, network rules - is sbx's to read, and
unibox passes it on untouched.

## Adding one

`unibox init` in the team's repository finds its sbxenv files and makes an
env of each you pick ([Getting Started](./#set-it-up)). One at a time:

```sh
unibox env add                                   # the one sbxenv file in this directory
unibox env add dev ~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml
unibox env add dev dev.sbxenv.yaml local.sbxenv.yaml --name dev-mine
unibox env add dev dev.sbxenv.yaml --default
```

`env add` names one or more files, which sbx merges in the order given: a
later file, such as a local overlay of your own, overrides an earlier one.
Run it again with other files to change an env. The files are read before
the env is kept, and their paths are kept as absolute ones.

| Flag | Meaning |
| --- | --- |
| `--name <name>` | the sandbox's name; without it, the file's `name:`, or the env's own |
| `--default` | make it the env new sandboxes come from (`default_env`) |

```sh
unibox env ls             # the envs, their sandboxes and files
unibox env show dev       # what the files declare, and what you set
unibox env rm dev         # forget it; its files and sandbox stay
```

Envs are kept in `~/.config/unibox/config.yaml`, with the repository the
files are in when init found them, and the address it was cloned from when
init cloned it.

## Keeping the team's repository current

```sh
unibox env update          # every repository unibox cloned
unibox env update dev      # the one dev's files are in
```

brings each repository unibox init cloned from a git address up to its
remote, by a fast-forward alone, and says what that changes: an arg the
files now ask of you, a sandbox to recreate, a new sbxenv file for
`unibox init` to add. A checkout of your own is never pulled; update it
with git as you would. `unibox doctor` says when a clone is behind its
remote.

## Your values for its args

An arg the file requires and gives no default is yours to set, once:

```sh
unibox var ls -e dev               # what the env asks for, and what you set
unibox var set -e dev user jdoe
unibox var rm -e dev user
```

```
NAME    VALUE  FROM     FOR
memory  8g     default
user    jdoe   yours    Your user name on the registry
```

The values are kept in `~/.config/unibox/vars.toml`. One you give with
`-g` is for every env that declares the arg, and an env's own wins:

```toml
[args]
user = 'jdoe'

[envs.dev.args]
user = 'john'
```

`unibox var ls -g` lists the ones for every env; `var ls -e dev` shows
`global` for a value it takes from them.

Before each `sbx env create`, unibox writes the values the env declares into
a file only you can read, hands it to sbx as `--env-args-file`, and removes
it after. A value the file's `pattern` refuses stops a create before sbx is
asked (`arg_invalid`); one for an arg the env does not declare is left out,
and `unibox doctor` names it.

A sandbox is not created while a required arg is missing. `run` and
`sandbox create` ask for it at a terminal; elsewhere they stop with the
arg's name and the `unibox var set` that fixes it.

::: warning No secrets in vars.toml
The args file is plain text, and so is `vars.toml`. A value starting with
`secret:` is refused: tokens are [secrets](./secrets), kept by sbx.
:::

## Resources

How much of your machine a sandbox gets is yours to say, not the team's:
an env's `sandboxOptions` may set `cpus` and `memory`, and your
`vars.toml` sets them over it, for every env or for one:

```toml
[resources]
cpus = '100%'
memory = '75%'

[envs.dev.resources]
memory = '16g'
```

`cpus` is a count, `0` for all of them, or a share of this machine's;
`memory` a size or a share. sbx takes no share, so at each create unibox
works it out on the host - a share of the CPUs rounded and at least one,
`100%` as `0`, memory in MiB - and gives sbx the amounts in an sbxenv file
of its own, after the env's, so that they win. sbx gives a sandbox at
most 75% of the host's memory; asking for more gets that, said in the
output. What you leave unset stays the env's.

A change reaches a sandbox when it is recreated: the next start says the
env changed and that `unibox sandbox recreate` applies it.

## Its sandbox

An env has one sandbox by default, named by `--name`, else by the file's
`name:`, else after the env. `-s` gives another name for another sandbox of
the same env:

```sh
unibox sandbox create -e dev           # the env's sandbox
unibox sandbox create dev-2 -e dev     # a second one
unibox run -e dev -s dev-login         # created on first use
```

The first create runs on your terminal: sbx shows the bindings it is about
to apply and asks you to approve them, and runs the files' host commands.
unibox never passes `--force` for you. A create that stopped half way - sbx
refused without your approval, say - is tried again at the next `create` or
`run`, and a sandbox sbx did make is only readied.

### A sandbox made with sbx

A sandbox you created with `sbx env create` yourself is not unibox's, and a
`run` that would create one of the same name refuses. `--adopt` takes it
over:

```sh
unibox sandbox create --adopt team-dev -e dev
```

unibox then adds tasks to it, but does not know how it was made: it never
recreates it, `sandbox rm` removes it only with `--force`, and `destroy`
only forgets it.

## When the env changes

When the sandbox is created, unibox keeps a digest of everything it was
created from: the env's files, the files of its local kits, and your args. A
kit given as an image reference is in the files already.

When that changes - a teammate's commit to the kits, a new value of yours -
the next `run` says so, `unibox task check` reports `env_changed`, and
`unibox doctor` names the sandbox. Nothing is applied unasked, because
recreating discards what the sandbox holds outside its tasks:

```sh
unibox sandbox recreate dev
```

checks for running sessions and unpushed work, removes the sandbox, creates
it again from the files and args as they are now, and puts its tasks back:
mounted directories are mounted again, clones are made again from the forge.

## Checking an env

```sh
unibox doctor            # every env
unibox doctor -e dev     # one
```

checks that the files read, that every required arg is set, that
`sbx env plan` accepts the files with your args, that sbx has the env's
secrets, and whether the sandbox still matches the env.
