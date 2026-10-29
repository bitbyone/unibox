# AGENTS.md

Notes for whoever - person or model - works on unibox next. The README says
what it is for; [docs/design.md](docs/design.md) why it is built the way it
is and what was verified about sbx. Read the latter before changing how a
sandbox is created or signed in.

## The one hard rule

**Secrets are never yours to read.** They live in sbx's store on the
host (the keychain) and reach a sandbox only as
placeholders the proxy replaces. Never print a token or a secret's value,
never ask the user to paste one into the conversation, never `cat` an
auth file - read key names (`jq 'keys'`), whether a value is set, or the
shape sbx masks it to. A value the user types or pipes to `unibox secret
set` goes straight to `sbx secret set` on its standard input - never argv,
a log, `--json`, state or config.yaml - and a token never enters a
sandbox.

## Working here

```sh
make build             # the unibox binary
make test              # go test -race ./...
make test-fast         # while iterating; race before commit
make install           # into ~/.local/bin
make uninstall         # out of it again
make dist VERSION=1.2  # release archives for every system, into dist/
gofmt -l . && go vet ./...
```

Everything must be gofmt-clean, vet-clean and race-clean before a commit.

CI (`.github/workflows`): `test` runs on every push and pull request, the
race detector on Linux and the suite without it on macOS, and Windows
builds, vets and runs the tests that need no shell stand-in; `website`
builds `website/` with VitePress and publishes it to GitHub Pages; a
`vX.Y` tag on a commit of main whose tests passed releases it - `release`
builds it, checks `unibox --version` prints the tag's version, publishes
`make dist`'s archives for Linux, macOS and Windows as the tag's GitHub
release, and writes `Formula/unibox.rb` (the Homebrew tap this repository
is) and `bucket/unibox.json` (its Scoop bucket) whole from the archives'
sums, so brew and Scoop install the binary built there and nothing builds
on the person's machine. Release by pushing such a tag; never edit the
formula or the manifest by hand - change the workflow that writes them.
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
| `internal/unibox` | envs and their sbxenv files (`env.go`, `sbxenv.go`), finding them in a repository (`discover.go`) and cloning one (`clone.go`), the person's values (`envvars.go`), secrets, the manager, tasks and their mounts, ports and the team's (`teamports.go`), the network policy (`policy.go`), locks per system (`lock_unix.go`, `lock_windows.go`), agents, herdr, and `sbx.go`, the one client of sbx behind the `Runtime` interface |
| `internal/unibox/api` | the versioned JSON contract other programs read |
| `internal/unibox/cli` | the cobra commands, preflight, the `init` walk and doctor |
| `internal/version` | the release unibox was built from, set from the tag at build time |
| `internal/testenv` | the isolated environment every package's tests run in |

## Decisions worth knowing

- **The team's sbxenv files and kits are the configuration.** unibox has
  no format of its own for a sandbox: an env is a list of sbxenv files
  sbx merges in order, with the sandbox's name and the user's ports, in
  `config.yaml`. What sbx can say - image, kits, mounts, network, the
  agent - unibox does not repeat. docs/sbx-pivot.md has why and how it
  got here.
- **A person's values are theirs.** The args an env declares are given in
  `~/.config/unibox/vars.toml`, for every env or one, and reach sbx only
  through a private `--env-args-file` removed when sbx is done. A run
  stops while a required one is missing; `run`, `sandbox create` and
  `init` ask for it at a terminal. The CPUs and memory a sandbox gets are
  the person's alike (`[resources]`, by hand), over a default the team
  may give in its unibox.yaml files (`resources:`): shares are resolved
  on the host at each create and handed to sbx in a last sbxenv file of
  `sandboxOptions` alone (`resources.go`). Between every env and one env
  sits every env of one env repository (`env_repos`, by `EnvRepoKey`),
  in config.yaml and vars.toml alike.
- **One client of sbx, behind an interface.** `Runtime` in `runtime.go`
  is what the manager asks of sbx; `sbx.go` is the only implementation,
  `fake_test.go` the tests'. It is there for tests, not for another
  runtime: nothing promises one.
- **A sandbox is created by sbx from the env.** `sbx env create` with the
  files and args, at the user's terminal, since sbx asks there to approve
  an env and its host commands. A creation that did not finish is retried
  by the next `create` or `run`. The sandbox keeps a fingerprint of the
  files, kits and args; a change is told by doctor and the next start, and
  applied only by `sandbox recreate`, which brings the tasks back.
  `--adopt` takes over a sandbox made with sbx, which unibox then removes
  only when forced.
