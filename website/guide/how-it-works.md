# How it Works

## Three steps

1. **Your team describes the sandbox** in an env repository: sbxenv files
   and kits that say which image, tools and agent it has, which hosts it
   may reach and which tokens it needs.
2. **You set it up once** with `unibox init <env repository>`. unibox
   clones the repository, asks for your own values and tokens, and has sbx
   create the sandbox.
3. **You run the agent** with `unibox run` in a repository. unibox puts
   the repository into the sandbox and starts the agent on it, as a task
   you can resume.

unibox only drives sbx. The env files are plain sbx, and the sandbox is an
ordinary sbx sandbox.

## Terms

| Term | What it is | See it with |
| --- | --- | --- |
| env | the sandbox as one or more sbxenv files of the env repository say it, under a name such as `dev` | `unibox env ls` |
| kit | a package the env pulls in: an image, tools, network rules, the tokens it needs. The team's, not yours to edit | `unibox secret ls` shows what kits need |
| sandbox | a micro-VM sbx creates from an env. It keeps its disk until removed, stops when its last session ends and starts on the next run | `unibox sandbox ls` |
| task | one piece of work: your repositories at `/work/<task>/<repo>` in a sandbox and the agent running there. Several tasks may share a sandbox | `unibox task ls` |
| agent | claude, codex, copilot or opencode, as the env's kit installs it. unibox marks the task's directory trusted and adds a `Co-authored-by` line for the agent to its commits; you stay the author | |

## Where things live

| What | Where |
| --- | --- |
| the sandbox definition | the env repository (the team's) |
| your values for the env's args, your resources | `~/.config/unibox/vars.toml` |
| your envs, ports, network rules, settings | `~/.config/unibox/config.yaml` |
| tasks and sandboxes unibox made | `~/.config/unibox/state.json` |
| tokens | sbx's store on the host (the keychain on macOS) |

## What the agent can reach

| | |
| --- | --- |
| your files | only the repositories a task mounts |
| your tokens | no; the sandbox sees placeholders, which sbx's proxy replaces on requests to the hosts the env allows |
| the internet | the hosts the env and your [network rules](./network) allow |
| the forge | yes, with the env's token, on any branch the token may push to |

[Security](/reference/security) has the details.
