# Developing unibox

unibox is written in Go, MIT-licensed, at
[github.com/bitbyone/unibox](https://github.com/bitbyone/unibox); its module
path is `github.com/tobola/unibox`. The repository's `AGENTS.md` holds the
notes for contributors, people and models alike, and `docs/design.md` the
design decisions and what was verified about sbx. Read the latter before
changing how a sandbox is created or signed in.

## The one hard rule

**Logins are never yours to read.** They live in the runtime's store on the
host and reach a sandbox only as placeholders the proxy replaces. Never print
a token or a secret's value, never ask anyone to paste one into a
conversation, never `cat` an auth file - read key names, whether a value is
set, or the shape sbx masks it to. unibox itself never handles a value it
does not have to: a missing login is taken by the runtime on the user's
terminal.

## Building and testing

```sh
make build             # the unibox binary
make test              # go test -race ./...
make test-fast         # while iterating; race before commit
make install           # into ~/.local/bin
gofmt -l . && go vet ./...
```

Everything must be gofmt-clean, vet-clean and race-clean before a commit. It
needs Go 1.26 or newer.

## The map

| Package | What it owns |
| --- | --- |
| `cmd/unibox` | the entry point |
| `internal/unibox` | profiles, the manager, tasks, runtimes, the sbx adapter and its kit |
| `internal/unibox/api` | the versioned JSON contract other programs read |
| `internal/unibox/cli` | the cobra commands, preflight and its questions |
| `internal/unibox/templates` | what `unibox init` lays out and keeps current: `managed/` (`.shared`), `user/` (`_shared`'s examples), `profile/`, `seeds/<agent>/<file>`, and the `@install` runner |
| `internal/testenv` | the isolated environment every package's tests run in |
| `internal/version` | which unibox this is |

## Decisions worth knowing

- **A profile knows no runtime.** It says what the environment is, how each
  agent signs in and what it may reach; an adapter (`sbx.go`, `sbx_kit.go`,
  `sbx_auth.go`) turns that into the runtime's own means. Nothing above the
  adapter names sbx.
- **Host access is tracked.** Environment, caches, network rules, services and
  `auth` are `HostAccess`; a change of them is told at the next start. A new
  field of it is `omitempty` in JSON, or every recorded profile reads as
  changed.
- **Several agents mean a generated v3 kit**, with its bindings written into
  sbx's `credentials.yaml`, one OAuth login per sandbox from the first
  credential declaring one, so the order of declarations matters.
- **Agents are prepared before every session**: codex's login and trust in
  its `config.toml`, claude's trust in `~/.claude.json`, copilot's through
  `COPILOT_ALLOW_ALL`. What unibox writes into an agent's own file sits
  between markers or is merged, never over the user's settings.
- **Task names are unique across sandboxes**, so a task alone finds its
  sandbox.
- **Unknown profile fields are errors**, and a released migration is frozen.

## Tests

Every package's `TestMain` goes through `internal/testenv`, and a test checks
that it does: a temporary home, a git configuration of the run's own, and a
`PATH` with git, go and the shell's utilities only.

- A test that needs sbx, herdr or an agent puts a stand-in on its own `PATH`
  with `testenv.WriteExecutable`, and is serial (`t.Setenv`). Every other
  test calls `t.Parallel()`.
- The fake runtime in `unibox_test.go` runs a sandbox's commands in a
  temporary directory.
- No token appears in any argument list or environment unibox builds; tests
  hold it.
- Check against the real sbx on a throwaway sandbox, and remove it
  afterwards.

## Style

Plain English comments that explain the reason, not the mechanics. Errors are
lower case and say what to do about it. Commit messages are a sentence in the
imperative, then a paragraph about why, not a list of touched files.

## Releasing

A release is a `vX.Y` tag on a commit of `main` whose tests have passed:

1. Push the commit to `main`; the test workflow runs on it.
2. Tag it `vX.Y` and push the tag.
3. The release workflow waits for that commit's tests to pass, checks the tag
   is on `main`, builds the binary and checks that `unibox --version` prints
   `X.Y`.
4. It then points the Homebrew formula in the repository, `Formula/unibox.rb`,
   at the tag and its commit, and pushes that to `main`, where `brew upgrade`
   finds it.

Homebrew builds from source; nothing compiled is published. The tag never
moves: a failed test run is re-run, then the release.

`make build` sets the version from a release tag on `HEAD`; anywhere else the
binary calls itself `dev (<commit>)`.

## The website

This site lives in `website/` and is built with
[VitePress](https://vitepress.dev):

```sh
cd website
npm install
npm run dev        # a local server with live reload
npm run build      # the static site, into .vitepress/dist
```

A push to `main` that touches `website/` publishes it to GitHub Pages.
