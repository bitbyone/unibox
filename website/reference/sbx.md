# What unibox Asks of sbx

[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) does the
work: each sandbox is a micro-VM with its own kernel, Docker Engine and
disk, persistent until removed. unibox needs **sbx 0.50.0 or newer**,
verified on macOS (Apple Silicon); `unibox doctor` fails on an older one.

## Commands unibox runs

| unibox | sbx |
| --- | --- |
| create a sandbox | `sbx env create --name <sandbox> --env-args-file <file> <files...>`, on your terminal |
| `doctor`'s env check | `sbx env plan` with the same files and args |
| agent and tokens of an env | `sbx kit inspect --json`, with `--kit-arg` from your values |
| stored tokens | `sbx secret ls --json` |
| `secret set` | `sbx secret set <service> [--command <cmd>]`, on your terminal |
| mount a task | `sbx mount <sandbox> <host>:/work/<task>/<repo>`; `sbx umount` |
| a session | `sbx exec -it -w <dir> <sandbox> bash -c ...` |
| list | `sbx ls --json` |
| stop, remove | `sbx stop`; `sbx env rm --force` or `sbx rm --force`, after unibox's checks |
| ports | `sbx ports --json`, `--publish`, `--unpublish` |
| SSH agent | `sbx settings get/set ssh.agentForwardingEnabled`, `sbx daemon restart` |
| `remember_host_commands`, `doctor` | `sbx settings get/set/unset env.rememberHostCommands` |
| `clipboard_image_paste`, `doctor` | `sbx settings get/set/unset clipboard.imagePaste` |
| `network ls` | `sbx policy ls --json --type network`, `sbx policy log --json --type network --limit 500` |
| `network allow`/`deny` | `sbx policy allow/deny network --sandbox <sandbox> [--protocol <p>] <hosts>` |
| `network rm` | `sbx policy rm network --force --resource <hosts> [--sandbox <sandbox>]` |
| `doctor` | `sbx version`, `sbx diagnose --format json` |
| `prune` | `sbx ls --json`, `sbx umount` of a mount no task has |

unibox removes only sandboxes it created; an adopted one only with
`--force`.

## Behaviour unibox relies on

**Envs.** `sbx env plan` validates but prints text, so unibox reads
`name`, `agent`, `args`, `kits` and `bindings` from the YAML itself and
ignores other fields. The first create runs on your terminal so you
approve the bindings; unibox never passes `--force` to it. A kit with a
required arg is inspected with `--kit-arg`.

**Agents.** The agent's command is the workload kit's
`sandbox.entrypoint` and `sandbox.command.default`. `sbx exec` gives it
what `sbx run` would: proxy variables, the kit's `BASH_ENV` and `PATH`,
the credential placeholders. Commands run through bash, as `sh` is dash.
Two agents can run in one sandbox at once.

**Secrets.** An env needs the union of its kits' `credentials[].service`
and its bindings; `optional` is read from the kit's YAML, as inspect drops
it. `sbx secret ls` shows values masked. A command secret runs on the host
from a temporary directory with no terminal. `sbx env rm` deletes a
sandbox's own secrets, so unibox stores tokens globally. sbx's `github`
secret overrides any other `Authorization` to github.com.

**Mounts.** `sbx mount` is live and writable, kept across stop and start;
a mount whose host path is gone fails the start, so `task rm` unmounts
first and `doctor` checks paths. sbx lists no mounts; `doctor` reads
`/proc/self/mountinfo` in the sandbox. A mount's parent directories are
root's, so the agent cannot write beside the repository.

**Ports.** `sbx ports` publishes live on loopback; a taken host port is
refused. `--unpublish` needs the exact host port, so unibox records what
it published and leaves other ports alone. A kit's ports get new host
ports at each start. Publishing on a stopped sandbox would start it, so
unibox refuses.

**Network policy.** A sandbox's rule goes into sbx's `local` policy,
scoped to it. sbx refuses to remove default and kit rules. Hosts are
comma-joined, so unibox refuses a host containing a comma or whitespace or
starting with `-`.

**Lifecycle.** `sbx exec` starts a stopped VM. unibox stops a sandbox when
the last session it started there ends, unless `stop_idle` is `false`.
