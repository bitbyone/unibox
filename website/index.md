---
title: Introduction
---

# Introduction

unibox is a command-line companion to
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). sbx
runs coding agents - Claude Code, Codex, Copilot, OpenCode - in micro-VMs
on your machine, set up from sbxenv files and kits. unibox adds what the
same work needs every day: tasks over several repositories, a list of the
repositories a team works on, instructions for the agent, and interactive
lists for the operations that repeat.

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

```term ~/code/api
$ unibox run
preparing task api-4c1d2e9a
mounted /Users/jdoe/code/api at /work/api-4c1d2e9a/api in sandbox dev
Cloning into '/work/api-4c1d2e9a/adrs.unibox-clone'...

[dim]... claude runs here, in /work/api-4c1d2e9a, until you leave it ...[/]
```

## What unibox is not

unibox is not a sandbox of its own. Every sandbox it uses is an ordinary
sbx sandbox, created by sbx from the team's sbxenv files and kits, and
unibox has no format of its own for one. What it keeps for itself - the
team's ports, repositories and instructions - sits in `unibox.yaml` files
beside the sbx files in the same repository; sbx never reads them.

Using it is optional. A team member who does not install unibox works with
the same repository through sbx directly (`sbx env run`, `sbx run`), and
is limited by nothing unibox adds. A team can adopt unibox one person at a
time.

## What it adds to sbx

| | | |
| --- | --- | --- |
| [Setting up: init](/guide/init) | one command from the team's repository to a working sandbox | `unibox init <repository>` |
| [Tasks](/guide/tasks) | a unit of work: repositories mounted or cloned at `/work/<task>`, a branch, an agent, resumable | `unibox run`, `unibox resume` |
| [Interactive lists](/guide/interactive) | every noun alone at a terminal is a list acted on by keys | `unibox`, `unibox task` |
| [Repository index](/guide/repos) | the repositories an env's tasks take by name, the team's and yours | `unibox repo`, `unibox task new` |
| [Agent instructions](/guide/instructions) | what the agent is told of a task's repositories, written before each session | `unibox task instructions` |
| [Team and personal configuration](/guide/configuration) | what the team sets in the repository and each person overrides | `config.yaml`, `vars.toml` |
| [Sandboxes](/guide/sandboxes) | recreate after the env changed, resources as a share of the machine, defaults | `unibox sandbox` |
| [Ports](/guide/ports) | named ports switched on and off while the sandbox runs | `unibox port` |
| [Network](/guide/network) | what a sandbox was refused, rules per sandbox, rules handed back to a kit | `unibox network` |
| [Secrets](/guide/secrets) | which tokens the env's kits need, and where each is made | `unibox secret` |
| [Doctor](/guide/doctor) | what is missing or out of date, with the command that fixes it | `unibox doctor` |

## Terms

| Term | What it is |
| --- | --- |
| env | a sandbox as one or more sbxenv files of the team's repository describe it, under a name such as `dev` |
| kit | what an env pulls in: an image, tools, network rules, the tokens it needs. sbx's format, the team's to write |
| sandbox | a micro-VM sbx creates from an env. It keeps its disk until removed, and unibox stops it when its last session ends |
| task | a piece of work in a sandbox: repositories at `/work/<task>/<repo>`, a branch, the agent that works there. Several tasks share a sandbox |
| agent | claude, codex, copilot or opencode, as the env's kit installs it |

Next: [Installation](/guide/installation), then [Usage](/guide/usage).
