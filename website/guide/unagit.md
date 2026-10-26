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
- **unagit configures unibox only through its commands** - an env with
  `unibox env add`, a person's values with `unibox var set`, secrets with
  `unibox secret set`, a running sandbox's ports with `unibox port on`. unagit writes no YAML, and
  reads the team's sbxenv files only through `unibox env show --json`.

`api_version` versions the JSON contract, and changes only when a field a
reader relies on changes meaning or goes.

## Who does what

| unibox | unagit |
| --- | --- |
| its own configuration, state and cache | unibox on or off, among its integrations |
| envs, a person's values for their args, named ports | which worktree, which repositories, which branch |
| sandboxes and tasks, through sbx: create, start, stop, remove | pushing the branch, with its own token |
| a task's preparation: mounts or clones, author, co-author hook, the agent's trust | clone URLs from the forge's API |
| which secrets an env needs; sbx keeps them | pickers, a Sandboxes tab, Activity |
| running the agent, stopping an idle sandbox | the questions: set an arg, sign in, commit first, recreate |
| its herdr plugin | its own token vault |

## A start, as planned

1. unagit fills its pickers - env, sandbox, place - from
   `unibox env ls --json` and `unibox sandbox ls --json`, and asks
   `unibox secret ls --json` whether sbx has the env's tokens.
2. `unibox task check --json` answers with what is needed, each with a code
   unagit turns into a dialog of its own: set an arg, commit first, recreate.
3. For a clone, unagit pushes the branch from the host, with its own credentials.
4. `unibox task prepare --json`, with the worktree's directories to mount, or
   `--clone` and the repositories' https clone URLs from the forge's API -
   never from a local remote that might carry a token.
5. The place chosen - a terminal, Zellij, Ghostty or herdr - runs
   `unibox resume <task>`, the task `task prepare` made.

unagit's tokens never go into a sandbox: the push before a start is unagit's,
from the host, and everything after it is the sandbox's own, signed in by
sbx.
