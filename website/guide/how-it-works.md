# How it Works

```
 env repository (git)        your machine                 sandbox (micro-VM)
 dev.sbxenv.yaml        -->  unibox: envs, your values,    /work/<task>/<repo>
 kits/                       tasks, ports, network rules   (mounted or cloned)
                             sbx: the VM, your tokens,  -> the agent
                             proxy, network policy
```

**Env.** A name for one or more sbxenv files from the env repository,
such as `dev`. `unibox init` creates envs; `unibox env ls` lists them.

**Kit.** A package the sbxenv file pulls in: an image, tools, network
rules, the credentials it needs. One kit, the workload, runs the agent.
Kits are the team's; you do not edit them.

**Sandbox.** A micro-VM sbx creates from an env. It has its own disk,
Docker and network policy, and it persists until you remove it. It stops
when its last session ends and starts again on the next `unibox run`.
Several tasks can share one sandbox, with no isolation between them.

**Task.** One piece of work: one or more of your repositories at
`/work/<task>/<repo>` in a sandbox, and the agent running in that
directory. A task name is unique across sandboxes, so
`unibox resume <task>` finds its sandbox.

**Agent.** claude, codex, copilot or opencode, as the env's kit installs
and starts it. unibox runs it in the task's directory, marks that
directory trusted so the agent does not stop to ask, and adds a
`Co-authored-by` line for it to each commit. You stay the author.

## Where things live

| What | Where |
| --- | --- |
| the sandbox definition | the env repository (the team's) |
| your values for the env's args | `~/.config/unibox/vars.toml` |
| your envs, ports, network rules, settings | `~/.config/unibox/config.yaml` |
| tasks and sandboxes unibox made | `~/.config/unibox/state.json` |
| tokens | sbx's store on the host (the keychain on macOS) |

Tokens never enter a sandbox. Inside it, each token is a placeholder; the
sbx proxy on your machine replaces it with the real value on requests to
the hosts the env allows for that token.

## What the agent can reach

- **Your machine:** no, except the directories a task mounts.
- **Your tokens:** no, only placeholders.
- **The internet:** only the hosts the env and your
  [network rules](./network) allow.
- **The forge:** yes, with the env's token. It can push to any branch that
  token can.

[Security](/reference/security) has the full list.
