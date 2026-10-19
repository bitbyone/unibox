# sandbox.yaml

Every key a profile's `sandboxes/<name>/sandbox.yaml` may hold, in the order
unibox lays them out. An unknown key is an error, at any level.

## Top-level keys

| Key | Type | Meaning |
| --- | --- | --- |
| `version` | integer | the profile's format; first in the file. Absent means format 0 |
| `template` | string | `<template>/<profile>` it was copied from, written by `init --from` |
| `image` | string | an image built elsewhere; nothing is built. Without it, the image builds from `sandbox.dockerfile` |
| `resources` | mapping | `cpus` and `memory` for each sandbox |
| `agents` | list or mapping | the agents the image has, in order |
| `instructions` | list | shared text names, or `.md` paths relative to `sandbox.yaml` |
| `start` | path | a script run in the VM once per boot |
| `scripts` | mapping | name to script path, run only when asked for |
| `git` | mapping | `author` and `co_author` |
| `services` | mapping | name to a forge or host service |
| `network` | mapping | `allow`, `deny` and `ports` |
| `env` | list | every session's variables: a host variable by name, or `NAME: value` |
| `build_args` | mapping | build arguments for every runtime |
| `caches` | list | host directories mounted read-only |
| `vars` | mapping | name to what it is for: values each person sets for themselves |

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
| `args` | a list of arguments added after those unibox starts the agent with |
| `args_override` | `true` to start the agent with `args` alone, without unibox's arguments or the environment granting the same (default `false`) |

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
    command: secret:artifactory
    refresh: 30m
env:
  - ARTIFACTORY_TOKEN: ${services.artifactory}
```

| Key | Value |
| --- | --- |
| `git` | `true` to wire the sandbox's git to the hosts |
| `host` | a host or a list; a git service's may carry a path, `github.com/some-org` |
| `scheme` | a git service's: `basic`, `bearer` or `raw` - how git's `Authorization` header is formed |
| `user` | a git service's: the user paired with the token, for `basic`; may be `${vars.<name>}` |
| `command` | `secret:<name>`, or a host command that prints the token |
| `refresh` | how often the runtime fetches the token again; on demand when absent |
| `author` | who commits in the service's repositories, `Name <email>`; may be `${vars.<name>}` |
| `co_author` | `false` for no agent trailer in its repositories |

A service holds one token for HTTP requests to its hosts. git signs in through
a git service by itself; a tool finds another service's placeholder in a
variable `env` sets to `${services.<name>}`, and sends it in a header.

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
| `ports` | name to a mapping, or to `{port: <mapping>, default: on\|off}` |

A port mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, as
`unibox network port set --map` takes it. A mapping alone is published at
every start.

### `vars`, `env` and `build_args`

```yaml
vars:
  artifactory-user: your Artifactory user name
  gitlab-2n-author: you on gitlab.2n.cz, as Name <email>
  npm-mirror: your npm mirror
services:
  gitlab-2n: {git: true, host: gitlab.2n.cz, user: oauth2, command: secret:gitlab-2n, author: '${vars.gitlab-2n-author}'}
  artifactory-2n: {host: artifactory.2n.cz, command: secret:artifactory-2n}
env:
  - SPRING_PROFILES_ACTIVE
  - ORG_GRADLE_PROJECT_artifactoryUser2N: ${vars.artifactory-user}
  - ORG_GRADLE_PROJECT_artifactoryPassword2N: ${services.artifactory-2n}
build_args:
  NPM_MIRROR: ${vars.npm-mirror}
```

`vars` are what each person sets for themselves, by a name of their own with
what each is for: lower-case letters, digits and hyphens, and never a secret
(a name that says one is refused; a token stays `secret:<name>`). The values
are not in the profile: each person keeps theirs in
`~/.config/unibox/vars.yaml` with `unibox var set`. A var reaches nothing on
its own; the profile refers to it as `${vars.<name>}`.

`env` is every session's variables - an agent's, a shell's, the scripts' and
`start`'s. An item is a host variable by name, whose value passes in, or
`NAME: value`, where the value may refer to `${vars.<name>}` and to
`${services.<name>}`, the placeholder of a service's token.

`build_args` are passed to the build for every runtime, before a runtime's
`.<runtime>/build.args`; a value may refer to `${vars.<name>}`, and a change
of it builds the image again. A build argument stays in the image's history,
so it is never a secret.

A run or a build stops while a var it refers to has no value; a var nothing
refers to, and a service no variable carries, is told as a warning.

### `caches`

```yaml
caches:
  - host: ~/.gradle/caches
    env: GRADLE_RO_DEP_CACHE
```

Each cache is a host directory mounted read-only at its absolute path, its
path given to the sandbox in `env`.

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
| 5 | 6 | A service lost `env`: every variable is set in the profile's `env`, a service's placeholder as `NAME: ${services.<name>}`, and a service without git lost `scheme` and `user`. `vars` and `build_args` arrived |

A released migration is frozen: it names the files of its own time.
