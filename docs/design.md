# Sandboxed agents: agentbox

## Where it stands

Designed (2026-10-12), nothing built. This is the brief for whoever
implements it; the reasoning is here so that the decisions are not undone
by accident. It replaces "Agents in a container" in unagit's docs/herdr.md,
and wins wherever the two disagree: that plan mounted the worktree and the
main clone's `.git` into a Docker container, and its launcher for herdr is
not used either.

Checked so far: `sbx` 0.47.0 on macOS (Apple Silicon) - its commands and
flags below come from its `--help`; `sbx ls --json` answers
`{"sandboxes": []}`, `sbx template ls --json` `{"images": []}`. Everything
marked **spike** has not been tried. `agentbox` is a working name.

## What it is for

A worktree - one repository's or a group's - gets a coding agent that runs
in a VM instead of on the machine. The agent has full autonomy inside it;
the machine, its home, its keys and unagit's tokens are out of its reach.
The user picks the agent, a sandbox profile (a Dockerfile and the
resources, kept in a git repository of their own), and whether the agent
shares a VM with others or gets one of its own.

## Two programs

The sandboxing is **agentbox**, a program of its own, usable without
unagit; **unagit is a UI over it** and one of its users.

- agentbox lives in this repository for now, but is built as its own
  binary (`cmd/agentbox`, `make build` makes both) with its own home, its
  own configuration and its own runtimes. Moving it to a repository of its
  own later - as incomm is - is moving directories.
- `make install` builds and installs both binaries in one invocation, under
  the same `PREFIX`. It compares each built binary with its installed copy
  and installs only one whose contents differ (or which is missing). Go's
  build cache avoids recompiling unchanged packages; modification times
  alone do not decide whether to replace an installed binary.
