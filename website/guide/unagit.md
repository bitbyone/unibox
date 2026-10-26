# unagit

[unagit](https://bitbyone.github.io/unagit/) is a sibling project: one view
of every repository you work with. It is planned to start sandboxed agents
through unibox.

::: info Status
Planned. Nothing in unagit drives unibox yet.
:::

unagit will run the `unibox` binary and read its `--json` output; the two
share no code. It will configure unibox only through its commands
(`env add`, `var set`, `secret set`, `port on`), and start a task with
`unibox task check --json`, `unibox task prepare --json` and
`unibox resume <task>`. Every JSON object carries an `api_version`, which
changes only when a field changes meaning or goes away.
