# Repository Index

sbx knows the workspace a sandbox is created with. unibox keeps, for each
env, an index of the repositories its tasks take by name: the team's, in
the env's repository, and each person's own. A name from the index works
wherever a repository does.

```sh
unibox task new payments --branch feat/pricing
unibox run --repo . --repo adrs
unibox task add-repo login web
```

## The team's

The team writes the index under `repos:` in `unibox.yaml` at the root of
the env's repository (every env) or in `<env>.unibox.yaml` beside the
env's sbxenv file (that env alone):

```yaml
# sandboxes/dev.unibox.yaml
repos:
  adrs:
    url: https://github.com/acme/adrs.git
    ref: main
    default: always
    description: Architecture decisions
  spec:
    url: https://github.com/acme/spec.git
    default: on
    context: true
    description: Product specification
  payments: https://github.com/acme/payments.git
  web: {url: https://github.com/acme/web.git, description: The web front end}
```

| Key | Meaning |
| --- | --- |
| `url` | a forge address, cloned in the sandbox; or a directory of the machine (`/…`, `~/…`), mounted |
| `ref` | the branch a task works on; unset, the forge's default branch |
| `default` | `always`: every new task takes it, also `unibox run`; `on`: picked to begin with in `task new`, and by `run --defaults`; `off` (the default): only when named or picked |
| `context` | `true`: the task reads it - it stays on its `ref`, gets no task branch and is never pushed to. Unset, true for `always` and false otherwise |
| `description` | shown in the lists |
| `instructions` | what the agent is told of it; see [Agent instructions](./instructions) |

sbx does not read these files, so a teammate without unibox is not
affected by them.

## Yours

Your own entries go in `~/.config/unibox/config.yaml`, for every env or for
one, and win over the team's. The commands write them:

```sh
unibox repo add notes https://github.com/jdoe/notes.git --default on --every-env
unibox repo add api ~/code/api                 # a directory of yours: mounted
unibox repo default spec off                   # your default over the team's
unibox repo default spec -                     # the team's again
unibox repo rm notes
```

```term
$ unibox repo
[blue]repositories of env dev[/]

  REPO      ROLE     REF   DEFAULT     FROM              ABOUT
[blue]>[/] [b]adrs      context  main  always      the env's         Architecture decisions[/]
  api       work     -     [dim]off[/]         yours             ~/code/api
  notes     work     -     on          yours, every env  https://github.com/jdoe/notes.git
  payments  work     -     [dim]off[/]         the env's         Payments service
  spec      [dim]context[/]  -     [dim]off (yours)[/] the env's         Product specification
  web       work     -     [dim]off[/]         the env's         The web front end

[key]↑/↓ j/k[/] move · [key]/[/] filter · [key]n[/] new task with it · [key]space[/] off/on/always · [key]a[/] add · [key]d[/] remove yours · [key]q[/] quit
```

## task new

`unibox task new` offers the index on one screen. Each row shows where the
repository comes from (`clone` or `mount`), its role, the branch it will
get - `default` for the forge's default branch - and whether that branch
is pushed. The task's name stands at the top as it will be.

```term
$ unibox task new
[blue]New task in env dev[/]                                          task [b]payments-40d7f34f[/] [dim](generated)[/]

  branch  [b]feat/pricing[/]
  task    [dim]generated from its repositories and branches[/]

     REPO      ROLE     FROM   BRANCH        PUSH  ABOUT
  [ok]✓[/]  adrs      [dim]context[/]  clone  main                always · Architecture decisions
  [dim]•[/]  api       work     mount  feat/pricing  [dim]no[/]    ~/code/api
  [ok]✓[/]  payments  work     clone  feat/pricing  [ok]yes[/]   Payments service
[blue]>[/] [ok]✓[/]  [b]spec      context  clone  default             Product specification[/]
  [dim]•[/]  web       work     clone  feat/pricing  [ok]yes[/]   The web front end

[dim]3 picked · branch feat/pricing in 1, pushed from 1[/]
[key]↑/↓ j/k[/] move · [key]space[/] pick · [key]c[/] work/context · [key]p[/] push · [key]b[/] branch · [key]t[/] task name · [key]/[/] filter · [key]enter[/] start · [key]q[/] cancel
```

- The branch typed applies to the repositories worked in. A pushed branch
  that is missing is made on the forge from its default branch; one not
  pushed is made in the sandbox's clone, or your checkout, alone.
- A context repository keeps its `ref` whatever the branch.
- A generated task name follows the first repository you picked by hand
  and work in, so two tasks on the same decisions stay apart.
- Nothing is asked of a forge while you pick. As the task starts, unibox
  looks once, with git, whether the branch exists already in a repository
  worked in, and asks whether to work on it there.

`unibox task new .` adds the current checkout as the first row, mounted
and picked, on its own branch until a branch is typed. A checkout of a
repository the index clones takes that row's place and name.

Named, without the screen, `task new` takes those named and the `always`
ones, with `--branch`, `--task` and `--no-push`.

## In run

`unibox run` takes the `always` repositories beside what it mounts or
clones. `--defaults` adds the `on` ones; `--only` takes nothing of the
index but what is named.

| | `always` | `on` | `off` |
| --- | --- | --- | --- |
| `unibox run` | yes | | |
| `unibox run --defaults` | yes | yes | |
| `unibox run --only` | | | |
| `unibox task new` (screen) | picked | picked | offered |
