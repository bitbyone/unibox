# Sandboxed agents: unibox

## Where it stands

Unibox 2 (2026-10-25) is a thin layer over Docker Sandboxes (sbx). A
team describes its sandboxes in its own repository, as sbx's env files
(`*.sbxenv.yaml`) and kits v3; unibox takes any such env as it is and adds
only what sbx does not do: each person's values for the env's args, the
secrets the kits need, tasks with an agent in the user's terminal, named
ports (the team's among them), a doctor, recreation when the env changed,
and herdr's status of the agents. Someone using unibox never needs sbx's
CLI: init sets everything up, and the network policy and sbx's settings
have unibox commands. Using the same files with plain `sbx env create`
still works; unibox is never a condition, and nothing it adds to a team's
repository limits someone without it.

Unibox 1 had a format of its own (profiles, `sandbox.yaml`), templates,
image builds, generated kits and a second runtime; the sbxenv files and
kits now say all of that, and it is gone. [From unibox 1](#from-unibox-1)
says what went and why; [sbx-pivot.md](sbx-pivot.md) is the plan of the
change, with what each phase actually did.

Checked against sbx 0.49.2 on macOS (Apple Silicon); the oldest release
unibox accepts (`MinSBXVersion`) is 0.50.0, whose kit-declared host mounts
the teams' kits use and which stops sbx's own host git from running what a
workspace's .git configures. It builds without cgo for Linux, macOS
and Windows on amd64 and arm64 ([Platforms](#platforms-and-releases)). What sbx was found to do, with when and
how, is in [What was verified about sbx](#what-was-verified-about-sbx);
some of it was checked against 0.47 for unibox 1 and is kept because the
decisions here still rest on it.

This began as unagit's plan - it replaced "Agents in a container" in
unagit's docs/herdr.md - and keeps its reasoning, so that the decisions
are not undone by accident.

## What it is for

A checkout - one repository or several - gets a coding agent that runs in
a VM instead of on the machine. The agent has full autonomy inside it; the
machine, its home, its keys and the user's tokens are out of its reach.
The team decides what the VM is (image, tools, agent, network, which
services it may sign in to); each person adds their own values and
secrets, and picks the work.

## Two programs

Unibox is a program and a repository of its own, usable alone; **unagit
is a UI over it** and one of its users.

- They share no code. Unibox imports nothing of unagit's; unagit runs the
  unibox binary and reads its `--json`, whose types are
  `internal/unibox/api`, versioned by `api_version` in every object (2
  since unibox 2: a sandbox names its env, not a profile).
- unagit drives it as it drives herdr or Zellij: through its command
  line, `--json` for what it reads and `--yes` where a question would
  come. Under `--json` unibox never asks.
- What the team's env says is sbx's to read; unagit reads none of it.

## The picture

The parts:

```
 team repository (git)              host                                sandbox (sbx microVM)
 +---------------------------+   +--------------------------------+   +-------------------------+
 | sandboxes/dev.sbxenv.yaml |-->| unibox                         |   | the kits' image         |
 | kits/...                  |   |   envs, your args, secrets,     |   | /work/<task>/<repo>     |
 +---------------------------+   |   tasks, ports, doctor         |   |   your checkout mounted,|
                                 |   herdr plugin                 |   |   or a clone            |
 ~/.config/unibox/               |        |                       |   | the kit's agent, run by |
   config.yaml  envs, ports,  -->|        | sbx env create/rm,     |   |   sbx exec              |
                settings         |        | exec, mount, ports,    |-->| placeholders for tokens |
   vars.toml    your args        |        | secret ls/set,         |   | Docker Engine           |
   state.json   sandboxes, tasks |        v kit inspect, env plan  |   +-------------------------+
                                 | sbx: daemon, proxy, policy,    |               |
                                 |      secrets in the keychain   |<--------------+ requests, tokens
                                 +--------------------------------+                 swapped by the proxy
```

Where each kind of setting lives:

```
 team repository              ~/.config/unibox/                    sbx
 (the team's, in git)         (the person's, on this machine)      (sbx's own store)
 +----------------------+     +-------------------------------+    +---------------------+
 | *.sbxenv.yaml        |<----| config.yaml                   |    | secrets (secrets),   |
 |   image, kits, args, |     |   envs: files, name, ports    |    |   global            |
 |   mounts, bindings,  |     |   default_env, context,       |    | settings: SSH agent,|
 |   resources          |     |   stop_idle, ssh_agent        |    |   rememberHost-     |
 | kits/<kit>/          |     | vars.toml                     |    |   Commands          |
 |   spec.yaml, files   |     |   [envs.<env>] your args      |    | policy, ports,      |
 +----------------------+     | state.json                    |    |   mounts of each    |
                              |   sandboxes, fingerprints,    |    |   sandbox           |
                              |   tasks, runs, mounted dirs   |    +---------------------+
                              +-------------------------------+
```

What crosses the line between the machine and the VM:

```
   +------------- trusted ----------------+       +----- untrusted -----+
   | host: home, ~/.ssh, the keychain,    |       | VM: the agent, sudo,|
   | the team repository, unibox's config | <-X-- | /work, its Docker   |
   +------------------+-------------------+       +----------+----------+
                      | only:                                ^ | 
                      |  a task's directories, mounted ------+ |  (read-write,
                      |  (the user's choice, .git included)  | |   told once)
                      |  the env's args and mounts ----------+ |
                      |  placeholders for tokens ------------+ |
                      |                                        |
                      +---- git fetch <---- forge <------------+ git push (a clone)
```

## The model, and why

### The team's files are the configuration

- **An env is a list of sbxenv files**, merged by sbx in their order, kept
  in the user's `config.yaml` with the sandbox's name and the user's
  ports. The team's file comes first; a file of the person's own after it
  adds a kit or a mount without changing theirs.
- **unibox never writes, generates or merges an sbxenv.** It reads the
  little it needs - `args` (declared, required, default), `kits`,
  `bindings`, `agent`, `name` - and leaves every other field to sbx, so a
  newer sbx adding fields does not break it (`sbxenv.go`). Whoever does
  not want unibox runs `sbx env create` with the same files.
- **What sbx can say, unibox does not repeat**: image, tools, agent,
  network policy, mounts of the team's, resources and credential bindings
  are the env's and its kits'. Unibox 1 said them in a format of its own
  and turned them into sbx flags and generated kits; once sbxenv and kits
  v3 could say all of it, the second format was only a second place for
  the same facts.
- **No secret is an arg.** vars.toml and the args file are plain text, so
  `unibox var set` refuses a value that is a secret reference; a token is
  a secret, kept by sbx.

### Envs, sandboxes, tasks

- An **env** names a team's sbxenv files; the person's values for its
  args are theirs, in `~/.config/unibox/vars.toml`: `[args]` for every
  env that declares them, `[envs.<env>.args]` for one, which wins.
- A **sandbox** is the VM sbx creates from an env, named by `--name` in
  the env's configuration, else the file's `name:`, else the env's own
  name.
- A **task** is one piece of work in a sandbox: one or more directories
  at `/work/<task>/<repo>`, mounted from the host or cloned from the
  forge, with the kit's agent in the user's terminal. A task's name is
  its first repository and a digest unless `--task` names it, and is
  **unique across sandboxes**, so a task alone finds its sandbox
  (`unibox resume <task>`).

### A task mounts by default

```
host                                   sandbox VM
~/src/payments  <── sbx mount ──────>  /work/<task>/payments
  (yours, .git included)                 the agent works on your files
```

- **The directories named are mounted** (`sbx mount <sandbox>
  <host>:/work/<task>/<repo>`, read-write), the current one when none is
  named. The agent works on the user's files and `.git` as they are, on
  what is checked out there, and its changes and commits are on the host
  at once. This is what people asked for: no push before a start, no pull
  after.
- **The cost is the host's `.git`.** A `.git` the agent can write holds
  commands the host will run: a hook, `core.fsmonitor`, `core.sshCommand`,
  a filter, `credential.helper` - and editors and git tools run git in a
  checkout all the time. So the first time a directory is mounted unibox
  says so once (`State.Mounted`), and points at `--clone`. A read-only
  `.git` over the mount (`--protect-git` in the plan) was left out: the
  agent could then not commit, which is most of what it is for.
- **The mount goes to the task's place, `/work/<task>/<repo>`**, not the
  host's path: tasks have one shape for mounts and clones, and no path of
  the host appears in the guest. sbx creates the mount point's parents as
  root (verified), which is why unibox makes `/work` the agent's before
  the first task.
- **A worktree is refused**: its `.git` names the main repository by the
  host's path, and the main repository names the worktree so; mounted
  elsewhere, git in the guest would find neither, and a `git worktree
  prune` there would delete the worktree's record on the host. The
  repository's own checkout can be mounted.
- **unibox writes nothing into the host's `.git`.** The task's author and
  co-author hook reach git in the guest through an `includeIf
  "gitdir:..."` of the guest's global configuration, kept under
  `/var/lib/unibox/mounted/`, which also sets `gc.worktreePruneExpire =
  never` - the repository's git directory records the host's worktrees,
  which the guest does not have, and gc would prune their records on the
  host.
- Nothing is cloned or compared with a forge, so `--push` has nothing to
  do. `--branch` switches the host's checkout to the branch when the task
  is prepared, making it from HEAD when the host has none of that name;
  git refuses a switch that would overwrite uncommitted changes.
- **Removal unmounts before it deletes.** `rm -rf` of the task would go
  into a mount, so `task rm` unmounts first (`sbx umount`), has the guest
  check every mount point is empty, removes the root-owned mount points
  with `rmdir` as root, and only then deletes the task's directory. It
  keeps the task, deleting nothing, when an unmount fails or a directory
  is still mounted. It does not check for unpushed work, which is on the
  host. Forced or not, a mounted directory of the host is never deleted.
- **Mounts outlive a stop** (verified), and a mount whose host path is
  gone stops the sandbox from starting; doctor names such a task. A
  recreated sandbox has no mounts; preparing its tasks again mounts them
  again.

### A clone instead, when asked

```
host                                   sandbox VM
your checkout  <── git pull ───┐       /work/<task>/<repo>  a clone of the
  (yours)                      │         branch; the agent commits
                               │         and pushes here
                    forge ◄────┴──── git push
```

- `--clone` clones every repository from the forge inside the sandbox, on
  the task's branch, and nothing of the host is mounted. The agent
  commits and pushes; the user pulls as they would a colleague's commits.
  A fetch carries objects and refs, never config or hooks, so the host
  stays safe however the agent behaves. This was unibox 1's only way, and
  stays for work the host's checkout should not be exposed to.
- **The cost**: what the agent has not pushed is not seen on the host.
- Unibox takes the clone URLs from `origin` (credentials stripped, an ssh
  URL turned into https where the host is plain), or `--repo <https
  URL>`. It never pushes on its own: a branch the forge does not have is
  refused ("push it first"), unless `--push` asks it to run the user's
  own `git push`. The sandbox signs in to the forge only as the env's
  kits and bindings let it.
- **Removal checks for work only the sandbox has**: uncommitted files,
  unpushed commits, stashes, files outside the clones. Any of it keeps the
  task, unless forced.
- **A task can grow while it runs** (`unibox task add-repo <task>
  <directory>...`): one more clone, or one more mount for a mounted task -
  a task's repositories are all mounted or all cloned. The agent in a
  running session sees it at once and needs no restart. The task lock
  stays with the session; the sandbox guard serializes the addition with
  other preparations.

### Considered and rejected

- **The worktree and the main clone's `.git` mounted read-write, with
  `config`, `hooks` and `config.worktree` read-only over them** (herdr.md's
  plan). Every other branch's ref stays writable, and the list of what
  must be locked is only as complete as our knowledge of git. The mount
  unibox makes now is plainer: the whole checkout, its risk told.
- **The main clone read-only, the worktree on a detached HEAD with an
  object directory of its own** (`GIT_OBJECT_DIRECTORY`, the main objects
  as an alternate). Safe, but the agent sees a detached HEAD and the
  branch has to be moved after it. The user found it unpleasant.
- **The host's path as the mount target** (`sbx mount S /a/b/repo`, sbx's
  default). It works (verified), but a task of clones and a task of mounts
  would then have different shapes, and the guest would see the host's
  layout.
- **Another runtime.** unibox 1 kept a runtime-neutral profile and an
  experimental second runtime (microsandbox) to show it carried over. It
  cost a format, a translation per runtime and features only one runtime
  had; nobody used the second. Unibox 2 builds on sbx alone.

## Sandboxes and VMs

One sbx sandbox is one microVM: its own kernel, its own Docker Engine, its
own disk, persistent until removed. The agent runs in the VM itself, as a
non-root user with sudo (sbx's security docs); `sbx exec` runs another
process in the same environment, like `docker exec`.

- **A shared sandbox** is the usual one: an env's sandbox holds every task
  started in it, the agents processes side by side. Memory, caches and
  Docker images are paid once. Between the agents there is no isolation
  at all - one user with sudo, one filesystem, one Docker, one set of
  ports (two `docker compose` Postgreses on 5432 collide). Two agents in
  one sandbox at once work (verified).
- **A sandbox of its own** for a task is another env, or the same files
  with another `--name` (`unibox env add dev-2 <files> --name dev-2`).

Both persist when the agent ends, for their caches; removing is the
user's action. A sandbox in which the last session unibox started has
ended is **stopped** (`sbx stop`), so idle VMs hold no memory; its disk,
caches and tasks stay, and the next start boots it again. `stop_idle:
false` in config.yaml turns that off. Before stopping, the guest is asked
whether a session unibox started is still running there (a record per
run under `/var/lib/unibox/runs`), since a host process that went away is
no proof its guest process did.

## A start

### What unibox does

`unibox run` (and `task prepare`, without the agent):

1. **Finds the sandbox and its env**: `-s`, else `-e` and the name the
   env gives, else `UNIBOX_SANDBOX`/`UNIBOX_ENV`, the context, or there
   being only one. A task alone finds its sandbox.
2. **Checks the person's args.** A required arg with no value and no
   default stops the run; at a terminal `run` and `sandbox create` ask for
   it and keep it in vars.toml, elsewhere they name `unibox var set`.
3. **Creates the sandbox when sbx has none**: the args go into a file only
   the user can read, under unibox's cache, for `--env-args-file`, removed
   as soon as sbx is done; then `sbx env create --name <sandbox>
   --env-args-file <file> <files...>` runs **on the user's terminal**, as
   sbx asks there to approve the env's bindings and host commands. Unibox
   never passes `--force` for the user. The sandbox's state is
   `creating` until it is ready, so a creation that stopped - refused,
   interrupted - is tried again by the next `create` or `run`, and when
   sbx did make it, only readied.
4. **Readies it**, as root: `/work` and `/var/lib/unibox` made and given
   to the agent's user, not recursively, since what is mounted under
   `/work` is the host's. It records sbx's id for the sandbox, the
   fingerprint of what it was made from, and the kit's agent.
5. **Says when the env changed** since the sandbox was made: the
   fingerprint covers the env's files, its local kits' files and the
   person's args. A change is told, never applied: `unibox sandbox
   recreate` applies it.
6. **Publishes the env's ports** it should, and leaves others alone.
7. **Prepares the task**: mounts its directories, or clones them; sets
   the author and the co-author hook; the first time a directory is
   mounted, says what that means.
8. **Runs the agent**: trusts the task's directories in the agent's own
   settings, then `sbx exec -it -w /work/<task>[/<repo>] <sandbox> bash -c
   '<the kit's entrypoint and default arguments> <arguments after -->'`,
   as its child - not `exec`'d, so that when the agent ends it can stop an
   idle sandbox and update its records. It passes the terminal, signals
   and the window size through, and exits with the agent's status.

A repository given as a URL without `--branch` works on the forge's
default branch (`git ls-remote --symref`); the forge's branch protection
bounds what a push may do there.

`unibox resume <task>` runs a prepared task's agent again; `--agent` another
the sandbox has; `--shell` opens
a shell in the task beside its running agent instead. `unibox sandbox
create` does steps 1 to 6 with no task.

### Adopting a sandbox made with sbx

A sandbox sbx has under the name an env gives, which unibox did not
create, is not taken over silently: a run refuses with
`sandbox_taken`. `unibox sandbox create --adopt <name> -e <env>` records
it, readies it and reads its agent. Unibox then adds tasks to it but never
recreates it, `sandbox rm` removes it only with `--force`, and `destroy`
only forgets it - it was made by someone else's hand.

### From unagit

unagit asks the user what to work on and where, then drives the same
commands with `--json`: `task check` for what stands in the way (each a
`need` with a `code`: `arg_missing`, `env_changed`, `uncommitted`,
`branch_not_pushed`, ...), `task prepare` for the sandbox and the task,
and `unibox resume <task>` in the place the user picked - this terminal,
Zellij, Ghostty or herdr. unagit's side is its own plan, in its
docs/sandboxes.md.

## Envs and the person's values

- **`unibox init` starts from the team's repository**, not from a file:
  the current directory (its git root, from anywhere inside), a path, or
  a git address. It finds every `*.sbxenv.yaml` (`discover.go`, skipping
  hidden directories, `.git` among them, and `node_modules`), offers them
  to pick, several at once, and makes an env of each, named as the file names its sandbox. One env
  becomes `default_env` by itself; of several, init asks. Running it again
  adds what is new and says what is in place. config.yaml keeps the
  repository an env's files are in as `repo`.
- **A git address is cloned with the person's own git** - their keys,
  their credential helper; unibox never sees them - into
  `$XDG_DATA_HOME/unibox/repos/<host>/<path>` (`clone.go`), and the env
  keeps the address as `source`. `unibox env update` fast-forwards such
  clones alone and says what the change means: a new arg asked of the
  person, a sandbox to recreate, a new sbxenv file for init. A checkout of
  the person's own has no `source` and is pulled the same way only while
  it has nothing uncommitted and its branch follows a remote one: their
  uncommitted work, and a branch of theirs alone, are not unibox's to
  touch. doctor fetches a clone and
  warns when it is behind. `destroy --all` leaves the clones; they are a
  repository, not unibox's state.
- **Without a terminal, or with `--no-input`, init asks nothing**: it sets
  up every file found or those of `--file`, takes values from `--var
  name=value`, and fails naming what is missing and the flag that gives
  it. A token is never a flag; `unibox secret set` stores it.
- `unibox env add [<env>] [<file...>] [--name N] [--default]` keeps the files
  in config.yaml after checking they read; `env show` prints what they
  declare and what the person set; `env rm` forgets the env, its files and
  sandbox untouched.
- **The args are the person's.** An env's file declares them (`args:`
  with `required`, `default`, `description`, a pattern); the values are in
  `vars.toml` under `[envs.<env>]`, never in the team's repository, and
  go to sbx only through the args file, which exists only while sbx runs.
  A value for an arg the env does not declare is left out (sbx refuses
  one) and doctor names it.
- **Resources are the person's too.** How much of a machine a sandbox
  gets depends on the machine, which a team's env cannot know; a share of
  it fits every machine, so the team may give one as the env's default
  under `resources:` in its unibox.yaml files (the repository's, the
  env's over it), which sbx does not read and which wins over the
  sbxenv's own `sandboxOptions`. The person's win over the team's:
  `[resources]` and `[envs.<env>.resources]` in vars.toml give `cpus` and
  `memory`, absolute or as a share of the host, written by hand - no
  command, as two values a person sets once need none. sbx takes no
  share, so unibox resolves it at each create (as unibox 1 did: CPUs
  rounded, at least one, 100% as 0; memory in MiB, capped at the 75% sbx
  allows, with a line saying so) and gives sbx the amounts in a temporary
  sbxenv file of `sandboxOptions` alone, last, which sbx's merge lets
  win. They are part of the fingerprint only when set, so a sandbox made
  before keeps its own.
- **A person's say has three scopes**: every env, every env of one env
  repository, and one env, each over the one before and all over the
  team's files. Someone working with several teams wants instructions,
  repositories or values that make sense for one team's envs alone; the
  middle scope is `env_repos.<key>` in config.yaml and
  `[env_repos."<key>"]` in vars.toml, keyed by `EnvRepoKey` - the host
  and path of `source` or the checkout's origin, else its path - so
  every clone of a repository shares it. An env of no repository has no
  such scope, and `--env-repo` refuses it.
- **A kit's own required arg** (`2n-artifactory` has one) is read with
  `--kit-arg` from the env's values, or `sbx kit inspect` could not read
  the kit; one neither the person nor the env gives is a placeholder, as
  unibox only reads what a kit declares.
- **`sbx env plan`** checks the files with the args and changes nothing;
  unibox runs it only for its exit code and its summary line (doctor
  shows it), as it prints no JSON (verified).
- **The fingerprint** is a hash of everything a sandbox is made from: the
  files' contents, every file of a local kit (a `kits[].source` directory
  beside the file), and the args. An image kit is named by its reference,
  which the files already carry. A change makes doctor and the next start
  say so, and `unibox sandbox recreate` apply it: the sandbox is removed
  after the checks for sessions and unpushed work, created again from the
  env as it is now, and its tasks come back - mounted directories mounted
  again, clones cloned again. What it held outside its tasks is lost,
  which is why a change is never applied unasked.
- A sandbox is removed through its env (`sbx env rm`), which takes with
  it what sbx made for that sandbox alone, its sandbox-scoped secrets
  included; one whose files are gone is removed by name.

## Secrets

**Secrets are never unibox's to read.** They live in sbx's store on the
host (the keychain) and reach a sandbox only as placeholders the proxy
replaces.

- **Which tokens an env needs** is the union of `credentials[].service`
  over its kits, read with `sbx kit inspect --json` (a local kit is
  built first, which takes seconds), and the services its bindings name.
  `optional` is read from the kit's YAML, as inspect drops it.
- **Which sbx has** is `sbx secret ls --json`: name, scope and a mask
  (`(stored)`, `(oauth configured)`), never a value.
- **`unibox secret set`** has `sbx secret set <service>` ask for each
  missing one on the user's own terminal, which unibox only hands over;
  a value piped to `unibox secret set` goes straight to `sbx secret set`
  on its standard input. Either way it never reaches argv, a log,
  `--json`, state or config.yaml. `--command` stores instead a host
  command that prints the token - a keychain read, a vault - which sbx
  runs itself whenever it needs the token.
- **Where a token is made** is not in a kit, so unibox knows GitHub's
  fine-grained token page for `github` and `copilot`, filled in with the
  permissions through its URL parameters (`user_copilot_requests=read`
  for copilot), and the team names any other, or its own for those, under
  `secrets.<service>: {page, hint}` in its unibox.yaml files
  (`TokenGuides`). Before sbx asks, `secret set` offers to open the page
  in the browser; `--generate` opens it without asking. The page is all
  unibox hands over: the token is still typed into sbx.
- **Global, or a command a scope keeps.** `sbx env rm` deletes a
  sandbox's own secrets with the sandbox, so a token stored for one
  sandbox would be lost at the first recreation. A token of an env's or
  a sandbox's own is therefore a command in config.yaml, which unibox has
  sbx keep for each sandbox again at every start; the sandbox's wins over
  the env's, the env's over the global one. A value typed for one goes to
  the system's keychain by its own program, asked on the terminal -
  `security add-generic-password -w` on macOS, `secret-tool store` on
  Linux, `Read-Host -AsSecureString` into a DPAPI-encrypted file on
  Windows, which has no Credential Manager command that prints a secret -
  and the command kept reads it, so the value never passes through
  unibox. Not yet checked against real sbx on Windows: how sbx runs a
  `--command` there decides whether the PowerShell read needs other
  quoting.
- **sbx asks once to bind a credential** at the first `create` (verified);
  unibox writes no bindings. Unibox 1 wrote `~/.config/sbx/credentials.yaml`
  itself because sbx 0.47 asked nothing and injected a kit's credential
  only with a binding there ([kits v3](#kits-v3-multi-agent-credentials-validated-2026-10-13));
  with bindings in the sbxenv and sbx asking, writing them would be
  approving on the user's behalf.
- **The forge is the boundary for a clone's push.** The agent can push to
  any branch the token may, force-push, delete. Protect `main` and
  release branches on the forge, and restrict the token to the work
  branches where possible (GitLab push rules and protected branches,
  GitHub rulesets). Issue it narrow: a GitLab project or group token, a
  GitHub fine-grained one.

## The agent

- **The agent is the kit's.** The workload kit - the kit the sbxenv's
  `agent` names, else the first of kind `sandbox` - says how it starts:
  `sbx kit inspect --json` gives `sandbox.entrypoint` and
  `sandbox.command.default` (verified: `["copilot"]`, `["--yolo"]`).
  unibox records both when it readies the sandbox, and runs them, with
  the arguments after `--`, through `sbx exec` in the task's directory.
  Permissions, version and configuration of the agent are the kit's.
- **Through bash, not sh.** `sh` in the guest is dash, which drops
  variables whose names are not a shell's (`SBX_CRED_GITLAB-2N_MODE`) and
  does not read `BASH_ENV`, where mise puts its tools on PATH (verified).
- **Prepared before every session.** unibox knows four agents by their
  command (`agents.go`) and answers what each would otherwise stop to
  ask: claude's `hasTrustDialogAccepted` for `/work`, the task and its
  repositories, `bypassPermissionsModeAccepted` and
  `hasSeenAutoDefaultNudge` (unibox runs it bypassing permissions, so an
  offer of auto mode as the default is moot), merged into
  `~/.claude.json`, as sbx's own claude kit seeds; copilot's
  `trusted_folders` in `~/.copilot/config.json`, added to; codex's
  `trust_level = "trusted"` for `/work`'s task directories in its
  config.toml, between `# unibox:begin` and `# unibox:end` markers. What
  unibox writes into an agent's file is merged or between markers, never
  over the user's or the kit's settings. claude also gets
  `DISABLE_AUTOUPDATER=1`, as the kit's image fixes its version.
- **`--agent` picks another agent the sandbox has**: one of the env's
  other workload kits, as that kit starts it, or one of the four unibox
  knows found on the sandbox's PATH, with the arguments sbx's kit for it
  passes. The task remembers it. One the sandbox lacks is refused with
  the env's agents named; unibox does not check its credentials, which
  are the kit's.
- **An agent unibox does not know is still run**, with nothing prepared.
  An env whose kits run no agent gets a shell (`--shell`).

### Commits

- The author is the user's: `user.name` and `user.email` from the host's
  git configuration of the checkout, or `--author`. A forge's push rule
  that the author must be one of its users passes. In a clone they are
  set in the clone; in a mount, through the guest's include, since unibox
  writes nothing into the host's `.git`.
- **Co-authors are the agents' own, or off.** By default each agent
  names itself co-author or not as its settings say. With `co_author =
  false` in `vars.toml`, for every env or in `[envs.<env>]` for one
  (`unibox co-author off [-e <env>]`), `run` and `resume` name the agent
  (`UNIBOX_AGENT`, `UNIBOX_CO_AUTHOR`) to a `commit-msg` hook - the
  clone's `core.hooksPath`, or the mount's include - which takes out any
  co-author trailer the agent added of itself, by its name or address
  (codex and copilot add one whatever their settings say). A commit made
  in a shell, with no agent running, is left as it is. The hook is rewritten before each session; it lives
  in the guest, outside the host's `.git`.

## Repositories by name

An env's tasks may be given repositories by name from an index (`repos:`,
`repoindex.go`): the team's in its unibox.yaml files and the person's in
config.yaml, for every env or one, laid as ports are, a later layer's
fields over an earlier one's and a person's entry of `default` alone a
switch. Each is a forge address, a `ref` and a default: `always` puts it
into every new task of the env (`run`, `prepare`, `check`), cloned even
beside mounted directories - a task's repositories are mounted or cloned
each on its own (`Repository.Mounted`, `RepoInput.Clone`), and `decide`
leaves those always cloned out of whether the rest are mounted; `on`
has `task new` pick it to begin with; `off` is only there to pick. A task
that exists keeps what it holds. `context` (unsaid: `always`) marks one a
task reads: a shared branch and push leave it on its ref
(`RepoInput.Context`). A directory of the host in the index is mounted.
Per input, `Push` makes a work branch on the forge from its default, or
pushes a mounted checkout's after it is switched; `Fork` makes a clone's
branch in the sandbox alone from `Base` (`api.Repository.Base`, the
prepare script cloning the base and switching), which a later
preparation leaves as it is until the forge has it. A name works wherever a repository does,
unless a directory of that name is here. `task new` picks at a terminal
on one screen (`taskPickModel`, `tasknewui.go`): roles, the work branch,
push per row and the task's name, asking no forge anything while it is
drawn: a repository with no branch shows `default`, the forge's default
branch, which the clone in the sandbox finds (the host never resolves it,
an empty `Branch` standing for it), and each row shows what it gets and
the name the task will have
(`unibox.TaskID`, mounted repositories first as a task holds them, named
after the first picked by hand and worked in; `run` names a task after
its first repository worked in; `task new` passes the name it showed).
As the task starts, the work branch is looked for once - `git ls-remote`
for a clone, the checkout and its origin for a mount - and a branch that
exists already is asked about rather than taken unawares
(`confirmBranch`). `r` in the task list adds repositories
to a task on the same screen (`AddRepositories`). Where a branch made in
the sandbox started (`Base`) is no part of a task's identity.
Lists filter fuzzily after `/` (`fuzzy.go`, letters in order, side by
side and at a word's start counting more). `unibox repo add|rm|default` write the
person's.

## What a task's agents are told

`instructions` - the env's, at the top of the team's unibox.yaml files and
the person's config.yaml (every env, then `envs.<env>`), in that order, and
each indexed repository's - are gathered into `/work/<task>/AGENTS.md`
before each session (`agentdoc.go`), in the same `sbx exec` that starts
it: the task's repositories with their role and branch, the env's, then
each repository's. `CLAUDE.md` links to it, as Claude reads that name;
Codex and Copilot read `AGENTS.md`. With it the session starts in the
task's directory rather than its one repository. The file is marked as
unibox's and goes when nothing is to be said - a task of one repository
and no instructions, whose session starts in it; a task of several always
has the list of them. A task's directory the
agent cannot write in - sbx makes a mount's parents as root, so
`mountTask` makes it as the agent first - is said and left.

sbx says the same of the sandbox: each kit's `agentContext` goes to
`~/kits-agent-context/<kit>.md`, listed in a `## Kits` section of the
agent's own file. The two do not overlap - the kits describe the sandbox's
tools, `instructions` a task's work. Verified with sbx 0.49: for Copilot
that file is `~/AGENTS.md`, which Copilot never reads (only the git root's
and the working directory's), in sbx's workspace or a task's; a link from
`~/.copilot/copilot-instructions.md`, or `COPILOT_CUSTOM_INSTRUCTIONS_DIRS`,
makes it read. Most of that file, though, some 18 KB, is the agent kit's
guidance on sbx itself (between `sbx:guidance` marks), which no setting
leaves out; the `## Kits` section alone, between `sbx:kits-section` marks,
copied into Copilot's file at each start, is what the kits say. That is
the team's kit's to do, not unibox's. A
`CLAUDE.md` linked to `AGENTS.md` in one directory is read by Copilot once.

The git unibox runs on the host takes `-c` over every key of the
repository's own configuration that names a program (`gitGuard` in
repos.go): a mounted checkout's `.git` is the agent's to write, and unibox
runs git there unasked - reading a branch, checking one on the forge. The
keys are listed with `git config --local --includes --name-only`, which
runs nothing; hooks, fsmonitor, askpass and the ext transport are off
always. sbx 0.50 did the same for its own host git.

`unibox task instructions` reads in the running sandbox, never starting
one, the files each agent's documentation says it reads (Copilot's
`/help`, Claude's and Codex's memory files; `agentInstructionFiles`), from
the directory a session starts in, each once by its real path - Copilot,
checked, reads a `CLAUDE.md` linked to `AGENTS.md` once. Unibox's own file
is shown as the next session writes it.

A task is removed while others of its sandbox work: it waits for its own
session lock alone, checks its own clones, and takes only its record out
of the state (`removeTask`).

## Ports

A host port is the machine's business - what already runs there, what
number a browser knows - so a person's last word on it is in config.yaml,
never in an sbxenv.

- **Ports are written by hand.** The team's files and config.yaml's
  `ports:` say each port's mapping and whether it is published at start;
  no command writes a mapping, as a command per field only stood between
  a person and a short file. Whether a port is on at start is the one
  exception, being what a person changes while looking at the list:
  `port default <name> on|off|-` (`d` in the list) writes `{default:
  on|off}` under the env in config.yaml, drops it when the layers beneath
  say the same, and `-` takes it back, a port of one's own mapping
  keeping it. The list and `port ls` mark a default that is the
  person's, and `env show` one the person set for a port the team names. `port on|off [name[=HOST_PORT]...] [--map]`
  publish and unpublish on a running sandbox alone, for as long as it
  runs (`PortSwitches`, `PortMaps` in the state); the next start goes
  back to the files. `name=HOST_PORT` keeps the named port's sandbox
  port, host address and protocol. A stopped sandbox is refused, since
  publishing would start it.
- Unibox records what it published, by name and mapping, finds sbx's
  entries by the mapping (any host port for an ephemeral one), unpublishes
  them by what sbx lists, and **leaves a port it did not publish alone** -
  one a kit declares (`port@1`) or one published by hand with `sbx ports`.
  `port ls` shows those apart, without a name.
- A kit's ports get new host ports at every start (verified), which is
  why named ports stay the person's.
- **The team may name the ports everyone uses**, beside the sbxenv file in
  `<name>.unibox.yaml` (`teamports.go`): `ports.<name>: {port,
  description, default}`, `default` off unless said. sbx never reads the
  file, so it limits nobody without unibox; that is why it is not a field
  of the sbxenv, which sbx would refuse or a newer sbx might mean
  otherwise. The file is read strictly, an unknown key an error. Of an
  env's several files, a later one's ports win, as its sbxenv does.
  `unibox.yaml` at the root of the repository (the first directory up
  with `.git`) names ports for every env in it, under the env's own file.
  The same files say where the env's tokens are made (`secrets:`, see
  the secrets above).
- **Four layers, the person's last.** The repository's, the env's file's,
  the person's for every env (`ports:` at the top of config.yaml) and the
  person's for the env. A switch for every env that an
  env does not name is skipped there, since another env may; one for a
  single env is an error, as it can only be a mistake. Each port carries
  where it was first named (`From`), which `port ls` shows.
- **The person's choice wins.** An entry without a mapping
  (`{default: on}`) in config.yaml switches the team's port alone; one
  with `port` gives it another mapping, its description kept.

## Network

What a sandbox may reach is sbx's network policy; unibox gives it
commands so nobody needs `sbx policy` (`policy.go`, `cli/network.go`):
`network init|ls|allow|deny|rm`. A fresh sbx creates no sandbox until the
global policy every sandbox starts from is chosen (`sbx policy init
balanced|allow-all|deny-all`, once; until then `env create` fails with
412). `network init` chooses it, asking at a terminal with balanced
first, and init asks it in its first step; `sandbox create` and `run`
ask too, and away from a terminal stop before sbx is called, naming
`unibox network init`. Whether it is chosen is sbx diagnose's `Global
network policy` check. A rule is one sandbox's, never every
sandbox's, as a forgotten `-s` should not open a host to all of them.
sbx refuses a rule for a sandbox it does not have, so unibox keeps the
person's in config.yaml (`sandboxes.<name>.network`, `network.go`) and
applies what sbx lacks after each create and when one is added; they go
when the sandbox is removed. `ls` is about the difference: the person's
rules, ones added with sbx itself, and the hosts `sbx policy log` says
were refused (the DNS search-domain guesses under `docker.internal` left
out) - what the env's files may want to say; the kits' and the defaults'
hosts are only counted unless `--all`. `export` hands that difference to
the team: the person's rules and those added with sbx, less a host a kit
allows already (ports ignored, as a kit's allow list ignores them), as a
kit's `network-policy@2` entry to merge, the refused hosts as lines to
uncomment. A kit's entry has no protocol, so a udp rule is marked. Hosts are joined with commas for
sbx, so one holding a comma or whitespace, or starting with `-`, is
refused first. A rule the whole team needs belongs in a kit, reviewed
with the env.

## Doctor

`unibox doctor` checks what a run needs before a run fails on it, each
failure with the command that fixes it. A check whose fix is one unibox
command with nothing left to choose carries it (`Fix`), and `doctor -i`
offers them one at a time at a terminal, running each through `runStep`
as init runs its steps; one needing a value or a choice (an arg, the SSH
agent yes or no) stays a hint:

- **sbx**: installed; its own `sbx diagnose` (details that may name the
  user's account left out); a version of at least `MinSBXVersion` (0.50.0),
  an older one a failure; `env.rememberHostCommands`
  (on is a warning: sbx would run an env's host commands again without
  asking); the SSH agent against the user's decision.
- **each env**: its files read; each required arg given, and an arg set
  that the env does not declare; `sbx env plan` with the complete args,
  its summary line or the first line of its error; each secret stored or
  missing (optional ones a warning); a change since its sandbox was made.
- **the sandboxes**: one sbx no longer has; in a running one, the mounts
  under `/work` compared with the tasks'. sbx lists no mounts; the
  sandbox's `/proc/self/mountinfo` does, and a virtiofs mount's root is
  the host path (verified), so a stray mount is unmounted exactly by
  `unibox prune`, which does so for every running sandbox, under each
  one's guard so a busy sandbox is passed and named. A task's mount the sandbox lacks is a
  warning - its next `resume` mounts it again, as after a recreation; a
  mounted host directory that is gone is a failure, since the sandbox
  cannot start.

Every fix doctor or an error names is a unibox command - `unibox prune`,
`unibox config set remember_host_commands false`, `unibox secret set` -
never sbx's, so that nobody using unibox needs its CLI.

`init` sets up what doctor checks, as a walk of five steps (`cli/init.go`):
sbx (installed, its daemon started, `sbx login` run for the person, the
SSH agent, the network policy), Envs,
Values, Secrets, Sandboxes. Each step runs the command it stands for
(`env add`, `var set`, `secret set`, `sandbox create`), so the
walk and the commands never differ; what is in place already is only said.

## The SSH agent

sbx forwards the host's SSH agent into sandboxes by default, which lets an
agent sign with every key the user has. That is the user's call, not
sbx's default: unibox refuses to run while sbx forwards it and
`ssh_agent` in config.yaml says neither yes nor no. `init` asks,
recommending no; `unibox config set ssh_agent false|true` decides it and
sets sbx to match (restarting sbx's daemon, which a change takes effect
with). With it, sessions get `GIT_SSH_COMMAND` with
`StrictHostKeyChecking=accept-new`, as the guest has no known_hosts.

## Editors over SSH

sbx serves each sandbox to editors and desktop apps as `ssh
<sandbox>.sbx` when its `feature.ssh` flag is on, a flag that may also
arrive from Docker's side (`source: remote`); its daemon registers the
endpoint only when it starts with the flag on, so a daemon started before
`sbx login` - as unibox starts one to ask whether sbx is signed in -
answers 404 until it restarts. `editor_ssh` in config.yaml is the user's
say, asked by init after the SSH agent: on, unibox sets the flag,
restarts the daemon and runs `sbx setup ssh`, which writes the client's
host entry; off, `sbx setup ssh remove` goes first. Doctor compares the
flag with the say. An editor's session is not unibox's, so stop_idle may
stop a sandbox under it.

## Trust

- **The team's repository is trusted**, as its sbxenv files run host
  commands and bind credentials. sbx asks the user to approve both at the
  first create, on their terminal; unibox never answers for them, and
  doctor warns while `env.rememberHostCommands` lets sbx stop asking.
- **A change of the env is told, not asked about, and not applied**:
  doctor and the next start say the env changed since the sandbox was
  made, and the user recreates when they choose. Recreating discards what
  the sandbox holds outside its tasks, so it is never done unasked. The
  agents' conversations of each task are not discarded: kept on the host
  after every session and before the sandbox goes, they are put back as
  the task is prepared again, so resume continues them. Each agent finds
  its last conversation by the directory it had, so a task's are those of
  its directory and its repositories', and no other task's.
- **The worked-on repositories choose nothing of the sandbox.**

## Unibox

### Its home

```
~/.config/unibox/config.yaml   the user's choices: envs, ports, default_env,
                               context, stop_idle, ssh_agent
~/.config/unibox/vars.toml     the person's args, per env
~/.config/unibox/state.json    what use taught: sandboxes (env, files, sbx id,
                               fingerprint, agent), tasks, runs, ports
                               published, directories told of
~/.config/unibox/locks/        a guard per sandbox, usage and task locks
~/.cache/unibox/args/          an args file while sbx env create or plan runs
~/.local/share/unibox/repos/   the team repositories init cloned
```

```yaml
default_env: dev
envs:
  dev:
    files: [~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml]
    repo: ~/team/agent-sandboxes
    ports:
      fe: '3000:3000'
      devtools: {port: '9333:9222', default: off}
      vite: {default: on}           # the team's port, switched on
stop_idle: true
ssh_agent: false
```

unagit writes this file only through `unibox config set` and the commands,
never by hand. `remember_host_commands`, `claude_remote_control` and
`clipboard_image_paste` are
sbx's settings (`env.rememberHostCommands`, `claude.remoteControl`,
`clipboard.imagePaste`) the
person may say here: unibox sets sbx to match before it creates or starts
a sandbox, so the file can be edited by hand. Left out, sbx's own stand. init asks
for `remember_host_commands` after editors over SSH, and for
`claude_remote_control` once an env it sets up runs claude, and for
`clipboard_image_paste` once one runs claude or codex, which paste images
with Ctrl+V; sbx serves them none until told to. doctor says so while it
is off and unsaid, and leaves turning it on to the person, as it lets a
sandbox read images copied on the host.

### Platforms and releases

Unibox builds with `CGO_ENABLED=0` for Linux, macOS and Windows, amd64 and
arm64. What differs per system is small and behind build tags: file locks
are `flock` on Unix and `LockFileEx` on Windows (`lock_unix.go`,
`lock_windows.go`), as is detaching herdr's watcher (`Setsid`, or a new
process group without a console); the environment handed to sbx and git
keeps Windows' own variables there (`SYSTEMROOT`, `USERPROFILE`, ...); a
clone's directory is cleaned of characters Windows refuses. CI runs the
whole suite on Linux and macOS; on Windows it builds, vets and runs the
tests that need no shell stand-in, as the stand-ins are `sh` scripts.

A `vX.Y` tag publishes `make dist`'s archives - one binary, the licence
and the README per system - with their SHA-256 sums as the GitHub release.
Homebrew and Scoop install those archives, each checked against its sum:
the release writes `Formula/unibox.rb` for macOS and Linux and
`bucket/unibox.json` for Windows whole, so no machine builds unibox but
CI and none needs Go. A binary installed by brew carries no quarantine
mark, so macOS runs it without a Developer ID signature or notarisation;
Go signs a darwin/arm64 binary ad hoc as it links, which Apple silicon
needs. Only an archive downloaded with a browser is quarantined. The jobs that write run none of
the repository's code.

### The command line

Everything a list shows has `--json`; a long operation prints its
progress as lines, for a log. Called with `--json` or `--yes` it never
asks: what stands in the way is an answer, not a prompt. `unibox
--version` and `unibox version` print the release (`dev (<commit>)` for a
build that is not one); `unibox version --json` adds `api_version`.

| command | does |
| --- | --- |
| `unibox init` | the walk: sbx, an env, its args, secrets, the sandbox |
| `unibox doctor [-e E] [-i]` | what a run needs, and what to fix; `-i` offers to run each fix |
| `unibox env add\|ls\|rm\|show` | the envs: sbxenv files, the sandbox's name |
| `unibox var ls\|set\|rm -e E` | the person's values for an env's args |
| `unibox secret ls\|set -e E [--command C] [--all] [--generate]` | the tokens an env needs, and having sbx store them |
| `unibox run [dir\|url] [--repo ...] [--clone] [-- args]` | prepares a task, runs the kit's agent (or `--shell`) here |
| `unibox resume <task> [--shell \| --agent]` | a prepared task's agent again, in the sandbox that holds it |
| `unibox task check\|prepare …` | what stands in the way, as `needs` with a `code` each; the task ready, JSON at the end |
| `unibox task ls [-s S]`, `task add-repo`, `task rename`, `task instructions`, `task rm` | tasks across sandboxes; one more repository; an alias; what its agent reads; removal |
| `unibox sandbox create [--adopt]\|ls\|inspect\|shell\|stop\|recreate\|rm` | as named; removal checks for sessions and unpushed work |
| `unibox port on\|off\|ls` | a running sandbox's ports |
| `unibox context [use -s S -e E \| clear]` | the sandbox and env commands work in when not told |
| `unibox config get\|set\|unset` | `default_env`, `stop_idle`, `ssh_agent`, `editor_ssh` |
| `unibox prune` | forgets the sandboxes sbx no longer has |
| `unibox destroy [--all] [--force]` | every sandbox unibox created; with `--all` its state, configuration and the person's args too |
| `unibox herdr install-plugin\|uninstall-plugin\|status` | [herdr](#herdr) |

**Nouns, then verbs** (the user's call): commands are grouped by what they
work on, and only the everyday ones stand alone: `run`, `resume`, `init`,
`doctor`, `context`, `config`, `prune`, `destroy`, `version`. The
positional argument is only the thing a command acts on; the env it works
in is `-e`/`--env`, the sandbox `-s`/`--sandbox`, the same flag
everywhere.

Left out, `-e` and `-s` come from `UNIBOX_ENV` and `UNIBOX_SANDBOX`, then
from the context, then from there being only one, as kubectl keeps a
context. `unibox context use` sets it after checking it exists, `unibox
context` shows it and why, `unibox context clear` forgets it; removing the
context's sandbox clears it. Two places do not default, so a forgotten
context cannot do harm: `unibox sandbox rm` always names its sandbox, and
`unibox task ls` lists every sandbox's tasks unless given `-s`.

`destroy` and `--all` leave the team's files, the kits and the secrets in
sbx alone: they are the team's and sbx's.

Exit codes say what kind of failure: 4 refused (`arg_missing`,
`sandbox_taken`, `unpushed_work`, `guest_busy`, ...), the agent's own
status after a session, 1 any other error. Under `--json` a failure is
`{"api_version", "code", "message"}`. Errors are lower case and say what
to do: `env dev needs artifactoryUser; set it with unibox var set -e dev
artifactoryUser <value>`.

### The runtime

`Runtime` in `runtime.go` is what the manager asks of sbx: list, exec,
stop, `env create`/`env rm`/`env plan`, `kit inspect`, secrets, mounts,
ports, settings, `diagnose`, the SSH agent. `sbx.go` is the only
implementation, `fake_test.go` the tests'. It is there so the manager's
tests run against a stand-in in Go rather than scripts on PATH, **not for
another runtime**: nothing above it promises one, and its methods are
sbx's own verbs.

- Every sbx command runs with a filtered environment (`HostEnvironment`:
  PATH, HOME, the locale, the terminal and little else), so sbx cannot
  pick up an unrelated token or SSH agent by accident.
- **sbx's JSON changes shape between releases** (0.48 made `ls` and
  `template ls` bare lists); a list is read through `sbxList`, which takes
  either.
- `MinSBXVersion` is the oldest sbx unibox was checked against: sbxenv
  files, args files, kit args and mounts as unibox uses them. It rises
  when unibox starts relying on something newer.

### herdr

herdr recognises an agent by its pane's foreground process, and an agent
behind `sbx exec` is not one it knows: no agent, no state. Unibox fills
that in with **a herdr plugin of its own**. It is the one way; there is no
launcher and no third-party plugin to choose instead.

What it does, as long as herdr runs:

- `unibox run` started in a herdr pane records the pane
  (`HERDR_PANE_ID`), the agent and the sandbox in unibox's state. For
  every pane so recorded whose `unibox run` is alive, the plugin reads the
  screen (`pane.read`), has herdr classify it by herdr's own rules for that
  agent (`herdr agent explain --file … --agent <kind> --json`), and reports
  the state (`pane.report_agent`, `pane.release_agent` when the agent is
  gone), with the sandbox's name as the pane's metadata
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
- The path changes when unibox is upgraded or moved. Every unibox
  command that finds the plugin installed compares the linked manifest
  with what it would write and rewrites it if it differs - the user
  consented to the plugin, not to a path.
- A sandboxed agent started in herdr without the plugin runs, with no
  state.

The agents' own hooks (Claude's session id, opencode's states) do not cross
into the VM; the state is the screen's.

### The boundary

- Unibox's code is `cmd/unibox` and `internal/unibox/...`, in its own
  repository with its own AGENTS.md. It imports nothing of unagit's.
- unagit imports nothing of unibox's. It runs the binary and decodes
  `--json` into types of its own, written against `internal/unibox/api`
  and refusing an `api_version` it does not know. `api_version` changes
  only when a field unagit reads changes meaning or goes.
- unagit's side of the integration is unagit's plan, in its
  docs/sandboxes.md.

## What was verified about sbx

Facts about sbx the design rests on, each found on a throwaway sandbox
that was removed afterwards. Some were found for unibox 1, against sbx
0.47, for features since gone (services, generated kits, network and
port commands); they are kept for what they say of sbx, and the parts
about how unibox 1 used them are marked as history.

### sbx as the configuration: verified for the pivot (2026-10-25)

Phase 0 of [sbx-pivot.md](sbx-pivot.md), checked against sbx 0.49.2 on a
throwaway sandbox created from the team's `my2n-dev.sbxenv.yaml`, then
removed.

- **`sbx env plan` has no JSON.** It prints text only. It does validate:
  an undeclared or missing required `--env-arg` exits 1 with the
  argument's name and description. Unibox reads `args`, `kits` and
  `bindings` from the YAML itself, read-only, and runs `plan` only for
  its exit code and to show the user.
- **Bindings are approved once, by the user.** A binding not yet applied
  is "present, not recorded as applied, needs your approval"; `create`
  asks once, and `--force` approves without asking. Unibox runs the
  first create with the user's terminal and never passes `--force` on
  their behalf.
- **The agent's command is the workload kit's.** `sbx kit inspect --json
  <kit>` gives `sandbox.entrypoint` (`["copilot"]`) and
  `sandbox.command.default` (`["--yolo"]`). A local source kit is built
  first (seconds; progress on stderr, the JSON alone on stdout).
- **Which secrets a sandbox needs** is the union of `credentials[].service`
  over its kits from `kit inspect --json`; inspect drops a credential's
  `optional`, which only the kit's YAML has. What is stored is
  `sbx secret ls --json`: `.secrets[]` with `name`, `scope` and `secret`
  masked as `(stored)` or `(oauth configured)`, never a value.
- **`sbx exec` gives the agent what `sbx run` does.** The environment of
  `copilot` started by `sbx run` and that of `sbx exec [-it]` are the
  same: proxy variables, `JAVA_TOOL_OPTIONS`, `BASH_ENV`, `PATH`,
  `SBX_CRED_*`. Only the terminal's variables differ, as expected without
  `-it`. `sh` is dash: it drops variables whose names are not a shell's
  (`SBX_CRED_GITLAB-2N_MODE`) and does not read `BASH_ENV`, so mise's
  tools are found under bash only. `sbx exec -w /tmp utest copilot -p …`
  signed in and answered, while a session started by `sbx run` was
  working in the same sandbox: two agents at once are fine.
- **A same-path mount** (`sbx mount S /a/b/repo` is `/a/b/repo:/a/b/repo`)
  creates its parents as root, mode 755: the agent cannot write beside the
  repository. Inside it git works, a commit is the host's at once, and
  `.git/hooks` is writable - the risk the plan warns of.
- **`sbx mount` needs a running sandbox** (0.49: `409 Conflict: sandbox
  "x" is not running`), while `sbx exec` starts a stopped one first, and
  sbx has no start of its own; Mount starts the sandbox with `sbx exec
  <name> true` and mounts again.
- **Mounts are kept across stop and start**, and a saved mount whose host
  path is gone fails the start until it is removed with `sbx umount` while
  stopped. Removing a task therefore unmounts before anything else, and
  doctor checks every mounted host path exists. sbx lists no mounts;
  `/proc/self/mountinfo` in a running sandbox shows each with its target,
  and the root of a virtiofs mount there is the host path.
- **A kit's ports (`port@1`) get new host ports at every start**
  (49226 became 49228), which is why unibox's named ports stay the user's.
- **The network policy** (`sbx policy ls --json --type network`) is
  `.rules[]` with `applies_to` (`all` or `sandbox:<name>`),
  `provenance.created_via` and, for a kit's rule, a name `kit:...`, which
  sbx refuses to remove. `allow|deny network --sandbox S` puts a rule into
  the `local` policy scoped to S; `rm network --force --resource h1,h2`
  removes by hosts; `policy log --type network --limit N` shows the
  decisions. Checked on a throwaway sandbox, then removed.
- **A fresh sbx has neither its daemon running nor a network policy**
  (0.49): `sbx diagnose --format json` then exits 1 with its report on
  stdout, the daemon `fail` and the sign-in `skip`; `sbx daemon start
  --detach` starts it without asking, after which diagnose says whether
  sbx is signed in and that the `Global network policy` is `not
  initialized`.
- **`sbx settings set|unset|get <key> --json`** reads and writes sbx's
  settings; `env.rememberHostCommands` is false by default.
- Gradle's locks in a cache shared by several sandboxes are the team's
  concern, not unibox's.

### Mounts into a running sandbox (unibox 1, sbx 0.47)

`sbx mount <sandbox> HOST:TARGET`, missing from sbx's top-level help,
mounts into a running sandbox: checked on a throwaway sandbox, it is live,
writable, kept across a stop, taken out by `sbx umount` (which names the
mount by both host path and target), leaving an empty mount point, and
removing the sandbox leaves the host's files alone. Mounting again what is
mounted is harmless.

### Credentials and the proxy (unibox 1, sbx 0.47)

Checked on throwaway sandboxes while unibox 1 wired host services (a
forge, an Artifactory) as sbx custom secrets. Unibox 2 leaves such wiring
to the team's kits and bindings; what sbx does stays true.

- A **local** custom secret only substitutes its placeholder wherever the
  sandbox sends it; `--header` and `--format` are `--cloud` only (sbx says
  so and refuses them). So the proxy puts the stored value in place of the
  placeholder verbatim, in the request headers, on requests to `--host`.
  Verified: a secret stored as `Basic <base64>` under placeholder
  `ubx-ph-test` turned `Authorization: ubx-ph-test` into
  `Authorization: Basic <base64>` at the host, and nothing elsewhere.
- **Git over https** therefore cannot go through git's own credential: git
  base64-encodes `user:password`, so a placeholder password never appears
  in the header for the proxy to find. What worked (unibox 1 wrote it):
  `git config --global http.https://<host>/.extraHeader "Authorization:
  <placeholder>"`; git sends the placeholder verbatim and the proxy swaps
  in the whole credential, the stored value being the entire header,
  composed on the host (`printf '%s:%s' <user> "$(<command>)" | base64`),
  so the token and its base64 never leave it. It is forge-neutral, and the
  way around a v3 kit ignoring `apiKey.inject.scheme`.
- A **command** secret is stored as its source, not its value: the sbx
  daemon runs the command on the host at verification and again whenever
  it refreshes, from a fresh temporary directory (sbx's own help). A custom
  secret refreshes on demand unless `--refresh` gives a duration (a
  built-in `sbx secret set` defaults to 55 minutes). So a credential
  helper runs outside the process tree that created the sandbox, at any
  hour, with no terminal: it must reach its secret alone - a keychain, an
  agent socket - and an environment variable or an unlocked parent cannot
  carry it there. A short-lived token fits this well with a refresh
  shorter than its lifetime: each refresh fetches the next one. This is
  what `unibox secret set --command` relies on.
- **A sandbox made again under its name ignores a token set for it**
  (sbx 0.50.1). Checked on a throwaway env: after `sbx env rm` and `sbx
  env create`, `sbx secret set github --sandbox S --command ...` says it
  applied the token and `secret ls` lists it, yet the proxy sends the
  global one - after a restart and a second set alike. A `sbx secret rm
  github --sandbox S` before the set, though nothing is listed to remove,
  makes it take, every time; a sandbox never made before takes a set
  alone. So unibox removes before it sets.
- **sbx's own `github` secret takes github.com over.** With the built-in
  `github` service stored (copilot's `gh auth token`), the proxy sets
  Authorization on requests to github.com itself, and overwrites any the
  sandbox sent. Checked from a sandbox: git-receive-pack answered 200
  with no header, with a deliberately invalid one, and with a custom
  secret's placeholder alike, while gitlab.com answered 401 without one.
  So a narrower token for git on github.com never reaches GitHub while
  that secret is stored - the token behind it does, whatever its reach.
- **Copilot without the built-in secret.** copilot reads its token from
  `COPILOT_GITHUB_TOKEN`, and accepts a placeholder of the token's shape
  there. Checked on a throwaway sandbox with the global `github` secret
  removed: a custom secret (`--env COPILOT_GITHUB_TOKEN --placeholder
  'gho_{rand}'`, the hosts copilot calls - api.github.com,
  api.githubcopilot.com and its individual, business and enterprise
  hosts, copilot.github.com - and a host command for the token) let
  copilot answer, an unregistered `gho_` value got its login prompt, and
  github.com answered 401 without a header. The variable is not set in a
  sandbox created before the secret.
- **One forge, several owners.** A fine-grained GitHub token has one
  resource owner - the user or one organization - so repositories of
  several owners on github.com need a token each, told apart by path, not
  host. git's `http.<url>.extraHeader` is a list, gathered from every
  matching URL: checked on a local server, `.../org/repo` received both
  the host's and the org's `Authorization`. An empty value resets the
  list, and git applies the matches from the least to the most specific
  URL, whatever their order in the file - so `http.https://github.com/org/`
  with `extraHeader = ""` then the org's header sends the org's alone,
  and every other path keeps the host's. The proxy still matches the bare
  host, and sbx keeps two custom secrets on one host apart by their
  placeholders (checked: each swapped for its own value).
- A **host behind the user's VPN** is reached like any other: the proxy
  resolves and connects from the host, so the host's VPN routes it.
  Checked on a throwaway sandbox: gitlab.2n.cz (a private 10.x address)
  answered `Approval required` until allowed, then GitLab's own 302 - sbx
  does not refuse private addresses.
- **Basic authentication is substituted too.** A request with
  `curl -u user:<placeholder>` reached httpbin.org with the real value
  inside the Basic header (checked on a throwaway sbx sandbox,
  2026-10-19), so a build's `credentials { username; password }` can take
  the placeholder as its password. sbx takes upper-case variable names
  only for a custom secret (`^[A-Z_][A-Z0-9_]*$`), and injects a custom
  secret's variable only at create, so a tool reading its own variable in
  its own case (`ORG_GRADLE_PROJECT_artifactoryPassword2N`) needs it set
  some other way - unibox 1 set it in each session.
- `git config --global includeIf "hasconfig:remote.*.url:<url>**"`
  selects an identity by the remote's URL (git 2.36 or later; checked with
  a host and a path of one forge); unibox 1 used it for clones an agent
  made itself.

### Network rules and ports (unibox 1, sbx 0.47)

- **sbx changes a running sandbox's network rules live.** Checked on a
  throwaway sandbox: `sbx policy allow network --sandbox` let a refused
  host through at once (403 to 200), `sbx policy deny network
  --sandbox` shut a host the default policy allows (200 to 403), and
  `sbx policy rm network --id <rule> --sandbox <name> --force` undid
  either, all without a restart. sbx refuses a deny for a host an allow
  rule of the sandbox names exactly; the allow goes first. sbx refuses
  what its policy does not list (403 from the proxy) and already allows
  the usual registries.
- **Ports** (`sbx ports`): a port is published live on the host's
  loopback, tcp4 when no protocol is named; `--json` lists `host_ip`,
  `host_port`, `sandbox_port`, `protocol`; a host port taken already is a
  409; and published ports survive a stop - listed as none while stopped,
  back at the start, an ephemeral one with a new host port. `--unpublish`
  takes a port back only by its exact host port and does nothing, quietly,
  for another. Hence unibox's bookkeeping of what it published, and why a
  start drops the switches of a sandbox that was stopped: sbx would
  otherwise bring back whatever was on. A switch on a stopped sandbox is
  refused, as publishing would start it.
- `sbx create --help`: sbx refuses more than 75% of the host's memory, and
  `--cpus 0` means all host CPUs.
- `sbx env create` (0.49) has no `--memory` or `--cpus`; it merges its
  files in order, later ones winning, so a last file with nothing but
  `schemaVersion` and `sandboxOptions` sets them for any env (checked: 12g
  and 3 CPUs over an env's own). On a 64 GiB host 49152m was accepted and
  49153m refused.

### Images and templates (unibox 1, sbx 0.47)

- `docker/sandbox-templates` has a `shell` tag beside the agents' (claude,
  codex, copilot, cursor, devin, docker, droid, gemini, kiro, opencode),
  checked 2026-10-12.
- sbx makes an image per kit of a sandbox (`sbx-kit-src:<sandbox's
  runtime name>-<digest>`). `sbx template rm --force` removes a template
  a sandbox was made from, and the sandbox starts and runs as before - it
  needs its image only to be made.

### Kits v3: multi-agent credentials (validated 2026-10-13)

Unibox 1 took no kits at first; a v3 workload kit turned out to be how one
sandbox gets several agents' credentials, so it generated one. Verified by
hand (`agent-sandboxes/_spike/personal-kit`): a local kit directory - a
`# syntax=docker/sandbox-kit:3` descriptor beside a Dockerfile that is
just `FROM` a built image - creates a sandbox (`sbx create --name X
./dir`) whose `credential@1` declarations bind the host's stored secrets:
the spike sandbox had `SBX_CRED_OPENAI_MODE=oauth` over a three-agent
image. Today the team writes such kits, and unibox reads them.

- The kind of a sandbox is what binds credentials: built-in kinds are v2
  kits. v3 kits cannot mix with v2 built-ins, so the multi-agent path
  needs a v3 workload, created from a local directory or a registry.
- The agents' authoritative declarations - endpoints, sentinels,
  credential files, onboarding seeds - are docker's example mixins in
  github.com/docker/sandbox-kit-spec (claude-mixin, codex-mixin, ...).
- `sbx` ignores `apiKey.inject[].scheme` in v3 kits, so git-over-https
  basic auth cannot be declared by a kit as such; the `extraHeader` way
  above works around it.
- A kit's credential is injected only with a binding in
  `~/.config/sbx/credentials.yaml` ("no binding authorizes ... the
  credential was not injected"). sbx 0.47 asked no question at creation,
  not even on a terminal, so unibox 1 wrote the bindings itself. History:
  with sbxenv `bindings` and sbx asking for approval at the first create
  (2026-10-25), unibox 2 writes none.
- sbx keeps one OAuth policy per sandbox, taken from the first credential
  declaring OAuth whether or not a login is stored for it. With Claude
  declared first, the stored ChatGPT login (`sbx secret set openai
  --oauth`, the one OAuth sbx stores locally) was never wired; the order
  of declarations in a kit matters.
- Copilot calls its plan's API host - api.individual, .business or
  .enterprise.githubcopilot.com - besides api.githubcopilot.com. An
  undeclared host gets the placeholder token as it is, and Copilot answers
  400 "Authorization header is badly formatted"; a kit routes all of them
  to the github service, as sbx's own kits do.
- Codex signs in only through its own files - `forced_login_method`, the
  `sandboxd` provider routing to chatgpt.com, an `auth.json` placeholder -
  which codex-mixin seeds from `SBX_CRED_OPENAI_MODE`. Claude's kit
  declaration renders `~/.claude/.credentials.json` in the guest (on a
  Mac host Claude keeps its login in the keychain, so no such file exists
  there).

### Follow-ups (2026-10-13)

- The session-lock tests that failed one run in a few under full parallel
  load were a real race, now fixed: a flock belongs to the open file
  description, and a process another goroutine forked holds a copy of it
  until its exec, so closing the lock's file left it held for that moment
  and the next non-blocking try found it busy (or, with the error
  ignored, took nothing and stopped a sandbox in use). FileLock.Close
  unlocks first, which releases every copy.

## From unibox 1

Unibox 1 had its own description of a sandbox, `sandbox.yaml` in a
"profile", over sbx and an experimental microsandbox runtime. Once sbx's
env files and kits v3 could say the same - image, kits, args, mounts,
network, bindings - the profile was a second place for the team's
configuration, and the parts that turned it into sbx's were the most
fragile code unibox had. So unibox 2 dropped:

- profiles, their format, its versions and migrations, and the commands
  that edited them (`profile`, `agent`, `service`, `network`, `secret`,
  `approve`, `script`, `instructions`);
- templates and the environments repository's layout, which a team's own
  repository of sbxenv files and kits replaces;
- image builds and the agents' check, which sbx does from the kits;
- generated kits and writing `~/.config/sbx/credentials.yaml`, which the
  team's kits and the sbxenv's bindings replace;
- the agents' configuration seeds, which a kit carries;
- the microsandbox runtime, and with it the promise of runtime
  neutrality;
- tokens kept in the platform's store by unibox: secrets are sbx's
  secrets.

What stayed is what sbx does not do: tasks and their mounts and clones,
the person's values, secrets checked against the kits, named ports,
doctor, the agents' trust and co-author, herdr. Unibox 2 sets the old
state aside as `state.v1.json` and names the old sandboxes once, to be
removed with `sbx rm` when their work is safe; config.yaml's old keys are
read (`runtimes.sbx.ssh_agent` and `stop_idle` carry over) and dropped at
the next write. [sbx-pivot.md](sbx-pivot.md) has the plan and what each
phase did.

## Tests

`make test` runs `go test -race ./...`; `make test-fast` the same without
the race detector, while iterating.

- Every package's `TestMain` goes through `internal/testenv`
  (`TestEveryPackageRunsInTheTestEnvironment` checks it): a temporary
  home, a git configuration of the run's own, and a PATH with git, go and
  the shell's utilities only.
- The manager's tests run over `fakeRuntime` (`fake_test.go`), which runs
  a sandbox's commands in a temporary directory and makes mounts as
  symlinks: creation from the env with the args, a creation retried, a
  changed env told and applied only by recreation, adoption, a mounted
  directory never deleted with its task, a worktree refused, mount drift,
  ports, idle stopping, task names unique across sandboxes.
- sbxenv reading (`sbxenv_test.go`): args, required and defaults, several
  files merged, unknown fields ignored; the fingerprint changing with a
  file, a local kit's file or an arg, and not otherwise.
- The commands' tests that need sbx put a stand-in on their own PATH
  (`fakeSBX` in `cli/login_test.go`, through `testenv.WriteExecutable`)
  and are serial (`t.Setenv`); every other test calls `t.Parallel()`. A
  command run through `hostCommand` gets a filtered environment, so a
  stand-in is steered by files, not variables.
- No host secret in the arguments or environment unibox gives sbx.
- On Windows CI only the portable tests run: locks (`lock_test.go`),
  clone directories (`clone_test.go`) and reading the team's ports.
- The real sbx is checked by hand on a throwaway sandbox from the team's
  env: `run`, `resume`, `task rm`, `sandbox rm`, removed afterwards.

## Order of work

The pivot to sbx was done in phases 0 to 5 of
[sbx-pivot.md](sbx-pivot.md); each phase's "Hotovo" paragraph says what
was actually built and where it differs from the plan; its "Další kroky"
followed - init over a whole repository, team ports, the network, no need
of sbx's CLI, every platform. What comes next for unagit's integration is
in unagit's docs/sandboxes.md.
