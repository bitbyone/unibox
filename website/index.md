---
title: Introduction
---

# Introduction

[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) runs
coding agents - Claude Code, Codex, Copilot, OpenCode - in micro-VMs on
your machine. A team describes its sandboxes once, as sbxenv files and
kits in a repository, and each member creates a sandbox from them.

unibox is a command-line companion to sbx for the work that follows:
setting a person up from the team's repository, starting and resuming
tasks over several repositories, telling the agent what it works on, and
the operations that repeat every day. It adds no sandbox of its own and is
optional for each person on the team.

- **Interactive by default, scriptable when needed.** Each command alone
  at a terminal is a list acted on by keys; the same commands take
  arguments, `--json` and `--yes`, and print plain lines without a
  terminal.
- **One command to set up.** `unibox init` takes a person from the team's
  repository to a working sandbox: sign-in, their own values, the tokens
  the kits need with the page that makes each, the sandbox itself.
- **Daily work in few steps.** A task is started in a checkout with
  `unibox run`, or picked on one screen with `unibox task new`, and
  resumed by name. Several tasks share a sandbox, and a task holds several
  repositories, mounted or cloned.
- **The team's defaults, each person's overrides.** The repositories a
  team works on, the agent's instructions, ports and resources are set in
  the team's repository; each person adds or overrides their own.
- **Problems named with their fix.** `unibox doctor` checks sbx, the env,
  values, tokens and sandboxes, names the command that fixes each problem,
  and with `-i` runs it.
- **Plain sbx underneath.** No sandbox format of its own, nothing sbx
  needs to know about, and optional for each person on a team.

## sbx alone and with unibox

The same operations, for a team whose repository describes one env,
`dev`:

| | sbx alone | with unibox |
| --- | --- | --- |
| First setup | `sbx login`, `sbx policy init`, `sbx secret set` for each token the kits need, `sbx env run sandboxes/dev.sbxenv.yaml --env-arg registryUser=jdoe` | `unibox init <repository>` |
| Work on a checkout | the workspaces given when the sandbox is created; `sbx mount` adds directories later | `unibox run` in the checkout: a task of its own at `/work/<task>/api` |
| Several pieces of work | side by side in the sandbox's workspaces, or a sandbox each | tasks in one sandbox, each with its repositories, branch and agent |
| The team's other repositories | cloned by hand or by the agent | named in the team's index, taken by name or picked |
| Back to work | `sbx run --name dev` | `unibox resume <task>`, or `unibox task` and `enter` |
| A port | `sbx ports dev --publish 5173:5173` | `unibox port on vite`, named by the team |
| A missing token | `sbx secret ls`, compared with what the kits ask | `unibox secret ls` says which the kits need and which are stored |
| The env changed | `sbx env rm …`, then `sbx env run …` again | `unibox doctor` says so; `unibox sandbox recreate dev` brings the tasks back |

## What belongs to whom

| sbx | unibox |
| --- | --- |
| the micro-VM, its image and tools | tasks: repositories at `/work/<task>`, branches, sessions |
| kits and sbxenv files, and their format | the repository index and the agent's instructions |
| the proxy that puts tokens into requests | which tokens an env needs, and where each is made |
| the secret store | interactive lists over sbx's operations |
| the network policy and its enforcement | refused hosts, rules per sandbox, rules handed back to a kit |
| publishing ports | named ports, on or off at start |
| | checks of the whole setup (`unibox doctor`) |

Both live in the team's repository, side by side:

```
agent-sandboxes/
├── unibox.yaml              # unibox: ports, repositories, instructions for every env
├── kits/                    # sbx
│   └── java/java.yaml
└── sandboxes/
    ├── dev.sbxenv.yaml      # sbx: the env dev
    └── dev.unibox.yaml      # unibox: the env dev alone
```

sbx does not read the `unibox.yaml` files. A team member without unibox
uses the same repository with sbx alone and is limited by nothing in them.

## A day

```term ~/code/api
$ unibox init https://github.com/acme/agent-sandboxes
cloned https://github.com/acme/agent-sandboxes into /Users/jdoe/.local/share/unibox/repos/github.com/acme/agent-sandboxes
[b]1/5 sbx[/]
  installed and signed in
[b]2/5 Envs[/]
  env dev, sandboxes/dev.sbxenv.yaml: added
[b]3/5 Values[/]
env dev asks for values of your own, kept for it in vars.toml:
[b]4/5 Secrets[/]
  sbx has no token for anthropic, github of env dev
[b]5/5 Sandboxes[/]
creating sandbox dev from env dev
[dim]... the questions of each step, answered ...[/]
[ok]set up; start a task with unibox run <directory>, or unibox run --shell for a shell[/]

$ unibox run --branch fix/session-timeout
preparing task api-4c1d2e9a
mounted /Users/jdoe/code/api at /work/api-4c1d2e9a/api in sandbox dev
[dim]... claude works in /work/api-4c1d2e9a, beside the team's decisions in adrs/ ...[/]
sandbox dev stopped: no active sessions

$ unibox task
[blue]Tasks[/]

  TASK                        SANDBOX  AGENT   STATE    SESSIONS  REPOSITORIES
[blue]>[/] [b]api-4c1d2e9a                dev      claude  ready    0         api@fix/session-timeout, adrs@main[/]
  pricing (pricing-3f2a9c1e)  dev      codex   ready    0         payments@feat/pricing, spec@default, adrs@main

[key]↑/↓ j/k[/] move · [key]enter[/] resume · [key]t[/] new task · [key]a[/] attach agent · [key]r[/] add repos · [key]s[/] shell · [key]n[/] rename · [key]d[/] remove · [key]q[/] quit
```

The init above leaves out its questions; [Setting up: init](/guide/init)
shows each step.

## The guide

**Setting up.** [init](/guide/init) takes a person from the team's
repository to a sandbox. [Doctor](/guide/doctor) checks the setup and
names each fix. [Keeping the env current](/guide/updating) pulls the
team's changes and says what they need.

**Working.** [Tasks](/guide/tasks) are started, resumed, given another
agent or repository, and removed. The [repository index](/guide/repos)
holds what a task takes by name. [Agent instructions](/guide/instructions)
tell the agent what it works on. [Interactive lists](/guide/interactive)
cover every noun. [Team and personal configuration](/guide/configuration)
says what the team sets and what each person overrides.

**The sandbox.** [Sandboxes](/guide/sandboxes) are recreated, sized and
chosen. [Ports](/guide/ports) are named and switched.
[Network](/guide/network) shows what was refused and takes rules.
[Secrets](/guide/secrets) are the tokens the kits need.

Start with [Installation](/guide/installation), then [Usage](/guide/usage).
