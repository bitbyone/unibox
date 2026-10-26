# Files on Disk

## On the host

Paths follow `XDG_CONFIG_HOME`, `XDG_CACHE_HOME` and `XDG_DATA_HOME`, and
default to these, on Windows too:

| Path | Holds |
| --- | --- |
| `~/.config/unibox/config.yaml` | envs, named ports, network rules, default env, context, settings |
| `~/.config/unibox/vars.toml` | your values for env args, and resources (mode `0600`) |
| `~/.config/unibox/state.json` | sandboxes and what each was created from, tasks, runs |
| `~/.config/unibox/state.lock`, `locks/` | locks between concurrent unibox commands |
| `~/.config/unibox/herdr-plugin/` | the herdr plugin, when installed |
| `~/.cache/unibox/args/` | the args file for one `sbx env create`, removed after |
| `~/.local/share/unibox/repos/<host>/<path>` | env repositories `unibox init` cloned; `destroy --all` keeps them |

In the env repository, besides sbxenv files and kits, unibox reads the
team's named ports: `unibox.yaml` at the root (every env) and
`<env>.unibox.yaml` beside `<env>.sbxenv.yaml`. Tokens are in none of
these files; they are in sbx's store.

### `config.yaml`

```yaml
default_env: dev
envs:
  dev:
    files:
      - /home/me/.local/share/unibox/repos/github.com/acme/agent-envs/dev.sbxenv.yaml
    name: dev                     # the sandbox's name
    repo: /home/me/.local/share/unibox/repos/github.com/acme/agent-envs
    source: git@github.com:acme/agent-envs.git   # set when init cloned it
    ports:
      api: '8080:8080'
      vite: {default: on}         # a team port, switched on for you
ports:                            # yours, for every env
  devtools: {port: '19222:9222', default: off}
sandboxes:                        # unibox network rules
  dev:
    network:
      - allow: jitpack.io
      - allow: stun.example.com:3478
        protocol: udp
      - deny: gist.github.com
context:
  sandbox: dev
stop_idle: true
ssh_agent: false
```

Edit it through `unibox env`, `config`, `context` and `network`. Only
`ports:` is written by hand ([Ports](/guide/ports#named-ports)).

### `vars.toml`

```toml
[args]                          # every env that declares them (var set -g)
user = 'jdoe'

[envs.dev.args]                 # one env, wins over [args]
user = 'john'

[resources]                     # every env, by hand
cpus = '100%'
memory = '75%'

[envs.dev.resources]            # one env, by hand
memory = '16g'

[sandboxes.dev-big.resources]   # one sandbox: sandbox create / sandbox resources
cpus = '8'
```

`cpus` is a count, `0` for all, or `1%`-`100%` of this machine; `memory` a
size (`k`, `m`, `g`) or a share. sbx takes no shares, so unibox converts
them at each create and caps memory at sbx's 75% of the host. Values are
one line each; one starting with `secret:` is refused.

## In the sandbox

| Path | Is |
| --- | --- |
| `/work/<task>/` | a task; the agent starts here when it has several repositories |
| `/work/<task>/<repo>/` | a mounted directory or a clone |
| `/var/lib/unibox/runs/` | a mark per running session |
| `/var/lib/unibox/mounted/` | per mounted repository, the git config for author and co-author hook |
| `<clone>/.git/unibox-hooks/` | a clone's co-author hook (`core.hooksPath`) |

Before each session unibox merges trust of the task's directories into
`~/.claude.json`, `~/.codex/config.toml` and `~/.copilot/config.json`,
leaving the rest of each file alone.
