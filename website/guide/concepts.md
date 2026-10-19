# Concepts

A handful of words carry the whole of unibox. Most are a page of their own later; this
one says how they fit.

```
 environments repo (git)            host                         sandbox (VM)
 ┌──────────────────────┐   ┌──────────────────────┐   ┌──────────────────────────┐
 │ sandboxes/<profile>/ │   │ unibox               │   │ /work/<task>/<repo>      │
 │   sandbox.yaml       │──▶│  profile → adapter   │──▶│   fresh clone, own branch│
 │   sandbox.dockerfile │   │                      │   │                          │
 │ _shared/  .shared/   │   │ runtime (sbx / msb)  │   │ claude / codex / copilot │
 │   @install @instr.   │   │  logins, proxy,      │◀──│   commits + pushes       │
 └──────────────────────┘   │  network policy      │   └──────────────────────────┘
                            └──────────────────────┘             │
                                      │ real tokens added        ▼
                                      └──────────────────────▶ forge, model APIs
```

## Environments repository

A git repository of yours that holds the profiles, along with installation
scripts and instruction texts they share. unibox knows it by its path
(`environments` in its configuration) and reads its working tree.
`unibox init --dir <path>` lays it out.
[More](./environments-repository).

## Profile

A directory `sandboxes/<name>/` with a `sandbox.yaml`. It describes an
environment: the image (`sandbox.dockerfile` beside it, or `image:`), the
resources, the agents and how each signs in, the forges and services the
sandbox reaches, the network rules and ports, scripts and instructions.
**A profile names no runtime**; the runtime's adapter turns it into the
runtime's own means. [More](./profiles).

## Sandbox

A VM made from a profile, with a name you choose. It persists until you
remove it: its disk, its Docker images, its caches and its tasks stay when an
agent ends. A sandbox in which the last agent unibox started has ended is
stopped, so idle sandboxes hold no memory; the next start boots it again.

A sandbox can be **shared** by several tasks - memory, caches and images are
paid once, but between the agents there is no isolation at all: one user
with sudo, one filesystem, one Docker, one set of ports. Or it can be **its
own** for one task, and nothing it does reaches another.

```sh
unibox run --profile web                      # sandbox "web", named for the profile
unibox run --profile web --sandbox web-login  # a sandbox of its own
unibox run --sandbox web-login                # an existing one
```

## Task

One piece of work in a sandbox: fresh clones of one or more repositories at
`/work/<task>/<repo>`, on a branch, with an agent running in your terminal. A
task's name is unique across all sandboxes, so the task alone finds its
sandbox:

```sh
unibox resume <task>
unibox add-repo <task> ../api
unibox rm-task <task>
```

By default a task is named after its first repository and a digest; `--task`
names it. [More](./tasks).

## Runtime

What actually makes and runs the VM. unibox has one adapter per runtime,
behind one Go interface (create, exec, stop, build, and optionally store
logins):

| Runtime | Status | Isolation |
| --- | --- | --- |
| `sbx` (Docker Sandboxes) | supported, the default | micro-VM, egress proxy, credentials kept on the host |
| `msb` (microsandbox) | experimental, used only when asked for | micro-VM, egress proxy, credentials kept on the host |
| Podman / Docker | planned | container |

A new sandbox goes to the default runtime, sbx unless
`unibox config set default_runtime <name>` says otherwise;
`--runtime <name>` asks for one sandbox, and a sandbox keeps the runtime it
was made in. msb shows that a profile runs unchanged on another runtime; it is not
meant for daily use yet. `unibox runtimes` shows what is
there; [The sbx Runtime](/reference/sbx) and
[The msb Runtime](/reference/msb) what was verified about each.

## Agent

A coding agent the image installs and the profile declares: `claude`,
`codex`, `copilot` or `opencode`. Each is started with its permission prompts
off, signs in through the runtime, and is credited as a co-author in the
commits it makes. One image can hold several. [More](./agents).

## Service

A forge or another host service the sandbox authenticates to - GitHub,
GitLab, an Artifactory - with a token kept on the host and a placeholder in
the sandbox. [More](./services-and-secrets).

## Host access

Everything a profile asks of the host: environment variables, mounted caches,
logins, services and network rules. unibox remembers what each profile asked
for last; when it changes, the next start says what changed and what it leads
to - an image built, a sandbox recreated, or rules applied live. Nothing asks
you to approve your own profile.