- **Secrets are sbx's secrets, global unless a scope says otherwise.**
  `unibox secret` reads which services an env needs from its kits (`sbx
  kit inspect`, with `--kit-arg` from the env's values) and bindings,
  compares them with `sbx secret ls`, and has `sbx secret set` store the
  rest. A token of an env's or a sandbox's own (`--scope`, `s` in the
  list) is a command in config.yaml (`envs.<env>.secrets`,
  `sandboxes.<name>.secrets`, the narrowest winning), which `syncSecrets`
  has sbx keep as each sandbox's own at every start, as a recreate takes
  those away. Typed in, it goes to the system's keychain through its own
  program (`keychain.go`: security, secret-tool, PowerShell with DPAPI),
  which asks on the terminal, and the command reads it from there.
- **A task mounts by default.** Directories go to `/work/<task>/<repo>`
  through `sbx mount`; the agent can write the host's `.git`, which is
  told once per directory (`State.Mounted`). `task rm` unmounts, checks
  nothing of the host is left, and removes the root-owned mount points
  with `rmdir` as root - never `rm -rf` over a mount. `--clone` clones
  from the forge inside instead. sbx lists no mounts; a running sandbox's
  `/proc/self/mountinfo` does, with the host path as the mount's root,
  which doctor compares with the tasks.
- **The agent is the kit's.** Its entrypoint names it; unibox runs it
  with `sbx exec` in the task's directory and prepares it before every
  session: claude's trust in `~/.claude.json`, copilot's
  `trusted_folders`, codex's config.toml. What unibox writes into an
  agent's own file sits between markers or is merged, never over the
  user's settings. An agent unibox does not know is still run.
  An agent with `Resume` and `HadSession` (claude, codex, copilot)
  continues its last conversation in the session's directory when it had
  one there, tested in the guest before it starts; `--new` starts it
  afresh. copilot's `--continue` takes the last session of the whole
  sandbox, so its test finds the id of the directory's own, which
  `sessionArg` puts in its `--resume=`.
- **Task names are unique across sandboxes**, so a task alone finds its
  sandbox (`unibox resume <task>`).
- **init starts from the team's repository** (`cli/init.go`): the
  current one, a path, or a git address, which it clones with the user's
  own git into `$XDG_DATA_HOME/unibox/repos/<host>/<path>`. It offers every
  sbxenv file found and walks five steps: sbx (signed in, the network
  policy every sandbox starts from), Envs, Values, Secrets, Sandboxes. A step runs its command (`secret set`, `sandbox create`), so the walk and the commands never differ. Without a
  terminal or with `--no-input` it asks nothing and fails naming the flag
  (`--file`, `--var`) that gives what is missing. `unibox doctor` checks
  what the walk sets up.
- **unibox pulls what it cloned, and a clean checkout.** An env keeps the
  repository its files are in (`repo`) and the address unibox cloned it
  from (`source`); `env update` fast-forwards those, and the user's own
  checkout only while nothing in it is uncommitted and its branch follows
  a remote one (`Untouched`). `destroy --all` leaves the clones.
- **What only unibox reads sits beside the sbxenv file.** The team's
  ports are in `<name>.unibox.yaml`, which sbx never reads, so a team
  member without unibox is limited by nothing. Ports are written by
  hand; the user's in config.yaml win over the team's, `port default`
  (`d` in the list) writes only whether one is on at start for the user,
  and `port on|off` only switches a running sandbox's.
- **Nobody using unibox needs sbx's CLI.** The network policy
  (`unibox network`), `env.rememberHostCommands` (`unibox config`) and
  stray mounts (`unibox prune`) have commands, and every hint doctor or
  an error gives names a unibox command.
- **Every platform, without cgo.** unibox builds for Linux, macOS and
  Windows; what differs is behind build tags (locks, detaching a
  process). `GOOS=windows go vet ./...` checks it from any machine.
- **sbx's JSON changes shape between releases** (0.48 made `ls` bare
  lists); read a list through `sbxList`, which takes either. The oldest
  sbx unibox was checked against is `MinSBXVersion`; raise it when unibox
  starts relying on something newer.
- **The SSH agent is the user's call.** sbx forwards it by default; unibox
  refuses that until `ssh_agent` in config.yaml says yes or no (`init`
  asks, recommending no) and sets sbx to match. With it, sessions get
  `GIT_SSH_COMMAND` with `StrictHostKeyChecking=accept-new`. Editors
  over SSH (`editor_ssh`, `ssh <sandbox>.sbx`) are asked alike; turning
  it on restarts sbx's daemon, which serves SSH only from a start with
  `feature.ssh` on.
