# What unibox Asks of sbx

[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) does the
work: one sbx sandbox is one micro-VM with its own kernel, its own Docker
Engine and its own disk, persistent until removed, with the agent running in
it as a non-root user with sudo. unibox runs nothing of its own in it beyond
a few lines of shell per task.

unibox needs **sbx 0.49.2 or newer**, the version it was verified against on
macOS (Apple Silicon), on throwaway sandboxes. `unibox doctor` fails on an
older one. Where sbx's behaviour shows through, this page says so.

::: tip Before you start
`sbx login` once. `unibox doctor` checks the rest.
:::

## The commands unibox runs

| unibox | sbx |
| --- | --- |
| a sandbox created from an env | `sbx env create --name <sandbox> --env-args-file <file> <files...>`, on your terminal |
| `doctor`'s check of an env | `sbx env plan` with the same files and args |
| which agent and which tokens | `sbx kit inspect --json` of each kit, with `--kit-arg` from your values |
| which tokens are stored | `sbx secret ls --json` |
| `secret set` | `sbx secret set <service> [--command <cmd>]`, on your terminal |
| a mounted task | `sbx mount <sandbox> <host>:/work/<task>/<repo>`; `sbx umount` to remove it |
| a session | `sbx exec -it -w <dir> <sandbox> bash -c ...` |
| sandboxes and their state | `sbx ls --json` |
| stop and remove | `sbx stop`; `sbx env rm --force` with the env's files, or `sbx rm --force`, after unibox's own checks |
| named ports | `sbx ports --json`, `--publish`, `--unpublish` |
| the SSH agent decision | `sbx settings get/set ssh.agentForwardingEnabled`, then `sbx daemon restart` |
| `config set remember_host_commands` | `sbx settings set/unset env.rememberHostCommands` |
| `network ls` | `sbx policy ls --json --type network <sandbox>`, `sbx policy log --json --type network --limit 500 <sandbox>` |
| `network allow`, `deny`, and after a create | `sbx policy allow/deny network --sandbox <sandbox> [--protocol <p>] <hosts>` |
| `network rm` | `sbx policy rm network --force --resource <hosts> [--sandbox <sandbox>]` |
| `prune` | `sbx ls --json`, and `sbx umount` of a mount no task has |
| `doctor` | `sbx version`, `sbx diagnose --format json`, `sbx settings get env.rememberHostCommands` |

unibox removes only sandboxes it created; one it adopted with
`sandbox create --adopt` only with `--force`.

## Envs

- **`sbx env plan` prints text, not JSON.** It does validate: a missing
  required arg or an undeclared one exits 1 with the arg's name. So unibox
  reads `args`, `kits`, `bindings`, `agent` and `name` from the YAML itself,
  read-only, ignores every other field so a newer sbx's additions do not
  break it, and runs `plan` for its verdict.
- **Bindings are approved once, by you.** A binding not yet applied needs
  your approval; `sbx env create` asks once, and `--force` would approve
  without asking. unibox runs the first create on your terminal and never
  passes `--force` for you.
- **A kit with a required arg of its own** is read with `--kit-arg` from the
  env's values, or `sbx kit inspect` would refuse it. A local kit is built
  first, which takes seconds.

## Agents

- **The agent's command is the workload kit's.** `sbx kit inspect --json`
  gives `sandbox.entrypoint` (`["copilot"]`) and `sandbox.command.default`
  (`["--yolo"]`).
- **`sbx exec` gives the agent what `sbx run` does**: the proxy's variables,
  the kit's `BASH_ENV` and `PATH`, the credential placeholders. Commands go
  through bash, because `sh` is dash, which drops variables whose names are
  not a shell's and does not read `BASH_ENV`.
- **Two agents in one sandbox at once work**, each in its own `exec`.

## Secrets

- **Which tokens a sandbox needs** is the union of `credentials[].service`
  over its kits, from `kit inspect --json`, and the env's bindings. Inspect
  drops a credential's `optional`, which only the kit's YAML has.
- **What is stored** is `sbx secret ls --json`: a name, a scope and the
  secret masked as `(stored)` or `(oauth configured)`, never a value.
- **A command secret is stored as its source.** The sbx daemon runs the
  command on the host when it verifies the secret and whenever it refreshes,
  from a fresh temporary directory, with no terminal. The command must reach
  its secret alone - a keychain, an agent socket.
- **`sbx env rm` deletes a sandbox's own secrets**, which is why unibox
  stores every token globally.
- **sbx's own `github` secret takes github.com over**: with it stored, the
  proxy sets `Authorization` on every request to github.com itself, over any
  the sandbox sent.

## Mounts

- `sbx mount <sandbox> HOST:TARGET` mounts into a running sandbox: live,
  writable, taken out by `sbx umount`, leaving an empty mount point.
  Removing the sandbox leaves the host's files alone.
- **Mounts are kept across stop and start**, and a mount whose host path is
  gone fails the start until it is removed with `sbx umount`. So `task rm`
  unmounts before anything else, and `doctor` checks every mounted path
  exists.
- **sbx lists no mounts.** `/proc/self/mountinfo` in a running sandbox shows
  each with its target, which is how `doctor` finds a mount no task has.
- **The parents of a mount are root's**, mode 755: the agent cannot write
  beside the repository. Inside it git works, a commit is the host's at
  once, and `.git/hooks` is writable.

## Ports

- `sbx ports` publishes a port live on the host's loopback, tcp4 when no
  protocol is named. A host port already taken is refused.
- `--unpublish` takes a port back only by its exact host port. So unibox
  records what it published by name and mapping, and leaves a port it did
  not publish alone.
- **A kit's ports get new host ports at every start**, which is why named
  ports are the person's, in unibox's configuration.
- A switch on a stopped sandbox is refused, since publishing would start it.

## Network policy

- `sbx policy ls --json` gives `rules[]`, each with `applies_to` (`all`, or
  `sandbox:<name>`), how it was made, and for a kit's rule a name starting
  `kit:`. unibox lists them as they are and removes only rules it can name
  by their hosts; sbx refuses to remove the defaults' and the kits'.
- A rule for one sandbox goes into sbx's `local` policy, scoped to it.
  Hosts are joined by commas, so unibox refuses a host with a comma or
  whitespace in it, or one starting with `-`, before sbx is asked.

## Lifecycle

- `sbx exec` starts a stopped VM.
- unibox stops a sandbox when the last session it started there ends,
  unless `stop_idle` is `false`. The disk and tasks stay.
- With `env.rememberHostCommands` on, sbx runs an env's host commands again
  without asking; `unibox doctor` warns and suggests
  `unibox config set remember_host_commands false`.
