# Changelog

What each release brought, newest first. Every release's archives are on
[GitHub](https://github.com/bitbyone/unibox/releases); `brew upgrade
unibox` or `scoop update unibox` takes the latest.

## Unreleased

- `unibox run` with no directory named starts a task of no repository,
  holding only what the index gives every task, rather than mounting the
  directory it was typed in: `unibox run .` mounts the current one. A
  mount hands the agent your files and your `.git`, so it is asked for.
- The team's `unibox.yaml` files give defaults for each person's settings
  of the whole machine under `settings:` (`ssh_agent`, `ssh_sandbox`,
  `remember_host_commands`, the clipboard's and `claude_remote_control`):
  one not said yet is taken without a question, by `init` and when a
  sandbox starts; one said otherwise `init` asks again, the team's first.
- `version_required` in those files refuses an older unibox with how to
  update it, and a key unibox does not know says it may be too old.

- `init` asks whether agents paste images from your clipboard whatever
  the env's agent, not only for claude and codex: sbx serves them to any
  process in a sandbox.

- `unibox` alone at a terminal opens the task list, the one come back to
  most; `unibox sandbox` is the sandbox list.

- `task new` starts a task of no repository: with nothing picked it is a
  directory of its own, named after `t` or `--task`, else with a name made
  up such as `brave-otter`. An env whose index names none opens the screen
  too. A task of one context repository alone starts in the task's
  directory, not in the repository it only reads. `run`, `task prepare`
  and `task check` take such a task with `--bare`.

- The copilot token is asked for before the github token, and an env or
  sandbox with a copilot token of its own offers *The copilot token* for
  github, so one token can be given for both. The team's `unibox.yaml`
  names the way it recommends under `secrets.<service>.recommended`,
  which the question starts on.

- `sandbox recreate` no longer prepares every task again: each is mounted
  or cloned, and given its conversations back, when it is next resumed,
  so a recreate takes as long as sbx does.

- `unibox config` alone at a terminal is the settings in tabs: Global,
  then each env with its own over Global and its args, to change in place.
- `stop_idle` can be an env's own (`unibox config set stop_idle false -e
  dev`).
- `editor_ssh` is `ssh_sandbox` now, as it lets anything SSH into every
  sandbox, not editors alone; the old key is read and written anew. A
  sandbox started while none runs turns sbx's SSH back on when sbx lost
  it, and `init` that finds it on writes the SSH client's entry too.

- At a terminal unibox tells, after a command and in every list's right
  corner, when an env repository has new commits or a newer unibox or sbx
  is out, with the command that brings it. It asks in the background, at
  most daily for releases and every four hours for the repositories;
  `update_check: false` or `UNIBOX_NO_UPDATE_CHECK` stops it. doctor asks
  there and then, and checks your own checkouts of env repositories too.

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
- A sandbox's creation canceled at sbx's prompt says so and exits 130,
  rather than reading as a failure; the next `run` or `sandbox create`
  goes on with it.

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
