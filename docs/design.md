# Sandboxed agents: unibox

## Where it stands

Built and in daily use (2026-10-15): profiles of format 6, sbx as the
supported runtime, microsandbox as an experimental one that shows profiles
carry over (2026-10-17), tasks, the agents' logins, services and network rules, the herdr
plugin. This began as unagit's plan - it replaced "Agents in a container"
in unagit's docs/herdr.md - and keeps its reasoning, so that the decisions
are not undone by accident. What unagit builds over unibox is unagit's
plan, in its docs/sandboxes.md and docs/sandbox-credentials.md; what the
two agree on is [the boundary](#the-boundary), and what comes next for
both is [the order of work](#order-of-work).

Checked against `sbx` 0.47 on macOS (Apple Silicon): its commands and
flags below come from its `--help` and from use. What is still marked
**spike** was not written back after it was tried.

## What it is for

A worktree - one repository's or a group's - gets a coding agent that runs
in a VM instead of on the machine. The agent has full autonomy inside it;
the machine, its home, its keys and unagit's tokens are out of its reach.
The user picks the agent, a sandbox profile (a Dockerfile and the
resources, kept in a git repository of their own), and whether the agent
shares a VM with others or gets one of its own.

## Two programs

The sandboxing is **unibox**, a program and a repository of its own,
usable without unagit; **unagit is a UI over it** and one of its users.

- They share no code. unibox imports nothing of unagit's; unagit runs the
  unibox binary and reads its `--json`, whose types are
  `internal/unibox/api`, versioned by `api_version` in every object.
- unagit drives it as it drives herdr or Zellij: through its command
  line, `--json` for what it reads and `--yes` where a question would
  come.
- unagit also **configures** it, and only through its commands: the
  environments repository picked in unagit's Settings and the runtimes
  with `unibox config set`, a profile's logins, services and network
  rules with `unibox agent|service|network|secret`, an older profile with
  `unibox init <profile>`. unagit writes no YAML.

| unibox | unagit |
| --- | --- |
| its home: configuration, state, cache | Integrations: unibox on or off |
| the environments repository: profiles, their format and its migration, build, the agents' check | which worktree, which repositories, which branch |
| the runtimes: sbx now, Podman later | pushing the branch, with the vault's token |
| sandboxes and tasks: create, start, stop, remove | clone URLs from the forge's API |
| a task's preparation: clones, author, co-author hook, instructions, seeds, `start` | the Sandboxes tab, the pickers, Activity |
| the tokens a profile names, in the platform's store (`unibox secret`), reaching a sandbox as placeholders | its vault; later a source of short-lived tokens for a service's `command:` |
| running the agent, stopping an idle sandbox | the questions: build, sign in, migrate, commit first |
| its herdr plugin | the commit dialog's warning, the pull |
| | picking the environments repository from the list, handing unibox its path |

## The picture

The parts:

```
+------------------------------- unagit (host) --------------------------------+
|  internal/ui                                                                 |
|  +------------------+ +------------------+ +------------+ +----------------+ |
|  | Sandboxes tab [6]| | Open in Sandboxed| | Activity   | | Settings [5]   | |
|  | Profiles         | | Agent...         | |            | | Integrations:  | |
|  | Sandboxes (VMs)  | | agent > profile  | |            | | unibox, env    | |
|  | Tasks            | | > sandbox > place| |            | | repo, runtimes | |
|  +--------+---------+ +--------+---------+ +-----+------+ +-------+--------+ |
|           |                    |                 |                |          |
|           +--------------------+--------+--------+----------------+          |
|                                         | unibox ... --json                  |
|  workspace, gitx: push the branch, pull | (and unibox config set ...)        |
+---------+-------------------------------+------------------------------------+
          |                               |
          |                               v
          |      +-------------------- unibox (host) ----------------------+
          |      | ~/.config/unibox/config.yaml, state.json                |
          |      | profiles: format, migrate, build, agents' check, edits  |
          |      | tasks: clones, instructions, author, hook, seeds        |
          |      | Runtime: Available List Images Build Create Exec Stop   |
          |      |          Remove                                         |
          |      |   +----------------+      +----------------+            |
          |      |   | sbx adapter    |      | podman adapter |            |
          |      |   |                |      | (later)        |            |
          |      |   +-------+--------+      +----------------+            |
          |      | herdr plugin: unibox herdr observe                      |
          |      +-----------+---------------------------------------------+
          |                  | sbx CLI
          | push, pull       v
          v           +--------------------------------------+
   +-------------+    | sbx microVM  unibox-<sandbox>        |
   | forge       |<-->|  template = the profile's image      |
   | GitLab,     |    |  /work/<task>/<repo>   git clones    |
   | GitHub      |    |  the agents (claude, codex, ...)     |
   | protected   |    |  Docker Engine (compose, tests)      |
   | branches    |    |  GRADLE_USER_HOME (the VM's own)     |
   +-------------+    |  ~/.gradle/caches :ro <- host        |
     clone, push:     +--------------------------------------+
     a narrow token,
     a placeholder the
     proxy swaps
```

Where each kind of setting lives:

```
 environments repository         ~/.config/unibox/           unagit
 (git, the user's or a team's)                               config.yaml
 +--------------------------+   +-----------------------+   +----------------------+
 | kotlin/                  |   | config.yaml           |   | integrations:        |
 |   sandbox.dockerfile     |<--+   environments: <path>|<--+   unibox:            |
 |   sandbox.yaml           |   |   runtimes:           |   |     on: true         |
 |   start.sh               |   |     sbx: {on, stop_   |   |     environments:    |
 |   instructions.md        |   |           idle}       |   |       gl/team/envs   |
 |   .sbx/build.args        |   | state.json            |   +----------------------+
 | _shared/ .shared/        |   |   access seen, builds,|    the user's choices,
 +------------+-------------+   |   sandboxes, tasks    |    handed to unibox
              |                 +-----------------------+
  what the    |                  unibox's own
  environment v
  is      +--------------------------------------+
          | generated just before a start        |
          | ~/.cache/unibox/sbx/                 |
          |   unibox/kotlin:<tree>    template   |
          |   flags: --cpus -m -e --deny-network |
          +--------------------------------------+
            a cache: never edited, regenerated
```

A start from unagit:

```
 user              unagit                     unibox              sbx, VM        forge
  | Open in          |                          |                    |             |
  | Sandboxed Agent  |                          |                    |             |
  +----------------->| profiles, VMs --json --->|                    |             |
  | agent, profile,  |                          |                    |             |
  | sandbox, place   |                          |                    |             |
  +----------------->| check --json ------------>| built? signed in?  |             |
  |<- build? sign in?|<-- needs: build ---------+                    |             |
  +-- yes ---------->| build (log) ------------>| docker build, load>|             |
  |                  | push the branch ---------+--------------------+------------>|
  |                  | prepare (log) ---------->| create/start, ---->| clone ----->|
  |                  |                          | start.sh, clones   |             |
  |                  | the place runs:          |                    |             |
  |                  |   unibox run ----------->| sbx exec -it ----->| the agent   |
  |                  |                          |                    | push ------>|
  | Pull ----------->| fetch, fast-forward <----+--------------------+-------------+
```

What crosses the line between the machine and the VM:

```
   +------------- trusted --------------+       +----- untrusted -----+
   | host: home, ~/.ssh, unagit's vault |       | VM: the agent, sudo,|
   | main clones, the user's worktrees  | <-X-- | its clones, its     |
   | the configurations, the            |       | Docker              |
   | environments repository (trusted)  |       |                     |
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
- **A directory is mounted instead, when the user says so** (`--mount`,
  the user's call). It gives up the above on purpose: the agent works on
  the host's files and `.git` as they are, on what is checked out there,
  and a hook or `core.fsmonitor` it writes runs on the host later. sbx
  mounts into a running sandbox (`sbx mount HOST:TARGET`, missing from
  its top-level help; checked on a throwaway sandbox: live, writable,
  kept across a stop, taken out by `sbx umount`, leaving an empty mount
  point, and removing the sandbox leaves the host's files alone). The
  directory is mounted at its place in the task, `/work/<task>/<name>`,
  so no path of the host appears in the guest.
  - **A worktree is refused** (the user's call): its `.git` names the
    main repository by the host's path, and the main repository names
    the worktree so; mounted elsewhere, git in the guest would find
    neither, and a `git worktree prune` there would delete the worktree's
    record on the host. The repository's own checkout can be mounted.
  - unibox writes nothing into the host's `.git`: the task's author and
    co-author hook reach git in the guest through an `includeIf
    "gitdir:..."` of the guest's global configuration, which also sets
    `gc.worktreePruneExpire = never` - the repository's git directory
    records the host's worktrees, which the guest does not have, and gc
    would prune their records on the host.
  - Nothing is cloned, pushed or compared with a forge, so `--push` is
    refused with it. `--branch` switches the host's checkout to the
    branch when the task is prepared, making it there from HEAD when the
    host has none of that name - it need not be on the forge, as nothing
    is cloned from there; `check` only says so (`branch_switch`), and git
    refuses a switch that would overwrite uncommitted changes.
  - **Removal unmounts before it deletes.** `rm -rf` of the task would go
    into a mount, so `rm-task` unmounts first, has the guest check every
    mount point is empty, and keeps the task, deleting nothing, when an
    unmount fails or a directory is still mounted. It does not check for
    unpushed work, which is on the host. A recreated sandbox has no
    mounts; preparing its tasks again mounts them again.
- **A task can grow while it runs** (`unibox add-repo <task> <directory>...`).
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
- **Podman**: not now. unibox's runtimes are neutral so that it can be
  added without touching unagit.

## Sandboxes and VMs

One sbx sandbox is one microVM: its own kernel, its own Docker Engine, its
own disk, persistent until removed. The agent runs in the VM itself, beside
that engine, as a non-root user with sudo (sbx's security docs); `sbx exec`
runs another process in the same environment, like `docker exec`.

Both arrangements are offered, and the user picks at the start:

- **A shared sandbox** (named for its profile unless `--sandbox` names it): tasks are directories
  `/work/<task>`, agents are processes side by side. Memory, the Gradle
  cache and Docker images are paid once. Between the agents there is no
  isolation at all - one user with sudo, one filesystem, one Docker, one
  set of ports (two `docker compose` Postgreses on 5432 collide). The
  picker says so.
- **A sandbox of its own** for one task (`--sandbox <new name> --profile <p>`): nothing it does reaches
  another task.

Both persist when the agent ends, for their caches; removing is the user's
action. A VM in which the last agent unibox started has ended is
**stopped** (`sbx stop`), so idle VMs hold no memory; its disk, caches and
tasks stay, and the next start boots it again (and runs the profile's
`start`). unibox's configuration can turn the stopping off
(`runtimes.sbx.stop_idle`). It stops only sandboxes it made (`unibox-` names), and
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
   (the existing `agentPlaces`). The pickers are filled from `unibox
   profiles --json` and `unibox sandboxes --json`. The sandbox picker is also
   where the runtime is chosen: the profile's sandboxes of every runtime,
   each carrying its own, then New Sandbox in <runtime>… for each runtime
   unibox has on - plain New Sandbox… while there is one.
2. `unibox check --json` with the profile, sandbox, agent, branch and
   repositories. It answers with `needs`, each a `code` unagit asks about
   in its own dialogs: `uncommitted` and `local_commits` (they will not reach
   the sandbox, which clones from the forge - Commit… or go on without
   them), `branch_not_pushed` (the push of step 3), `image_required`
   (Build…, with the profile's diff), `login_required` and
   `secret_missing` (Sign In…: `unibox agent login` or `unibox secret
   set`, on a terminal of their own, since unibox takes the value), and
   the warnings `profile_changed` and `service_overridden`, shown and not
   asked about. A profile of an older format does not load: unagit offers
   Migrate…, which runs `unibox init <profile>` and shows what it says.
3. unagit **pushes the branch** of every repository of the worktree that
   has no upstream or is ahead of it - from the host, with its own
   credentials, through the path `P` uses. A branch origin has moved past
   stops the start with the reason; nothing is forced.
4. `unibox prepare --json` with the repositories' clone URLs **from the
   forge's API** (`--repo <dir>=<https URL>`, the project's http URL),
   never from a local remote, which can carry a token (a chezmoi origin,
   AGENTS.md); https always - the sandbox has no SSH key. A `runTask` log
   of its output; it prints the sandbox and the task as JSON at the end.
5. The place chosen runs `unibox resume <task>`, the task `prepare` made - This Terminal,
   Zellij, Ghostty or herdr, as for any agent; `unibox resume <task>
   --agent <id>` runs a task again, with any of the profile's agents.
6. unagit's session record (`<config>/sessions/`) gets the sandbox and the
   task beside the pane, so Activity lists the agent with them.

### What unibox does

`unibox prepare` (and `run`, when not prepared):

1. Creates the VM if new (`sbx create`, see [runtime](#the-runtime)) or
   starts it, and runs the profile's `start` if the VM booted since it last
   ran.
2. For each repository, clones at the branch into `/work/<task>/<dir>`
   (`--filter=blob:none`), or fetches and fast-forwards when it is there
   already.
3. Sets the author and the co-author hook ([commits](#commits)), writes
   the agent's [instructions](#instructions-for-the-agent) into the VM's
   home.

`unibox run` then runs the agent: `sbx exec -it -w /work/<task> <vm>
<agent command>` as its child - not `exec`'d, so that when the agent ends
it can stop an idle VM and update its records. It passes the terminal,
signals and the window size through, and exits with the agent's status.

Without unagit, unibox takes what unagit would give it from where it is
run: the repositories and the branch from the current directory or the
directories named, their clone URLs from `origin` (credentials stripped,
an ssh URL turned into https where the host is plain). It never pushes on
its own: a branch the forge does not have is refused ("push it first"),
unless `--push` asks it to run the user's own `git push -u`.

## Credentials

- **unagit's tokens never go into a sandbox.** The push before a start is
  unagit's, from the host; everything after it is the sandbox's own.
- **The tokens a profile names are unibox's to keep** (the user's call,
  2026-10-14): `unibox secret set` takes one on a terminal, holds it in
  memory on its way to the platform's store (the login keychain, the
  Secret Service), and a profile names it as `secret:<name>`. A sandbox
  sees only placeholders the proxy swaps on the host. A service's
  `command:` can name any other host source instead - unagit's, once it
  is one (unagit's docs/sandbox-credentials.md).
- **The sandbox pushes with a token of its own**, issued by the user for
  this and wired as a git service (`unibox service add`). Narrow: a GitLab project or group access token with
  Developer on the projects concerned; a GitHub fine-grained token on the
  repositories, contents write.
- **Any branch may be worked on, the default one too** (the user's call;
  a task on the default branch was refused before). The agent pushes to
  the branch it was given, so the forge's protection of that branch is
  what bounds it.
- **The forge is now the boundary.** The agent can push to any branch the
  token may, force-push, delete. Protect `main` and release branches on the
  forge, and if possible restrict the token to the work branches (GitLab
  push rules and protected branches, GitHub rulesets). The README says so
  where it explains the feature.
- The agents' own logins are sbx's (OAuth, `sbx secret set`) or tokens
  unibox keeps (`unibox agent login`); see
  [one image, several agents](#one-image-several-agents) and the kits
  below.

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
    command: op read op://vault/artifactory/token
env:
  - ARTIFACTORY_TOKEN: ${services.artifactory}   # the tool reads the placeholder here
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
- **Basic authentication is substituted too.** A request with
  `curl -u user:<placeholder>` reached httpbin.org with the real value
  inside the Basic header (checked on a throwaway sbx sandbox,
  2026-10-19), so a build's `credentials { username; password }` can take
  the placeholder as its password. The variable is the tool's own in its
  own case (`ORG_GRADLE_PROJECT_artifactoryPassword2N`), set in the
  profile's `env` to `${services.<name>}`. sbx takes upper-case names only
  (`^[A-Z_][A-Z0-9_]*$`), so it gets `UNIBOX_SVC_<NAME>`, and unibox sets
  the tool's variable with the placeholder in each session it starts -
  agent, shell, scripts.
  The placeholder is deterministic and no secret, and so the variable is
  there in a sandbox made before the service, too. Checked on sbx: the
  variable held the placeholder and Basic authentication carried the
  value.
- A service **without git** needs no git wiring: the tool reads the
  placeholder from the variable the profile's `env` names and sends it in
  its own header, which the proxy substitutes. sbx injects a custom
  secret's variable only at create, so unibox sets the variable itself in
  each session instead.

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
│   │   ├── sandbox.dockerfile
│   │   ├── start.sh
│   │   ├── instructions.md
│   │   └── .sbx/
│   │       └── build.args
│   └── node/
│       ├── sandbox.yaml
│       └── sandbox.dockerfile
├── _shared/
│   ├── @install/
│   │   └── tools/jdk-certs.sh
│   └── @instructions/
│       ├── review-style.md
│       └── no-migrations.md
├── .shared/
│   ├── @install/
│   │   ├── base.sh
│   │   └── agents/claude.sh
│   └── @instructions/
│       └── unibox.md
└── README.md
```

A directory of `sandboxes/` with a `sandbox.yaml` is a profile, named after
the directory; everything inside it is the profile's own. Only the parts
unibox reads by name are held to a shape (`CheckLayout`): profiles under
`sandboxes/` (every directory there must carry a `sandbox.yaml`), and the
texts of `_shared/@instructions` and `.shared/@instructions` (`.md` files
alone, none named `instructions.md`). Anything else in the repository is
the user's to arrange, and a profile names such a file by its path. Runtime directories inside a profile are
hidden (`.sbx/build.args`), and a profile's root holds only its definition,
`sandbox.dockerfile`, start script, scripts and `instructions.md` - custom files, an agent
configuration seed say, go under an underscore directory of the profile's
own, `_data` by convention. `unibox init <profile>` lays the whole shape
out (interactively with `-i`) and `unibox validate` checks it, every
profile and every text included. `.shared/` and `_shared/` are copied into
every profile's build context, so a Dockerfile can copy what it needs
from them. The installation scripts reach it another way, for a short
Dockerfile: the build gets a second context, `unibox`, holding the runner
at `/usr/local/bin/@install` and the scripts at
`/usr/local/lib/unibox/install`, unibox's with the user's of the same name
over them; `COPY --from=unibox / /` puts both into the image and `RUN
@install base agents/claude custom` runs them by name (or a heredoc's
lines). The runner is a template of the unibox repository, not a file of
the environments repository. `@install` is the command's name because
`install` is coreutils', which builds use. A file of `.shared` or
`_shared`, but the instruction texts, and the runner itself are part of
every profile's revision - a change there marks every image outdated.
`init` rewrites the lines an older init generated to run the scripts
(`/unibox/.install`, `/unibox/.shared/install.sh`) into these two. The
profile's `sandbox.yaml`, its `instructions.md` and its agent configuration
seeds are not image inputs: they change nothing of the image and force no
rebuild.

- **What unibox ships it keeps current, and what it writes for the user
  it never touches again** (the user's call). `.shared` is unibox's -
  hidden, as the user does not edit it - and `_shared` the user's, with
  the same `@install` and `@instructions`; the `@` marks a directory a
  profile names things from, and the user's of a name wins over unibox's,
  so a script or text is overridden by putting one in `_shared`, never by
  editing unibox's. `init`, `check`, `build` and a task's start - each once
  per command, before a profile is read, so an image is built from them -
  rewrite `.shared` to this unibox's own where it differs and take out
  anything else in it, saying so; a repository init did not lay out (no
  `.shared`) is not written into. `init` writes `_shared` once with an
  example of each kind. A repository of the older layout (`_shared/.install`,
  `_shared/installations`, `instructions/`) is refused until `init` moves
  it: a script or text that is unibox's as it was is dropped, one that
  differs moves to `_shared` where it wins, and a Dockerfile's generated
  `RUN` line is rewritten. A profile's files - `sandbox.yaml`, Dockerfile,
  `instructions.md`, `.sbx/build.args`, the seeds - are written once by
  init and are the user's from then on. Both kinds live in the unibox
  repository as files, not strings in the code: `internal/unibox/templates`
  embeds `managed/` (`.shared` as it is laid out), `user/` (`_shared`'s
  examples), `profile/` (text templates of a new profile) and
  `seeds/<agent>/<file>`. An update of a managed script outdates every
  image, as any change of an installation script does.

unibox knows the repository by its path (`environments` in its
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

`sandbox.yaml` is unibox's own format, kept small, and **knows no
runtime**. It describes the environment; where it runs is chosen at the
start.

```yaml
version: 4                    # the format; see below
# the image builds from sandbox.dockerfile beside it; or image: <ref>
resources: {cpus: 8, memory: 50%} # fixed, or a share of the host's
env: [SPRING_PROFILES_ACTIVE] # names; values from the environment
caches:
  - host: ~/.gradle/caches
    env: GRADLE_RO_DEP_CACHE
network:
  allow: [jitpack.io]         # hosts the runtime's policy would refuse
  deny: [gist.github.com]     # hosts it would allow; deny wins
  ports:                      # published on the host's loopback, by name
    fe: '3000:3000'           # at every start
    devtools: {port: '9333:9222', default: off} # when switched on
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
scripts:                      # in the VM only when asked for, by name
  update-skills: update-skills.sh
instructions:                 # names of shared texts, or .md paths
  - unibox                    # relative to sandbox.yaml; the profile's own
  - review-style              # instructions.md attaches itself
git:
  author: user                # the user's identity, from the host's git config
  co_author: true             # a Co-Authored-By trailer naming the agent
```

- **Resources are fixed or a share of the host's** (format 2, the
  user's call): `resources: {cpus, memory}`, each `4`/`8g` or `1%`-`100%`.
  The profile and the manager pass them as written; the adapter turns a
  share into what its runtime takes, as a runtime may know no shares.
  For sbx: a share of CPUs is a count rounded, at least one, and `100%` is
  `--cpus 0`, sbx's own "all host CPUs"; a share of memory is MiB of the
  host's (`sysctl hw.memsize`, `/proc/meminfo`). sbx refuses more than
  75% of the host's memory (`sbx create --help`), so the adapter caps
  memory there and says so, rather than fail a start; CPUs are shared by
  time, so all of them takes nothing from the host's own work but
  priority. A share is resolved when a sandbox is created, and the
  recorded settings keep the share, so another host is no recreation.
- **A profile says its format** (the user's call): `version` is the
  first key (none is format 0), `ProfileFormat` is the one this unibox
  reads, and `unibox version --json` names it. Loading never migrates: a
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
  the checkout - a project's dependencies, generated code - is the
  agent's to do, told by its instructions.
- `network.allow` and `network.deny` are one pattern each - a host,
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
  of `network.allow` or `network.deny` needs no recreate: unibox applies
  the difference to the running sandbox, removing the rules it added by
  id, and recreates only for what cannot change live - the image,
  environment, caches, agents.
- **Ports are published by name** (format 3, the user's call): a profile
  names each port, says whether a sandbox starts with it, and `unibox
  port <sandbox> name:on|off` switches it while the sandbox runs without
  writing the profile. Checked on a throwaway sandbox (`sbx ports`, sbx
  0.47): a port is published live on the host's loopback, tcp4 when no
  protocol is named; `--json` lists `host_ip`, `host_port`,
  `sandbox_port`, `protocol`; a host port taken already is a 409; and
  published ports survive a stop - listed as none while stopped, back at
  the start, an ephemeral one with a new host port. `--unpublish` takes a
  port back only by its exact host port and does nothing, quietly, for
  another. So unibox records what it published by name and spec, finds
  the runtime's entries by the spec (any host port for an ephemeral one),
  unpublishes them by what sbx lists, and leaves a port it did not
  publish alone. Switches last while a sandbox runs: a start that finds
  it stopped drops them and publishes the profile's defaults, since sbx
  would otherwise bring back whatever was on. A switch on a stopped
  sandbox is refused, as publishing would start it. Ports are not part of
  what makes a sandbox: a change of them is applied live, at once to
  running sandboxes when made through unibox and at the next start when
  the profile is edited by hand. A browser for an agent belongs in the
  image (Chrome for Testing and chrome-devtools-mcp), its DevTools port
  published to look on; an agent driving the host's browser would have
  its cookies, sessions and files.
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
- **The image builds from `sandbox.dockerfile`** beside `sandbox.yaml`
  (format 5, the user's call): the layout says it, so the profile no
  longer names it. `image:` names one built elsewhere (a team's CI)
  instead, and then nothing is built; a profile with both, or with
  neither, is refused, and a `Dockerfile` left in its root is named for
  renaming. The migration renames the file `dockerfile:` named and drops
  the key, keeping a comment the user wrote above it.
- **Caches are read-only from the host.** `~/.gradle/caches` mounted
  read-only as Gradle's read-only dependency cache: what the host already
  has is not downloaded again, and what is missing goes to the VM's own
  `GRADLE_USER_HOME`, kept with the VM. Read-write would let the agent put
  a jar or an `init.d` script that the host's next build runs, and Gradle's
  file locks do not hold between VMs over virtiofs. Gradle does not promise
  consistency while the host writes the cache - at worst a build in the VM
  fails and runs again. A mount is at the host's absolute path, as sbx
  mounts everything.
- Considered: devcontainer.json (a standard, but unibox would honour a
  fraction of it and the rest would silently not work; the read-only cache
  and the network rules do not map onto it) and sbx's `sbxenv.yaml`
  (sbx's alone, and experimental).

### Values of each person's own

A profile is meant to be shared by a team, so it holds nothing personal.
What each person has to set - a user name a build reads, a commit identity
on one forge - the profile declares under `vars`, by name with what it is
for. The values live in the user's `~/.config/unibox/vars.yaml`: shared ones
under `vars`, a profile's own under `profiles.<name>`, which win. A run stops
while one is missing (`var_missing`, and `ensure` refuses), since a sandbox
without it would fail later and less clearly. A value reaches the sandbox
where the profile refers to it, most often as a variable of every session
and of `start`; a build argument is for what is not personal or secret, as
it stays in the image's layers and history. A secret is never a
var - a name that says one is refused - it stays `secret:<name>`.

A var has a name of its own, such as `artifactory-user`, and reaches nothing
until the profile refers to it: `${vars.<name>}` in `env` (every session's
variables), in `build_args` (the build's, which join the image's revision,
so a change builds again) and in a service's `user` and `author`. `env` is
the one place a variable is set; an item is a host variable by name or
`NAME: value`, and a value may also refer to `${services.<name>}`, the
placeholder of a service's token. That replaced a service's own `env`
(format 6): a service holds one token for HTTP requests to its hosts and
says no more than where it may go and where it comes from. A var nothing
refers to, and a service without git no variable carries, are warnings.

### Scripts

Each at its own moment, all running inside the VM:

| what | lives in | runs | for |
| --- | --- | --- | --- |
| `sandbox.dockerfile` | the profile | when the image is built | everything static: toolchains, tools, certificates |
| `start` | the profile | once per boot of the VM | services and daemons (a shared `docker compose`), configuration from the environment |
| `scripts` | the profile | when asked for: `--script` before a session, `unibox script` now | whatever the user wants on demand: updating skills, resetting a database |

- **unibox runs scripts and knows nothing of what they do** (the user's
  call): updating skills an image installed, say, is the user's script,
  not a command of unibox's. A script is a profile file read when it
  runs - not an input of the image, not a setting the sandbox is made
  from - so changing one rebuilds and recreates nothing. It runs from a
  file of its own in the VM so its `#!` line counts, as the agent's user,
  with `UNIBOX_SANDBOX` and `UNIBOX_SCRIPT` (and `UNIBOX_TASK`,
  `UNIBOX_TASK_DIR` before a session); the first to fail stops the rest
  and, before a session, the agent. `unibox script` counts as a session,
  as a shell does, so an idle sandbox is stopped after it.

- **`start` runs once per boot, whoever booted the VM.** A VM can be
  started by something else (`sbx exec` from a terminal, sbx's TUI), so
  before every agent unibox compares the kernel's `boot_id` with a mark
  it left in the VM (`/var/lib/unibox/started`) and runs the script only
  when they differ, by `sbx exec`, its output in the preparation's output.
  A failure stops the start.
- **No script comes from the worked-on repositories** (the user's call,
  format 4). A `bootstrap` named a script each worked-on repository was
  to carry, run after its clone; no repository would carry one for
  unibox, so it is gone, and the migration says so.
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
  the environments repository: the texts live in `_shared/@instructions`
  (the user's) and `.shared/@instructions` (unibox's, the user's of a name
  winning), one `.md` per name, and the former built-in rules are just a
  text there (`unibox`) a profile lists like any other.
- **The profile's `instructions` are names or paths**: a bare name is a
  shared text, the one namespace a sandbox attaches more from
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

`sandbox.yaml` says nothing about a runtime, but an image may have to be
built a little differently for one - another `FROM`, a step only one of
them needs. That is said with Docker's own means, build arguments, from a
subdirectory named after the runtime:

```dockerfile
ARG BASE=ubuntu:24.04
FROM ${BASE}

ARG UNIBOX_RUNTIME
USER root
RUN apt-get update && apt-get install -y openjdk-21-jdk git curl
RUN if [ "$UNIBOX_RUNTIME" = sbx ]; then echo "sbx alone"; fi
```

```
# kotlin/.sbx/build.args
BASE=docker/sandbox-templates:shell
```

- Building for runtime X, unibox always passes `--build-arg
  UNIBOX_RUNTIME=X`, so a condition needs no file, and every `KEY=VALUE`
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
  services + github, network.allow + jitpack.io` - and goes on. A change
  made through unibox's own commands is not told again.
- What the change leads to is said too: an image built from changed
  inputs is built without a question, its diff left to `unibox diff`
  (the user's call: the log is no place for it), and a sandbox made from changed settings is recreated, naming
  them - `recreating sandbox web-1: services, the image was rebuilt`.
  The checks for unpushed work and running sessions still stand before a
  recreation.
- What the runtime can change live is applied live instead: each adapter
  says what it can (`NetworkUpdater` for network rules), and what it
  cannot is part of the settings a recreation makes. `unibox network
  allow|deny|rm` changes a profile's rules and applies them to its
  sandboxes at once, asking only where a recreation would be needed.
- The worked-on repositories choose nothing of the sandbox.

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
  adds to one (`--kit`). Kits v3 are Early Access. **unibox neither
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
  unibox runs `docker run --rm <image> sh -c 'command -v …'` for the
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

unibox has its own table of agents - id, command, its instructions file,
its co-author line, sbx's name for it - and never reads unagit's
`agents.All`; the ids are the same, so unagit maps them.

### Building

- **Old images go** (the user's call). The tag carries the revision, so
  each build of a profile is an image of its own, gigabytes each, and
  sbx makes one more per kit of a sandbox (`sbx-kit-src:<sandbox's runtime
  name>-<digest>`). Checked on a throwaway sandbox: `sbx template rm
  --force` removes a template a sandbox was made from, and the sandbox
  starts and runs as before - it needs its image only to be made. So a
  build removes the profile's other builds, from sbx's store and Docker
  (`ImagePruner`); making a sandbox removes its older kit images, the
  newest kept for a recreation to reuse; and `unibox prune` removes the
  images of profiles and sandboxes that are gone. A profile that does not
  load keeps its builds, as which is current cannot be told.

- `<tree>` is the git tree hash of the profile's directory at HEAD, so
  whether the image is current is a comparison with `sbx template ls
  --json`, per runtime: **built**, **outdated**, **not built**. A profile
  with uncommitted changes is marked so, and builds as it is, tagged
  `<tree>-dirty`.
- For sbx: `docker build` with the build arguments, the agents' check,
  `docker save`, `sbx template load`, tagged `unibox/<profile>:<tree>`.
  A local build is for this machine's architecture, which is all a local
  sandbox needs.
- **What a build would build is there to look at, not printed.** `unibox
  diff <profile>` prints what changed in the profile's directory since
  its image last built (the tree is in the tag) as a unified diff; a
  build, `unibox build` or the one before a start, says that it builds and
  points at it, and asks nothing. unagit can draw it in its own differ -
  the one the Changes dialog draws (`drawDiff`, `diffdraw.go`) - where it
  offers Build…. A teammate's Dockerfile runs in the host's Docker when
  built; isolated there, but `RUN --mount` and build secrets can reach
  out, which is what `unibox diff` is for.

## unibox

### Its home

```
~/.config/unibox/config.yaml   the user's choices
~/.config/unibox/state.json    what use taught: host access last seen per
                               profile, builds, sandboxes, tasks, runs
~/.cache/unibox/               generated: kits, image tars; never edited
```

```yaml
environments: ~/workspace/team/agent-sandboxes
default_runtime: sbx
runtimes:
  sbx:
    on: true
    stop_idle: true
```

A runtime not named is on when installed. unagit writes this file only
through `unibox config set`, never by hand, and only what the user chose
in unagit's Settings.

### The command line

The contract unagit is written against. Everything a list shows has
`--json`; a long operation prints its progress as lines, for a log.
Called with `--json` or `--yes` it never asks: what stands in the way is
an answer, not a prompt. `unibox --version` and `unibox version` print the
release, as unagit's do (`0.1`; `dev (<commit>)` for a build that is not a
release, the version coming from the tag at build time); `unibox version
--json` adds the profile format it reads and `api_version`, the JSON
contract's own version, apart from it.

| command | does |
| --- | --- |
| `unibox init [<profile>] [-i] [--dir D]` | lays out the environments repository and adds a profile; on an existing one completes it, and migrates an older format |
| `unibox validate [--json]` | the repository's layout, every profile, every attachable text |
| `unibox profiles [--json]` | profiles: name, agents, image state per runtime, error, last commit |
| `unibox instructions <profile>` | the instructions as the agent gets them |
| `unibox diff <profile>` | what changed since the last build, a unified diff |
| `unibox build <profile> [--runtime R]` | builds, checks the agents, loads |
| `unibox agent login\|co-author\|ls`, `unibox service add\|ls\|rm`, `unibox network allow\|deny\|port\|rm\|ls`, `unibox secret set\|ls\|rm` | configure a profile and keep its tokens (see Configured through unibox) |
| `unibox check … --json` | what stands in the way of a start, as `needs` with a `code` each |
| `unibox prepare …` | the sandbox and the task, ready; JSON of both at the end |
| `unibox run …` | the agent, in this terminal; prepares first if needed |
| `unibox resume <task> [--agent A]` | a prepared task's agent again, in the sandbox that holds it |
| `unibox add-repo <task> [directory...]` | one more clone into a task, also while its agent runs |
| `unibox sandboxes [--json]` (also `ls`), `unibox tasks [<sandbox>] [--json]`, `unibox inspect <sandbox>` | sandboxes, their tasks, a sandbox's record |
| `unibox script <sandbox> [name...]` | the profile's scripts, or run them in the sandbox now |
| `unibox port <sandbox> [name:on\|off...]` | a sandbox's ports and their URLs; switched while it runs, the profile untouched |
| `unibox shell <sandbox>`, `stop`, `recreate`, `rm`; `unibox rm-task <task>` | as named; removal checks for sessions and unpushed work |
| `unibox prune` | removes the images unibox no longer needs |
| `unibox config get\|set\|unset <key> [<value>]` | its configuration |
| `unibox runtimes` | the adapters and what each isolates |
| `unibox herdr install-plugin`, `uninstall-plugin`, `status [--json]` | [herdr](#herdr) |

**One rule for arguments** (the user's call): what a command works on is
its argument - a sandbox (`shell`, `stop`, `recreate`, `rm`, `inspect`,
`port`, `script`, `tasks`, `create`), a task (`resume`, `add-repo`,
`rm-task`, a task's name being unique across sandboxes), a profile
(`build`, `diff`, `init`, `agent`, `network`, `service`, `instructions`) -
and a flag says where or how. `run`, `check` and `prepare` make a task, so
their arguments are its directories and the sandbox is where: `--sandbox`.

`check`, `prepare` and `run` take the same task: `--profile`, `--agent`,
`--runtime`, `--sandbox <name>` (one that does not exist yet is made from
`--profile`), `--task`, `--mount`,
`--branch`, `--repo [<dir>=]<path or https URL>` (repeated; or the
current directory, as without unagit), `--author "Name <email>"`,
`--instructions`, `--push`, `--ignore-local`; `run` and `resume` take
`--script <name>` too.

Exit codes say what kind of failure: 3 an image to build first, 4
refused (`unpushed_work`, `guest_busy`, `confirmation_required`,
`host_access`), the agent's own status after a session, 1 any other
error. Under `--json` a failure is `{"api_version", "code", "message"}`.
Errors are lower case and say what to do: `sbx is not signed in - run sbx
login`.

### The runtime

unibox's neutral vocabulary; nothing above it says sbx.

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
| `sandbox.dockerfile` | the built template, `-t unibox/<profile>:<tree>` |
| `image` | `-t <image>` |
| `resources` | `--cpus`, `--memory`; a share resolved by the adapter (below) |
| `network.deny` | `--deny-network`; changes by `sbx policy` live |
| `network.allow` | `sbx policy allow network --sandbox`, live |
| `network.ports` | `sbx ports --publish\|--unpublish`, live |
| `env` | `-e NAME` |
| `caches` | `<host>:ro` as an extra workspace, `-e <env>=<host>` |
| `start` | run by `sbx exec` when the VM's `boot_id` is new |

Spike questions about it:

- `sbx create` takes an agent and mounts the current directory when given
  no path. A VM must be created with no repository mounted: give it a
  directory of unibox's own (`~/.cache/unibox/vms/<vm>`, empty,
  read-only) as its workspace, unless sbx can do without one.
- Does `sbx exec` take `-w`?
- Do read-only mounts of host directories outside the workspace work
  (`<path>:ro` as an extra workspace)?
- Does `sbx template load` take a tar from `docker save`, or only one from
  `sbx template save`? If only the latter: build in a sandbox and `sbx
  template save` it, or push to a registry.

### microsandbox (msb), an experimental runtime

Checked against msb 0.7.8 and its Go SDK v0.7.8 on macOS (Apple Silicon),
2026-10-17. msb is experimental: it shows the profile format carries over
to a second runtime, and sbx stays the supported one and the default. It
is never chosen for the user - only as `default_runtime` or with
`--runtime msb` - and `unibox check` warns when it is used. A sandbox keeps
the runtime it was made in.

**Why the SDK and the CLI both.** The SDK installs the runtime
(`EnsureRuntime`: msb and libkrunfw into `~/.microsandbox`), so whoever
installs unibox has a runtime without installing anything else, and it
rotates a secret live (below). It is cgo over an embedded library of about
27 MB, which `msb_sdk.go` keeps behind the `cgo` build tag: built without
cgo, unibox uses an `msb` on `PATH` and cannot rotate live. Everything else
goes through the `msb` the SDK installed: `create --conf` takes a whole
configuration file, and `exec --tty` handles the terminal, which the SDK's
`Exec` returning `*exec.Cmd` in unibox could not.

| Runtime | msb |
| --- | --- |
| `List` | `msb list --format json` |
| `Images` | `msb image ls --format json` |
| `Build` | `docker build`, `docker save`, `msb image load --input` (5 GB in 18 s) |
| `Create` | `msb create --name N --conf <unibox's file>`, under the secrets shell |
| `Start` | `msb start N`, under the secrets shell |
| `Exec` | `msb exec [--tty \| --stream [--no-stdin]] [-w] [-u] N -- cmd…` |
| `Stop`, `Remove` | `msb stop N`, `msb remove --force N` |
| ports listed | `msb inspect N --format json`, `config.network.ports` |

What was verified, and what it decided:

- **Secrets.** A secret in the configuration names its value by a host
  variable (`value: {$msb_env: UNIBOX_MSB_<ENV>}`); msb stores the name,
  not the value, and reads the variable when the sandbox boots. Without it
  `msb start` - and `msb exec` on a stopped sandbox, which would start it -
  fails, so a sandbox never runs without its secrets. unibox runs msb
  under `sh -c` that fills each variable with the output of the secret's
  host command, so a value goes from the command to msb, never through
  unibox. `~/.cache/unibox/msb/<sandbox>/secrets.json` keeps the commands
  for the next boot. A custom `placeholder` and `header_fields` work: the
  guest sees the placeholder, a request to an allowed host carries the
  value, and a git service's value goes into `Authorization` alone.
- **Rotation.** msb refreshes nothing. `msb modify --secret` rotates only
  by resetting the placeholder, which needs a restart; the SDK's `Modify`
  with the same placeholder and source rotates live (checked: the next
  request carried the new value). Before each session on a running sandbox
  unibox runs itself as `__msb-secrets` under the secrets shell, and that
  process calls `Modify`.
- **No logins of its own.** msb has no OAuth flow and no local secret
  store (its `store` source is cloud-only; the local resolver refuses it).
  Agents sign in by `token`, `api-key` (kept by unibox as
  `<agent>-api-key`) or `command`. Codex's ChatGPT login is refused: its
  refresh would hand the guest real tokens; a later look may find a way
  through the proxy. Codex reads its key from `auth.json`, which
  `codexSetup` writes with the placeholder `proxy-managed`, as for sbx.
- **TLS.** Interception is on for every sandbox, as secrets and hostname
  rules need it. msb puts its CA into the guest's store and sets
  `NODE_EXTRA_CA_CERTS`; claude, copilot (node), codex (Rust), git and curl
  all went through.
- **Network.** msb's own default is the public internet. A non-empty allow
  list makes it deny the rest, so unibox gives every sandbox sbx's default
  policy (`msbBaseAllow`, from `sbx policy ls`), DNS, the services' hosts
  and the profile's `allow`; `deny` goes first and wins. Rules cannot change
  on a running sandbox (neither CLI nor SDK), so a change recreates it.
- **DNS behind a VPN.** On macOS msb asks only the global resolvers
  (`State:/Network/Global/DNS`); it does no split DNS, so a name a VPN
  resolves through its own resolver - gitlab.2n.cz through 10.132.64.20,
  with Tailscale's 100.100.100.100 global - is unknown in the sandbox. It
  also drops an answer with a private address unless a rule names the
  address, and puts a secret into a request only to an address it saw a DNS
  answer for: pinning the name in `/etc/hosts` resolved it but blocked the
  token. So when a sandbox is made, unibox resolves the profile's service
  and `allow` hosts as the host does; for one with a private address it
  finds the host's resolver (`scutil --dns`) that gives it, and that answers
  public names too, since msb falls through only when a resolver does not
  answer. That resolver goes first in `network.dns.nameservers`, before the
  global ones, and the address is allowed. Checked: git on gitlab.2n.cz with
  its token, public hosts, and another private address still refused. It
  holds until the sandbox is recreated - connect the VPN first. On Linux
  msb reads `/etc/resolv.conf`, whose stub resolver does the split.
- **Ports** are published at creation only; SDK `Modify` gains ports after
  0.7.8. A port without a host port gets a free one when the sandbox is
  made. A change recreates the sandbox; `unibox port` lists them but
  cannot switch them.
- **Mounts** cannot be added to a sandbox, by CLI or SDK, so `--mount` is
  refused. A disk snapshot restored under the same name with a new mount,
  ports and rules keeps `/work` (checked), but the CLI's restore drops the
  secrets and TLS; not used.
- **Disk.** msb's root is 4 GiB; unibox asks for 64 GiB (`upper_size`),
  sparse on the host.
- **Logs.** `msb exec` keeps a command's output in
  `~/.microsandbox/sandboxes/<name>/logs/exec.log` on the host.

### herdr

herdr recognises an agent by its pane's foreground process, and an agent
behind `sbx exec` is not one it knows: no agent, no state. unibox fills
that in with **a herdr plugin of its own**. It is the one way; there is no
launcher and no third-party plugin to choose instead.

What it does, as long as herdr runs - unagit open or not:

- `unibox run` started in a herdr pane records the pane
  (`HERDR_PANE_ID`), the agent and the VM in unibox's state. For every
  pane so recorded whose `unibox run` is alive, the plugin reads the
  screen (`pane.read`), has herdr classify it by herdr's own rules for that
  agent (`herdr agent explain --file … --agent <kind> --json`), and reports
  the state (`pane.report_agent`, `pane.release_agent` when the agent is
  gone), with the VM's name as the pane's metadata
  (`pane.report_metadata`). It reports only on a change.
- It never parses argv: unibox started the agent and wrote it down.
- unagit's Activity matches herdr's agents by pane, so the state arrives
  there with no change to it.

The idea, and the herdr calls, come from
[sbx-herdr](https://github.com/dvdksn/sbx-herdr) (read 2026-10-12 at
0.1.0). It has no licence, so none of its code is taken.

**The plugin is a manifest and nothing to build.** `herdr-plugin.toml`, its
commands calling the unibox binary: a startup command and the
`pane.created` / `pane.focused` events run `unibox herdr ensure`, which
starts one `unibox herdr observe` per herdr socket (a lock in
`HERDR_PLUGIN_STATE_DIR`, so a second ensure does nothing), detached, its
log in that directory. Both are hidden commands that refuse to run outside
herdr (no `HERDR_SOCKET_PATH`).

**Installed only with the user's consent**, never quietly - it changes
herdr's configuration and leaves a process running with herdr:

- `unibox herdr install-plugin` asks unless `--yes`; it writes the
  manifest, embedded in the binary, to `~/.config/unibox/herdr-plugin/`
  with the absolute path of the running unibox, then `herdr plugin link
  <dir>` and `enable`. `uninstall-plugin` undoes it.
- unagit's Settings › Integrations, in herdr's block, shows `unibox herdr
  status --json` and offers Install herdr Plugin… (and Uninstall) behind
  its own question, then calls the command with `--yes`.
- The path changes when unibox is upgraded or moved. Every unibox
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

- unibox's code is `cmd/unibox` and `internal/unibox/...`, in its own
  repository with its own AGENTS.md. It imports nothing of unagit's.
- unagit imports nothing of unibox's. It runs the binary, found through
  `a.executable("unibox")` like any program, and decodes `--json` into
  types of its own, written against `internal/unibox/api` and refusing an
  `api_version` it does not know.
- Two versions, kept apart: `api_version` is the JSON contract's, and
  changes only when a field unagit reads changes meaning or goes; the
  profile format is the YAML's, and unagit never reads YAML, so it learns
  of it only as a profile that does not load until it is migrated.
- unagit's side of the integration - Settings, the Sandboxes tab, Open in
  Sandboxed Agent…, the commit warning, removal with a worktree - is
  unagit's plan, in its docs/sandboxes.md; credentials from unagit's vault
  in its docs/sandbox-credentials.md.

## Tests

`make test-unibox` runs only `cmd/unibox/...` and `internal/unibox/...`, with
the race detector; `make test-unibox-fast` runs the same packages without it.
These are the checks while implementing unibox, including before commits
that change only unibox. The unagit suite is not run for those changes.
`make test` and `make test-fast` retain the whole repository's suite for
changes across the boundary or to shared build machinery. The binary and
package name agreed after the original design is `unibox`.

unibox:

- A stand-in `sbx` and a stand-in `docker` on their own PATH
  (`testenv.WriteExecutable`), answering `ls --json` and `template ls
  --json` from files and logging every argv: the commands for a shared and
  an own VM, the task script, `start` run on a new `boot_id` and not on the
  same one, the VM stopped when its last agent ends and not one it did not
  make.
- Profiles: parsing (an unknown field, a missing instructions file are
  errors), the tree hash and the image state, the build arguments
  (`UNIBOX_RUNTIME` always, `build.args` of the runtime alone), the
  generated flags, the agents' check failing a build that lacks one, the
  change of host access told once and a changed seed recreating nothing,
  the instructions put together in order, the author and the trailer added
  by the hook once.
- The command line: `--json` shapes against `internal/unibox/api`, the
  exit codes, no question asked under `--json` or `--yes`; without unagit,
  URLs from `origin` with credentials stripped, a branch the forge lacks
  refused.
- No token in any argv or environment unibox builds.
- The herdr plugin: the manifest written with the binary's path and
  rewritten when it moves; `observe` against a stand-in `herdr` reporting a
  state on a change only and releasing a pane whose `unibox run` ended;
  the install asking first. A real herdr only in `make integration`.
- A real sbx only opt-in by a variable on macOS (`UNIBOX_SBX_TEST=1`), as
  Ghostty and the keychain are - sbx needs a hypervisor, not the Docker
  image of `make integration`.

unagit's tests are its own plan's (unagit's docs/sandboxes.md).

## Order of work

Built, in this order: the spike by hand; unibox's core (the runtime with
sbx, profiles, the build and the agents' check); tasks (`check`,
`prepare`, `run`, `resume`, instructions, author and hook, `start`,
stopping idle sandboxes), usable from a terminal without unagit; kits for
several agents; services, secrets and network rules configured through
unibox; agents' seeds; the herdr plugin; the versioned profile format.

Next, for unagit to build on:

1. **The contract, before unagit reads it** (unibox):
   - `profiles --json` drops `approved`, left from the approval this
     replaced, and says a profile's `format` with a code for one of an
     older format (`profile_format`), so unagit can offer Migrate…
     without parsing an error's text. `api_version` 2, since a field
     goes.
   - `init --json` reports what a migration did (`migrated`), as it now
     does, under that version.
   - Done: `--sandbox` alone, making the sandbox from `--profile` when it
     is not there; `rm-task <task>`; `add-repo <task> [directory...]`;
     `sandboxes`, with `ls` its alias.
2. **unagit's integration**, in unagit's plan: Settings and the
   configuration handed over; Open in Sandboxed Agent… for one
   repository's worktree in This Terminal, then groups, then every place.
3. The Sandboxes tab with its configuration actions (Build…, Sign In…,
   Services…, Network…, Migrate…), removal with a worktree, the commit
   warning, Activity.
4. The herdr plugin's install from Integrations.
5. unagit as a source of tokens (its docs/sandbox-credentials.md):
   `stored` grants first, then GitLab's minted ones.

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
