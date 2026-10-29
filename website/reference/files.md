# Files on Disk

## On the host

Paths follow `XDG_CONFIG_HOME`, `XDG_CACHE_HOME` and `XDG_DATA_HOME`, and
default to these, on Windows too:

| Path | Holds |
| --- | --- |
| `~/.config/unibox/config.yaml` | envs, named ports, network rules, default env, context, settings |
| `~/.config/unibox/vars.toml` | your values for env args, and resources (mode `0600`) |
| `~/.config/unibox/state.json` | sandboxes and what each was created from, their tasks, the directories whose mount was told |
| `~/.config/unibox/state.v1.json` | Unibox 1's state, set aside |
| `~/.config/unibox/state.lock`, `locks/` | locks between concurrent unibox commands |
| `~/.config/unibox/herdr-plugin/` | the herdr plugin, when installed |
| `~/.cache/unibox/args/` | the args and resources files for one `sbx env create`, removed after |
| `~/.cache/unibox/kits/` | what `sbx kit inspect` said of the env's kits |
| `~/.local/share/unibox/repos/<host>/<path>` | env repositories `unibox init` cloned; `destroy --all` keeps them |
| `~/.local/share/unibox/conversations/<task>.tar.gz` | the task's agents' conversations, kept after each session and before a recreate (mode `0600`); removed with the task |

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
    repos:                        # yours for this env, over the team's
      adrs: {default: off}        # only switches a team's repository
    secrets:                      # tokens of this env's sandboxes alone, read by a command
      copilot: {command: security find-generic-password -s copilot-2n -w}
      github:                     # typed into the keychain by unibox secret set --scope env
        command: security find-generic-password -s 'unibox' -a 'env/dev/github' -w
        keychain: env/dev/github
    instructions: |               # yours for this env's tasks' agents
      Use the staging database.
ports:                            # yours, for every env
  devtools: {port: '19222:9222', default: off}
repos:                            # yours, for every env
  notes: {url: https://github.com/me/notes.git, default: always}
instructions: |                   # yours for every task's agents
  Answer in Czech.
env_repos:                        # yours for every env of one env repository:
  github.com/acme/agent-envs:     # its host and path, or a checkout's path
    repos:                        # ports, repos and instructions, as above
      spec: https://github.com/acme/spec.git
    instructions: |
      Tickets are in Jira project ACME.
sandboxes:                        # one sandbox's own, dropped with it
  personal:
    secrets:                      # its tokens, over its env's and the global ones
      github: {command: pass github-personal}
  dev:
    network:                      # unibox network rules
      - allow: jitpack.io
      - allow: stun.example.com:3478
        protocol: udp
      - deny: gist.github.com
context:
  sandbox: dev
stop_idle: true
ssh_agent: false
editor_ssh: true
remember_host_commands: true      # sbx's settings, set to match at each start
claude_remote_control: true
clipboard_image_paste: true       # agents paste images from your clipboard
```

Edit it through `unibox env`, `config`, `context`, `network`, `port
default` and `repo` (`--env-repo` for `env_repos:`). `instructions:` is
written by hand, and `ports:` may
be ([Ports](/guide/ports#named-ports)): a port is its mapping alone, on
at each start, or `{port, default}`. A repository takes the keys of the
team's (below); one of yours that says only `default` switches the
team's of its name. `remember_host_commands`, `claude_remote_control` and `clipboard_image_paste`
are sbx's settings: unibox sets sbx to match before it creates or starts
a sandbox, so editing them here is enough; left out, sbx's own stand.

### `vars.toml`

```toml
[args]                          # every env that declares them (var set -g)
user = 'jdoe'

[env_repos."github.com/acme/agent-envs".args]   # every env of one env repository (var set --env-repo)
user = 'jdoe-acme'

[envs.dev.args]                 # one env, wins over both
user = 'john'

[resources]                     # every env, by hand
cpus = '100%'
memory = '75%'

[env_repos."github.com/acme/agent-envs".resources]   # every env of one env repository, by hand
cpus = '50%'

[envs.dev.resources]            # one env, by hand
memory = '16g'

[sandboxes.dev-big.resources]   # one sandbox: sandbox create / sandbox resources
cpus = '8'
```

`cpus` is a count, `0` for all, or `1%`-`100%` of this machine; `memory` a
size (`k`, `m`, `g`) or a share. sbx takes no shares, so unibox converts
them at each create and caps memory at sbx's 75% of the host. Values are
one line each; one starting with `secret:` is refused. All of these win
over the team's `resources:` in the env repository's `unibox.yaml` files.

## In the env repository

Besides the sbxenv files and kits, unibox reads two files of the team's
that sbx never does: `unibox.yaml` at the root of the repository, for
every env, and `<env>.unibox.yaml` beside `<env>.sbxenv.yaml`, for that
env alone and over the first. Each may hold:

```yaml
ports:                          # named ports (see Ports)
  vite: {port: 5173, description: the dev server, default: on}
  devtools: {port: '19222:9222', default: off}
secrets:                        # where a service's token is made, for unibox secret set
  gitlab:
    page: https://gitlab.example.com/-/user_settings/personal_access_tokens?name=unibox&scopes=read_repository
    hint: a personal access token with read_repository
  anthropic:
    ways:                         # several ways; unibox asks which
      - {name: Claude subscription, command: claude setup-token, hint: prints a token to paste}
      - {name: API key, page: https://platform.claude.com/settings/keys}
resources:                      # what the env's sandboxes start from
  cpus: 50%
  memory: 16g
repos:                          # repositories a task is given by name
  payments: https://gitlab.example.com/shop/payments.git
  adrs:
    url: https://gitlab.example.com/arch/adrs.git
    ref: main                   # the branch a task works on (default: the forge's default)
    default: always             # always, on or off (default off)
    context: true               # read, kept on its ref, never pushed to (default: true for always)
    description: architecture decisions
    instructions: |             # told to a task's agents when it has this repository
      Read the ADR index before proposing a design.
instructions: |                 # told to every task's agents of the env
  Run make test before you commit.
```

A port is `{port, description, default}`, `default` on or off. A
repository is its address alone, or the keys above; an address that starts
with `/`, `~` or `.` is a directory of the host, mounted rather than
cloned. A task's `AGENTS.md` gathers the `instructions:` of the
repository's `unibox.yaml`, then the env's file, then yours in
`config.yaml` for every env, for its env repository and for the env, then
each of its repositories'. Tokens themselves are in none of these files;
they are in sbx's store. See [Repository index](/guide/repos) and
[Ports](/guide/ports#named-ports).

## In the sandbox

| Path | Is |
| --- | --- |
| `/work/<task>/` | a task; the agent starts here when it has several repositories or instructions, else in its one repository |
| `/work/<task>/AGENTS.md`, `CLAUDE.md` | the task's instructions, written by unibox before each session; `CLAUDE.md` links to `AGENTS.md`; edits are lost |
| `/work/<task>/<repo>/` | a mounted directory or a clone |
| `/var/lib/unibox/runs/` | a mark per running session |
| `/var/lib/unibox/mounted/` | per mounted repository, the git config for author and co-author hook |
| `<clone>/.git/unibox-hooks/` | a clone's co-author hook (`core.hooksPath`) |

Before each session unibox merges trust of the task's directories into
`~/.claude.json`, `~/.codex/config.toml` and `~/.copilot/config.json`,
leaving the rest of each file alone.
