# Developing unibox

Go, MIT-licensed, at [github.com/bitbyone/unibox](https://github.com/bitbyone/unibox);
module path `github.com/tobola/unibox`. `AGENTS.md` has the contributor
notes and `docs/design.md` the design and what was verified about sbx -
read it before changing how a sandbox is created or signed in.

## The one hard rule

**Never read a secret.** Never print a token, ask anyone to paste one, or
`cat` an auth file; read key names, whether a value is set, or sbx's
masked form. unibox never handles a value: sbx asks for it on the user's
terminal, and no token enters a sandbox.

## Build and test

```sh
make build             # the unibox binary
make test              # go test -race ./...
make test-fast         # while iterating; race before commit
make install           # into ~/.local/bin
make dist VERSION=1.2  # release archives for every system, into dist/
gofmt -l . && go vet ./...
```

Needs Go 1.26 or newer. Everything must be gofmt-, vet- and race-clean
before a commit. Builds without cgo for Linux, macOS and Windows, amd64
and arm64; `GOOS=windows go vet ./...` checks Windows from any machine. CI
runs the suite on Linux (with race) and macOS, and builds, vets and runs
the shell-free tests on Windows.

## Layout

| Package | Owns |
| --- | --- |
| `cmd/unibox` | entry point |
| `internal/unibox` | envs and the sbxenv reader, values, secrets, the manager, tasks, mounts, ports and team ports (`teamports.go`), network policy (`policy.go`), finding sbxenv files (`discover.go`), cloning (`clone.go`), file locks, herdr, `Runtime` and its sbx implementation |
| `internal/unibox/api` | the versioned JSON contract |
| `internal/unibox/cli` | cobra commands, `init`, `doctor`, preflight, the list UIs |
| `internal/testenv` | the isolated environment every package's tests run in |
| `internal/version` | the release, set from the tag at build time |

## Decisions

- **The team's files are sbx's.** unibox reads `name`, `agent`, `args`,
  `kits` and `bindings`, ignores the rest, and writes no sbxenv file, kit
  or sbx configuration.
- **What is personal stays out of the team's files**: values in
  `vars.toml`, ports and sandbox names in `config.yaml`, tokens in sbx.
- **Team ports live in `<env>.unibox.yaml` and `unibox.yaml`**, which sbx
  never reads; a person's `config.yaml` wins.
- **`env update` fast-forwards** what init cloned, and a person's own
  checkout only when clean and tracking a remote branch.
- **Hints name unibox commands**, never sbx's.
- **Changes are told, not applied.** A sandbox records a digest of its
  env's files, local kits and args; `sandbox recreate` applies a change.
- **The user approves bindings**; unibox never passes `--force` to
  `sbx env create`.
- **Tokens are stored globally.**
- **Agents are prepared before each session** by merging trust into their
  own files, never over the user's settings.
- **Task names are unique across sandboxes.**

## Tests

Every package's `TestMain` goes through `internal/testenv` (a test checks
it): a temporary home, its own git config, and a `PATH` with git, go and
shell utilities only. A test needing sbx, herdr or an agent puts a
stand-in on its `PATH` with `testenv.WriteExecutable` and is serial
(`t.Setenv`); every other test calls `t.Parallel()`. The fake runtime in
`fake_test.go` stands in for sbx. No token appears in any argument list
or environment unibox builds. Check against real sbx on a throwaway
sandbox, and remove it afterwards.

## Style

Comments explain why, in plain English. Errors are lower case and say
what to do. Commit messages: an imperative sentence, then a paragraph on
why.

## Releasing

Push a `vX.Y` tag on a commit of `main` whose tests passed. The release
workflow checks the tag is on `main`, builds, checks `unibox --version`
prints `X.Y`, publishes archives and checksums from `make dist` as the
GitHub release, then points `Formula/unibox.rb` at the tag and commit and
writes `bucket/unibox.json` for the Windows archives, and pushes both to
`main`. Never edit the formula or manifest by hand; never move a tag.
`make build` takes the version from a release tag on `HEAD`; otherwise
the binary is `dev (<commit>)`.

## The website

```sh
cd website
npm install
npm run dev        # live-reload server
npm run build      # into .vitepress/dist
```

A push to `main` that touches `website/` publishes it to GitHub Pages.
