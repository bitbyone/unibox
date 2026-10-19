# The Environments Repository

A git repository of yours holding the profiles. It is edited and versioned as
any repository is - in your editor, committed, pushed, shared with a team.
unibox never commits to it; it is yours to commit.

## Lay it out

```sh
unibox init --dir ~/my-envs      # lay it out here and configure it
unibox init -i                   # ask where (here by default) when nothing is configured
```

With nothing configured, `unibox sandboxes` and `unibox profiles` say so and how to
start, and `init` without `--dir` refuses rather than guess. Once configured,
`init` names the repository it writes into.
`unibox config set environments <path>` points unibox at an existing one; `unibox config unset environments`
forgets it and leaves the files.

## The layout

```
my-envs/
├── sandboxes/
│   └── web/
│       ├── sandbox.yaml         profile definition
│       ├── sandbox.dockerfile   the image (or image: in sandbox.yaml)
│       ├── start.sh             runs in the VM at every boot (optional)
│       ├── instructions.md      extra instructions for the agents (optional)
│       ├── _data/               files the profile refers to (optional)
│       └── .sbx/
│           └── build.args       build arguments for the sbx runtime
├── _shared/                     yours
│   ├── @install/custom.sh       installation scripts, named in a Dockerfile
│   └── @instructions/custom.md  instruction texts, named in a profile
├── .shared/                     unibox's, kept at its own version
│   ├── @install/                base, agents/claude, tools/nvm, ...
│   └── @instructions/unibox.md
├── .templates/                  templates' shared files (unibox init --from)
│   └── <template>/              @install, @instructions, source.yaml
└── README.md
```

A directory of `sandboxes/` with a `sandbox.yaml` is a profile, named after
the directory, and everything inside it is the profile's own. Every directory
there must carry a `sandbox.yaml`.

A profile's root holds only its definition, `sandbox.dockerfile`, its start
script, its scripts and `instructions.md`. Custom files - an agent's
configuration seed, say - go under an underscore directory of the profile's
own, `_data` by convention. Runtime directories inside a profile are hidden:
`.sbx/build.args`.

Anything else in the repository is yours to arrange, and a profile names such
a file by its path from `sandbox.yaml` - `../../notes/style.md`.

## The `@` directories

`@install` and `@instructions` are what a profile names things from:

- a Dockerfile runs **installation scripts** by name:
  `RUN @install base tools/nvm agents/claude custom` runs
  `@install/base.sh`, `@install/tools/nvm.sh`, and so on;
- a profile's `instructions` names **instruction texts**: `unibox` is
  `@instructions/unibox.md`.

Both exist twice: in `_shared/`, which is yours, and in `.shared/`, which is
unibox's. **Yours win by name.** A profile copied from a
[template](./templates) also finds its template's, in `.templates/<template>/`,
between the two. Overriding one of unibox's scripts or texts
is putting one of the same name into `_shared`, never editing `.shared`.

The texts of `@instructions` are `.md` files, and none may be named
`instructions.md`.

## What unibox manages

`.shared/` is unibox's alone. `init`, `check`, `build` and a task's start -
each once per command, before a profile is read - rewrite it to this unibox's
version where it differs, take out anything else in it, and say what they
changed.

::: warning
An edit in `.shared/` is lost the next time unibox runs. Put your own scripts
and texts in `_shared/`.
:::

What unibox ships today:

| `.shared/` | What it does |
| --- | --- |
| `@install/base.sh` | the images' common base: a place for apt packages, certificates, toolchains |
| `@install/tools/nvm.sh` | nvm, with no Node version chosen; the agents' scripts fall back to the current LTS |
| `@install/tools/mise.sh` | mise, the polyglot tool version manager |
| `@install/tools/sdkman.sh` | SDKMAN, the JVM toolchain manager |
| `@install/agents/claude.sh` | Claude Code, from npm |
| `@install/agents/codex.sh` | Codex |
| `@install/agents/copilot.sh` | GitHub Copilot CLI |
| `@install/agents/opencode.sh` | opencode |
| `@instructions/unibox.md` | how an agent is meant to work in a sandbox: its branch, commit and push as it goes, never force |

`init` writes `_shared/` once, with an example of each kind
(`@install/custom.sh`, `@instructions/custom.md`). From then on it is yours,
as is everything a profile holds and anything else in the repository.

A repository `init` did not lay out (no `.shared/`) is not written into.

## What makes an image outdated

A file of `.shared` or `_shared` - but not the instruction texts - and the
`@install` runner itself are part of every profile's revision: a change there
marks every image outdated, including an update of a script unibox manages.
A profile's `sandbox.yaml`, its `instructions.md` and its agents'
configuration seeds are not image inputs, and changing them rebuilds nothing.
See [Images](./images).

## Checking it

```sh
unibox validate
```

checks the layout, every profile and every instruction text, and names what
is wrong.

## An older layout

A repository of an older layout (`_shared/.install`, `_shared/installations`,
`instructions/`) is refused until `unibox init` moves it: a script or text
that is unibox's as it was is dropped, one that differs moves to `_shared`
where it wins, and a Dockerfile's generated `RUN` line is rewritten.
