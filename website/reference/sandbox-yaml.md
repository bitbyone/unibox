# sandbox.yaml

Every key a profile's `sandboxes/<name>/sandbox.yaml` may hold, in the order
unibox lays them out. An unknown key is an error, at any level.

## Top-level keys

| Key | Type | Meaning |
| --- | --- | --- |
| `version` | integer | the profile's format; first in the file. Absent means format 0 |
| `image` | string | an image built elsewhere; nothing is built. Without it, the image builds from `sandbox.dockerfile` |
| `resources` | mapping | `cpus` and `memory` for each sandbox |
| `agents` | list or mapping | the agents the image has, in order |
| `instructions` | list | shared text names, or `.md` paths relative to `sandbox.yaml` |
| `start` | path | a script run in the VM once per boot |
| `scripts` | mapping | name to script path, run only when asked for |
| `git` | mapping | `author` and `co_author` |
| `services` | mapping | name to a forge or host service |
| `network` | mapping | `allow`, `deny` and `ports` |
| `env` | list | host environment variables passed in by name |
| `caches` | list | host directories mounted read-only |

`env`, `caches`, the agents' `auth`, `services` and `network` are **host
access**: what the profile asks of the host. A change of them is told at the
next start.

### `image`

```yaml
image: registry.example.com/team/agents:2026-10
```

A profile has either `image:` or a `sandbox.dockerfile`, never both and never
neither. A `Dockerfile` left in the profile's root is named for renaming.

### `resources`

```yaml
resources: {cpus: 4, memory: 8g}
resources: {cpus: 50%, memory: 50%}
```

| Key | Value |
| --- | --- |
| `cpus` | a number of CPUs, or a share of the host's from `1%` to `100%` |
| `memory` | a size such as `8g`, or a share of the host's from `1%` to `100%` |

For sbx, a share of CPUs is a rounded count, at least one, and `100%` is all
the host's CPUs. A share of memory is resolved from the host's memory, and
capped at sbx's limit of 75%, with a note. msb takes the count and the size
as they resolve.

### `agents`

A list of names:

```yaml
agents: [claude, codex]
```

or a mapping of names to what the profile says of each:

```yaml
agents:
  claude:
    auth: {token: secret:claude}
    co_author: Claude <noreply@anthropic.com>
    config:
      .claude.json: _data/claude/.claude.json
      settings.json: _data/claude/settings.json
```

The names are `claude`, `codex`, `copilot` and `opencode`. The order is kept:
the runtime signs a sandbox in by the OAuth of the first agent that declares
one.

| Key | Value |
| --- | --- |
| `auth` | `oauth`, `api-key`, `{command: <cmd>}` or `{token: <cmd>}`; `<cmd>` is `secret:<name>` or a host command |
| `co_author` | `Name <email>`, or `Name <>` for no address |
| `config` | the agent's file name to a seed's path in the profile |

The files a `config` may name:

| Agent | Files |
| --- | --- |
| `claude` | `.claude.json`, `settings.json` |
| `codex` | `config.toml` |
| `copilot` | `settings.json`, `config.json` |
| `opencode` | `opencode.json` |

Two agents with `auth: oauth` in one profile are refused.

### `instructions`

```yaml
instructions: [unibox, review-style, ../../notes/style.md]
```

A bare name is `_shared/@instructions/<name>.md`, or else
`.shared/@instructions/<name>.md`. An entry with a path or extension is a
`.md` file relative to `sandbox.yaml`, inside the repository. The profile's
`instructions.md` is added after them on its own. A missing one is an error.

### `start` and `scripts`

```yaml
start: start.sh
scripts:
  update-skills: update-skills.sh
  reset-db: reset-db.sh
```

Paths relative to the profile. See
[Scripts and Instructions](/guide/scripts-and-instructions).

### `git`

```yaml
git:
  author: user
  co_author: true
```

