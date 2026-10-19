# Images

A profile is one image holding every agent it offers. Everything static goes
into it - toolchains, the agents' command lines, certificates. What depends on
the checkout - a project's dependencies, generated code - is the agent's to
do, told by its instructions.

## `sandbox.dockerfile`

`unibox init` writes one like this:

```dockerfile
ARG BASE=ubuntu:24.04
FROM ${BASE}

ARG UNIBOX_RUNTIME
USER root

COPY --from=unibox / /
RUN @install base tools/nvm agents/claude

# The shell template ships an agent user; a plain ubuntu base needs one made.
RUN id agent >/dev/null 2>&1 || useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent
```

It is an ordinary Dockerfile, built through unibox: `COPY --from=unibox`
reads a build context unibox hands the build (below), so a plain `docker
build` of it fails on that line. `init` lists `base`, `tools/nvm` and the
agents you chose; the list is yours to edit.

## `@install`

unibox hands the build a second build context named `unibox`. It holds the
`@install` runner at `/usr/local/bin/@install` and the installation scripts
at `/usr/local/lib/unibox/install` - unibox's from `.shared/@install`, with
yours from `_shared/@install` of the same name over them.
`COPY --from=unibox / /` puts both into the image, and `RUN @install ...`
runs scripts by name, in order:

- `agents/claude` runs `agents/claude.sh`;
- an executable script runs by its own `#!` line (nvm and sdkman need bash),
  any other with `sh`;
- a missing name stops the build, naming it.

The names may also come one per line, from a heredoc:

```dockerfile
RUN @install <<EOF
base
tools/nvm
agents/claude
agents/codex
custom
EOF
```

Blank lines and `#` comments pass. The command is called `@install` because
`install` is coreutils', which builds use.

`.shared/` and `_shared/` are also copied into every profile's build context,
so a Dockerfile can `COPY` what else it needs from them.

### Your own script

Put it in `_shared/@install/`, and name it in the list:

::: code-group

```sh [_shared/@install/tools/psql.sh]
#!/bin/sh
set -eu
apt-get update
apt-get install -y --no-install-recommends postgresql-client
rm -rf /var/lib/apt/lists/*
```

```dockerfile [sandbox.dockerfile]
RUN @install base tools/psql tools/nvm agents/codex custom
```

:::

One named as a script of unibox's - `agents/claude.sh`, say - runs instead of
unibox's.

## Build arguments

Building for runtime X, unibox always passes `--build-arg UNIBOX_RUNTIME=X`,
so a condition needs no file, and every `KEY=VALUE` line of the profile's
`.X/build.args` as a `--build-arg`. `init` writes `.sbx/build.args`:

```
BASE=docker/sandbox-templates:shell
```

so the sbx image starts from Docker's agent-less sandbox base, while a plain
build falls back to `ubuntu:24.04`.

```dockerfile
RUN if [ "$UNIBOX_RUNTIME" = sbx ]; then echo "sbx alone"; fi
```

`build.args` is the one file a runtime directory has; an unknown file there is
an error.

## Building

```sh
unibox profile build web               # build if outdated, check the agents, load
unibox profile build web --rebuild     # build again even when current
unibox profile diff web                # what changed since the last build
```

`unibox run`, `task prepare` and `sandbox recreate` build when needed, without
a question; `sandbox recreate --rebuild` forces a fresh build. A build says
that it builds and points at `unibox profile diff`, which prints what changed in the profile's directory
since its image last built as a unified diff.

::: tip Read a teammate's change before it builds
A Dockerfile runs in your host's Docker when it is built. Isolated there, but
`RUN --mount` and build secrets can reach out. `unibox profile diff` is there for
exactly that.
:::

For sbx a build is `docker build` with the build arguments, the agents'
check, `docker save`, and `sbx template load`. A local build is for this
machine's architecture, which is all a local sandbox needs.

### The agents' check

After a build, before the image is loaded into the runtime, unibox looks for
each declared agent's command in the image. One missing fails the build,
naming it - "profile web declares codex but its image cannot find it; install
it in the Dockerfile" - so the declaration cannot quietly stop being true. An
`image:` is checked the same way once pulled.

## Image tags

An image is tagged with its profile's revision, a digest of everything it is
built from: `unibox/web:1508ba9befee705c`. So unibox tells a current one from
an outdated one, per runtime - **built**, **outdated**, **not built** - and
`unibox profile ls` shows it. unibox reads the working tree, so uncommitted
changes count as any other.

The revision covers the profile's files, `.shared/` and `_shared/` (but not
the instruction texts), the `@install` runner and the declared agents.
`sandbox.yaml`, `instructions.md`, the agents' configuration seeds and the
profile's scripts are not image inputs.

## Old images go

Each build is an image of its own, gigabytes each, so unibox cleans up:

- a build removes the profile's older images, from sbx's store and Docker's;
- making a sandbox removes the older images sbx made of its kit;
- `unibox prune` removes the rest unibox no longer needs: the images of
  profiles and sandboxes that are gone.

A sandbox does not need the image it was made from once it exists. A profile
that does not load keeps its builds, as which is current cannot be told.

## An image built elsewhere

```yaml
version: 6
image: registry.example.com/team/agents:2026-10
agents: [claude, codex]
```

Nothing is built; the agents are checked once the image is pulled.
