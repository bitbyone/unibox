# unibox

Coding agents in sandboxes. A profile - a Dockerfile, resources, the agents
its image has, how they sign in and what they may reach - lives in an
environments repository of the user's; unibox builds it, creates a sandbox
from it on a runtime (Docker Sandboxes, `sbx`, today), and prepares tasks
inside: independent clones of the repositories worked on, on a branch of
their own, with the agent run in the terminal. The machine, its home and
its keys stay out of the agent's reach; logins stay on the host and reach
the sandbox through the runtime's proxy.

It is a program of its own and usable alone; unagit is one UI over it.

## Install

```sh
make install          # into ~/.local/bin
```

`sbx` must be installed and signed in (`sbx login`).

## Use

```sh
unibox init personal --agents claude,codex,copilot   # lay out a profile
unibox build personal                                # build its image
unibox run --profile personal --repo . --branch feat/x
unibox tasks                                         # every sandbox's tasks
unibox resume <task>                                 # run its agent again
unibox recreate personal                             # after a profile change
```

`unibox --help` lists every command; `--json` gives versioned output for
other programs.

## Profiles

`sandboxes/<name>/sandbox.yaml` in the environments repository
(`unibox config set environments <path>`):

```yaml
dockerfile: Dockerfile
cpus: 4
memory: 8g
agents: [claude, codex, copilot]
instructions: [unibox]
auth:                                  # how each agent signs in
  codex: oauth                         # the ChatGPT login sbx keeps
  claude: {token: security find-generic-password -s claude-code-oauth-token -w}
  copilot: {command: gh auth token}
allow_network: [jitpack.io]            # beyond the runtime's policy
deny_network: [gist.github.com]        # deny wins
```

What a profile asks of the host - environment, caches, network rules,
logins - is approved once (`unibox approve`) and again when it changes.
The design, its reasons and what was verified about sbx are in
[docs/design.md](docs/design.md).
