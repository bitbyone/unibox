# Getting Started

unibox runs a coding agent - claude, codex, copilot or opencode - on your
repositories inside a Docker Sandbox (`sbx`), a micro-VM on your machine.
Your team describes the sandbox in an env repository of `*.sbxenv.yaml`
files and kits; unibox sets it up from there and starts the agent in it.
Your tokens stay on the host: the sandbox only ever sees placeholders.

## Prerequisites

- macOS, Linux or Windows
- [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) 0.49.2
  or newer, signed in to Docker (`unibox init` offers `sbx login` if not)
- git
- the address of your team's env repository

## Install

Homebrew (builds the release from source):

```sh
brew tap bitbyone/unibox https://github.com/bitbyone/unibox
brew trust bitbyone/unibox
brew install unibox
```

Scoop, on Windows:

```powershell
scoop bucket add unibox https://github.com/bitbyone/unibox
scoop install unibox
```

A release archive: download it from
[Releases](https://github.com/bitbyone/unibox/releases) (Linux, macOS,
Windows; amd64 and arm64) and put `unibox` on your `PATH`.

From source (Go 1.26 or newer):

```sh
git clone https://github.com/bitbyone/unibox
cd unibox && make install     # installs ~/.local/bin/unibox
```

Check it with `unibox version`. Upgrade with `brew upgrade unibox` or
`scoop update unibox`.

## Set up the team's env

```sh
unibox init git@github.com:acme/agent-envs.git
```

init clones the env repository into
`~/.local/share/unibox/repos/<host>/<path>` (or uses it in place if you
pass a local path, or run `unibox init` inside a checkout), then walks
five steps:

1. **sbx** - checks it is installed and signed in, and asks whether
   sandboxes may use your SSH agent. Answer no unless you need git over
   SSH; see [Security](/reference/security#your-ssh-agent).
2. **Envs** - pick the sbxenv files you want. Each becomes an env, such as
   `dev`.
3. **Values** - type the values the env asks each person for, such as your
   registry user name. They go to `~/.config/unibox/vars.toml`.
4. **Secrets** - sbx asks for each missing token on your terminal: the
   agent's login, a forge token, a registry token. The value goes straight
   to sbx.
5. **Sandboxes** - creates the env's sandbox. sbx shows what it will set
   up and asks you to approve it.

Run `unibox init` again at any time: it adds what is new and skips what is
done. Then check everything:

```sh
unibox doctor
```

## Run an agent

```sh
cd ~/code/api
unibox run
```

This mounts `~/code/api` into the sandbox at `/work/<task>/api` and starts
the env's agent there, in your terminal. The agent edits your files
directly. When it exits, the sandbox stops unless another session is still
in it; its disk and the task stay.

Come back to it later:

```sh
unibox task ls          # every task
unibox resume <task>    # run its agent again
```

::: warning Protect your branches
The agent can push wherever the sandbox's forge token may. Protect `main`
and release branches on the forge.
:::

## What next

- [How it Works](./how-it-works) - sandbox, env, task, and where your
  values and tokens live
- [Tasks](./tasks) - branches, clones, several repositories
- [Sandboxes](./sandboxes) - stop, recreate, resources, shell
- [Ports](./ports) - open the dev server in your browser
- [Secrets & Agents](./secrets) - signing agents in
- [Troubleshooting](./troubleshooting)
