# unagit

[unagit](https://bitbyone.github.io/unagit/) - the Universal Navigator Around
GIT - is a sibling project: one place to see every repository you work with,
and merge requests reviewed as pending edits. It is meant to become a user
interface over unibox.

::: info Status
The integration is **planned**. unibox is built and usable on its own from a
terminal; unagit's side - its Settings, a Sandboxes tab, Open in Sandboxed
Agent - is unagit's to build, and not there yet.
:::

## Two programs, one boundary

- **They share no code.** unibox imports nothing of unagit's and unagit
  nothing of unibox's. unagit runs the `unibox` binary and reads its `--json`,
  whose objects each carry an `api_version`.
- **unagit drives unibox through its command line**, as it drives herdr or
  Zellij: `--json` for what it reads, `--yes` where a question would come.
- **unagit configures unibox only through its commands** - the environments
  repository with `unibox config set environments <path>`, a profile's logins,
  services and network rules with `unibox agent`, `service`, `network` and
  `secret`, an older profile with `unibox init <profile>`. unagit writes no
  YAML.

Two versions are kept apart: `api_version` is the JSON contract's, and
changes only when a field a reader relies on changes meaning or goes; the
profile format is the YAML's, which unagit never reads.

## Who does what

| unibox | unagit |
| --- | --- |
| its own configuration, state and cache | unibox on or off, among its integrations |
| the environments repository: profiles, their format and migration, builds | which worktree, which repositories, which branch |
| the runtimes | pushing the branch, with its own token |
| sandboxes and tasks: create, start, stop, remove | clone URLs from the forge's API |
| a task's preparation: clones, author, co-author hook, instructions, seeds, `start` | pickers, a Sandboxes tab, Activity |
| the tokens a profile names, in the platform's store | its own token vault |
| running the agent, stopping an idle sandbox | the questions: build, sign in, migrate, commit first |
| its herdr plugin | |

## A start, as planned

1. unagit fills its pickers - agent, profile, sandbox, place - from
   `unibox profiles --json` and `unibox sandboxes --json`.
2. `unibox check --json` answers with what is needed, each with a code
   unagit turns into a dialog of its own: build, sign in, commit first.
3. unagit pushes the branch from the host, with its own credentials.
4. `unibox prepare --json` with the repositories' https clone URLs from the
   forge's API, never from a local remote that might carry a token.
5. The place chosen - a terminal, Zellij, Ghostty or herdr - runs
   `unibox resume <task>`, the task `prepare` made.

unagit's tokens never go into a sandbox: the push before a start is unagit's,
from the host, and everything after it is the sandbox's own.
