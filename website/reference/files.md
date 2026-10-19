# Files on Disk

## On the host

unibox's home follows the XDG variables: `$XDG_CONFIG_HOME/unibox` and
`$XDG_CACHE_HOME/unibox`, which default to the paths below.

| Path | What it holds |
| --- | --- |
| `~/.config/unibox/vars.yaml` | your own values for what profiles ask of you (`vars`): shared ones, and each profile's over them |
| `~/.config/unibox/config.yaml` | your choices: the environments repository, the default runtime, the runtimes' settings |
| `~/.config/unibox/state.json` | what use taught: host access last seen per profile, builds, sandboxes, tasks, runs |
| `~/.config/unibox/herdr-plugin/` | the herdr plugin's manifest, when installed |
| `~/.cache/unibox/sbx/` | generated for sbx: build contexts, kits; never edited, regenerated |
| `~/.cache/unibox/msb/<sandbox>/` | an msb sandbox's configuration and the commands that fetch its secrets; never their values |
| `~/.microsandbox/` | the microsandbox runtime unibox installed, its images and sandboxes |
| `~/.config/sbx/credentials.yaml` | sbx's own file; unibox adds the bindings a multi-agent sandbox needs, and never removes one |

### `config.yaml`

```yaml
environments: ~/workspace/team/agent-sandboxes
default_runtime: sbx
runtimes:
  sbx:
    on: true
    stop_idle: true
```

A runtime not named is on when installed. Change it with `unibox config`
rather than by hand:

```sh
unibox config set environments ~/my-envs
unibox config set runtimes.sbx.stop_idle false
unibox config get
```

### Secrets

Tokens are not in any of these files. They are in the platform's store - the
login keychain on macOS, the Secret Service on Linux - or in the runtime's
own store for the logins it keeps. `unibox secret ls` lists their names.

### Images

Built images live in Docker and in the runtime's own image store, tagged
`unibox/<profile>:<revision>`, a digest of what the image is built from. `unibox prune` removes those no longer needed.

## In the environments repository

```
my-envs/
├── sandboxes/
│   └── <profile>/
│       ├── sandbox.yaml
│       ├── sandbox.dockerfile
│       ├── start.sh
│       ├── instructions.md
│       ├── _data/<agent>/<file>
│       └── .sbx/build.args
├── _shared/
│   ├── @install/
│   └── @instructions/
├── .shared/
│   ├── @install/
│   └── @instructions/
└── README.md
```

`.shared/` is unibox's and rewritten on `init`, `task check`, `profile build` and a start;
`.templates/<template>/` is a template's, written by `init --from` and
`template update`; everything else is yours. See
[The Environments Repository](/guide/environments-repository).

## In the image

| Path | What it is |
| --- | --- |
| `/usr/local/bin/@install` | the installation runner, from `COPY --from=unibox / /` |
| `/usr/local/lib/unibox/install/` | the installation scripts, `.shared/@install` with `_shared/@install` over it |

## In the sandbox

| Path | What it is |
| --- | --- |
| `/work/` | where tasks live |
| `/work/<task>/` | a task's directory; the agent starts here |
| `/work/<task>/<repo>/` | a repository's clone, or a mounted host directory with `--mount` |
| `/var/lib/unibox/started` | the boot id `start` last ran for |
| `/var/lib/unibox/start.sh` | the profile's `start`, as it runs |
| `/var/lib/unibox/scripts/` | the profile's scripts, as they run |
| `/var/lib/unibox/runs/` | a mark for each running session, so an idle sandbox can be told |
| `/var/lib/unibox/mounted/` | bookkeeping for mounted directories |
| `~/.config/unibox/identities/` | commit identities of the profile's git services, included by git |

The agents' instructions go into the sandbox's home, never into `/work`:

| Agent | Instructions | Configuration merged before every session |
| --- | --- | --- |
| `claude` | `~/.claude/CLAUDE.md` | `~/.claude.json`, `~/.claude/settings.json` |
| `codex` | `~/.codex/AGENTS.md` | `~/.codex/config.toml` |
| `copilot` | `~/.copilot/copilot-instructions.md` | `~/.copilot/settings.json`, `~/.copilot/config.json` |
| `opencode` | `~/.config/opencode/AGENTS.md` | `~/.config/opencode/opencode.json` |
