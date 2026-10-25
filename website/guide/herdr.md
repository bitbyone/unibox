# herdr

[herdr](https://herdr.dev) recognises an agent by its
pane's foreground process. An agent behind `sbx exec` is not one it
knows, so a sandboxed agent would show no state. unibox fills that in with a
herdr plugin of its own.

## Install

```sh
unibox herdr status            # whether herdr is there, and the plugin
unibox herdr install-plugin    # asks first, unless --yes
unibox herdr uninstall-plugin
```

It needs herdr 0.9.1 or newer. The plugin is installed only with your
consent, never quietly: it changes herdr's configuration and leaves a process
running with herdr. `install-plugin` writes the plugin's manifest to
`~/.config/unibox/herdr-plugin/` with the absolute path of the running unibox,
then links and enables it in herdr.

When unibox is upgraded or moved, that path changes. Every unibox command
that finds the plugin installed compares the linked manifest with what it
would write and rewrites it if it differs - you consented to the plugin, not
to a path.

## What it does

As long as herdr runs:

- `unibox run` started in a herdr pane records the pane, the agent and the
  sandbox in unibox's state.
- For every pane so recorded whose `unibox run` is alive, the plugin reads the
  screen, has herdr classify it by herdr's own rules for that agent, and
  reports the state - with the sandbox's name as the pane's metadata. It
  reports only on a change, and releases the pane when the agent is gone.
- It never parses a command line: unibox started the agent and wrote it down.

The plugin is a manifest and nothing to build: its commands call the unibox
binary, which starts one observer per herdr session.

A sandboxed agent started in herdr without the plugin runs as before, with no
state shown.

The agents' own hooks - claude's session id, opencode's states - do not cross
into the VM; the state is the screen's.

The idea comes from [sbx-herdr](https://github.com/dvdksn/sbx-herdr); none of
its code is taken.
