# Interactive Lists

Run a unibox command without arguments at a terminal and it opens a list
of what it names. Each list hints its keys under it. A key runs the same command you would type, so nothing differs
between the list and the command line.

```term
$ unibox
[blue]Sandboxes[/]

  SANDBOX  ENV  AGENT   STATE    TASKS  SESSIONS
[blue]>[/] [b]dev      dev  claude  running  2      1[/]
  dev-big  dev  claude  stopped  0      0

[key]↑/↓ j/k[/] move · [key]enter[/] tasks · [key]s[/] shell · [key]p[/] ports · [key]i[/] inspect · [key]r[/] resources · [key]t[/] stop · [key]c[/] recreate · [key]d[/] remove · [key]q[/] quit
```

Lists open from `unibox` (the sandboxes), `task`, `task new`, `repo`,
`port`, `network`, `secret`, `env`, `var` and `context`. Most filter with
`/`.

## Without a terminal

In a pipe, a script or with `--json`, the same command prints what its
`ls` prints:

```term
$ unibox secret | cat
SERVICE    STORED  NEEDED    FOR
anthropic  global  yes       the Claude API key
github     no      yes       the token git pushes with
```
