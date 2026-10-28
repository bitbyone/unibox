# Usage

A day with unibox, once `unibox init` has set up the team's env. The
examples follow one person, `jdoe`, at a company whose sandboxes are
described in `https://github.com/acme/agent-sandboxes`: one env, `dev`,
whose kit runs Claude Code.

## Work on a checkout

```sh
cd ~/code/api
unibox run
```

`run` mounts the current checkout into the env's sandbox, clones beside it
the repositories the team says every task takes, and starts the env's
agent in your terminal. Nothing is asked.

```term ~/code/api
$ unibox run
preparing task api-4c1d2e9a
mounted /Users/jdoe/code/api at /work/api-4c1d2e9a/api in sandbox dev
[dim]note: the agent can write to /Users/jdoe/code/api, its .git included; a hook or setting it leaves there runs on this host with your next git command in it. Look at what it changed, or use --clone to keep it out of the checkout.[/]
Cloning into '/work/api-4c1d2e9a/adrs.unibox-clone'...

[dim]... claude runs here until you leave it ...[/]

sandbox dev stopped: no active sessions
```

The agent edits the checkout directly; your editor sees each change. When
the last session in the sandbox ends, unibox stops it; the disk and the
task stay.

## Pick on one screen

```sh
unibox task new        # repositories of the env's index, cloned
unibox task new .      # the same screen, with this checkout mounted among them
```

```term ~/code/api
$ unibox task new .
[blue]New task in env dev[/]                                          task [b]api-b6a2f036[/] [dim](generated)[/]

  branch  [dim]none: each its ref, its checkout's branch or the forge's default[/]
  task    [dim]generated from its repositories and branches[/]

     REPO      ROLE     FROM   BRANCH               PUSH  ABOUT
[blue]>[/] [ok]✓[/]  [b]api       work     mount  fix/session-timeout        your checkout, mounted from the host: ~/code/api[/]
  [ok]✓[/]  adrs      [dim]context[/]  clone  main                       always · Architecture decisions
  [ok]✓[/]  spec      [dim]context[/]  clone  [dim]default[/]                    Product specification
  [dim]•[/]  payments  work     clone  [dim]default[/]                    Payments service
  [dim]•[/]  web       work     clone  [dim]default[/]                    The web front end

[dim]3 picked[/]
[key]↑/↓ j/k[/] move · [key]space[/] pick · [key]c[/] work/context · [key]p[/] push · [key]b[/] branch · [key]t[/] task name · [key]/[/] filter · [key]enter[/] start · [key]q[/] cancel
```

[Repository index](./repos) explains the roles and defaults.

## Come back

```sh
unibox                  # the sandboxes; enter shows a sandbox's tasks
unibox task             # the tasks: enter resumes one
unibox resume api-4c1d2e9a
```

```term
$ unibox task
[blue]Tasks[/]

  TASK                        SANDBOX  AGENT   STATE  SESSIONS  REPOSITORIES
[blue]>[/] [b]api-4c1d2e9a                dev      claude  ready  0         api@fix/session-timeout, adrs@main[/]
  pricing (pricing-3f2a9c1e)  dev      codex   ready  1         payments@feat/pricing, spec@default, adrs@main

[key]↑/↓ j/k[/] move · [key]enter[/] resume · [key]t[/] new task · [key]a[/] attach agent · [key]r[/] add repos · [key]s[/] shell · [key]n[/] rename · [key]d[/] remove · [key]q[/] quit
```

## Check the setup

```sh
unibox doctor           # what is missing or out of date, and the command that fixes it
unibox doctor -i        # and offer to run each fix
```

## Other commands of a day

```sh
unibox port             # switch the sandbox's ports on and off
unibox network          # what the sandbox was refused, and your rules
unibox repo             # the repositories a task can take
unibox task instructions <task>   # what the agent reads when a session starts
unibox env update       # pull the team's repository and see what changed
```

Without a terminal, or with `--json`, every list prints as its `ls` does,
so scripts get the same information.

## Terms

| Term | What it is |
| --- | --- |
| env | a sandbox as one or more sbxenv files of the team's repository describe it, under a name such as `dev` |
| kit | what an env pulls in: an image, tools, network rules, the tokens it needs. sbx's format, the team's to write |
| sandbox | a micro-VM sbx creates from an env. It keeps its disk until removed; unibox stops it when its last session ends |
| task | a piece of work in a sandbox: repositories at `/work/<task>/<repo>`, a branch, the agent that works there. Several tasks share a sandbox |
| agent | claude, codex, copilot or opencode, as the env's kit installs it |
