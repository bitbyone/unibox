# Security

The sandbox is the boundary, so it matters exactly what crosses it. This page
lists it.

## Trusted and untrusted

```
   +------------- trusted --------------+       +----- untrusted -----+
   | host: home, ~/.ssh, your logins    |       | VM: the agent, sudo,|
   | your configuration, the team's     | <-X-- | its clones, its     |
   | sbxenv files and kits              |       | Docker              |
   +-----------------+------------------+       +----------+----------+
                     | only:                               ^ | only:
                     |  what the env's kits put in --------+ |
                     |  directories a task mounts ---------+ |
                     |                                       |
                     +---- git fetch <---- forge <-----------+ git push
                          (objects and refs only,     (a narrow token,
                           no config, no hooks)        protected branches)
```

**Trusted**: your machine, and the team's sbxenv files and kits - reviewed in
the team's repository, applied by sbx after you approve their bindings. No
agent edits them on its own, and the repositories being worked on choose
nothing of the sandbox.

**Untrusted**: everything inside the VM - the agent, which has sudo there, its
clones, its Docker - and whatever the agent can write on the host, which is
a mounted directory.

## What goes in

| What | How |
| --- | --- |
| the sandbox | made by sbx from the env's files and kits, with your args |
| a task's directories | mounted from the host, by default; or cloned inside the sandbox from the forge, over https, with `--clone` |
| placeholders | for every token, in place of its value |
| the agent's trust of the task | merged into its configuration in the sandbox's home |
| the author and co-author hook | in the sandbox's git configuration, or the clone's |

## What comes out

| What | How |
| --- | --- |
| files and `.git` of a mounted directory | at once, on your disk |
| commits of a clone | pushed to the forge by the agent; fetched by you |
| ports | published on the host's **loopback** only |
| network requests | through sbx's proxy, under its policy |

Nothing of the VM is mounted on the host.

## Tokens

- **unibox never reads a token.** It learns which tokens an env needs from
  its kits and bindings, and whether sbx has each from `sbx secret ls`,
  which masks every value. `unibox login set` has sbx ask for a token on
  your terminal; a token piped in is handed to sbx unread.
- **A token never enters a sandbox.** Tokens are kept by sbx on the host, in
  its store, and reach the sandbox only as placeholders. sbx's proxy, on the
  host, replaces a placeholder with the value on requests to the hosts a
  binding names, and nowhere else.
- **No token is in unibox's files.** `vars.toml` refuses a value beginning
  with `secret:`, and nothing of unibox's state, logs or `--json` output
  carries one.
- **A command source runs on the host.** A token stored with
  `login set --command` is read by sbx's daemon running that command on the
  host, whenever it refreshes. It is your command, run as you.
- **Tokens are stored globally**, for every sandbox that needs them.

## Git

A `.git` directory the agent can write holds commands the host would run: a
hook, `core.fsmonitor`, `core.sshCommand`, a filter, `credential.helper`.
Editors, git GUIs and shell prompts run git in your checkouts all the time.

**A mounted task gives the agent your `.git`.** That is the default, because
it is how most people work with an agent, and unibox says so once, the first
time it mounts each directory. Mount a repository you would let the agent
configure.

**A cloned task keeps it out.** With `--clone` the agent works on its own
clone, and what you take back is a fetch from the forge, which carries
objects and refs, never config or hooks.

unibox itself writes nothing into a host `.git`. The author and the
co-author hook of a mounted repository reach git in the sandbox through the
sandbox's own global configuration. See [Mount or Clone](/guide/mount).

## The forge is the boundary

The sandbox pushes with the token the env gives it. Within that token's reach
the agent can push to any branch, force-push and delete.

- **Protect `main` and release branches** on the forge: GitLab protected
  branches and push rules, GitHub rulesets.
- **Issue the token narrow**: a GitLab project or group access token with
  Developer on the projects concerned; a GitHub fine-grained token on the
  repositories concerned, contents write, and `workflow` left out so the
  agent cannot change GitHub Actions.
- **If possible, restrict the token to the work branches.**

::: warning sbx's built-in github secret
With sbx's built-in `github` secret stored, sbx signs every request to
github.com itself, over any narrower token a kit would send.
:::

## Your SSH agent

sbx can forward your SSH agent into a sandbox. unibox refuses to work while
sbx does and you have not said whether it may, as that would hand every key
over unasked. `unibox init` asks, recommending no, and
`unibox config set ssh_agent true|false` says it later; either sets sbx to
match.

| | No (recommended) | Yes |
| --- | --- | --- |
| git | over HTTPS, with the env's tokens | also over SSH, with no token - a mounted checkout pushes as you do |
| What an agent can reach | the hosts and owners the env's tokens allow | every repository and server your SSH keys reach, as you |
| Your keys | out of reach | never seen, but used to sign, as many times as the agent wants |
| Tools beyond git (`gh`, registries) | the env's tokens | the env's tokens all the same |

With it, git in a session trusts a server the first time it meets it
(`StrictHostKeyChecking=accept-new`), as nobody is there to answer, and
refuses one whose key has changed.

## Host commands

An sbxenv file can run commands on the host when a sandbox is created. sbx
asks before it runs them, unless `env.rememberHostCommands` is on, in which
case it runs them again without asking - after a teammate changed them, too.
`unibox doctor` warns while it is on.

## Env changes are told, not applied

A teammate's change to the env's files or kits reaches your sandbox only
when you recreate it: unibox says the env changed (`env_changed`) and
`unibox sandbox recreate` applies it, with sbx asking again for any binding
not yet approved. Read the change in the team's repository first.

## Sandboxes shared by tasks

A shared sandbox gives no isolation between the agents in it: one user with
sudo, one filesystem, one Docker, one set of ports, and every directory any
of its tasks mounted. Give a task that must not reach another a sandbox of
its own.
