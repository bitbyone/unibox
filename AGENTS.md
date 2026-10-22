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

CI (`.github/workflows`): `test` runs on every push and pull request, the
race detector on Linux and the suite without it on macOS; `website`
builds `website/` with VitePress and publishes it to GitHub Pages; a
`vX.Y` tag on a commit of main whose tests passed releases it - `release`
builds it, checks `unibox --version` prints the tag's version, and points `Formula/unibox.rb`, the
Homebrew tap this repository is, at the tag and its commit. Release by
pushing such a tag; never edit the formula's tag or revision by hand.
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
| `internal/unibox` | profiles, the manager, tasks, runtimes, the sbx adapter and its kit, the msb adapter (`msb*.go`) |
| `internal/unibox/api` | the versioned JSON contract other programs read |
| `internal/unibox/cli` | the cobra commands, preflight and its questions |
| `internal/unibox/templates` | every file unibox puts into an environments repository, as files: `managed/` mirrors `.shared`, which unibox owns and keeps current, `user/` the examples init writes once into `_shared`, `profile/` what init writes once into a new profile, `seeds/` the agents' configuration seeds, `build/` the `@install` runner a build gets |
| `internal/version` | the release unibox was built from, set from the tag at build time |
| `internal/testenv` | the isolated environment every package's tests run in |

## Decisions worth knowing

- **A profile holds nothing personal.** What each person sets for
  themselves it declares under `vars`; the values are in the user's
  `~/.config/unibox/vars.toml`, and reach the sandbox only where the
  profile refers to them as `${vars.<name>}` - in `env`, `build_args`, a
  service's `user` and `author`. A run stops while one is missing. `env`
  is the one place a variable is set; a service's placeholder goes there
  as `${services.<name>}`. Tokens stay `secret:<name>`.
- **A template is copied, never inherited.** `init --from` copies a
  profile, which is then the user's; the template's `_shared` goes to
  `.templates/<template>/`, unibox's to rewrite, looked up between the
  user's `_shared` and `.shared` for profiles with `template:` alone.
- **msb is an experimental runtime.** sbx is the supported one and the
  default; msb shows that profiles carry over, and is used only when set
  as `default_runtime` or asked for with `--runtime` (`Experimental`). The
  microsandbox Go SDK installs it on first use. The SDK (cgo, behind the `cgo` build tag) installs it and rotates
  secrets live; everything else runs the `msb` CLI. msb reads a secret
  from a host variable when it boots a sandbox, so unibox boots it under a
  shell that fills the variables from the secrets' commands - a value
  never passes through unibox. docs/design.md has what was verified.

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
  through `COPILOT_ALLOW_ALL`, which goes with its default arguments and
  is left out when a profile's `args_override` replaces them. What unibox writes into an agent's own file
  sits between markers or is merged, never over the user's settings.
- **Task names are unique across sandboxes**, so a task alone finds its
  sandbox (`unibox resume <task>`).
- **init -i is a walk of steps** (`wizard.go`): Runtime, Repository,
  Profile, Values, Agents, Services, Build, Run, each shown as it goes;
  from Agents on, each can be skipped or the rest left for later. A step
  reuses its command (`service add` runs as itself), so the walk and the
  commands never differ. `unibox doctor` checks what the walk sets up.
- **sbx's JSON changes shape between releases** (0.48 made `ls` and
  `template ls` bare lists); read a list through `sbxList`, which takes
  either.
- **The SSH agent is the user's call.** sbx forwards it by default; unibox
  refuses that until `runtimes.sbx.ssh_agent` says yes or no (`init -i`
  asks, recommending no) and sets sbx to match. With it, sessions get
  `GIT_SSH_COMMAND` with `StrictHostKeyChecking=accept-new`.
- **A profile says what a token is, vars.toml where it comes from.**
  Commands unibox writes for a token go into the person's vars.toml
  (`[secrets.names]`), the profile naming `secret:<name>`; only
  `--in-profile` writes one into the profile, for a source everyone reads
  alike. It keeps a profile portable - to another person, to the user's
  other machine, through a template update without a merge.
- **A person adapts a shared profile in their vars.toml, not in it.** A
  template's profile stays as it is: values (with `${vars.x:-default}` for
  what may be left alone) and the command reading `secret:<name>` come from
  `~/.config/unibox/vars.toml`. That command is carried as `Via` on
  `AgentAuth` and `Service`, `json:"-"`, so it is no part of what
  `unibox approve` covers; `Profile.SecretCommand` puts it into the
  sandbox's snapshot, so a change reaches the runtime.
- **Every field of sandbox.yaml is a command's to write.** A program
  configuring unibox needs no YAML of its own: `unibox profile show --json`
  gives the document, `profile set|unset <path>` changes any field,
  validated as `LoadProfile` reads it. A new field needs nothing more than
  its validation there; frequent ones get commands of their own
  (`network`, `service`, `agent`), which stay the everyday way.
- **Commands are a noun, then a verb** (`unibox sandbox stop`); only
  `run`, `resume`, `init`, `context`, `config`, `prune` and `version`
  stand alone. The argument is what a command acts on; the profile it
  works in is `-p`, the sandbox `-s`, which default to `UNIBOX_PROFILE`
  and `UNIBOX_SANDBOX`, then the context (`unibox context use`), then
  the only one. `sandbox rm`, `task ls` and `var` never take the default,
  so a forgotten context cannot remove, hide or misplace anything.

## Tests

Every package's `TestMain` goes through `internal/testenv`
(`TestEveryPackageRunsInTheTestEnvironment` checks it): a temporary home,
a git configuration of the run's own, and a PATH with git, go and the
shell's utilities only. A test that needs sbx, herdr or an agent puts a
stand-in on its own PATH with `testenv.WriteExecutable` and is serial
(`t.Setenv`); every other test calls `t.Parallel()`. The fake runtime in
`unibox_test.go` runs a sandbox's commands in a temporary directory.
Check the real sbx on a throwaway sandbox, and remove it afterwards.
