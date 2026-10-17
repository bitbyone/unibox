# What unibox Is

unibox is a command-line wrapper over sandbox runtimes: today Docker
Sandboxes (`sbx`) and microsandbox (`msb`), perhaps others later. It gives
them one profile format and does the setup - the image, the agents' logins,
forge tokens, network rules, ports - so a coding agent runs in a sandbox on
your machine with one command.

That is all it is. The isolation, the credential proxy and the network
policy are the runtime's; unibox configures them and does not add security
of its own. It is meant for local development, not for running agents for
other people, in CI or in the cloud, and it has no plans beyond that.

A coding agent is most useful when it may do anything: install a package,
run the tests, start a database. Doing that on your own machine puts your
home directory, your SSH keys and your logins within its reach; a sandbox
runtime keeps them out, and unibox makes the runtime quick to set up.

## What it is for

A repository - or several - gets a coding agent that runs in a VM instead of
on your machine. The agent has full autonomy inside it, and its permission
prompts are off: the sandbox is the boundary, not a dialog. You pick the
agent, a profile (an image and its resources, kept in a git repository of
yours) and whether the agent shares a sandbox with other tasks or gets one of
its own.

## The model

```
host                                   sandbox VM
your checkout  <── git pull ──┐        /work/<task>/<repo>  clones of the
                              │          branch; the agent commits
                              │          and pushes here
                   forge ◄────┴──────── git push
```

- **The agent clones from the forge, commits and pushes; you pull.** The
  branch is on the forge before the agent starts, the sandbox clones every
  repository of the task at that branch, the agent pushes back to it, and you
  pull as you would a colleague's commits.
- **Nothing of the sandbox is mounted on the host, and nothing git-related of
  the host is mounted in the sandbox**, unless you ask for it with
  [`--mount`](./mount).
- **The cost**: what the agent has not pushed is not seen on the host. Its
  instructions tell it to commit and push as it goes.

## The threat model, in plain words

Assume the agent may be wrong, confused or steered by something it read. What
can it reach?

**Not your machine.** The runtime runs it in a micro-VM with its own kernel,
its own disk and its own Docker Engine. Your home, your keys and your other projects
are not there.

**Not your git.** A `.git` directory the agent can write holds commands your
machine would run later: a hook, `core.fsmonitor`, `core.sshCommand`, a
filter, `credential.helper`. Editors, git GUIs and your shell prompt run git
in a checkout all the time. So by default the agent never sees your
checkout: it works on clones of its own, and what returns to you is a
`git fetch` from the forge, which carries objects and refs, never config or
hooks.

**Not your tokens.** Logins and tokens stay in the platform's store on the
host. The sandbox gets placeholders; the runtime's proxy, on the host, puts
the real value in place of the placeholder on requests to the hosts the login
names. An agent that prints its environment prints a placeholder.

**Not every host on the internet.** The runtime's network policy refuses what
it does not list, and a profile allows or denies more.

**What it can reach is the forge**, with the token you gave the sandbox. It
can push to any branch that token may push to, force-push and delete. So the
forge's branch protection is the real boundary: protect `main` and release
branches, and issue the sandbox's token narrow - a GitLab project or group
access token with Developer on the projects concerned, a GitHub fine-grained
token on the repositories it needs.

::: tip
[Security](/reference/security) lists exactly what crosses between the host
and the sandbox.
:::

## Why profiles in git

An environment is code: a Dockerfile, a handful of settings, a few scripts.
Keeping it in a repository of yours means it is reviewed, versioned and
shared like any other code, and a teammate's change arrives as a commit you
can read. unibox reads the working tree, so what is in your editor is what
runs; a change of what a profile asks of the host - environment variables,
caches, logins, services, network rules - is told at the next start, along
with what it leads to.

## Why a wrapper

The runtime does the isolation, and each can be used directly. unibox adds
the setup around it: a profile format that names no runtime, tasks that
span repositories, several agents in one image, logins and services set up
by commands rather than by hand, network rules and ports by name, and a JSON
contract other programs - such as [unagit](./unagit) - can use. Adding a
runtime means implementing one Go interface; profiles do not change.

## Considered and rejected

- **Mounting your worktree read-write with its git configuration locked.**
  Every other branch stays writable, the agent's files land on the host to be
  run by mistake, and the list of what must be locked is only as complete as
  one's knowledge of git.
- **The same with the main clone read-only and a detached HEAD.** Safe, but
  unpleasant to work with: the branch has to be moved after the agent.
- **`sbx run --clone`.** One repository per sandbox, and how its commits come
  back was undocumented.