| Key | Value |
| --- | --- |
| `author` | `user`: your identity from the host's git configuration (the only value) |
| `co_author` | `true` adds a `Co-Authored-By` trailer for the session's agent; `false` adds none |

Absent, both are as shown.

### `services`

```yaml
services:
  github:
    git: true
    host: github.com
    user: x-access-token
    command: secret:github
  artifactory:
    host: [artifactory.example.com, "*.jfrog.io"]
    scheme: bearer
    env: ARTIFACTORY_TOKEN
    command: secret:artifactory
    refresh: 30m
```

| Key | Value |
| --- | --- |
| `git` | `true` to wire the sandbox's git to the hosts |
| `host` | a host or a list; a git service's may carry a path, `github.com/some-org` |
| `scheme` | `basic`, `bearer` or `raw` - how the `Authorization` header is formed |
| `user` | the user paired with the token, for `basic` |
| `env` | the variable a tool reads the placeholder from |
| `command` | `secret:<name>`, or a host command that prints the token |
| `refresh` | how often the runtime fetches the token again; on demand when absent |
| `author` | who commits in the service's repositories, `Name <email>` |
| `co_author` | `false` for no agent trailer in its repositories |

Exactly one of `git` or `env` is set.

### `network`

```yaml
network:
  allow: [jitpack.io]
  deny: [gist.github.com]
  ports:
    fe: '3000:3000'
    api: '8080'
    devtools: {port: '9333:9222', default: off}
```

| Key | Value |
| --- | --- |
| `allow` | patterns the runtime's policy would refuse: a host, `*.domain`, an IP or a CIDR, with an optional port |
| `deny` | patterns it would allow; deny wins |
| `ports` | name to a spec, or to `{port: <spec>, default: on\|off}` |

A port spec is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. A spec alone is
published at every start.

### `env` and `caches`

```yaml
env: [SPRING_PROFILES_ACTIVE]
caches:
  - host: ~/.gradle/caches
    env: GRADLE_RO_DEP_CACHE
```

`env` names host variables whose values pass in. Each cache is a host
directory mounted read-only at its absolute path, its path given to the
sandbox in `env`.

## Files beside it

| File | Meaning |
| --- | --- |
| `sandbox.dockerfile` | the image, unless `image:` is set |
| `instructions.md` | the profile's own instructions, attached after the listed ones |
| `start.sh`, scripts | what `start` and `scripts` name |
| `_data/` | seeds and other files the profile refers to, by convention |
| `.sbx/build.args` | `KEY=VALUE` build arguments when building for sbx |
| `.msb/build.args` | the same, when building for msb |

## Format versions

A unibox reads exactly one profile format, named by `unibox version --json`
(`profile_format`).
Loading never migrates: a profile of an older format is refused with "run
unibox init &lt;profile&gt; to migrate it", one of a newer format with
"upgrade unibox".

`unibox init <profile>` migrates on the YAML, one format at a time, keeping
comments. It is best effort: what has no place in the new format is left out
and named, and a result that does not load is not kept.

| From | To | What changed |
| --- | --- | --- |
| 0 | 1 | Everything about an agent moved under it. Format 0 said it in three sections - `auth`, `git.co_authors` and `agent_config` keyed `<agent>.<file>` - which become each agent's `auth`, `co_author` and `config`; seeds move to `_data/<agent>/<file>`. Format 0 had no `version` |
| 1 | 2 | Top-level `cpus` and `memory` moved under `resources`, which also took shares of the host's |
| 2 | 3 | `allow_network` and `deny_network` moved under `network` as `allow` and `deny`; `network.ports` arrived, ports published by name |
| 3 | 4 | `bootstrap` - a script each worked-on repository was to carry - was dropped, and the migration says so |
| 4 | 5 | `dockerfile:` was dropped: the image builds from `sandbox.dockerfile` beside `sandbox.yaml`, and the file the profile named is renamed to it |

A released migration is frozen: it names the files of its own time.
