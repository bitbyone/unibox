---
title: Introduction
---

# Introduction

unibox is a command-line companion to
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). It
works with the sandboxes a team already describes for sbx, and makes
everyday work in them frictionless.

It is a thin layer on top of sbx. The sandbox, its isolation, the proxy
that keeps tokens out of it and the network policy stay sbx's own; unibox
adds the conveniences sbx leaves out.

## Why use it

- **One command to start.** `unibox init` reads the team's sbx files,
  asks only for what you must give - your tokens and values - and
  creates the sandbox.
- **Many tasks, one sandbox.** Each task is its own repositories, branch
  and agent. `unibox resume feature-oauth` brings that task back, and the
  agent carries on its last conversation there. The task outlives even a
  full recreate of its sandbox.
- **Your checkout or a fresh clone.** A task mounts the directory you
  work in, or, for maximum isolation, clones from the forge, so the agent
  never touches your files. Per repository.
- **No commands to remember.** By default unibox is a minimal
  interactive TUI; every command is scriptable too, with `--json`.
- **The agent knows the ground.** Each task tells it which repositories
  it works in, which it only reads, and what the team expects of them.
- **The env changes, the work stays.** When the team's files change,
  unibox says so and recreates the sandbox with its tasks.
- **Plain sbx underneath.** No format of its own and nothing locked in:
  stop using unibox and the sandbox is still yours.

## A team's sbx environment

Let's assume this sbx environment: a team keeps its sandbox in a
repository, as plain sbx files.

```
agent-sandboxes/
├── kits/
│   ├── java/java.yaml         # tools, hosts, tokens
│   └── claude/claude.yaml     # the agent
└── sandboxes/
    └── dev.sbxenv.yaml        # the env dev: an image and the kits
```

Every member creates the sandbox `dev` from it and runs the agent there.
unibox needs nothing more than this.

## Setting up

With sbx alone, each member reads the kits to learn which tokens they
need, and passes their own values on every command. unibox reads the same
files and does it in one step: it asks for what the env needs, offers the
page where each token is made, and runs the sbx commands.

::: code-group

```sh [sbx]
sbx login
sbx policy init balanced
sbx secret set anthropic
sbx secret set github
sbx env run sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe
```

```sh [unibox]
unibox init https://github.com/acme/agent-sandboxes
```

:::

## Working

sbx gives the agent the sandbox's workspace. unibox adds tasks: each task
is a set of repositories in the sandbox, a branch and an agent, and is
resumed by name. Several tasks share one sandbox.

```term ~/code/api
$ unibox run
mounted /Users/jdoe/code/api at /work/api-4c1d2e9a/api in sandbox dev
[dim]... claude works ...[/]

$ unibox task
[blue]Tasks[/]

  TASK          SANDBOX  AGENT   STATE  SESSIONS  REPOSITORIES
[blue]>[/] [b]api-4c1d2e9a  dev      claude  ready  0         api@main[/]
  pricing       dev      codex   ready  1         payments@feat/pricing, web@feat/pricing

[key]enter[/] resume · [key]t[/] new task · [key]a[/] attach agent · [key]r[/] add repos · [key]q[/] quit
```

## When the team changes the env

unibox records what each sandbox was made from. When the env's files
change, it says so, and recreates the sandbox with its tasks.

::: code-group

```sh [sbx]
sbx env rm sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe
sbx env run sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe
# then mount or clone each repository again
```

```sh [unibox]
unibox sandbox recreate dev
```

:::

## What unibox adds to the repository

What sbx has no place for - the repositories a task can take, instructions
for the agent, named ports - unibox keeps in its own files beside the sbx
files:

```
agent-sandboxes/
├── unibox.yaml                # for every env
├── kits/
└── sandboxes/
    ├── dev.sbxenv.yaml
    └── dev.unibox.yaml        # for the env dev
```

sbx ignores them. A member without unibox uses the repository with sbx
alone, as before.

Next: [Installation](/guide/installation), [Usage](/guide/usage).
