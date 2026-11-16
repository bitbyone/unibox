# Changelog

What each release brings, newest first. Breaking changes come first in a
release and say what to do. Every release's archives are on
[GitHub](https://github.com/bitbyone/unibox/releases); `brew upgrade
unibox` or `scoop update unibox` takes the latest.

## 0.11 <Badge type="tip" text="2026-11-16" />

Long-running sandboxes, and ready for sbx 0.54.

### Breaking changes

- Stop in the sandbox list is `x` now, not `t`.

### New

- **Keep a sandbox running.** `b` in `unibox sandbox`, or
  `unibox sandbox start`, starts a sandbox with no task in it and keeps
  it up until you stop it, whatever `stop_idle` says: for an editor over
  SSH or a dev server. The list shows it as `running (kept)`.
  [Sandboxes](/guide/sandboxes)

### Improvements

- Works with sbx 0.54's new `sbx port` commands; an older sbx still
  works as before.
- After you upgrade sbx, unibox refreshes its SSH entry
  (`sbx setup ssh`) for you, as sbx 0.54 asks, when `ssh_sandbox` is on.

### Fixes

- The "commits behind" notice goes away right after `unibox env update`.
- The "newer version available" notice goes away once you have upgraded.

## 0.10 <Badge type="info" text="2026-11-11" />

Tasks without a repository, and team defaults for personal settings.

### Breaking changes

- **`unibox run` alone no longer mounts the current directory**: it
  starts a task of no repository. Use `unibox run .` to mount where you
  are. A mount gives the agent your files and your `.git`, so it is
  asked for, not assumed.
- **`unibox` alone opens the task list**, the one you come back to most;
  the sandbox list is `unibox sandbox`.
- **`default_env` is gone.** The context says which env commands work in;
  `init`, `env add --default` and the env list set it. An existing
  `default_env` is moved into the context once, nothing to do.

### New

- **Team defaults for personal settings.** The team's `unibox.yaml` can
  give defaults under `settings:` (`ssh_agent`, `ssh_sandbox`, clipboard,
  `claude_remote_control` and more). One you have not set is taken
  without a question; one you set otherwise, `init` asks about again.
  [Team and Personal Configuration](/guide/configuration)
- **`version_required`** in the team's files stops an older unibox with
  how to update it.

### Improvements

- `init` asks about pasting clipboard images for every agent, not only
  claude and codex.
- An unknown key in the team's files says unibox may be too old.

## 0.9 <Badge type="info" text="2026-11-07" />

Faster recreates, and one token for Copilot and GitHub.

### New

- **Tasks with no repository.** `task new` with nothing picked starts a
  task in a directory of its own, named by you or made up
  (`brave-otter`). `run`, `task prepare` and `task check` take one with
  `--bare`. [Tasks](/guide/tasks)

### Improvements

- **`sandbox recreate` is as fast as sbx**: tasks are mounted or cloned
  again, with their conversations, when each is next resumed rather than
  all at once.
- The Copilot token is asked for first, and can be used for GitHub too,
  so one token does both. The team can recommend how to give each token
  (`secrets.<service>.recommended`).
- A task of one context repository starts in the task's directory, not
  in the repository it only reads.

## 0.8 <Badge type="info" text="2026-11-06" />

### Fixes

- `init` on a machine whose sbx had never built a kit no longer fails
  with "sandbox sbx-kit-builder already exists".

## 0.7 <Badge type="info" text="2026-11-05" />

Settings in tabs, and update notices.

### Breaking changes

- **unibox needs sbx 0.51 or newer.** Update Docker Sandboxes first.
- **`unibox herdr` is gone**: herdr now shows the agent's state on its
  own. The first command after upgrading unlinks the old plugin.
- **`editor_ssh` is `ssh_sandbox` now**, as it lets anything SSH into a
  sandbox, not editors alone. The old key is read and rewritten for you.
- **A team's `unibox.yaml` no longer places tokens** (variable,
  placeholder, hosts): each token's kit says where it goes. Move any such
  entries into the kit.

### New

- **Settings in tabs.** `unibox config` alone opens the settings: Global,
  then each env with its own on top, changed in place.
  [Team and Personal Configuration](/guide/configuration)
- **Update notices.** After a command and in each list's corner, unibox
  says when an env repository has new commits or a newer unibox or sbx is
  out, with the command to get it. `update_check: false` or
  `UNIBOX_NO_UPDATE_CHECK` turns it off.
  [Keeping the Env Current](/guide/updating)
- **`clipboard_text_copy`** says whether text an agent copies reaches
  your clipboard, which sbx 0.51 no longer does unless told; `init` asks.
- `stop_idle` can be set per env (`unibox config set stop_idle false -e
  dev`).

### Improvements

- `doctor` and `unibox prune` check and tidy stopped sandboxes too,
  without starting them.
- `doctor` checks for new releases there and then, and checks your own
  checkouts of env repositories.
- A sandbox started while none runs turns sbx's SSH back on if sbx lost
  it.

### Fixes

- Cancelling a sandbox's creation at sbx's prompt is reported as
  cancelled (exit 130), not as a failure; the next `run` goes on with it.

## 0.6 <Badge type="info" text="2026-11-02" />

Commit authors, co-authors and your own ports.

### New

- **Commit author** for every env, an env or a sandbox
  (`unibox author`), which the team can require: commits in a mounted
  repository go out as you, not the sandbox's user.
- **Co-author lines** as each agent's settings say, or none
  (`unibox co-author`).
- **Ports of your own**: `unibox port add` and `port rm`, or `a` and `r`
  in the port list, name a port and publish it at once.
  [Ports](/guide/ports)
- `task rm-repo`, or `u` in the task list, takes a repository out of a
  task.

### Improvements

- `sandbox recreate` and `task rm` take `--force` to discard work the
  forge lacks.
- Agents show images in Ghostty, kitty and iTerm.
- The herdr plugin installs from inside a herdr pane without restarting
  herdr.

### Fixes

- A task whose mounted checkout was switched to another branch on the
  host is refused until it is back, instead of resumed on the wrong
  branch.
- A repository added to a task is mounted or cloned as its row says.
- A recreated sandbox takes its own token again, not the global one.

## 0.5 <Badge type="info" text="2026-10-29" />

### Fixes

- A recreated sandbox gets its env's and its own tokens again, instead
  of sbx's global one.
- Restoring a task's conversations no longer prints tar's complaints
  about directories the sandbox already has.

## 0.4 <Badge type="info" text="2026-10-29" />

### New

- **Conversations survive a recreate.** A task's agent conversations are
  kept on the host, so `unibox resume` carries on the last one even in a
  new sandbox.

### Fixes

- Copilot resumes the last session of the task's own directory, not the
  last of the whole sandbox.
- `make uninstall` removes what `make install` put in place.

## 0.3 <Badge type="info" text="2026-10-29" />

Tasks, the repository index and interactive lists.

### New

- **`unibox resume`** continues the agent's last conversation in a task;
  `--new` starts afresh.
- **Repository index.** Tasks get repositories by name from the env's
  index, picked on one screen with their role, mount or clone, branch and
  push; repositories you used before are offered again.
  [Repository Index](/guide/repos)
- **Interactive lists** for every noun, scrolled to fit; `unibox` alone
  opens the sandbox list. [Interactive Lists](/guide/interactive)
- **Tokens per env or sandbox**, kept in the system's keychain, beside
  sbx's global ones.
- Agents can paste images from your clipboard.
- Editors over SSH, sbx's settings kept in `config.yaml`, and `doctor`'s
  fixes offered one by one.

### Improvements

- A spinner says what a run is doing until the session starts.
- Homebrew installs the release's binary instead of building it.

## 0.2 <Badge type="info" text="2026-10-26" />

unibox becomes a companion to Docker Sandboxes.

### Breaking changes

- **A new start, not an upgrade of 0.1**: sandboxes are created by sbx
  from the team's sbxenv files and kits, and 0.1's profiles are not
  migrated. Set up again with `unibox init`.

### New

- `unibox init` sets up from the team's repository, local or cloned, and
  `unibox doctor` checks what it set up.
- Your own values are kept apart in `vars.toml`.
- Ports, the network policy and secrets have commands of their own, so
  nobody needs sbx's CLI.
- A sandbox's CPUs and memory, set by the team or by you.
- Builds for Linux, macOS and Windows, with Homebrew and Scoop.

## 0.1 <Badge type="info" text="2026-10-16" />

The first release: one CLI over sandbox runtimes for coding agents, with
profiles in an environments repository and an adapter per runtime.
