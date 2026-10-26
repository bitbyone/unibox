# Agents

The agent a sandbox runs is its env's choice, not unibox's. One kit of the
env is the workload: it installs the agent, says how it is started, and
declares the token it signs in with. unibox runs it in the task's directory,
has it trust that directory first, and credits it in the commits it makes.

## Which agent

The sbxenv file's `agent:` names the workload kit; without it, the first kit
of the sandbox kind with an entrypoint is. Its command is what the kit gives as the
sandbox's entrypoint and default command, the permissive flags the kit
chose included - the sandbox is the boundary, not the agent's questions.

```sh
unibox env show dev       # env dev, sandbox team-dev, agent sbx-kit-copilot
```

The env's agent is the one its `agent:` names. When the env has a kit for
another agent too, or its image carries one, `--agent` runs that one
instead, as sbx's kit for it would start it; the task remembers it for the
next `resume`:

```sh
unibox run --agent codex
unibox resume login --agent claude
```

An agent the sandbox does not have is refused, with the ones it has; ask
the team to add a kit for it to the env. Signing in is the kit's: an agent
without its kit has no credentials of its own.

Arguments after `--` on `run` or `resume` go to the agent, after the kit's
own:

```sh
unibox run -- --model <model>
unibox resume login -- --continue
```

An env whose kits run no agent - only tools, say - is used with
`unibox run --shell`, which opens a shell in the task instead.

## What unibox knows of each

unibox recognises four agents by the command the kit starts, and prepares
each before every session so it does not stop to ask:

| Agent | Prepared before each session |
| --- | --- |
| `claude` | the task, `/work` and each repository trusted, and the bypass mode accepted, in `~/.claude.json`; no self-update (`DISABLE_AUTOUPDATER=1`) |
| `codex` | the task's directories trusted in `~/.codex/config.toml`, between unibox's markers |
| `copilot` | the task's directories added to `trusted_folders` in `~/.copilot/config.json` |
| `opencode` | nothing |

What unibox writes is merged into the agent's own file: its other settings
and state stay. Any other agent a kit starts is run all the same, with
nothing prepared and no co-author.

The kit's image fixes the agent's version, and a new version is a new kit.

## Signing in

The kit declares the service its agent signs in to, and the env's bindings
the hosts that token may reach; sbx keeps the token and its proxy puts it
into the agent's requests. `unibox secret ls` shows whether sbx has it, and
`unibox secret set` has sbx ask for it - see [Secrets](./secrets).

## Co-authors

Commits carry your git identity and one co-author line for the agent of the
session that commits; a commit you make in a shell credits none.

unibox alone says it. A `commit-msg` hook takes out a trailer an agent adds
of itself - copilot always does - before adding unibox's. For a clone the
hook is the clone's own; for a mount it comes through the sandbox's global
git configuration, so your `.git` is not changed.

| Agent | Credited as |
| --- | --- |
| `claude` | `Claude <noreply@anthropic.com>` |
| `codex` | `Codex <noreply@openai.com>` |
| `copilot` | `Copilot <Copilot@users.noreply.github.com>` |
| `opencode` | `opencode <noreply@opencode.ai>` |

## Instructions and settings

What an agent is told and how it is configured - an `AGENTS.md`, skills, MCP
servers, its model - is the kit's to install, or the repository's own. unibox
adds nothing to it, so a change to it is a change to the team's kit,
reviewed there, and reaches a sandbox at its recreation.
