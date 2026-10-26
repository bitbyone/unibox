# herdr

[herdr](https://herdr.dev) shows each pane's agent state. It cannot see an
agent inside a sandbox, so unibox provides a herdr plugin that reports it.

```sh
unibox herdr status              # is herdr installed, and the plugin
unibox herdr install-plugin      # asks first, unless --yes
unibox herdr uninstall-plugin
```

Needs herdr 0.9.1 or newer. Once installed, a `unibox run` or `resume`
started in a herdr pane shows the agent's state there, with the sandbox's
name. The plugin points at the unibox binary; when you upgrade or move
unibox, the next unibox command updates it.
