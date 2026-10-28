---
title: Introduction
---

# Introduction

unibox is a command-line companion to
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). It
works with the sandboxes a team already describes for sbx, and makes the
everyday work with them shorter.

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

## Why use it

- You set up from the team's repository in one command, without reading
  its kits.
- You keep several pieces of work in one sandbox and return to each by
  name, with the agent you choose.
- The agent is told which repositories it works in and what the team
  expects of them.
- Every command is a list at the terminal, and plain output with `--json`
  for scripts.
- `unibox doctor` names what is wrong and the command that fixes it.
- Nothing is locked in: the sandbox is plain sbx, and you can stop using
  unibox at any time.

Next: [Installation](/guide/installation), [Usage](/guide/usage).
