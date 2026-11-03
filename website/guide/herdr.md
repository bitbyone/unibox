# herdr

[herdr](https://herdr.dev) shows each pane's agent state. It finds an agent
by the processes in the pane, and a sandboxed agent's is `sbx exec`, so
unibox starts that with `HERDR_AGENT` naming the agent - claude, codex,
copilot or whichever the session runs - as herdr asks of a wrapper. herdr
then reads the agent's state from its screen by its own rules for it.

Nothing to install or set up. A `unibox run` or `resume` in a herdr pane
shows the agent and its state there; a shell (`--shell`) shows as one.
The variable is on the host's `sbx` process alone; the sandbox never sees
it. herdr reads it on Linux and macOS.

Earlier releases had a herdr plugin for this. The first unibox command
after upgrading unlinks it from herdr and removes it.

## Images

Copilot knows herdr by `HERDR_*` variables, which stay on the host, so in a
herdr pane unibox tells a session `COPILOT_MULTIPLEXER=herdr` instead. It
adds `COPILOT_INLINE_IMAGES_HERDR=1`, which Copilot needs to show images in
herdr, when herdr draws them: `terminal.kitty_graphics` in its
`config.toml`, on by default since herdr 0.9.0. Either variable set on the
host is passed as it is instead.
