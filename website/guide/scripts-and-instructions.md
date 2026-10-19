# Scripts and Instructions

Three kinds of script make and serve a sandbox, each at its own moment:

| What | Lives in | Runs | For |
| --- | --- | --- | --- |
| `sandbox.dockerfile` | the profile | when the image is built, in Docker on the host | everything static: toolchains, tools, certificates |
| `start` | the profile | once per boot, in the VM | services and daemons - a shared `docker compose` - and configuration from the environment |
| `scripts` | the profile | when asked for, in the VM | whatever you want on demand: updating skills, resetting a database |

`start` and `scripts` run inside the VM. A Dockerfile runs in your host's
Docker; read a teammate's change with `unibox profile diff` before it builds.

## `start`

```yaml
start: start.sh
```

runs once per boot, whoever booted the VM - unibox, `sbx exec` from a
terminal, sbx's own interface. Before every agent unibox compares the
kernel's boot id with a mark it left in the VM (`/var/lib/unibox/started`) and
runs the script only when they differ, its output in the preparation's. A
failure stops the start.

Nothing runs once when the VM is made: what should happen once is static, and
belongs in the Dockerfile.

## `scripts`

```yaml
scripts:
  update-skills: update-skills.sh
  reset-db: reset-db.sh
```

A script runs only when asked for, by name:

```sh
unibox run -p web --agent claude --script update-skills   # before the agent starts
unibox resume login-flow --script reset-db --script update-skills
unibox script ls -s web-1                                 # list them
unibox script run reset-db -s web-1                       # run now, in /work
```

`--script` is repeatable and runs in order, in the task's directory, before
the agent starts. `unibox script run <name>... -s <sandbox>` runs them now, in
`/work`, one after another, and counts as a session as a shell does.

A script runs:

- as the agent's user;
- from a file of its own, so its `#!` line is honoured - `sh` without one;
- with `UNIBOX_SANDBOX` and `UNIBOX_SCRIPT` set, and before a session
  `UNIBOX_TASK` and `UNIBOX_TASK_DIR` too.

The first to fail stops the rest, and before a session the agent. A script is
read when it runs, so changing it builds and recreates nothing.

unibox runs scripts and knows nothing of what they do. What a script does is
yours.

## Instructions

The agent has to know how it is meant to work - its branch, to commit and
push as it goes, never to force - and you will have more to say. None of it
may go into the repository being worked on, where it would be committed. So
every word comes from the environments repository, and is written into the
sandbox's home as the agent's user-level instructions - `~/.claude/CLAUDE.md`,
`~/.codex/AGENTS.md` and so on - never into `/work`.

```yaml
instructions: [unibox, review-style, ../../notes/no-migrations.md]
```

- A **bare name** is a shared text: `_shared/@instructions/<name>.md`, yours,
  or else `.shared/@instructions/<name>.md`, unibox's.
- An entry with a **path or extension** is a `.md` file relative to
  `sandbox.yaml`, confined to the repository.
- The profile's own **`instructions.md`** attaches itself after the listed
  texts.

Each file goes in once; a name or path that does not exist is an error.

`unibox` is the text unibox ships: how to work in a sandbox. List it unless
you replace it - with a `_shared/@instructions/unibox.md` of your own, say.

### Attaching texts to a sandbox

A sandbox can carry more shared texts than its profile lists:

```sh
unibox run -p web -s web-review --instructions review-style
unibox instructions attach review-style no-migrations -s web-review
unibox instructions attach -s web-review       # attach none
```

`attach` replaces the sandbox's list of attached names.

### Reading them

```sh
unibox instructions -p web              # as a profile's agents get them
unibox instructions -s web-1            # as a sandbox's, attached texts included
```

## Skills

Agent skills are files the agent reads, so they belong in the image like any
other tool: an installation script of yours in `_shared/@install` that runs
the [`skills`](https://www.npmjs.com/package/skills) command line. It installs
into the home of whoever runs it - `~/.agents/skills`, which codex, copilot
and opencode read, with a link from `~/.claude/skills` for claude - so run it
as the agent's user, after `USER agent`:

::: code-group

```sh [_shared/@install/skills.sh]
#!/bin/bash
set -eu
. /etc/profile.d/nvm.sh    # node, from tools/nvm
npx -y skills add https://github.com/anthropics/skills --skill frontend-design \
  -g -y -a claude-code codex github-copilot opencode
```

```dockerfile [sandbox.dockerfile]
RUN @install base tools/nvm agents/claude agents/codex

RUN id agent >/dev/null 2>&1 || useradd -m -s /bin/bash agent
USER agent
RUN @install skills        # as the agent, so the skills land in its home
WORKDIR /home/agent
```

:::

To update them between builds without a rebuild, a profile script does it on
demand:

::: code-group

```yaml [sandbox.yaml]
scripts:
  update-skills: update-skills.sh
```

```sh [update-skills.sh]
#!/bin/bash
. /etc/profile.d/nvm.sh
npx -y skills update -g -y
```

:::

```sh
unibox resume login-flow --script update-skills
```
