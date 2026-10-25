# Ports

A sandbox's ports reach your machine only where they are published, on the
host's loopback. Which host port a person uses is theirs, not the team's,
so unibox keeps named ports per env in its own `config.yaml`, out of the
sbxenv files.

## Named ports

```sh
unibox port set fe --map 3000:3000 -e dev              # at every start
unibox port set api --map 8080 -e dev                  # any free host port
unibox port set devtools --map 9333:9222 --off -e dev  # only when published by hand
unibox port unset devtools -e dev                      # take it out
```

```yaml
envs:
  dev:
    files: [/home/me/team/sandboxes/dev.sbxenv.yaml]
    ports:
      fe: '3000:3000'                               # host 3000 to sandbox 3000
      api: '8080'                                   # sandbox 8080 on any free host port
      devtools: {port: '9333:9222', default: off}   # published only when asked to
```

The mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, as Docker
says it. A port is published whenever the env's sandbox starts unless
`--off`, and a running sandbox gets a change at once. A port without a host
port gets a free one, and another one at each start: `unibox port ls` shows
where it is now. Give a host port for a page you keep open.

`unibox env show` lists an env's ports with the rest of it.

## Switching ports on a running sandbox

```sh
unibox port ls -s dev                          # its ports, with their URLs
unibox port publish devtools -s dev            # by the env's names
unibox port unpublish fe -s dev
unibox port publish --map 8080:5173 -s dev     # a mapping the env does not name
```

`publish` and `unpublish` change the running sandbox and leave the env as it
is; the sandbox's next start goes back to the env's ports. A switch on a
stopped sandbox is refused, as publishing would start it.

A port unibox did not publish is left alone: one published with
`sbx ports`, and one a kit declares, which sbx publishes itself, on a new
host port at every start.

::: tip A browser for the agent
A browser an agent drives belongs in the sandbox - Chrome for Testing and
chrome-devtools-mcp from a kit, say - with its DevTools port published for
you to look on. An agent driving your own browser would have its cookies,
sessions and files.
:::

## Network rules are sbx's

What a sandbox may reach is not unibox's to say. sbx refuses what its policy
does not list; the env's kits add the hosts they need, and the hosts a
binding names are reached with the token put in. To change the rest, use
sbx's own command:

```sh
sbx policy ls
sbx policy allow network jitpack.io
sbx policy deny network gist.github.com
```

`sbx policy --help` has the rest. A rule the whole team needs belongs in a
kit, where it is reviewed with the env.
