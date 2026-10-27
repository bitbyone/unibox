# Interactive Lists

Every unibox noun run alone at a terminal is a list of what it names,
acted on by a key. A key runs the same command its subcommand would, so a
list and the command line never differ in what they do.

| Command | Lists | Keys |
| --- | --- | --- |
| `unibox`, `unibox sandbox` | sandboxes | `enter` its tasks, `s` shell, `p` ports, `i` inspect, `r` resources, `t` stop, `c` recreate, `d` remove |
| `unibox task`, `unibox resume` | tasks | `enter` resume, `t` new task, `a` attach another agent, `r` add repositories, `s` shell, `n` rename, `d` remove |
| `unibox task new` | the index's repositories, for a new task | `space` pick, `c` work/context, `p` push, `b` branch, `t` task name, `enter` start |
| `unibox repo` | the env's repository index | `n` new task with it, `space` off/on/always, `a` add, `d` remove yours |
| `unibox port` | a running sandbox's ports | `h`/`l` off/on, `space` switch, `m` on another host port, `d` on or off at start |
| `unibox network` | a sandbox's network rules | `a` allow, `n` deny, `d` remove, `x` write them for a kit |
| `unibox secret` | the tokens an env needs | `enter` store, `g` generate a new token, `c` store a command for it |
| `unibox env` | envs | `enter` show, `space` make default, `u` update, `c` create a sandbox, `d` forget |
| `unibox var` | your values for an env | `enter` set, `a` add, `d` remove |
| `unibox context` | what commands work in when not told | `enter` use, `c` clear |

In every list `↑`/`↓` or `j`/`k` move and `q` or `esc` leaves; a removal
asks for `y` first. The lists of `repo`, `env`, `var`, `secret`, `network`
and `context`, and the `task new` screen, filter with `/`.

```term
$ unibox
[blue]Sandboxes[/]

  SANDBOX  ENV  AGENT   STATE    TASKS  SESSIONS
[blue]>[/] [b]dev      dev  claude  running  2      1[/]
  dev-big  dev  claude  stopped  0      0

[key]↑/↓ j/k[/] move · [key]enter[/] tasks · [key]s[/] shell · [key]p[/] ports · [key]i[/] inspect · [key]r[/] resources · [key]t[/] stop · [key]c[/] recreate · [key]d[/] remove · [key]q[/] quit
```

## Filtering

`/` narrows a list as you type; the letters need only appear in order, so
`pay` finds `payments`. `enter` keeps the filter and returns to the keys,
`esc` clears it.

```term
$ unibox repo
[blue]repositories of env dev[/]

[blue]>[/] /pay_  [dim]1 of 5[/]

  REPO      ROLE  REF  DEFAULT  FROM       ABOUT
[blue]>[/] [b]payments  work  -    off      the env's  Payments service[/]

[key]↑/↓[/] move · [key]enter[/] keep the filter · [key]esc[/] clear it
```

## When a key needs the terminal

Resuming a task, opening a shell or asking a further question ends the
list and runs the command as typed on the command line. Short actions -
switching a port, making an env the default, removing a rule - run inside
the list, which is drawn again with the result.

## Without a terminal

Without a terminal, or with `--json`, a noun alone prints what its `ls`
prints, so scripts and pipes get the same information:

```term
$ unibox secret | cat
SERVICE    STORED  NEEDED    FOR
anthropic  global  yes       the Claude API key
github     no      yes       the token git pushes with
npm        no      optional  the registry's token
```

With `ACCESSIBLE=1` the screens become numbered questions, for screen
readers.
