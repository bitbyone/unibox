# Files on Disk

## On the host

unibox's home follows the XDG variables: `$XDG_CONFIG_HOME/unibox`,
`$XDG_CACHE_HOME/unibox` and `$XDG_DATA_HOME/unibox`, which default to the
paths below, under your home directory on Windows too.

| Path | What it holds |
| --- | --- |
| `~/.config/unibox/config.yaml` | your choices: the envs, their sandbox names and named ports, the default env, the context, `stop_idle`, `ssh_agent` |
| `~/.config/unibox/vars.toml` | your own values for the args envs declare, and the CPUs and memory your sandboxes get |
| `~/.config/unibox/state.json` | what use taught: sandboxes and what each was created from, tasks, runs, the directories you were told about mounting |
| `~/.config/unibox/state.lock`, `locks/` | locks that keep two unibox commands from changing the same thing at once |
| `~/.config/unibox/herdr-plugin/` | the herdr plugin's manifest, when installed |
| `~/.cache/unibox/args/` | the args file for one `sbx env create`, readable only by you and removed after it |
| `~/.local/share/unibox/repos/<host>/<path>` | the team repositories `unibox init` cloned from a git address, kept current by `unibox env update`; `destroy --all` leaves them |

The team's sbxenv files and kits are wherever the team keeps them; unibox
records their paths and never writes them. Beside an sbxenv file, the team
may keep the [ports it names](/guide/ports#the-team-s-ports) in a file sbx
does not read: `dev.sbxenv.yaml` has `dev.unibox.yaml`. Tokens are in none of these
files: they are sbx's, in its own store (the keychain on macOS).

### `config.yaml`

```yaml
default_env: dev
envs:
  dev:
    files:
      - /home/me/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml
      - /home/me/team/agent-sandboxes/sandboxes/local.sbxenv.yaml
    name: dev-mine
    repo: /home/me/.local/share/unibox/repos/github.com/team/agent-sandboxes
    source: git@github.com:team/agent-sandboxes.git
    ports:
      web: 3000:5173
      storybook: {port: '6006:6006', default: off}
      vite: {default: on}     # a port of the team's, switched on for you
context:
  sandbox: dev-mine
stop_idle: true
ssh_agent: false
```

`repo` is the repository init found the files in, and `source` the
address it cloned it from; only a repository with a `source` is pulled by
`unibox env update`. A port with no mapping of its own is one of the team's,
switched by `unibox port on` or `off`.

Change it with unibox's commands rather than by hand - `unibox env`,
`unibox port`, `unibox context`, `unibox config`:

```sh
unibox config set default_env dev
unibox config set stop_idle false
unibox config get
```

### `vars.toml`

```toml
[args]                 # for every env that declares them
user = 'jdoe'

[resources]            # for every env
cpus = '100%'
memory = '75%'

[envs.dev.args]        # one env's own, over the ones above
user = 'john'

[envs.dev.resources]
memory = '16g'
```

Kept with mode `0600`. Args are changed with `unibox var set` and
`unibox var rm` (`-g` for `[args]`); values are one line each, and one
beginning with `secret:` is refused: a token is a
[login](/guide/logins).

Resources are written by hand. `cpus` is a count, `0` for all of them, or
a share of this machine's from `1%` to `100%`; `memory` a size (`k`, `m`,
`g`) or a share. They are read at each create, as described in
[Envs](/guide/envs#resources).

### Coming from unibox 1

unibox 2 reads a unibox 1 `config.yaml` and keeps what still means something
- `runtimes.sbx.stop_idle` and `runtimes.sbx.ssh_agent` become `stop_idle`
and `ssh_agent` - and drops the rest at its next write. A unibox 1 `state.json` is set aside as `state.v1.json`, with a
notice naming the old sandboxes to remove with `sbx rm`; unibox 2 does not
manage them.

## In the sandbox

| Path | What it is |
| --- | --- |
| `/work/` | where tasks live |
| `/work/<task>/` | a task's directory; the agent starts here when the task has several repositories |
| `/work/<task>/<repo>/` | a mounted host directory, or a clone from the forge |
| `/var/lib/unibox/runs/` | a mark for each running session, so an idle sandbox can be told |
| `/var/lib/unibox/mounted/` | per mounted repository, the git configuration that sets its author and co-author hook |
| `<clone>/.git/unibox-hooks/` | a clone's co-author hook, named by its `core.hooksPath` |

Before every session unibox merges trust of the task's directories into the
agent's own configuration in the sandbox's home - `~/.claude.json`,
`~/.codex/config.toml`, `~/.copilot/config.json` - and leaves the rest of it
as it is. See [Agents](/guide/agents).
