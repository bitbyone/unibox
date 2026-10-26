# Security

## Trust

**Trusted:** your machine, and the env repository's sbxenv files and kits,
which the team reviews and sbx applies after you approve their bindings.

**Untrusted:** everything in the sandbox - the agent (which has sudo
there), its clones, its Docker - and any host directory a task mounts.

## What crosses

| Into the sandbox | How |
| --- | --- |
| the sandbox itself | built by sbx from the env's files and kits, with your values |
| a task's repositories | mounted from the host (default), or cloned from the forge over HTTPS |
| tokens | placeholders only |
| the agent's trust of the task, author, co-author hook | written into the sandbox's own config |

| Out of the sandbox | How |
| --- | --- |
| a mounted directory's files and `.git` | at once, on your disk |
| a clone's commits | pushed to the forge, fetched by you |
| ports | published on `127.0.0.1` only |
| network requests | through sbx's proxy, under its policy |

Nothing of the sandbox is mounted on the host.

## Tokens

- **unibox never reads a token.** It learns which an env needs from its
  kits and bindings, and whether sbx has each from `sbx secret ls`, which
  masks values. `unibox secret set` has sbx prompt on your terminal; a
  piped token goes to sbx unread.
- **No token enters a sandbox.** sbx's proxy on the host replaces the
  placeholder on requests to the hosts the binding names, nowhere else.
- **No token is in unibox's files, logs or `--json`.** `vars.toml` refuses
  values starting with `secret:`.
- **A `--command` secret runs on the host**, as you, whenever sbx refreshes
  it.
- **Tokens are stored globally**, because `sbx env rm` deletes a sandbox's
  own secrets.

::: warning sbx's built-in github secret
With sbx's `github` secret stored, sbx signs every request to github.com
with it, over any narrower token a kit sends.
:::

## Git

A `.git` holds commands your machine runs later: hooks, `core.fsmonitor`,
`core.sshCommand`, filters, `credential.helper`. Editors and shell prompts
run git all the time.

- **A mounted task gives the agent your `.git`.** unibox says so the first
  time it mounts each directory. Mount only repositories you would let the
  agent configure.
- **A cloned task keeps it out.** You get its work by `git fetch`, which
  carries objects and refs, not config or hooks.
- unibox writes nothing into a host `.git`; a mount's author and hook come
  from the sandbox's global git config.

## The forge is the boundary

The agent can do whatever the env's forge token allows: push any branch,
force-push, delete.

- Protect `main` and release branches (GitLab protected branches, GitHub
  rulesets).
- Keep the token narrow: a GitLab project or group token with Developer on
  the projects concerned; a GitHub fine-grained token on those
  repositories, contents write, no `workflow`.
- Restrict it to work branches where the forge allows.

## Your SSH agent

sbx can forward your SSH agent into sandboxes. unibox refuses to run while
sbx forwards it and you have not decided. `unibox init` asks (recommending
no); `unibox config set ssh_agent true|false` decides later and sets sbx
to match.

| | No (recommended) | Yes |
| --- | --- | --- |
| git | HTTPS with the env's tokens | also SSH, as you |
| the agent can reach | what the env's tokens allow | every repository and server your keys reach |
| your keys | out of reach | not readable, but usable to sign, as often as the agent wants |

With it, git in a session accepts a server's key the first time
(`StrictHostKeyChecking=accept-new`) and refuses a changed one.

## Host commands

An sbxenv file can run commands on the host at create. sbx asks first,
unless `env.rememberHostCommands` is on; then it runs them again without
asking, even after a teammate changed them. `unibox doctor` warns while it
is on; `unibox config set remember_host_commands false` turns it off.

## Env changes

A change to the env reaches your sandbox only when you run
`unibox sandbox recreate`; sbx asks again for any new binding. Read the
change in the env repository first.

## Shared sandboxes

Tasks in one sandbox are not isolated from each other: one user with sudo,
one filesystem, one Docker, every mounted directory. Give a task that must
not reach another its own sandbox.
