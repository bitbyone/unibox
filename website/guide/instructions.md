# Agent Instructions

sbx tells an agent about the sandbox: each kit may carry guidance, which
sbx lists in the agent's own instructions file. unibox tells it about the
task: which repositories it has, which of them it only reads, on which
branch each one is, and what the team and you want of the env and of each
repository.

## Writing them

`instructions` is plain text, written as a YAML block. The env's go at the
top of a file, a repository's in its entry of the [index](./repos):

```yaml
# sandboxes/dev.unibox.yaml, the team's
instructions: |
  Run ./gradlew check before you say a change is done.
repos:
  adrs:
    url: https://github.com/acme/adrs.git
    default: always
    instructions: |
      The team's architecture decision records: rules the code of every
      service is to follow, such as working with Kafka, error handling and
      authorization. Before you design or change code in such an area, look
      for a record of it and follow it. Change nothing here unless asked to
      write or amend a record.
```

```yaml
# ~/.config/unibox/config.yaml, yours
instructions: |                  # for every env
  Answer in English, briefly.
envs:
  dev:
    instructions: |              # for dev alone
      I review in IntelliJ; keep imports sorted as it does.
```

The env's instructions are all kept, in this order: the repository's
`unibox.yaml`, `<env>.unibox.yaml`, your top-level `instructions`, your
`envs.<env>.instructions`. A repository's are taken from the last place
that gives them, so yours replace the team's.

## What the agent gets

Before each session unibox writes `/work/<task>/AGENTS.md`, with
`CLAUDE.md` a link to it, and the session starts in `/work/<task>`:

```markdown
<!-- Written by unibox for this task before each session; edits are lost. -->

# Task pricing

Its repositories, each in a directory of its own here:

- `payments/` - work in it, on feat/pricing.
- `spec/` - read it, on the forge's default branch; change and commit nothing there.
- `adrs/` - read it, on main; change and commit nothing there.

## Env dev

Run ./gradlew check before you say a change is done.

Answer in English, briefly.

## adrs

The team's architecture decision records: ...
```

The file lies outside every repository, so it is never committed. It is
written again at each session from the configuration as it is then. A
task of several repositories always gets it; a task of one repository
without instructions does not, and its session starts in the repository.
A repository's own `AGENTS.md` or `CLAUDE.md` still applies inside it.

## What the agent reads

Which files an agent reads, and from where, differs between agents.
`unibox task instructions` lists them in the order the agent reads them,
read from the running sandbox, with unibox's file as the next session
writes it:

```term
$ unibox task instructions pricing --paths
/work/pricing-3f2a9c1e/CLAUDE.md -> AGENTS.md (unibox writes it before each session)
```

```term
$ unibox task instructions pricing
[blue]== /work/pricing-3f2a9c1e/CLAUDE.md -> AGENTS.md (unibox writes it before each session)[/]
<!-- Written by unibox for this task before each session; edits are lost. -->

# Task pricing

Its repositories, each in a directory of its own here:

- `payments/` - work in it, on feat/pricing.
- `spec/` - read it, on the forge's default branch; change and commit nothing there.
- `adrs/` - read it, on main; change and commit nothing there.
[dim]...[/]
```

A file in the agent's home, such as `~/.claude/CLAUDE.md`, or a kit's
guidance the agent reads, is listed first, as the agent reads it first.
`--agent codex` reads for another agent the sandbox has. A sandbox that is
not running is not started; only unibox's file is shown then. The files
are known for claude, codex, copilot and opencode.

## sbx's guidance and Copilot

sbx writes its own guidance on the sandbox, and the kits' list, into the
file it names for the agent. For Copilot that file is `~/AGENTS.md`, which
Copilot does not read - it reads `AGENTS.md` from the git root and the
working directory only. A team using Copilot can copy the kits' section
into Copilot's own file from a kit; see
[For env authors](./env-authors). `unibox task instructions` shows the
result.
