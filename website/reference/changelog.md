# Changelog

What each release brought, newest first. Every release's archives are on
[GitHub](https://github.com/bitbyone/unibox/releases); `brew upgrade
unibox` or `scoop update unibox` takes the latest.

## Unreleased

- herdr shows a sandboxed agent's state by `HERDR_AGENT` on the session's
  `sbx exec`, as herdr asks of a wrapper, instead of unibox's plugin and
  its observer, which did not work. `unibox herdr` is gone; the first
  command after upgrading unlinks the plugin.
- A team's `unibox.yaml` no longer gives a token a variable, placeholder
  and hosts of its own: where a token goes is its kit's to say, so it
  works the same for anyone using sbx alone.
- `clipboard_text_copy` says whether text an agent copies reaches your
  clipboard, which sbx 0.51 stopped doing until told; `init` asks it and
  doctor says while it is off and unsaid.
- unibox needs sbx 0.51 or newer. doctor and `unibox prune` read a
  sandbox's mounts from sbx, so they check and tidy stopped sandboxes
  too, without starting one.

## 0.6 <Badge type="tip" text="2026-11-02" />

- A person keeps who their commits are by, for every env, an env or a
  sandbox (`unibox author`), and the team can require one; commits in a
  mounted repository now go out as that author, not the sandbox's user.
- Agents name themselves co-author as their own settings say, or not at
  all for every env or one (`unibox co-author`).
- A task whose mounted checkout was moved to another branch on the host
  is refused until it is back, rather than taken up on the wrong branch.
- `unibox port add` and `port rm`, or `a` and `r` in the port list, name
  and forget a port of your own and publish it at once.
- A repository added to a task is mounted or cloned as its row says, and
  `task rm-repo` (`u` in the task list) takes one out again.
- `sandbox recreate` and `task rm` take `--force` to discard work the
  forge lacks.
- An agent's session is told which terminal it draws on, so agents show
  images in Ghostty, kitty and iTerm.
- A recreated sandbox takes its own token again instead of the global one.
- The herdr plugin installs from inside a herdr pane and starts its
  observer at once, without restarting herdr.

## 0.5 <Badge type="info" text="2026-10-29" />

- A sandbox made anew gets its env's and its own tokens again, instead of
  the global token sbx would otherwise give it after a recreate.
- A task's conversations are put back without tar's complaints about
  directories the sandbox already has.

## 0.4 <Badge type="info" text="2026-10-29" />

- A task's agent conversations are kept on the host after each session
  and before a recreate, so `unibox resume` carries on the last one even
  in a sandbox made anew.
- Copilot resumes the last session of the task's own directory, not the
  last of the whole sandbox.
- `make uninstall` removes what `make install` put in place.

## 0.3 <Badge type="info" text="2026-10-29" />

- `unibox resume` continues the agent's last conversation in the task;
  `--new` starts afresh.
- Tasks get repositories by name from the env's index, picked on one
  screen with their role, mount or clone, branch and push; repositories
  your tasks have held are offered again.
- Tokens of an env's or a sandbox's own, typed into the system's keychain,
  beside sbx's global ones.
- Every noun is a list to act in at a terminal, scrolled to fit, and
  `unibox` alone opens the sandbox list.
- A spinner says what a run is doing until the session starts.
- Agents can paste images from the host's clipboard.
- Editors over SSH, sbx's settings kept in config.yaml, and doctor's fixes
  offered one by one.
- Homebrew installs the release's binary instead of building it.

## 0.2 <Badge type="info" text="2026-10-26" />

- Unibox becomes a thin layer over sbx: sandboxes are created by sbx from
  the team's sbxenv files and kits, with each person's values kept apart
  in vars.toml.
- `unibox init` sets up from the team's repository, local or cloned from
  its address, and `unibox doctor` checks what it set up.
- Ports, the network policy and secrets have commands of their own, so
  nobody needs sbx's CLI.
- A sandbox's CPUs and memory, given by the team or the person.
- Builds for Linux, macOS and Windows, with a Homebrew tap and a Scoop
  bucket.

## 0.1 <Badge type="info" text="2026-10-16" />

- The first release: one CLI over sandbox runtimes for coding agents,
  with profiles in an environments repository and an adapter per runtime.
  It is set aside by 0.2, not migrated.