- **A noun alone at a terminal is a list to act in**, and `unibox`
  alone is `unibox sandbox` (help elsewhere, or before init). `unibox port`
  switches a running sandbox's ports, `unibox task` (and `resume` without
  a task) resumes, names or removes a task, `unibox sandbox` opens,
  stops, recreates or removes a sandbox; `env`, `var`, `secret`,
  `network`, `repo` and `context` are lists of `listModel` (`listui.go`),
  filtered fuzzily after `/` (`fuzzy.go`), whose
  actions run their subcommand through `runStep`, each by a key (`*ui.go`,
  bubbletea). A list's line is cut to the terminal's width, or it wraps
  and is no longer drawn over; its keys are wrapped between two keys
  instead (`helpLines`), as a cut line would hide the last ones. Every
  list's rows scroll within `listRoom` (`scroll.go`): at most
  `listMaxLines`, fewer on a short terminal, never under `listMinLines`,
  with a line above and below saying how many more lie there
  (`scrolled`). Every list's title has the context in its right corner
  (`contextCorner`, `titleLine`), where it matters or not. Nothing may write to the terminal while a list is drawn. An action that takes the terminal or asks
  more ends the list and runs as its command would; without a terminal
  or with `--json` the noun lists as `ls` does.
- **Commands are a noun, then a verb** (`unibox sandbox stop`); only
  `run`, `resume`, `init`, `doctor`, `context`, `config`, `prune`,
  `destroy` and `version` stand alone. The argument is what a command
  acts on; the env it works in is `-e`, the sandbox `-s`, which default
  to `UNIBOX_ENV` and `UNIBOX_SANDBOX`, then the context (`unibox context
  use`), then the only one. `sandbox rm` and `task ls` never take the
  default, so a forgotten context cannot remove or hide anything.
- **Repositories by name.** An env's index (`repos:` in the team's
  unibox.yaml files and the person's config.yaml, `repoindex.go`) gives
  tasks repositories by name; `default: always` ones go into every new
  task, cloned even beside mounted directories, as mount or clone is per
  repository (`RepoInput.Clone`); `context` ones (unsaid: `always`) keep
  their ref and are never pushed to. `task new` picks on one screen
  (`taskPickModel`): roles, work branch, push per row, task name.
  `instructions:` of the env and each repository become the task's
  `/work/<task>/AGENTS.md` (`CLAUDE.md` a link) before each session.
  Every repository a task held is kept in `State.Recent` (`recent.go`)
  with its sandbox's env, the newest first, and offered to that env's
  tasks alone, as another env's sandbox may lack the forge's network or
  secrets: the picker searches them with `/`, lists them with `r`, `a`
  names one in the person's index for the env and `d` forgets it, as
  `unibox repo recent rm` does.
- **Readying a task says what it does.** `Manager.Progress` is told each
  step of a run that takes sbx's seconds - checking the sandbox, its
  ports, network and tokens, the task, the agent - and "" whenever the
  terminal goes to `sbx env create` or the session; `runTask`
  (`cli/spinner.go`) draws it as a spinner, cleared before the manager's
  log writes. Anything new that hands the terminal on says "" first.
- **`--rm` removes a task as `task rm` would**, after its session's lock
  is let go and before an idle sandbox stops; with work the forge lacks it
  stays and says so. unibox's own AGENTS.md and its CLAUDE.md link are no
  such work.
- **unibox 1 is set aside, not migrated.** Its `state.json` becomes
  `state.v1.json` with the old sandboxes named once; its config.yaml keys
  are read and dropped at the next write.

## Tests

Every package's `TestMain` goes through `internal/testenv`
(`TestEveryPackageRunsInTheTestEnvironment` checks it): a temporary home,
a git configuration of the run's own, and a PATH with git, go and the
shell's utilities only. The manager's tests run over `fakeRuntime`
(`fake_test.go`), which runs a sandbox's commands in a temporary
directory, mounts as symlinks. The commands' tests that need sbx put a
stand-in on their own PATH (`fakeSBX` in `cli/login_test.go`, through
`testenv.WriteExecutable`) and are serial (`t.Setenv`); every other test
calls `t.Parallel()`. A command run through `hostCommand` gets a filtered
environment, so a stand-in is steered by files, not variables. Check the
real sbx on a throwaway sandbox, and remove it afterwards.
