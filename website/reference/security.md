# Security

The sandbox is the boundary, so it matters exactly what crosses it. This page
lists it.

## Trusted and untrusted

```
   +------------- trusted --------------+       +----- untrusted -----+
   | host: home, ~/.ssh, your logins    |       | VM: the agent, sudo,|
   | your checkouts and worktrees       | <-X-- | its clones, its     |
   | your configuration, the            |       | Docker              |
   | environments repository            |       |                     |
   +-----------------+------------------+       +----------+----------+
                     | only:                               ^ | only:
                     |  caches a profile names, :ro -------+ |
                     |  env values a profile names --------+ |
                     |  directories you --mount -----------+ |
                     |                                       |
                     +---- git fetch <---- forge <-----------+ git push
                          (objects and refs only,     (a narrow token,
                           no config, no hooks)        protected branches)
```

**Trusted**: your machine, and the environments repository - your choice, in
your git, edited by you or through unibox's commands. No agent edits it on
its own, and the repositories being worked on choose nothing of the sandbox.

**Untrusted**: everything inside the VM - the agent, which has sudo there, its
clones, its Docker.

## What goes in

| What | How |
| --- | --- |
| the image | built on the host from the profile's Dockerfile, loaded into the runtime |
| clones | made inside the sandbox from the forge, over https |
| `env` | the values of the host variables a profile names |
| `caches` | host directories a profile names, **read-only** |
| instructions, seeds, scripts | written into the sandbox's home and `/var/lib/unibox` |
| placeholders | for every login and service token |
| a host directory | only with `--mount`, at your request |

## What comes out

| What | How |
| --- | --- |
| commits | pushed to the forge by the agent; fetched by you |
| ports | published on the host's **loopback** only, by name |
| network requests | through the runtime's proxy, under its policy and the profile's rules |
| files in a mounted directory | at once, on your disk - only with `--mount` |

Nothing of the VM is mounted on the host.

## Tokens

- **A token never enters a sandbox.** Logins and service tokens are kept on
  the host - in the platform's store, or in the runtime's own - and reach the
  sandbox only as placeholders. The runtime's proxy, on the host, replaces a
  placeholder with the value on requests to the hosts its login or service
  names, and nowhere else.
- **unibox may hold a token in memory**, on its way from your terminal to the
  platform's store. It never puts one in a process's arguments, its log,
  `--json` output, its state or a profile; tests hold each of these.
- **A command source runs on the host.** A service's `command` is run by the
  runtime's daemon on the host, at verification and whenever it refreshes. It
  is your command, in your profile.
- **A runtime without a credential proxy** could not keep the value off the
  agent. unibox's runtimes declare the capability; a future adapter without
  it may only degrade knowingly, behind a warning that the token will be
  readable inside the sandbox.

## Git

A `.git` directory the agent can write holds commands the host would run: a
hook, `core.fsmonitor`, `core.sshCommand`, a filter, `credential.helper`. So by
default nothing git-related of the host is in the sandbox. The agent works on
its own clones; what you take back is a fetch from the forge, which carries
objects and refs, never config or hooks.

unibox writes nothing into a host `.git`, mounted or not. The author and the
co-author hook in a mounted repository reach git in the sandbox through the
sandbox's own global configuration.

## The forge is the boundary

The sandbox pushes with a token of its own. Within that token's reach the
agent can push to any branch, force-push and delete.

- **Protect `main` and release branches** on the forge: GitLab protected
  branches and push rules, GitHub rulesets.
- **Issue the token narrow**: a GitLab project or group access token with
  Developer on the projects concerned; a GitHub fine-grained token on the
  repositories concerned, contents write, and `workflow` left out so the
  agent cannot change GitHub Actions.
- **If possible, restrict the token to the work branches.**

::: warning sbx's built-in github secret
With sbx's built-in `github` secret stored, sbx signs every request to
github.com itself, over a profile's git services - and over a narrow token.
Sign copilot in by `token`, not `command`; `unibox task check` warns
(`service_overridden`) while the built-in secret is stored and a service names
github.com.
:::

## Mounts

`--mount` gives up the git protection above on purpose, for one task. The
agent works on your files and your `.git` as they are. Worktrees are refused,
and `unibox task rm` unmounts before it deletes and never deletes your files. See
[Mounting a Directory](/guide/mount).

## Caches

Caches are read-only. Read-write would let the agent put a jar or an
`init.d` script that the host's next build runs.

## Builds

A profile's Dockerfile runs in the host's Docker when it is built. A build is
isolated there, but `RUN --mount` and build secrets can reach out. Read
`unibox profile diff <profile>` before building a teammate's change.

## Profiles are told, not asked

The profile is yours, so nothing asks you to approve it. When what it asks of
the host changes, `unibox task check` says so (`profile_changed`) and the next start
names the change - `services + github, network.allow + jitpack.io` - and what
it leads to.

## Sandboxes shared by tasks

A shared sandbox gives no isolation between the agents in it: one user with
sudo, one filesystem, one Docker, one set of ports. Give a task that must not
reach another a sandbox of its own.
