# AGENTS.md

Notes for whoever - person or model - works on unibox next. The README says
what it is for; [docs/design.md](docs/design.md) why it is built the way it
is and what was verified about sbx. Read the latter before changing how a
sandbox is created or signed in.

## The one hard rule

**Logins are never yours to read.** They live in the runtime's store on the
host (sbx keeps them in the keychain) and reach a sandbox only as
placeholders the proxy replaces. Never print a token or a secret's value,
never ask the user to paste one into the conversation, never `cat` an
auth file - read key names (`jq 'keys'`), whether a value is set, or the
shape sbx masks it to. The unibox binary may carry a value the user types
on its way to the platform's store, in memory alone - never argv, a log,
`--json`, state or a profile (see "Configured through unibox" in
docs/design.md) - and a token never enters a sandbox.

## Working here

```sh
make build             # the unibox binary
make test              # go test -race ./...
make test-fast         # while iterating; race before commit
make install           # into ~/.local/bin
gofmt -l . && go vet ./...
```

Everything must be gofmt-clean, vet-clean and race-clean before a commit.
A test that fails one run in a few is a bug to find, not to rerun: the
lock tests did, until FileLock.Close unlocked before closing (a forked
child holds a copy of the descriptor, and with it the flock).

Commit and push every finished change without waiting to be asked. Never
force-push. Commit messages are a sentence in the imperative, then a
paragraph about *why*, not a list of touched files.

### Style

Plain English comments that explain the reason, not the mechanics. No
emoji, no exclamation marks. Errors are lower case and say what to do
about it. Keep command output short: pipe through `head`, `tail` or
`grep`, and read large files by range.

## The map

| Package | What it owns |
| --- | --- |
| `cmd/unibox` | the entry point |
| `internal/unibox` | profiles, the manager, tasks, runtimes, the sbx adapter and its kit |
| `internal/unibox/api` | the versioned JSON contract other programs read |
| `internal/unibox/cli` | the cobra commands, preflight and its questions |
| `internal/unibox/templates` | every file unibox puts into an environments repository, as files: `managed/` mirrors `.shared`, which unibox owns and keeps current, `user/` the examples init writes once into `_shared`, `profile/` what init writes once into a new profile, `seeds/` the agents' configuration seeds |
| `internal/testenv` | the isolated environment every package's tests run in |

## Decisions worth knowing

- **A profile has a format version.** Changing what sandbox.yaml may say
  in a way an existing profile no longer reads bumps `ProfileFormat` and
  adds a step to `migrations` from the previous format, best effort. A
  profile is migrated only by `unibox init <profile>`, never on load.
- **A profile knows no runtime.** It says what the environment is, how
  each agent signs in and what it may reach; an adapter (`sbx.go`,
  `sbx_kit.go`, `sbx_auth.go`) turns that into the runtime's own means.
  Nothing above the adapter names sbx.
- **A profile change is told, not approved.** Environment, caches,
  network rules, `auth` and `services` are `HostAccess`; a change of them
  is said at the next start, with what it leads to (a build, a
  recreation naming why, rules applied live). A new field of it is
  `omitempty` in JSON, or every profile reads as changed.
- **Each adapter says what it can change live**, by an optional
  interface (`NetworkUpdater`, `Authenticator`, `SessionEnver`); what it
  cannot is part of the settings a recreation makes.
- **Several agents mean a generated v3 kit.** sbx injects a kit's
  credentials only with a binding in `~/.config/sbx/credentials.yaml`,
  which unibox writes; it wires one OAuth login per sandbox, from the
  first credential declaring one, so the order of declarations matters;
  and a host an agent calls that the kit does not declare gets the
  placeholder as it is. docs/design.md has the evidence for each.
- **Agents are prepared before every session**: codex's login and trust
  in its config.toml, claude's trust in `~/.claude.json`, copilot's
  through `COPILOT_ALLOW_ALL`. What unibox writes into an agent's own file
  sits between markers or is merged, never over the user's settings.
- **Task names are unique across sandboxes**, so a task alone finds its
  sandbox (`unibox resume <task>`).

## Tests

Every package's `TestMain` goes through `internal/testenv`
(`TestEveryPackageRunsInTheTestEnvironment` checks it): a temporary home,
a git configuration of the run's own, and a PATH with git, go and the
shell's utilities only. A test that needs sbx, herdr or an agent puts a
stand-in on its own PATH with `testenv.WriteExecutable` and is serial
(`t.Setenv`); every other test calls `t.Parallel()`. The fake runtime in
`unibox_test.go` runs a sandbox's commands in a temporary directory.
Check the real sbx on a throwaway sandbox, and remove it afterwards.
