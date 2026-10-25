# Developing unibox

unibox is written in Go, MIT-licensed, at
[github.com/bitbyone/unibox](https://github.com/bitbyone/unibox); its module
path is `github.com/tobola/unibox`. The repository's `AGENTS.md` holds the
notes for contributors, people and models alike, and `docs/design.md` the
design decisions and what was verified about sbx. Read the latter before
changing how a sandbox is created or signed in.

## The one hard rule

**Logins are never yours to read.** They live in sbx's store on the host and
reach a sandbox only as placeholders the proxy replaces. Never print a token
or a secret's value, never ask anyone to paste one into a conversation, never
`cat` an auth file - read key names, whether a value is set, or the shape sbx
masks it to. unibox itself never handles a value: a missing login is asked
for by sbx on the user's terminal, and a token never enters a sandbox.

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
| `internal/unibox` | envs and the sbxenv reader, your values, logins, the manager, tasks, mounts, ports, the herdr plugin, and `Runtime` with its one implementation, sbx |
| `internal/unibox/api` | the versioned JSON contract other programs read |
| `internal/unibox/cli` | the cobra commands, `init`'s walk, `doctor`, preflight and its questions |
| `internal/testenv` | the isolated environment every package's tests run in |
| `internal/version` | which unibox this is |

## Decisions worth knowing

- **The team's files are sbx's.** unibox reads an sbxenv file's `name`,
  `agent`, `args`, `kits` and `bindings`, read-only, and ignores every other
  field, so a newer sbx's additions do not break it. It writes no sbxenv
  file, kit or sbx configuration.
- **What is the person's stays out of the team's files**: values in
  `vars.toml`, named ports and sandbox names in `config.yaml`, tokens in
  sbx's store.
- **A change is told, not applied.** A sandbox records a digest of its env's
  files, its local kits and its args; when it differs, unibox says so and
  `sandbox recreate` applies it. Nothing is recreated unasked.
- **The user approves bindings.** The first `sbx env create` runs on the
  user's terminal; unibox never passes `--force` to it.
- **Tokens are stored globally**, as `sbx env rm` deletes a sandbox's own.
- **Agents are prepared before every session**: trust of the task's
  directories in claude's `~/.claude.json`, copilot's `config.json` and
  codex's `config.toml`. What unibox writes into an agent's own file sits
  between markers or is merged, never over the user's settings.
- **Task names are unique across sandboxes**, so a task alone finds its
  sandbox.

## Tests

Every package's `TestMain` goes through `internal/testenv`, and a test checks
that it does: a temporary home, a git configuration of the run's own, and a
`PATH` with git, go and the shell's utilities only.

- A test that needs sbx, herdr or an agent puts a stand-in on its own `PATH`
  with `testenv.WriteExecutable`, and is serial (`t.Setenv`). Every other
  test calls `t.Parallel()`.
- The fake runtime in `fake_test.go` stands in for sbx behind the `Runtime`
  interface, running a sandbox's commands on files in a temporary
  directory.
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
