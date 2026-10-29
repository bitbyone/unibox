# Interactive Lists

Run a unibox command without arguments at a terminal and it opens a list
of what it names. Each list hints its keys under it. A key runs the same command you would type, so nothing differs
between the list and the command line.

```term
$ unibox
[blue]Sandboxes[/]                                    [dim]context sandbox[/] [b]dev[/] [dim]· env[/] [b]dev[/]

  SANDBOX  ENV  AGENT   STATE    TASKS  SESSIONS
[blue]>[/] [b]dev      dev  claude  running  2      1[/]
  dev-big  dev  claude  stopped  0      0

[key]↑/↓ j/k[/] move · [key]enter[/] tasks · [key]s[/] shell · [key]p[/] ports · [key]i[/] inspect · [key]r[/] resources · [key]t[/] stop · [key]c[/] recreate · [key]d[/] remove · [key]q[/] quit
```

Lists open from `unibox` (the sandboxes), `task`, `task new`, `repo`,
`port`, `network`, `secret`, `env`, `var` and `context`. Most filter with
`/`.

A list takes a part of the terminal below your prompt, not all of it: at
most 20 lines of rows, fewer when the terminal is short, never fewer than
3. Rows beyond scroll with the cursor: `↑ 9 more` stands above the rows
shown when some lie above, `↓ 25 more · rows 10-15 of 40` below them when
some lie below.

Every list says in its top right corner the sandbox and env commands work
in when not told - the context `unibox context use` sets, `UNIBOX_SANDBOX`,
or the only one - also in a list that does not depend on it, such as
`unibox secret --all`. Keys that do not fit
the terminal's width wrap onto the next line.

## Without a terminal

In a pipe, a script or with `--json`, the same command prints what its
`ls` prints:

```term
$ unibox secret | cat
SERVICE    STORED  NEEDED    FOR
anthropic  global  yes       the Claude API key
github     no      yes       the token git pushes with
```