- unagit drives it as it drives herdr or Zellij: through its command line,
  `--json` for what it reads. It never imports agentbox's code, beyond the
  package of the JSON types (see [the boundary](#the-boundary)).
- unagit also **configures** agentbox: what the user chooses in unagit's
  Settings (the environments repository picked from the list of
  repositories, the runtimes) is handed to agentbox as its configuration.

| agentbox | unagit |
| --- | --- |
| its home: configuration, state, cache | Integrations: agentbox on or off |
| the environments repository: profiles, build, the agents' check, approvals | which worktree, which repositories, which branch |
| the runtimes: sbx now, Podman later | pushing the branch, with the vault's token |
| VMs and tasks: create, start, stop, remove | clone URLs from the forge's API |
| a task's preparation: clones, author, co-author hook, instructions, `start`, bootstrap | the Sandboxes tab, the pickers, Activity |
| running the agent, stopping an idle VM | the questions: approve, build, commit first |
| its herdr plugin | the commit dialog's warning, the pull |
| refusing a default branch | picking the environments repository from the list, handing agentbox its path |

## The picture

The parts:

```
+------------------------------- unagit (host) --------------------------------+
|  internal/ui                                                                 |
|  +------------------+ +------------------+ +------------+ +----------------+ |
|  | Sandboxes tab [6]| | Open in Sandboxed| | Activity   | | Settings [5]   | |
|  | Profiles         | | Agent...         | |            | | Integrations:  | |
|  | Sandboxes (VMs)  | | agent > profile  | |            | | agentbox, env  | |
|  | Tasks            | | > sandbox > place| |            | | repo, runtimes | |
|  +--------+---------+ +--------+---------+ +-----+------+ +-------+--------+ |
|           |                    |                 |                |          |
|           +--------------------+--------+--------+----------------+          |
|                                         | agentbox ... --json                |
|  workspace, gitx: push the branch, pull | (and agentbox config set ...)      |
+---------+-------------------------------+------------------------------------+
          |                               |
          |                               v
          |      +------------------- agentbox (host) ---------------------+
          |      | ~/.config/agentbox/config.yaml, state.json              |
          |      | profiles: parse, tree hash, build, agents' check, trust |
          |      | tasks: manifest, instructions, author, hook             |
          |      | Runtime: Available List Images Build Create Exec Stop   |
          |      |          Remove                                         |
          |      |   +----------------+      +----------------+            |
          |      |   | sbx adapter    |      | podman adapter |            |
          |      |   |                |      | (later)        |            |
          |      |   +-------+--------+      +----------------+            |
          |      | herdr plugin: agentbox herdr observe                    |
          |      +-----------+---------------------------------------------+
          |                  | sbx CLI
          | push, pull       v
          v           +--------------------------------------+
   +-------------+    | sbx microVM  ab-<profile>[-<task>]   |
   | forge       |<-->|  template = the profile's image      |
   | GitLab,     |    |  /work/<task>/<repo>   git clones    |
   | GitHub      |    |  the agents (claude, codex, ...)     |
   | protected   |    |  Docker Engine (compose, tests)      |
   | branches    |    |  GRADLE_USER_HOME (the VM's own)     |
   +-------------+    |  ~/.gradle/caches :ro <- host        |
     clone, push:     +--------------------------------------+
     the VM's own
     narrow token
```

Where each kind of setting lives:

```
 environments repository         ~/.config/agentbox/         unagit
 (git, the user's or a team's)                               config.yaml
 +--------------------------+   +-----------------------+   +----------------------+
 | kotlin/                  |   | config.yaml           |   | integrations:        |
 |   Dockerfile             |<--+   environments: <path>|<--+   agentbox:          |
 |   profile.yaml           |   |   runtimes:           |   |     on: true         |
 |   start.sh               |   |     sbx: {on, stop_   |   |     environments:    |
 |   instructions.md        |   |           idle}       |   |       gl/team/envs   |
 |   .sbx/build.args         |   | state.json            |   +----------------------+
 | instructions/*.md        |   |   approvals, VMs,     |    the user's choices,
 +------------+-------------+   |   tasks, agents run   |    handed to agentbox
              |                 +-----------------------+
  what the    |                  agentbox's own
  environment v
  is      +--------------------------------------+
          | generated just before a start        |
          | ~/.cache/agentbox/sbx/               |
          |   agentbox/kotlin:<tree>  template   |
          |   flags: --cpus -m -e --deny-network |
          +--------------------------------------+
            a cache: never edited, regenerated
```

A start from unagit:

```
 user              unagit                     agentbox            sbx, VM        forge
  | Open in          |                          |                    |             |
  | Sandboxed Agent  |                          |                    |             |
  +----------------->| profiles, VMs --json --->|                    |             |
  | agent, profile,  |                          |                    |             |
  | sandbox, place   |                          |                    |             |
  +----------------->| check --json ------------>| approved? built?   |             |
  |<- approve? build?|<-- needs: build ---------+                    |             |
  +-- yes ---------->| build (log) ------------>| docker build, load>|             |
  |                  | push the branch ---------+--------------------+------------>|
  |                  | prepare (log) ---------->| create/start, ---->| clone ----->|
  |                  |                          | start.sh, clones   |             |
  |                  | the place runs:          |                    |             |
  |                  |   agentbox run --------->| sbx exec -it ----->| the agent   |
  |                  |                          |                    | push ------>|
  | Pull ----------->| fetch, fast-forward <----+--------------------+-------------+
```

What crosses the line between the machine and the VM:

```
   +------------- trusted --------------+       +----- untrusted -----+
   | host: home, ~/.ssh, unagit's vault |       | VM: the agent, sudo,|
   | main clones, the user's worktrees  | <-X-- | its clones, its     |
   | the configurations, the            |       | Docker              |
   | environments repository (approved) |       |                     |
   +-----------------+------------------+       +----------+----------+
                     | only:                               ^ | only:
                     |  ~/.gradle/caches  :ro  ------------+ |
                     |  the env values a profile names ----+ |
                     |                                       |
                     +---- git fetch <---- forge <-----------+ git push
                          (objects and refs only,     (a narrow token,
                           no config, no hooks)        protected branches)
```

## The model, and why

```
host                                   sandbox VM
grouped worktree  <── git pull ───┐    /work/<task>/<repo>  clones of the
  (yours)                         │      branch, the agent commits
                                  │      and pushes here
                       forge ◄────┴──── git push
```

- **The agent clones from the forge, commits and pushes; the user pulls.**
  The worktree's branch is pushed before the agent starts, the VM clones
  every repository of the worktree at that branch, the agent pushes back
  to it, and the user pulls into the worktree as they would a colleague's
  commits.
- **Nothing of the VM is mounted on the host, and nothing git-related of the
  host is mounted in the VM.** That is the point of the design. A `.git`
  the agent can write holds commands the host would run: a hook,
  `core.fsmonitor`, `core.sshCommand`, a filter, `credential.helper` - and
  unagit, gitsigns, lazygit and IntelliJ run git in a worktree all the
  time. Through `credential.helper` the agent would even receive the forge
  token unagit hands git. A fetch from the forge carries objects and refs,
  never config or hooks, so the host stays safe however the agent behaves.
- **The cost**: what the agent has not pushed is not seen on the host. Its
  instructions tell it to commit and push as it goes.
- **A task can grow while it runs** (`unibox add-repo <task> --repo ...`).
  Since nothing is mounted, adding a repository is one more clone into
  `/work/<task>/`, made through the runtime's exec. The agent in a running
  session sees it at once and needs no restart. The task lock stays with
  the session; the sandbox guard serializes the clone with other
  preparations. The task's record grows, so removal checks the new clone
  for unpushed work too, and the first repositories still find the task.

Considered and rejected:

- **The worktree and the main clone's `.git` mounted read-write** (herdr.md's
  plan, with `config`, `hooks` and `config.worktree` mounted read-only over
  them). It works - herdr.md verified the locks - but every other branch's
  ref stays writable, the agent's files are on the host to be run by
  mistake, and the list of what must be locked is only as complete as our
  knowledge of git.
- **The same with the main clone read-only, the worktree on a detached HEAD
  and an object directory of its own** (`GIT_OBJECT_DIRECTORY`, the main
  objects as an alternate). Safe, but the agent sees a detached HEAD and the
  branch has to be moved after it. The user found it unpleasant.
- **`sbx run --clone`**: sbx's own clone inside the VM, of the host
  repository mounted read-only. One repository per sandbox, and how its
  commits come back is undocumented (the docs' git-workflow page is a 404).
- **Podman**: not now. agentbox's runtimes are neutral so that it can be
  added without touching unagit.

## Sandboxes and VMs

One sbx sandbox is one microVM: its own kernel, its own Docker Engine, its
own disk, persistent until removed. The agent runs in the VM itself, beside
that engine, as a non-root user with sudo (sbx's security docs); `sbx exec`
runs another process in the same environment, like `docker exec`.

Both arrangements are offered, and the user picks at the start:

- **A shared VM** per profile (`ab-<profile>`): tasks are directories
  `/work/<task>`, agents are processes side by side. Memory, the Gradle
  cache and Docker images are paid once. Between the agents there is no
  isolation at all - one user with sudo, one filesystem, one Docker, one
  set of ports (two `docker compose` Postgreses on 5432 collide). The
  picker says so.
- **A VM of its own** (`ab-<profile>-<task>`): nothing it does reaches
  another task.

Both persist when the agent ends, for their caches; removing is the user's
action. A VM in which the last agent agentbox started has ended is
**stopped** (`sbx stop`), so idle VMs hold no memory; its disk, caches and
tasks stay, and the next start boots it again (and runs the profile's
`start`). agentbox's configuration can turn the stopping off
(`runtimes.sbx.stop_idle`). It stops only VMs it made (`ab-` names), and
only when no agent it started is left in them.

`<task>` is the worktree's folder name and a short hash of its host path,
since two roots can hold folders of the same name.

## A start

### From unagit

On a worktree, a group or a merge request's branch worktree (never a review
worktree: its HEAD is the merge base on purpose, nothing is committed
there):

1. **Open in Sandboxed Agent…** asks, each picker skipped when it has one
   item and ordered by use: the agent, the profile, the sandbox, the place
   (the existing `agentPlaces`). The pickers are filled from `agentbox
   profiles --json` and `agentbox ls --json`. The sandbox picker is also
   where the runtime is chosen: the profile's VMs of every runtime, each
   carrying its own, then New Sandbox in <runtime>… for each runtime
   agentbox has on - plain New Sandbox… while there is one.
2. `agentbox check --json` with the profile, runtime, branch and
   directories. It answers what stands in the way, and unagit asks about
   each in its own dialogs: a default branch (refused: "make a worktree with
   a branch first"), uncommitted changes (they will not reach the VM, which
   clones from the forge - Commit… or go on without them), a profile not
   approved (Approve…, see [trust](#trust)), an image outdated or not built
   (Build…, with the profile's diff).
3. unagit **pushes the branch** of every repository of the worktree that
   has no upstream or is ahead of it - from the host, with its own
   credentials, through the path `P` uses. A branch origin has moved past
   stops the start with the reason; nothing is forced.
4. `agentbox prepare` with the repositories' clone URLs **from the forge's
   API** (the project's http URL), never from a local remote, which can
   carry a token (a chezmoi origin, AGENTS.md); https always - the VM has
   no SSH key. A `runTask` log of its output; it prints the VM and the task
   as JSON at the end.
5. The place chosen runs `agentbox run …` for the task - This Terminal,
   Zellij, Ghostty or herdr, as for any agent.
6. unagit's session record (`<config>/sessions/`) gets the VM and the task
   beside the pane, so Activity lists the agent with them.

### What agentbox does

`agentbox prepare` (and `run`, when not prepared):

1. Creates the VM if new (`sbx create`, see [runtime](#the-runtime)) or
   starts it, and runs the profile's `start` if the VM booted since it last
   ran.
2. For each repository, clones at the branch into `/work/<task>/<dir>`
   (`--filter=blob:none`), or fetches and fast-forwards when it is there
   already.
3. Sets the author and the co-author hook ([commits](#commits)), writes
   the agent's [instructions](#instructions-for-the-agent) into the VM's
   home, and runs the profile's bootstrap script from each repository that
   has one.

`agentbox run` then runs the agent: `sbx exec -it -w /work/<task> <vm>
<agent command>` as its child - not `exec`'d, so that when the agent ends
it can stop an idle VM and update its records. It passes the terminal,
signals and the window size through, and exits with the agent's status.

Without unagit, agentbox takes what unagit would give it from where it is
run: the repositories and the branch from the current directory or the
directories named, their clone URLs from `origin` (credentials stripped,
an ssh URL turned into https where the host is plain). It never pushes on
its own: a branch the forge does not have is refused ("push it first"),
unless `--push` asks it to run the user's own `git push -u`.

## Credentials

- **unagit's tokens never go into a VM**, nor into agentbox, in any form
  (AGENTS.md, the hard rule). The push before a start is unagit's, from
  the host; everything after it is the VM's own.
- **The VM pushes with a token of its own**, issued by the user for this,
  stored by sbx (`sbx secret set`, its keychain and proxy). Neither
  program sees it. Narrow: a GitLab project or group access token with
  Developer on the projects concerned; a GitHub fine-grained token on the
  repositories, contents write.
- **The forge is now the boundary.** The agent can push to any branch the
  token may, force-push, delete. Protect `main` and release branches on the
  forge, and if possible restrict the token to the work branches (GitLab
  push rules and protected branches, GitHub rulesets). The README says so
  where it explains the feature.
- The agents' own logins (Claude, Codex) are sbx's service secrets
  ([below](#one-image-several-agents)).

### Host services the sandbox reaches

A profile says how each agent signs in (`auth`); it says the same, the
same way, for the host-facing services the sandbox needs - a git forge,
an Artifactory, a package registry, anything the host authenticates to
and the agent must reach. These are `services` in `HostAccess` (a change
is told at the next start, as any change of host access is), a map from a
name to a host or hosts, how the secret is fetched on the host, and the
header the proxy sets with it:

```yaml
services:
  github:                       # a git forge
    git: true                   # wire the sandbox's git to these hosts
    host: github.com
    scheme: basic               # Authorization: Basic base64(user:token)
    user: x-access-token
    command: gh auth token      # a host command that prints the token
  artifactory:                  # any host service
    host: ["artifactory.example.com", "*.jfrog.io"]
    scheme: bearer              # Authorization: Bearer <token>
    env: ARTIFACTORY_TOKEN      # the tool reads the placeholder here
    command: op read op://vault/artifactory/token
```

unibox turns each into one `sbx secret set-custom`, scoped to the sandbox,
as it is created, and never sees the value: the command runs on the host,
sbx keeps the result in its keychain, the sandbox gets a placeholder. The
token is the boundary - issue it narrow (a GitLab project token, a
GitHub fine-grained token) and the forge protects what it must.

What sbx does and does not do here was checked on a throwaway sandbox,
and the design turns on it:

- A **local** custom secret only substitutes its placeholder wherever the
  sandbox sends it; `--header` and `--format` are `--cloud` only (sbx says
  so and refuses them). So the proxy puts the stored value in place of the
  placeholder verbatim, in the request headers, on requests to `--host`.
  Verified: a secret stored as `Basic <base64>` under placeholder
  `ubx-ph-test` turned `Authorization: ubx-ph-test` into
  `Authorization: Basic <base64>` at the host, and nothing elsewhere.
- **Git over https** therefore cannot go through git's own credential: git
  base64-encodes `user:password`, so a placeholder password never appears
  in the header for the proxy to find. Instead unibox writes
  `git config --global http.https://<host>/.extraHeader "Authorization:
  <placeholder>"` into the sandbox's setup; git sends the placeholder
  verbatim and the proxy swaps in the whole credential. This is the way
  around a v3 kit ignoring `apiKey.inject.scheme`, and it is forge-neutral
  - the stored value is the entire header, composed on the host (`printf
  '%s:%s' <user> "$(<command>)" | base64`), so the token and its base64
  never leave it.
- A **command** secret is stored as its source, not its value: the sbx
  daemon runs the command on the host at verification and again whenever
  it refreshes, from a fresh temporary directory (sbx's own help). A custom
  secret refreshes on demand unless `--refresh` gives a duration (a
  built-in `sbx secret set` defaults to 55 minutes); a service's `refresh`
  sets it. So a credential helper runs outside the process tree that
  created the sandbox, at any hour, with no terminal: it must reach its
  secret alone - a keychain, an agent socket - and an environment
  variable or an unlocked parent cannot carry it there. A short-lived
  token fits this well with a `refresh` shorter than its lifetime: each
  refresh fetches the next one.
- **sbx's own `github` secret takes github.com over.** With the built-in
  `github` service stored (copilot's `gh auth token`), the proxy sets
  Authorization on requests to github.com itself, and overwrites any the
  sandbox sent. Checked from a sandbox: git-receive-pack answered 200
  with no header, with a deliberately invalid one, and with a service's
  placeholder alike, while gitlab.com answered 401 without one. So a git
  service on github.com never reaches GitHub while that secret is
  stored - the token behind `gh auth token` does, whatever its reach. A
  narrow GitHub token has to be that secret itself (copilot's `auth`
  pointing at the narrow token, which then also needs Copilot's
  permission), or the built-in secret has to go.
- **Copilot without the built-in secret.** It goes: copilot reads its
  token from `COPILOT_GITHUB_TOKEN`, and accepts a placeholder of the
  token's shape there. Checked on a throwaway sandbox with the global
  `github` secret removed: a custom secret (`--env COPILOT_GITHUB_TOKEN
  --placeholder 'gho_{rand}'`, the hosts copilot calls - api.github.com,
  api.githubcopilot.com and its individual, business and enterprise
  hosts, copilot.github.com - and a host command for the token) let
  copilot answer, an unregistered `gho_` value got its login prompt, and
  github.com answered 401 without a header - so git on github.com is
  left to the services. The variable is not set in a sandbox created
  before the secret, so unibox sets the placeholder in copilot's
  environment itself; it is not secret. This is copilot's `token` auth:
  a fixed placeholder (`SessionEnv` hands it to the session), the
  generated kit declares no github credential for it, and `check` warns
  (`service_overridden`) while the built-in secret is stored and a
  service names github.com.
- **One forge, several owners.** A fine-grained GitHub token has one
  resource owner - the user or one organization - so repositories of
  several owners on github.com need a token each, told apart by path, not
  host. git's `http.<url>.extraHeader` is a list, gathered from every
  matching URL: checked on a local server, `.../org/repo` received both
  the host's and the org's `Authorization`. An empty value resets the
  list, and git applies the matches from the least to the most specific
  URL, whatever their order in the file - so `http.https://github.com/org/`
  with `extraHeader = ""` then the org's header sends the org's alone,
  and every other path keeps the host's. A git service's host takes a
  path (`github.com/some-org`) and unibox writes each list that way; the
  proxy still matches the bare host, and sbx keeps two custom secrets on
  one host apart by their placeholders (checked: each swapped for its own
  value). One classic token for every owner stays a single service on the
  bare host.
- A **host behind the user's VPN** is reached like any other: the proxy
  resolves and connects from the host, so the host's VPN routes it.
  Checked on a throwaway sandbox: gitlab.2n.cz (a private 10.x address)
  answered `Approval required` until allowed, then GitLab's own 302 - sbx
  does not refuse private addresses. A service's hosts join the
  sandbox's allow policy, so nothing more is needed but the VPN being up
  when the sandbox reaches the host.
- A **bearer or raw** service needs no git wiring: the tool reads the
  placeholder from `env` and sends it as its own Authorization, which the
  proxy substitutes. One caveat found and not yet closed: the custom
  secret's env var is injected at create, not into a running sandbox, and
  did not appear in a bare `shell` sandbox's `sbx exec` environment in the
  test (the built-in `GH_TOKEN=******` did). The header substitution is
  the reliable channel; the env path wants confirming in a real unibox kit
  before the first non-git service ships.

A runtime without a credential proxy (`CredentialProxy` is a capability;
sbx has it, a later podman adapter may not) cannot keep the value off the
agent. There the adapter may degrade on purpose - inject the token into
the sandbox, a git credential helper or an env var - but only knowingly:
unibox raises a `Need` (a warning the user acknowledges, like an
uncommitted change) that says the token will be readable inside the
sandbox, so a narrow, short-lived one is chosen for the weaker boundary.
The proxy is not sbx's alone to keep; it is the property the hard rule
leans on, and a runtime that drops it says so to the user's face.

### Configured through unibox

Built (2026-10-15): `unibox secret set|ls|rm`, `unibox agent login`, `unibox
service add|ls|rm`, `secret:<name>` references, the keychain and Secret
Service stores, and the profile edited as a node tree. Not yet: applying
a network change to a running sandbox (below, in the network rules) and
the change to approval, which is decided: told, not asked (Trust).

The user's decision (2026-10-15): everything a profile needs can be set
up through unibox's command line, so signing something in needs no hand
edit of `sandbox.yaml`. unibox offers the primitives: it knows the
agents, supports particular services, stores tokens in each platform's
own store, and writes the profile. unagit is a UI over the same commands
and writes no YAML of its own. **The file stays the user's**: a hand
edit is as valid as unibox's, for whatever unibox does not know - a
token from HashiCorp Vault (`command: vault kv get -field=token
secret/ci/gitlab`), 1Password, a helper of one's own. unibox then only
validates it, as it validates every profile, and tells the change at
the next start as it tells the rest.

This moves one thing: **unibox may now hold a token in its memory**, on
its way from the user's terminal to the platform's store - unibox is
trusted. What stays is the boundary that matters: a token never enters a
sandbox, which still sees only placeholders the runtime's proxy swaps,
and a runtime without a proxy degrades only behind a warning. Within
unibox a value passes through memory alone: never argv (`security -w
<value>` would show it to `ps`), never the operation log, `--json`, the
state or the profile, and a test holds each of these.

- **Secrets, per platform.** `unibox secret set <name>` asks for a
  value on a hidden prompt and stores it where the platform keeps
  secrets: the login keychain on macOS (`security -i`, the value on
  stdin), the Secret Service on Linux (`secret-tool store`, on stdin).
  `unibox secret ls` says which exist, never what they hold; `unibox
  secret rm` removes one. The store stays the system's, so the user sees,
  rotates and deletes a token with the system's tools too.
- **Portable references.** A profile names a secret, not a platform's
  command: `claude: {token: secret:claude}`, `command: secret:gitlab-2n`.
  unibox turns the reference into the host command of the platform it
  runs on, and the runtime runs that - so a profile is shared between
  macOS and Linux, and survives a move from sbx to podman. A plain
  command stays allowed, written by hand, for a store unibox does not
  know.
- **Agents.** `unibox agent login [agent]`, for the agents of the profiles, or
  one: codex - the runtime's own OAuth login in the browser, as now;
  claude - `claude setup-token` on the user's terminal, then the hidden
  prompt; copilot - opens the page for a fine-grained token, says what to
  grant (Copilot Requests alone), then the hidden prompt. Each writes the
  agent's `auth` into the profile.
- **The user's gh login** can stand in for a GitHub token: `login --gh`
  writes `copilot: {token: gh auth token}` and `service add github --gh`
  a service with `command: gh auth token`; at a terminal with gh signed
  in and nothing of the service's own stored, unibox asks. It says what
  that gives away - the whole gh login - and copilot keeps it as a token
  login, on Copilot's hosts alone, so github.com stays the services'.
- **Services.** `unibox service add <profile> <name>` asks for, or takes
  by flags, the forge or the kind of service, its host and path, the user
  the forge expects, the author and co-author, then the token on the
  hidden prompt; stores it, and writes the `services` entry. unibox knows
  particular services - GitHub, GitLab, an Artifactory - with their
  defaults (`x-access-token`, `oauth2`, `bearer`) and the page where the
  token is made; anything else is the generic form. `unibox service ls`
  and `rm` complete it.
- **Writing the profile.** unibox edits `sandbox.yaml` as a YAML node
  tree, so the user's comments and order stay, and each change is host
  access, applied at the next start.
  The environments repository stays the user's to commit.
- **unagit** builds on these commands: its Settings drive `unibox agent login`,
  `unibox service add` and `unibox secret set`, and a token it mints from
  its vault reaches the platform's store through `unibox secret set`
  (stdin), not a file or a flag.

The `services` and `auth` reference, the warnings and the runtime's
storage are as above; this adds the way in.

## Profiles

### Where they live

Profiles are files in a git repository of the user's, the **environments
repository**, edited and versioned as any repository is - in the editor,
committed with unagit's commit dialog, pushed, shared with a team:

```
sandbox-envs/
├── sandboxes/
│   ├── kotlin/
│   │   ├── sandbox.yaml
│   │   ├── Dockerfile
│   │   ├── start.sh
│   │   ├── instructions.md
│   │   └── .sbx/
│   │       └── build.args
│   └── node/
│       ├── sandbox.yaml
│       └── Dockerfile
├── instructions/
│   ├── unibox.md
│   ├── review-style.md
│   └── no-migrations.md
├── _shared/
│   └── installations/
│       ├── claude.sh
│       └── copilot.sh
└── README.md
```

A directory of `sandboxes/` with a `sandbox.yaml` is a profile, named after
the directory; everything inside it is the profile's own. The layout is held
to its shape (`CheckLayout`): profiles under `sandboxes/` (every directory
there must carry a `sandbox.yaml`), attachable markdown under
`instructions/` (`.md` files alone, none named `instructions.md`), anything
of the user's own behind an underscore (`_shared` by convention),
`README.md` and hidden entries - and nothing else; a stray file is a named
error, never quietly ignored. Runtime directories inside a profile are
hidden (`.sbx/build.args`), and a profile's root holds only its definition,
Dockerfile, start script and `instructions.md` - custom files, an agent
configuration seed say, go under an underscore directory of the profile's
own, `_data` by convention. `unibox init <profile>` lays the whole shape
out (interactively with `-i`) and `unibox validate` checks it, every
profile and every text included. `_shared/` is copied into every profile's
build
context, so a Dockerfile composes what it needs
(its `.install` runner executes `_shared/installations` scripts listed in a
Dockerfile heredoc), and its contents are part of every
profile's revision - a change there marks every image outdated. The
profile's `sandbox.yaml`, its `instructions.md` and its agent configuration
seeds are not image inputs: they change nothing of the image and force no
rebuild.

agentbox knows the repository by its path (`environments` in its
configuration) and reads the working tree - what the user sees in their
editor is what runs. In unagit, Settings picks it **from the list of
repositories** (instance and project path), which keeps unagit's
config.yaml portable; unagit finds its main clone on this machine and
hands unibox the path (`unibox config set environments <path>`) at
every start and whenever the choice changes, if unibox's differs. With
nothing chosen in unagit, unibox's own setting is left as it is.

Without unagit the first command lays the repository out and configures
it: `unibox init --dir <path>`, or `unibox init -i`, which asks where
(here by default) when nothing is configured. With nothing configured,
`ls` and `profiles` say so and how to start, and init without `--dir`
refuses rather than guess; once configured, init names the repository it
writes into. `unibox config unset environments` forgets it and leaves
the files.

`unibox init <profile>` on a profile that exists completes it rather
than skipping it: its agents' seeds and `config` entries and
how each is credited, where it lacks them, and the layout. What it says
already - a seed of its own, a co-author, any section, its resources -
stays, and its agents are its own: `--agents` that differ are refused.
The seeds are not the image's inputs, so completing a profile rebuilds
nothing.

A profile is edited by hand or through unibox's commands (`agent`,
`service`, `network`), which write it as a node tree and keep its
comments. Every write - init's too - lays it out the same way
(`ArrangeProfile`): the sections it has in a fixed order, each under a
comment on what it does and the command that changes it, then a
commented tail of only the sections it lacks, recomputed each time, so a
section unibox writes never lands below examples of itself; a comment
the user wrote over a section is theirs and stays; unagit drives those commands and writes no YAML itself. The
Sandboxes tab shows profiles parsed, and opens a profile's directory or
file in the editor.

### The format

`profile.yaml` is agentbox's own format, kept small, and **knows no
runtime**. It describes the environment; where it runs is chosen at the
start.

```yaml
version: 1                    # the format; see below
dockerfile: Dockerfile        # relative to the profile; or image: <ref>
cpus: 8
memory: 12g
env: [SPRING_PROFILES_ACTIVE] # names; values from the environment
caches:
  - host: ~/.gradle/caches
    env: GRADLE_RO_DEP_CACHE
allow_network: [jitpack.io]  # hosts the runtime's policy would refuse
deny_network: [gist.github.com] # hosts it would allow; deny wins
agents:                       # what the image has, in order; a list of
  claude:                     # names will do, or for each agent:
    auth: {token: secret:claude}  # how it signs in
    co_author: Claude <noreply@anthropic.com> # how it is credited
    config:                   # seeds merged into its own files before every
      .claude.json: _data/claude/.claude.json # session, by the file's name:
      settings.json: _data/claude/settings.json # seed keys win, the agent's
  codex:                      # state stays, a list keeps what it added
    auth: oauth               # the provider's login, kept by the runtime
    config:
      config.toml: _data/codex/config.toml # TOML as well as JSON
start: start.sh               # in the VM, once per boot of the VM
bootstrap: .agent/bootstrap.sh # per worked-on repository, if present, in the VM
instructions:                 # names of instructions/*.md, or .md paths
  - unibox                    # relative to sandbox.yaml; the profile's own
  - review-style              # instructions.md attaches itself
git:
  author: user                # the user's identity, from the host's git config
  co_author: true             # a Co-Authored-By trailer naming the agent
```

- **A profile says its format** (the user's call): `version` is the
  first key (none is format 0), `ProfileFormat` is the one this unibox
  reads, and `unibox --version` shows it. Loading never migrates: a
  profile of an older format is refused with "run unibox init <profile>
  to migrate it", a newer one with "upgrade unibox". `init` migrates on
  the YAML, one format at a time (`migrations.go`, indexed by the format
  a step starts from), keeping comments; it is best effort, so what has
  no place in the new format is left out and named, and a result that
  does not load is not kept. A released migration is frozen: it names the
  files of its own time, not today's agent table.
- **Everything about an agent is under it** (the user's call, 2026-10-15):
  `auth`, `co_author` and `config` sat in three sections before
  (`auth`, `git.co_authors`, `agent_config` keyed `<agent>.<file>`); that
  form is format 0 and is no longer read. The order of
  `agents` is read from the YAML, as sbx signs in by the OAuth of the
  first agent declaring one. A `config` key is the file's own name, which
  unibox maps to its path in the guest home from its table, allowing only
  files it knows to merge: claude `.claude.json` and `settings.json`,
  codex `config.toml`, copilot `settings.json` and `config.json` (its
  state, known but not seeded), opencode `opencode.json`.

- An unknown field is an error naming it, never skipped: a misspelt
  `memmory` must not quietly change the environment. A profile that does
  not parse is listed with the error and cannot be started.
- **Agents start unasked.** `unibox init` writes each agent's seeds from
  unibox's table into `_data/<agent>/<file>` and names them under the
  agent's `config`, so a new sandbox does not stop at an agent's first
  questions: claude's onboarding and theme (`hasCompletedOnboarding`,
  `theme`, `bypassPermissionsModeAccepted`), copilot trusting `/work`
  (`trustedFolders`, kept beside what it trusts later) with its default
  theme, no banner, no updates and no byline of its own, all in its
  settings.json (`copilot help config` lists them), opencode allowed to
  edit, run and fetch (`permission`, `autoupdate: false`), and codex
  working unasked (`approval_policy = "never"`, `sandbox_mode =
  "danger-full-access"` in its config.toml seed), so a codex started by
  hand is as free as one unibox runs. A TOML seed merges as a JSON one
  does; the blocks between `# unibox:begin` and `# unibox:end` that
  codex's setup writes before each session - the proxy's login, the
  trust of /work - stay where they are. The image fixes
  each version, so claude's updater is off (`DISABLE_AUTOUPDATER`).
  Copilot heads its config.json with `//` lines; the merge keeps them.
- **The image is the environment.** Everything static - JDK, Node, the
  agents' CLIs, certificates - goes into the Dockerfile. What depends on
  the checkout belongs to the worked-on repository's bootstrap.
- `allow_network` and `deny_network` are one pattern each - a host,
  `*.domain`, an IP or a CIDR, with an optional port. sbx refuses what its
  policy does not list (403 from the proxy) and already allows the usual
  registries; an allow is added to the sandbox's own policy after
  creation, a deny is given at creation, and a deny wins where both match.
  Both are host access, and a change of them is applied live (below).
- **sbx changes a running sandbox's network rules live.** Checked on a
  throwaway sandbox: `sbx policy allow network --sandbox` let a refused
  host through at once (403 to 200), `sbx policy deny network
  --sandbox` shut a host the default policy allows (200 to 403), and
  `sbx policy rm network --id <rule> --sandbox <name> --force` undid
  either, all without a restart. sbx refuses a deny for a host an allow
  rule of the sandbox names exactly; the allow goes first. So a change
  of `allow_network` or `deny_network` needs no recreate: unibox can
  apply the difference to the running sandbox, keeping the rules it
  added by id, and recreate only for what cannot change live - the
  image, environment, caches, agents. Today any change of the profile
  recreates; a `unibox network allow|deny|rm` that edits the profile and
  applies it live is planned, asking before a recreate only when one is
  needed.
- `token` is for claude beside codex: sbx signs one agent of a sandbox in
  by OAuth, so claude's subscription goes through a long-lived token from
  `claude setup-token`, kept on the host where the command reads it (the
  keychain above). sbx stores the command as a custom secret; claude sees
  a placeholder in `CLAUDE_CODE_OAUTH_TOKEN`, the proxy sends the token to
  api.anthropic.com, and the login outlives a recreated sandbox.
- `auth` is host access like the rest. The runtime keeps
  the logins on the host; unibox only checks that each is there, and
  before a start stores what is missing. A `token` or `command` login is
  stored without a question: its host command is in the profile the user
  wrote, so unibox tells sbx the source and sbx runs it. An OAuth
  login or a key is offered instead - sbx asks for it on the terminal
  (`sbx secret set ...`, the browser for an OAuth login). Either way the
  value never passes through unibox. sbx stores locally only ChatGPT's
  OAuth login (claude's OAuth is a sign-in inside the sandbox, kept by
  the proxy) and signs one agent of a sandbox in by OAuth, so two `oauth`
  agents in one profile are refused, and the `oauth` one is declared
  first in the generated kit.
- `image:` instead of `dockerfile:` names an image built elsewhere (a
  team's CI); then nothing is built.
- **Caches are read-only from the host.** `~/.gradle/caches` mounted
  read-only as Gradle's read-only dependency cache: what the host already
  has is not downloaded again, and what is missing goes to the VM's own
  `GRADLE_USER_HOME`, kept with the VM. Read-write would let the agent put
  a jar or an `init.d` script that the host's next build runs, and Gradle's
  file locks do not hold between VMs over virtiofs. Gradle does not promise
  consistency while the host writes the cache - at worst a build in the VM
  fails and runs again. A mount is at the host's absolute path, as sbx
  mounts everything.
- Considered: devcontainer.json (a standard, but agentbox would honour a
  fraction of it and the rest would silently not work; the read-only cache
  and the network rules do not map onto it) and sbx's `sbxenv.yaml`
  (sbx's alone, and experimental).

### Three scripts

Each at its own moment, all running inside the VM:

| what | lives in | runs | for |
| --- | --- | --- | --- |
| `Dockerfile` | the profile | when the image is built | everything static: toolchains, tools, certificates |
| `start` | the profile | once per boot of the VM | services and daemons (a shared `docker compose`), configuration from the environment |
| `bootstrap` | the worked-on repository | after a task's clone or fetch | the project's dependencies, generated code |

- **`start` runs once per boot, whoever booted the VM.** A VM can be
  started by something else (`sbx exec` from a terminal, sbx's TUI), so
  before every agent agentbox compares the kernel's `boot_id` with a mark
  it left in the VM (`/var/lib/agentbox/started`) and runs the script only
  when they differ, by `sbx exec`, its output in the preparation's output.
  A failure stops the start.
- Nothing runs once when the VM is made: what should happen once is static
  and belongs in the Dockerfile. If that proves wrong, `setup:` comes the
  same way.
- The scripts run only inside the VM, so they need no approval.

### Instructions for the agent

The agent has to know how it is meant to work - its branch, to commit and
push as it goes, never to force - and the user has more to say, in general
and per profile. None of it may go into the worked-on repository, where
it would be committed. So:

- **Nothing is fixed in the binary.** Every word the agent gets comes from
  the environments repository: the texts live in `instructions/`, one `.md`
  per name, and the former built-in rules are just a text there (`unibox`
  by convention) a profile lists like any other.
- **The profile's `instructions` are names or paths**: a bare name is a
  text of `instructions/`, the one namespace a sandbox attaches more from
  (`--instructions`, `unibox instructions attach`); an entry with a path or
  extension is a `.md` relative to `sandbox.yaml`, confined to the
  repository. The profile's own `instructions.md` attaches itself, after
  the listed texts. Each file goes in once; a name or path that does not
  exist is an error. They go only into the VM, so they need no approval.
- At a task's preparation unibox writes the whole into the VM's home as
  the agent's user-level instructions (`~/.claude/CLAUDE.md`,
  `~/.codex/AGENTS.md` - each agent's file is in unibox's table of
  agents), never into `/work`.
- `unibox instructions <profile>` prints them as the agent gets them
  (`--sandbox` for a sandbox's, with its attached texts);
  the Sandboxes tab shows that in a pane under Profiles, rendered by
  `internal/md`, and Edit Instructions… opens the files in the editor.

### Commits

- `author: user` takes `user.name` and `user.email` from the host's git
  configuration of the worktree (unibox reads it itself; unagit passes
  `--author` only for an author the user chose), and
  sets them in each clone in the VM. The commits are the user's, so a
  forge's push rule that the author must be one of its users passes.
- A **git service** may name the author and the co-author for the
  repositories under its URLs (`author: "Name <email>"`, `co_author:
  false`). The author is the profile's default, over the host's
  configuration; an explicit `--author` still wins, since it is what the
  user asked for this run, so unagit passes `--author` only when the user
  chose one, or a service's author never shows. `co_author` has no flag
  and wins over `git.co_author`. Under several services each field comes
  from the most specific URL that sets it, as git picks its `http.<url>`
  settings. Services are host access; a change of the identity is told
  with the rest. The task's record holds the result, so changing it on a
  prepared task asks for another task name, as a changed `--author` does.
- **A clone the agent makes itself** gets its service's author too: the
  sandbox's setup - at creation and before every session - writes the
  services' extraHeaders and identities into its global git
  configuration, the identities as files under
  `~/.config/unibox/identities` included with
  `includeIf "hasconfig:remote.*.url:<url>**"`, least specific first so
  the most specific wins (git 2.36 or later; checked with a host and a
  path of one forge). A clone unibox prepared keeps its own
  `user.name`/`user.email`, which outrank the includes. Such a clone has
  no co-author hook and is not in the task's record: what must be kept
  belongs in `unibox add-repo`.
- `co_author: true` adds `Co-Authored-By: <agent>` to every commit, and
  unibox alone says it. A `commit-msg` hook in each clone in the VM takes
  out any co-author trailer the agent added of itself - copilot always
  does unless told, as `Co-authored-by: Copilot <223556219+Copilot@...>`,
  and claude would - but both seeds turn the byline off
  (`includeCoAuthoredBy: false` in claude's and copilot's settings.json
  seeds), the hook covering whatever an agent adds anyway -
  and adds the profile's. The agent is the
  session's: `run` and `resume` name it to the hook (`UNIBOX_AGENT`, as
  the table calls it, and `UNIBOX_CO_AUTHOR`, as the profile credits
  it), so a task resumed with another agent credits that one, and a
  commit made by hand in `unibox shell`, with no agent running, is left
  as it is. `co_author: false` - the profile's, or a git service's for
  its repositories - keeps the hook but has it add nothing, so off means
  no co-author, not the agent's own. The hook is rewritten in every clone
  before each session. An agent's `co_author` gives it another name and
  address than the table's (`unibox agent co-author <profile> <agent>
  "Name <email>"`); init writes the defaults out, so the profile shows
  them, and `Name <>` credits an agent with no address - a trailer is
  text, shown though linked to no account. It is read per session, so
  changing it recreates nothing. A hook in the VM's own clone is the VM's
  business; nothing of it reaches the host.
- Absent, `git` means both as shown.

### What differs between runtimes

`profile.yaml` says nothing about a runtime, but an image may have to be
built a little differently for one - another `FROM`, a step only one of
them needs. That is said with Docker's own means, build arguments, from a
subdirectory named after the runtime:

```dockerfile
ARG BASE=ubuntu:24.04
FROM ${BASE}

ARG AGENTBOX_RUNTIME
USER root
RUN apt-get update && apt-get install -y openjdk-21-jdk git curl
RUN if [ "$AGENTBOX_RUNTIME" = sbx ]; then echo "sbx alone"; fi
```

```
# kotlin/.sbx/build.args
BASE=docker/sandbox-templates:shell
```

- Building for runtime X, agentbox always passes `--build-arg
  AGENTBOX_RUNTIME=X`, so a condition needs no file, and every `KEY=VALUE`
  line of `X/build.args` as a `--build-arg`. No `X/` means the `ARG`
  defaults, which also keep the Dockerfile buildable by hand.
- Larger differences can be a stage per runtime (`FROM common AS sbx`,
  built with `--target`); not until one is needed.
- `build.args` is the one file a runtime directory has for now; an unknown
  file there is an error, as an unknown field is.
- The tree hash covers the whole profile, so a change under `podman/`
  marks sbx's image outdated too. A needless rebuild, kept for the simple
  rule.

### Trust

The environments repository is the user's: their choice, in their git,
edited by them or through unibox's commands, and no agent edits it on its
own. So a change of a profile is **told, not asked about** (the user's
decision, 2026-10-15; it replaced a `direnv allow`-like approval, which
only got in the way):

- unibox's state keeps, per profile, its host-facing fields as last seen.
  `check` names what changed as a warning (`profile_changed`); the next
  start says it once - `profile web changed what it asks of the host:
  services + github, allow_network + jitpack.io` - and goes on. A change
  made through unibox's own commands is not told again.
- What the change leads to is said too: an image built from changed
  inputs is built without a question (with the diff since its last
  build), and a sandbox made from changed settings is recreated, naming
  them - `recreating sandbox web-1: services, the image was rebuilt`.
  The checks for unpushed work and running sessions still stand before a
  recreation.
- What the runtime can change live is applied live instead: each adapter
  says what it can (`NetworkUpdater` for network rules), and what it
  cannot is part of the settings a recreation makes. `unibox network
  allow|deny|rm` changes a profile's rules and applies them to its
  sandboxes at once, asking only where a recreation would be needed.
- The worked-on repositories still choose nothing: they give the bootstrap
  script, which runs inside the VM.

### sbx's templates and kits

sbx has two words for shaping a sandbox; the design uses one and keeps away
from the other.

- **A template** is the image a sandbox is made from: an OCI image in sbx's
  own image store (apart from Docker's), its root filesystem being what is
  installed in the VM - the OS, the toolchains, the agents' CLIs. `sbx
  create -t <template>` picks it. It is how the environment gets into the
  VM, so it is what a profile's Dockerfile becomes: built, saved and
  loaded (`sbx template load`), tagged by the profile's tree.
- **A kit** is a declarative package laid over a sandbox: files, an
  install hook, a startup hook, network rules, credential bindings,
  instructions for the agent. The built-in agents are kits; a *mixin* kit
  adds to one (`--kit`). Kits v3 are Early Access. **agentbox neither
  writes nor takes kits**: everything a profile says is expressible as
  flags and `sbx exec`, and a format still moving is not one to build on.
  If the spike finds something only a kit can do - the credential for
  git's push is the candidate - that is decided then.

### One image, several agents

A profile is one image holding every agent it offers, built on Docker's
agent-less base, and its VMs are made as shell sandboxes:

```dockerfile
# kotlin/.sbx/build.args:  BASE=docker/sandbox-templates:shell
ARG BASE=ubuntu:24.04
FROM ${BASE}
USER root
RUN apt-get update && apt-get install -y openjdk-21-jdk nodejs npm
RUN npm install -g @anthropic-ai/claude-code @openai/codex
USER agent
```

`sbx create shell …` makes the VM, and each agent runs in it by `sbx exec
… claude`, `sbx exec … codex`.

- **A shared VM can hold several agents.** An image per agent
  (`sandbox-templates:claude`) would allow one.
- **One build per profile**, not per profile and agent: fewer builds, less
  disk, one image state on the tab.
- **The agents' versions are the user's**, in the Dockerfile, versioned. A
  new Claude Code is a rebuild; updating them in `start` instead trades
  that for a slower start.

What makes it likely to work, checked 2026-10-12: `docker/sandbox-templates`
has a `shell` tag beside the agents' (claude, codex, copilot, cursor,
devin, docker, droid, gemini, kiro, opencode). And the agents' logins are
not the agent kit's: sbx keeps them as service secrets (`anthropic`,
`openai`, `github`), global or per sandbox, and its proxy signs the
requests to those services with them, the key never entering the VM - so
they should hold for any process in it, an agent started by `exec`
included.

**Spike**: that `sbx exec … claude` in a shell VM does get its login
through the proxy, and whether sbx needs anything of a template beyond
`:shell`'s base. Should the login fail, the fallback is a VM per profile
and agent made by `sbx create <agent>`, the image still one.

### Which agents a profile offers

The profile declares them (`agents:`), and the build checks the
declaration against the image:

- **Declared**, so what to offer is known before anything is built, for an
  `image:` too without pulling it.
- **Checked**: after a build, before the image is loaded into the runtime,
  agentbox runs `docker run --rm <image> sh -c 'command -v …'` for the
  declared agents' commands. One missing fails the build, naming it
  ("profile kotlin declares codex, but the image has no codex - install it
  in the Dockerfile"), so the declaration cannot quietly stop being true.
  One present but not declared is mentioned and not offered. An `image:`
  is checked the same way once pulled.
- **Offered** by unagit: the agents declared by the profile and not
  turned off in unagit's Integrations. Whether an agent is installed on
  the host does not matter here; the VM runs the image's. The agent picker
  lists the agents of every profile, the profile picker then the profiles
  that have the one chosen, skipped when there is one.

agentbox has its own table of agents - id, command, its instructions file,
its co-author line, sbx's name for it - and never reads unagit's
`agents.All`; the ids are the same, so unagit maps them.

### Building

- `<tree>` is the git tree hash of the profile's directory at HEAD, so
  whether the image is current is a comparison with `sbx template ls
  --json`, per runtime: **built**, **outdated**, **not built**. A profile
  with uncommitted changes is marked so, and builds as it is, tagged
  `<tree>-dirty`.
- For sbx: `docker build` with the build arguments, the agents' check,
  `docker save`, `sbx template load`, tagged `agentbox/<profile>:<tree>`.
  A local build is for this machine's architecture, which is all a local
  sandbox needs.
- **A build shows what it is about to build.** `agentbox diff <profile>`
  prints what changed in the profile's directory since its image last
  built (the tree is in the tag) as a unified diff. unagit's Build…
  question draws it in unagit's own differ - the one the Changes dialog
  draws (`drawDiff`, `diffdraw.go`) - before it asks. A teammate's
  Dockerfile runs in the host's Docker when built; isolated there, but
  `RUN --mount` and build secrets can reach out, so the user sees it
  first. No approval of its own beyond that question.

## agentbox

### Its home

```
~/.config/agentbox/config.yaml   the user's choices
~/.config/agentbox/state.json    what use taught: approvals, VMs, tasks,
                                 the agents it runs, the boot marks
~/.cache/agentbox/               generated: tars, never edited
```

```yaml
environments: ~/workspace/bitbyone/sandbox-envs
runtimes:
  sbx:
    on: true
    stop_idle: true
```

A runtime not named is on when installed. unagit writes this file only
through `agentbox config set`, never by hand, and only what the user chose
in unagit's Settings.

### The command line

The contract unagit is written against. Everything a list shows has
`--json`; a long operation prints its progress as lines, for a log.
Called by unagit (`--json`, or `--yes` where a question would come) it
never asks: what stands in the way is an answer, not a prompt.

| command | does |
| --- | --- |
| `agentbox init <profile> [-i]` | lays out the environments repository and adds a profile; -i asks |
| `agentbox validate [--json]` | the repository's layout, every profile, every attachable text |
| `agentbox profiles [--json]` | profiles: name, agents, image state per runtime, approval, error, last commit |
| `agentbox instructions <profile>` | the instructions as the agent gets them |
| `agentbox diff <profile>` | what changed since the last build, a unified diff |
| `unibox agent login\|co-author\|ls`, `unibox service add\|ls\|rm`, `unibox network allow\|deny\|rm\|ls`, `unibox secret set\|ls\|rm` | configure a profile and keep its tokens (see Configured through unibox) |
| `agentbox build <profile> [--runtime R]` | builds, checks the agents, loads |
| `agentbox check …  --json` | what stands in the way of a start: default branch, uncommitted, approval, image |
| `agentbox prepare …` | the VM and the task, ready; JSON of both at the end |
| `agentbox run …` | the agent, in this terminal; prepares first if needed |
| `agentbox ls [--json]` | VMs: runtime, profile, state, memory, tasks |
| `agentbox tasks <vm> [--json]` | tasks: directory on the host, repositories, branch, agents |
| `agentbox shell <vm>` | a shell in the VM |
| `agentbox stop <vm>`, `rm <vm>`, `rm-task <vm> <task>` | as named |
| `agentbox config get/set <key> [<value>]` | its configuration |
| `agentbox herdr install-plugin`, `uninstall-plugin`, `status [--json]` | [herdr](#herdr) |

`check`, `prepare` and `run` take the same task: `--profile`,
`--agent`, `--runtime`, `--vm <name>` or `--new-vm`, `--branch`,
`--repo <dir>=<clone URL>` (repeated; or directories, as without unagit),
`--author "Name <email>"`.

Exit codes say what kind of failure: 3 image outdated or
not built, 4 refused (a default branch, a branch the forge lacks), 1 any
other error. Errors are lower case and say what to do: `sbx is not signed
in - run sbx login`.

### The runtime

agentbox's neutral vocabulary; nothing above it says sbx.

```go
type Runtime interface {
	Name() string
	Available() error                     // installed, signed in, daemon up
	List() ([]Box, error)                 // name, profile, state, memory
	Images() ([]Image, error)             // built templates, by tag
	Build(p Profile, log io.Writer) error // the profile's image, for this runtime
	Create(spec Spec) error               // detached, idempotent by name
	Exec(name string, cmd Command) *exec.Cmd // -it when Interactive
	Stop(name string) error
	Remove(name string) error
}
```

sbx's side, from its `--help`:

| Runtime | sbx |
| --- | --- |
| `List` | `sbx ls --json` |
| `Images` | `sbx template ls --json` |
| `Build` | `docker build`, `docker save`, `sbx template load` |
| `Create` | `sbx create --name N -t T --cpus C -m M --deny-network H -e K shell <mounts>` |
| `Exec` | `sbx exec [-it] [-w DIR] [-e K=V] N cmd…` (starts a stopped VM) |
| `Stop` | `sbx stop N` |
| `Remove` | `sbx rm N` |

How a profile becomes flags:

| profile | sbx |
| --- | --- |
| `dockerfile` | the built template, `-t agentbox/<profile>:<tree>` |
| `image` | `-t <image>` |
| `cpus`, `memory` | `--cpus`, `--memory` |
| `deny_network` | `--deny-network` |
| `env` | `-e NAME` |
| `caches` | `<host>:ro` as an extra workspace, `-e <env>=<host>` |
| `start` | run by `sbx exec` when the VM's `boot_id` is new |
| `bootstrap` | run by `sbx exec` after the clones |

Spike questions about it:

- `sbx create` takes an agent and mounts the current directory when given
  no path. A VM must be created with no repository mounted: give it a
  directory of agentbox's own (`~/.cache/agentbox/vms/<vm>`, empty,
  read-only) as its workspace, unless sbx can do without one.
- Does `sbx exec` take `-w`?
- Do read-only mounts of host directories outside the workspace work
  (`<path>:ro` as an extra workspace)?
- Does `sbx template load` take a tar from `docker save`, or only one from
  `sbx template save`? If only the latter: build in a sandbox and `sbx
  template save` it, or push to a registry.

### herdr

herdr recognises an agent by its pane's foreground process, and an agent
behind `sbx exec` is not one it knows: no agent, no state. agentbox fills
that in with **a herdr plugin of its own**. It is the one way; there is no
launcher and no third-party plugin to choose instead.

What it does, as long as herdr runs - unagit open or not:

- `agentbox run` started in a herdr pane records the pane
  (`HERDR_PANE_ID`), the agent and the VM in agentbox's state. For every
  pane so recorded whose `agentbox run` is alive, the plugin reads the
  screen (`pane.read`), has herdr classify it by herdr's own rules for that
  agent (`herdr agent explain --file … --agent <kind> --json`), and reports
  the state (`pane.report_agent`, `pane.release_agent` when the agent is
  gone), with the VM's name as the pane's metadata
  (`pane.report_metadata`). It reports only on a change.
- It never parses argv: agentbox started the agent and wrote it down.
- unagit's Activity matches herdr's agents by pane, so the state arrives
  there with no change to it.

The idea, and the herdr calls, come from
[sbx-herdr](https://github.com/dvdksn/sbx-herdr) (read 2026-10-12 at
0.1.0). It has no licence, so none of its code is taken.

**The plugin is a manifest and nothing to build.** `herdr-plugin.toml`, its
commands calling the agentbox binary: a startup command and the
`pane.created` / `pane.focused` events run `agentbox herdr ensure`, which
starts one `agentbox herdr observe` per herdr socket (a lock in
`HERDR_PLUGIN_STATE_DIR`, so a second ensure does nothing), detached, its
log in that directory. Both are hidden commands that refuse to run outside
herdr (no `HERDR_SOCKET_PATH`).

**Installed only with the user's consent**, never quietly - it changes
herdr's configuration and leaves a process running with herdr:

- `agentbox herdr install-plugin` asks unless `--yes`; it writes the
  manifest, embedded in the binary, to `~/.config/agentbox/herdr-plugin/`
  with the absolute path of the running agentbox, then `herdr plugin link
  <dir>` and `enable`. `uninstall-plugin` undoes it.
- unagit's Settings › Integrations, in herdr's block, shows `agentbox herdr
  status --json` and offers Install herdr Plugin… (and Uninstall) behind
  its own question, then calls the command with `--yes`.
- The path changes when agentbox is upgraded or moved. Every agentbox
  command that finds the plugin installed compares the linked manifest
  with what it would write and rewrites it if it differs - the user
  consented to the plugin, not to a path.
- A sandboxed agent started in herdr without the plugin runs, with no
  state; unagit says once (`note`) that Install herdr Plugin… in Settings
  would show it.

The agents' own hooks (Claude's session id, opencode's states) do not cross
into the VM; the state is the screen's.

**Spike**: the herdr here is 0.9.1 and sbx-herdr asks for 0.9.3 - find the
version whose plugins have `pane.read`, `agent explain` and the events, and
have `status` say "herdr 0.9.3 or newer" when older. Check that `herdr
plugin link` takes a manifest whose commands are absolute paths outside its
directory, and that a plugin's commands get `HERDR_SOCKET_PATH` and
`HERDR_PLUGIN_STATE_DIR`.

### The boundary

- agentbox's code is `cmd/agentbox` and `internal/agentbox/...`. It
  imports nothing of unagit's packages; a test (`go list -deps
  ./cmd/agentbox`) fails if it does, as `TestSharedFieldsAreTheOnlyWayToMakeThem`
  fails on a stray widget.
- unagit imports nothing of agentbox but `internal/agentbox/api`, the JSON
  types of its command line; a test fails on anything else. unagit runs
  the binary, found through `a.executable("agentbox")` like any program.
- agentbox follows the repository's rules - gofmt, vet, race, the
  testenv isolation, the hard rule on tokens - as any package here.

## In unagit

**An integration**, in Settings › Integrations: agentbox on or off
(unset, on when installed), and its block:

- the environments repository, picked from the list of repositories and
  handed to agentbox as a path (see [where they live](#where-they-live));
- the runtimes agentbox has, each on or off, and stopping idle VMs -
  written to agentbox's configuration with `agentbox config set`;
- in herdr's block, agentbox's herdr plugin.

unagit's config.yaml keeps only what it must to be portable: whether
agentbox is on, and the environments repository as a repository of the
list. The rest is agentbox's.

**A Sandboxes tab**, `6`, after Settings (`5` stays, as the messages that
point at `Settings [5]` do). Three parts, each a list made the way the
other screens' are (`pane.go`, its actions as `uiAction`s), filled from
agentbox's `--json`:

| part | rows | actions |
| --- | --- | --- |
| **Profiles** | name, image state per runtime, approval, agents, last commit to its directory; a pane with its instructions as the agent gets them | Open in Editor…, Edit Instructions…, Build…, Approve…, Open in Sandboxed Agent… |
| **Sandboxes** | VM: runtime, profile, state, memory, tasks | Shell… (`agentbox shell` in a place), Stop, Remove (a question naming its tasks) |
| **Tasks** | in the VM under the cursor: worktree, agent, its state | Open Worktree, Remove Task |

Without agentbox installed or an environments repository chosen, the tab
says what is missing and where to set it. Activity still lists every
agent, sandboxed ones with their VM: Activity is what runs, the Sandboxes
tab is what it runs in.

Elsewhere:

- **Open in Sandboxed Agent…** on repositories with a worktree, worktrees,
  groups, merge requests with a branch worktree. Alt picks again what use
  would skip (the profile and the sandbox, even when one).
- **Pull** of a worktree or a group is what brings the agent's work; it
  exists already for branches with an upstream, check it covers a group in
  one go.
- **A worktree whose branch an agent works on in a VM** (agentbox's tasks
  name the host directory) wears a mark in the lists, and the commit
  dialog on it warns before a commit: "an agent works on this branch in
  <vm> - pull first". Nothing is forbidden; two commits apart on one
  branch only end in a conflict at the pull.
- Deleting a worktree or a group with a task in a VM offers to remove the
  task (`agentbox rm-task`) and, for a VM of its own, the VM. The default
  keeps the VM.
- What unagit learns from use - the order of the profile and sandbox
  pickers - goes to its `state.json`, as the place picker's does.
- Help rows for the tab and the new keys; the README explains the
  feature, agentbox, the environments repository and the forge-side
  protection.

Later, not in the first version: following the branch on the forge
(`internal/watch`) to say when the agent pushed, and pulling on request
from that news.

## Tests

`make test-unibox` runs only `cmd/unibox/...` and `internal/unibox/...`, with
the race detector; `make test-unibox-fast` runs the same packages without it.
These are the checks while implementing unibox, including before commits
that change only unibox. The unagit suite is not run for those changes.
`make test` and `make test-fast` retain the whole repository's suite for
changes across the boundary or to shared build machinery. The binary and
package name agreed after the original design is `unibox`.

agentbox:

- A stand-in `sbx` and a stand-in `docker` on their own PATH
  (`testenv.WriteExecutable`), answering `ls --json` and `template ls
  --json` from files and logging every argv: the commands for a shared and
  an own VM, the task script, `start` run on a new `boot_id` and not on the
  same one, the VM stopped when its last agent ends and not one it did not
  make.
- Profiles: parsing (an unknown field, a missing instructions file are
  errors), the tree hash and the image state, the build arguments
  (`AGENTBOX_RUNTIME` always, `build.args` of the runtime alone), the
  generated flags, the agents' check failing a build that lacks one, the
  approval refusing a changed cache or `env` and passing a changed `cpus`,
  the instructions put together in order, the author and the trailer added
  by the hook once.
- The command line: `--json` shapes against `internal/agentbox/api`, the
  exit codes, no question asked under `--json` or `--yes`; without unagit,
  URLs from `origin` with credentials stripped, a branch the forge lacks
  refused.
- No token in any argv or environment agentbox builds.
- The herdr plugin: the manifest written with the binary's path and
  rewritten when it moves; `observe` against a stand-in `herdr` reporting a
  state on a change only and releasing a pane whose `agentbox run` ended;
  the install asking first. A real herdr only in `make integration`.
- A real sbx only opt-in by a variable on macOS (`AGENTBOX_SBX_TEST=1`), as
  Ghostty and the keychain are - sbx needs a hypervisor, not the Docker
  image of `make integration`.

unagit:

- A stand-in `agentbox` on its own PATH answering the `--json` commands
  from files and logging its argv: the order check → (approve, build) →
  push → prepare → run; the clone URLs from the forge, never from a
  remote; the environments repository handed over as a path; no token in
  any argv or environment unagit gives it.
- The questions on what `check` reports: a default branch refused,
  uncommitted changes named, Approve… and Build… drawing their diffs.
- The commit dialog's warning on a branch an agent works on.
- UI: the Sandboxes tab and the pickers at several terminal sizes,
  `assertLegible`, agentbox's block in Integrations, help rows,
  `TestEveryActionIsNamedAndExplained`, `TestNoTwoActionsShareAKey`.
- The boundary tests of [the boundary](#the-boundary).

## Order of work

1. **Spike by hand**, everything marked so above, written back into this
   file: a VM with no repository mounted, a clone and a push from it to
   the user's GitLab with an sbx secret, an agent by `sbx exec` in a shell
   VM with its login, `-w`, the read-only Gradle cache, `sbx template load`
   of a `docker save`, what a template needs beyond `:shell`, the herdr
   plugin API.
2. agentbox's core: the runtime with sbx, profiles, the build and the
   agents' check, approvals, its stand-ins and tests.
3. agentbox's tasks: `check`, `prepare`, `run`, the instructions, the
   author and hook, `start`, stopping idle VMs; usable from a terminal
   without unagit.
4. unagit's integration: Settings and the configuration handed over, Open
   in Sandboxed Agent… for one repository's worktree in This Terminal; then
   groups; then every place.
5. The Sandboxes tab, the removal with a worktree, the commit warning,
   Activity.
6. agentbox's herdr plugin and its install from Integrations.
7. AGENTS.md's map gains `cmd/agentbox` and `internal/agentbox`; the
   README its section, and one of agentbox's own.

## Kits v3: multi-agent credentials (validated 2026-10-13)

The decision above to take no kits is reversed for one purpose: a v3
workload kit is how one sandbox gets several agents' credentials. Verified
by hand (`agent-sandboxes/_spike/personal-kit`): a local kit directory -
a `# syntax=docker/sandbox-kit:3` descriptor beside a Dockerfile that is
just `FROM` the profile's built image - creates a sandbox
(`sbx create --name X ./dir`) whose `credential@1` declarations bind the
host's stored secrets: the spike sandbox had `SBX_CRED_OPENAI_MODE=oauth`
over the three-agent personal image. The sbx adapter will generate that
directory from the profile - the profile stays the one runtime-neutral
configuration, and a later podman adapter satisfies it its own way.

Facts the implementation stands on:

- The kind of a sandbox is what binds credentials: built-in kinds are v2
  kits; a single-agent profile already uses its agent's kind (done). v3
  kits cannot mix with v2 built-ins, so the multi-agent path needs a v3
  workload, created from a local directory (no registry; local kits are
  allowed by default).
- The agents' authoritative declarations - endpoints, sentinels,
  credential files, onboarding seeds - are docker's example mixins in
  github.com/docker/sandbox-kit-spec (claude-mixin, codex-mixin, ...).
  The adapter borrows their credential and lifecycle blocks; the neutral
  facts (service name, hosts) belong in unibox's agents table.
- `sbx` ignores `apiKey.inject[].scheme` in v3 kits, so git-over-https
  basic auth cannot be declared by a kit today; gh works (Bearer on
  api.github.com). Git push from a multi-agent kit sandbox is the open
  question; the built-in github service covers it for single-agent kinds.
- A kit's credential is injected only with a binding in
  `~/.config/sbx/credentials.yaml` ("no binding authorizes ... the
  credential was not injected"). sbx 0.47 asked no question at creation,
  not even on a terminal, so unibox writes the bindings itself when it
  creates a kit sandbox: each agent's service with exactly the domains its
  declaration names, added and never removed - what a built-in kind gets
  without one.
- sbx keeps one OAuth policy per sandbox, taken from the first credential
  declaring OAuth whether or not a login is stored for it. The profile's
  order put Claude first, and the stored ChatGPT login (`sbx secret set
  openai --oauth`, the one OAuth sbx stores locally) was never wired. The
  kit declares first the services `sbx secret ls` shows an OAuth login for.
- Copilot calls its plan's API host - api.individual, .business or
  .enterprise.githubcopilot.com - besides api.githubcopilot.com. An
  undeclared host gets the placeholder token as it is, and Copilot answers
  400 "Authorization header is badly formatted"; the kit routes all of
  them to the github service, as sbx's own kits do.
- Codex signs in only through its own files: before each codex session
  unibox writes what codex-mixin seeds - `forced_login_method`, the
  `sandboxd` provider routing to chatgpt.com, an `auth.json` placeholder -
  from `SBX_CRED_OPENAI_MODE`, between markers in `config.toml`. Claude
  needs no such step; its kit declaration renders
  `~/.claude/.credentials.json` in the guest (on a Mac host Claude keeps
  its login in the keychain, so no such file exists there).
- No agent stops to ask whether to trust what unibox prepared: codex gets
  `trust_level = "trusted"` for /work's task directories and repositories
  in its config.toml, claude `hasTrustDialogAccepted` for /work, the task
  and its repositories (and `bypassPermissionsModeAccepted`, as sbx's
  claude kit seeds) merged into `~/.claude.json` before each session, and
  copilot runs with `COPILOT_ALLOW_ALL=true`, which it reads as trust for
  every folder - what `--allow-all` already grants.

## Follow-ups (2026-10-13)

- Git-over-https from a kit sandbox is solved by `http.extraHeader`
  carrying a custom-secret placeholder the proxy substitutes (see Host
  services the sandbox reaches); the `services` profile field and its sbx
  wiring are the remaining implementation. Confirm the custom-secret env
  var reaches a non-git tool in a real unibox kit before shipping one.
- The session-lock tests that failed one run in a few under full parallel
  load were a real race, now fixed: a flock belongs to the open file
  description, and a process another goroutine forked holds a copy of it
  until its exec, so closing the lock's file left it held for that moment
  and the next non-blocking try found it busy (or, with the error
  ignored, took nothing and stopped a sandbox in use). FileLock.Close
  unlocks first, which releases every copy.
- unibox ls could hint at a missing sandbox's row that recreate restores
  it and rm removes the record.
