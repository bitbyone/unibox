---
title: Introduction
---

# Introduction

unibox is a command-line companion to
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). It
shortens the everyday work with sbx sandboxes. It is not a sandbox of its
own, and each team member may use it or not.

## Key points

- Interactive by default, scriptable when needed
- One command to set up: `unibox init`
- Several tasks per sandbox, several repositories per task
- The team's defaults, each person's overrides
- `unibox doctor` names each problem and its fix
- Plain sbx underneath

## sbx alone and with unibox

Setting up:

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

After the team changed the env:

::: code-group

```sh [sbx]
sbx env rm sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe
sbx env run sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe
# then clone or mount each repository again
```

```sh [unibox]
unibox sandbox recreate dev    # its tasks come back
```

:::

## In the team's repository

```
agent-sandboxes/
├── unibox.yaml            # unibox
├── kits/                  # sbx
└── sandboxes/
    ├── dev.sbxenv.yaml    # sbx
    └── dev.unibox.yaml    # unibox
```

sbx ignores the `unibox.yaml` files.

## A day

```term ~/code/api
$ unibox run
mounted /Users/jdoe/code/api at /work/api-4c1d2e9a/api in sandbox dev
[dim]... claude works ...[/]

$ unibox task
[blue]Tasks[/]

  TASK          SANDBOX  AGENT   STATE  SESSIONS  REPOSITORIES
[blue]>[/] [b]api-4c1d2e9a  dev      claude  ready  0         api@main, adrs@main[/]

[key]enter[/] resume · [key]t[/] new task · [key]a[/] attach agent · [key]r[/] add repos · [key]q[/] quit
```

Next: [Installation](/guide/installation), [Usage](/guide/usage).
