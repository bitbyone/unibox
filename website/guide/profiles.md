# Profiles

A profile is `sandboxes/<name>/sandbox.yaml` and the files beside it. It
describes an environment and **names no runtime**: where it runs is chosen at
the start, and the runtime's adapter turns the profile into the runtime's own
means.

## A walkthrough

```yaml
version: 5
resources: {cpus: 4, memory: 50%}    # fixed, or a share of the host's
agents:                              # or a list: [claude, codex]
  claude:
    auth: {token: secret:claude}     # unibox agent login
    co_author: Claude <noreply@anthropic.com>
    config:                          # seeds merged before every session
      .claude.json: _data/claude/.claude.json
      settings.json: _data/claude/settings.json
  codex:
    auth: oauth                      # provider login kept by the runtime
    config:
      config.toml: _data/codex/config.toml
instructions: [unibox]               # _shared/@instructions, then .shared/@instructions
start: start.sh                      # in the VM, at every boot of it
scripts:                             # run only when asked for, by name
  update-skills: update-skills.sh
git:
  author: user                       # your identity, from the host's git
  co_author: true                    # a Co-Authored-By trailer for the agent
services:
  github: {git: true, host: github.com, user: x-access-token, command: secret:github}
network:
  allow: [jitpack.io]                # hosts to allow
  deny: [gist.github.com]            # hosts to block
  ports:                             # published on the host's loopback, by name
    fe: '3000:3000'                  # at every start
    devtools: {port: '9333:9222', default: off}  # only when switched on
env: [SPRING_PROFILES_ACTIVE]        # host variables passed in by name
caches:                              # host directories mounted read-only
  - {host: ~/.gradle/caches, env: GRADLE_RO_DEP_CACHE}
```

### `version`

The profile's format, the first key. A unibox reads one format -
`unibox version --json` names it - and refuses another: an older one with
`run unibox init <profile> to migrate it`, a newer one with `upgrade unibox`. See
[migrations](/reference/sandbox-yaml#format-versions).

### The image

The image builds from `sandbox.dockerfile` beside `sandbox.yaml`; the profile
does not name it. `image: <ref>` names one built elsewhere - a team's CI -
instead, and then nothing is built. A profile with both, or with neither, is
refused. See [Images](./images).

### `resources`

CPUs and memory for each sandbox, each **fixed** (`4`, `8g`) or a **share of
the host's** (`1%` to `100%`). A share is resolved when a sandbox is created
and the record keeps the share, so the same profile fits a laptop and a
workstation. sbx gives a sandbox at most 75% of the host's memory; unibox caps
a larger share there and says so rather than fail.

### `agents`

The agents the image has, in order. A list of names will do; a mapping says,
for each, how it signs in (`auth`), how it is credited in commits
(`co_author`) and the seeds of its own configuration files (`config`). The
order matters: the runtime signs one agent of a sandbox in by OAuth, the
first that declares one. See [Agents](./agents).

### `instructions`

Names of shared texts (`_shared/@instructions`, then `.shared/@instructions`),
or paths of `.md` files relative to `sandbox.yaml`. The profile's own
`instructions.md` attaches itself after them. See
[Scripts and Instructions](./scripts-and-instructions).

### `start` and `scripts`

`start` runs in the VM once per boot. `scripts` run only when asked for, by
name. Both are files of the profile. See
[Scripts and Instructions](./scripts-and-instructions).

### `git`

Who commits. `author: user` takes your `user.name` and `user.email` from the
host's git configuration of the repository; `--author` on a run wins.
`co_author: true` adds a `Co-Authored-By` trailer naming the agent of the
session, `false` adds none. Absent, `git` means both as shown.

### `services`

Forges and other host services the sandbox reaches, each with a token kept on
the host. Written by `unibox service add`. See
[Services and Secrets](./services-and-secrets).

### `network`

Hosts allowed beyond the runtime's policy, hosts denied (deny wins), and
ports published on the host's loopback by name. All applied live. See
[Network and Ports](./network-and-ports).

### `env` and `caches`

`env` passes host environment variables in by name; their values come from
the environment unibox runs in. `caches` mounts host directories
**read-only**, named inside by a variable - `~/.gradle/caches` as Gradle's
read-only dependency cache, say: what the host already has is not downloaded
again, and what is missing goes to the sandbox's own cache. Read-write would
let the agent plant a file the host's next build runs.

## Strict by design

An unknown field is an error naming it, never skipped: a misspelt `memmory`
must not quietly change the environment. A profile that does not parse is
listed with its error by `unibox profiles` and cannot be started.

## Yours to edit, by hand or by command

The profile is yours. Commands that change it - `unibox agent`,
`unibox service`, `unibox network`, `unibox init` - edit the YAML as a node
tree, so your comments and order stay, and a hand edit is as valid as
theirs.

Each write lays the file out the same way: the sections it has in a fixed
order, each under a line on what it does and which command changes it, then,
commented out, only the sections it does not have yet - recomputed every
time, so nothing unibox writes lands under examples of itself. Your own
comment over a section stays.

## When a profile changes

Nothing asks you to approve your own profile. When what it asks of the host
changes - environment variables, mounted caches, logins, services, network
rules - `unibox check` names it as a warning, and the next start says it once
and what it leads to:

- an image built from changed inputs is built without a question; `unibox
  diff <profile>` shows what changed;
- a sandbox made from changed settings is recreated, naming them, after the
  usual checks for running sessions and unpushed work;
- what the runtime can change live - network rules and ports - is applied to
  running sandboxes instead.

A change made through unibox's own commands is not told again.

## Completing and migrating

`unibox init <profile>` on a profile that exists completes it to what `init`
writes today - its agents' seeds and `config` entries, how each is credited,
the layout - and changes nothing it already says. Its agents stay its own:
`--agents` that differ are refused. On a profile of an older format, it
migrates it first, format by format; what it cannot carry over it names.

## Per-runtime build differences

`sandbox.yaml` says nothing about a runtime, but an image may need building a
little differently for one. That is said with Docker's own means, build
arguments, from a hidden directory named after the runtime: every
`KEY=VALUE` line of `.sbx/build.args` is passed as a `--build-arg` when
building for sbx, and `UNIBOX_RUNTIME=sbx` always is. See
[Images](./images#build-arguments).
