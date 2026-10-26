# Concepts

A handful of words carry the whole of unibox. Most are a page of their own
later; this one says how they fit.

```
 team's repository (git)        host                          sandbox (VM)
 ┌──────────────────────┐   ┌───────────────────────┐   ┌───────────────────────┐
 │ sandboxes/           │   │ unibox                │   │ /work/<task>/<repo>   │
 │   dev.sbxenv.yaml    │──▶│  env, your args,      │──▶│   mounted or cloned   │
 │ kits/                │   │  tasks, ports         │   │                       │
 │   ...                │   │ sbx                   │   │ the kit's agent       │
 └──────────────────────┘   │  the VM, your secrets, │◀──│                       │
                            │  proxy, policy        │   └───────────────────────┘
                            └───────────────────────┘             │
                                      │ real tokens added         ▼
                                      └───────────────────────▶ forge, model APIs
```

## sbxenv file and kit

sbx's own words. An **sbxenv file** (`*.sbxenv.yaml`) describes a sandbox:
the kits it is made of, the args a person supplies, the bindings that say
which hosts each token may reach, and its name. A **kit** is a package laid
over a sandbox - an image, files, hooks, network rules, credentials; one of
them, the workload, runs the agent. They live in your team's repository, and
unibox never writes them.

## Env

A name for one or more sbxenv files, kept in unibox's `config.yaml`:
`unibox env add dev dev.sbxenv.yaml`. sbx merges the files in their order,
so a local overlay after the team's file changes what it needs to. An env
can give its sandbox a name of your own, and holds your
[named ports](./ports). [More](./envs).

## Args and your values

An sbxenv file declares args, some required: a user name, a mirror, an
amount of memory. Your values for them are yours, in
`~/.config/unibox/vars.toml`, set with `unibox var set` and passed to sbx at
every create. A sandbox is not created while a required one is missing.

## Secret

A token a sandbox uses - for a model provider, a forge, a registry. The kits
declare which they take; sbx keeps them, in its own store on the host, and
its proxy puts them into requests. `unibox secret ls` says which are stored,
`unibox secret set` has sbx ask for the missing ones. [More](./secrets).

## Sandbox

A VM sbx made from an env, with a name: the env's own, the one its file
gives, or one you choose. It persists until you remove it: its disk, its
Docker images and its tasks stay when an agent ends. A sandbox whose last
session ended is stopped, so idle sandboxes hold no memory; the next start
boots it again.

unibox remembers what the env's files, local kits and your args were when
the sandbox was made. When they change, the next start says so, and
`unibox sandbox recreate` makes it again, putting its tasks back.

A sandbox can be shared by several tasks - memory, caches and images are
paid once, but between the agents there is no isolation at all: one user
with sudo, one filesystem, one Docker, one set of ports.

## Task

One piece of work in a sandbox: one or more repositories at
`/work/<task>/<repo>`, mounted from the host or cloned from the forge, with
the agent running in your terminal. A task's name is unique across all
sandboxes, so the task alone finds its sandbox:

```sh
unibox resume <task>
unibox task add-repo <task> ../api
unibox task rm <task>
```

By default a task is named after its first repository and a digest; `--task`
names it. [More](./tasks).

## Agent

The coding agent the env's workload kit runs - `claude`, `codex`, `copilot`
or `opencode`, or another the kit starts. The kit says how it is started and
signed in; unibox runs it with `sbx exec` in the task's directory, has it
trust that directory first, and credits it in the commits it makes.
[More](./agents).
