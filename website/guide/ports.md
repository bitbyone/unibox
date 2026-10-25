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

## The team's ports

The ports a whole team uses - the dev server, Storybook, a debugger - can be
named once, beside the sbxenv file, in a file sbx does not read:
`dev.sbxenv.yaml` has its ports in `dev.unibox.yaml`.

```yaml
# sandboxes/dev.unibox.yaml
ports:
  vite:
    port: '5173:5173'
    description: the dev server
    default: on
  storybook:
    port: '6006'
    description: Storybook, when you work on components
```

`port` is the mapping, `description` says what it is for, and `default`
whether it is published at every start; left out, it is off. Someone using
sbx without unibox is not limited by the file in any way: sbx never reads
it.

Each person switches the team's ports for themselves; the choice is kept
in their `config.yaml`, and wins over the team's default:

```sh
unibox port on storybook -e dev
unibox port off vite -e dev
unibox port set vite --map 15173:5173 -e dev    # another host port, for you alone
unibox port unset vite -e dev                   # back to the team's mapping
```

`unibox init` offers the team's ports in its Ports step. `unibox port ls`
says of each where it comes from - the team, you, or neither, for one
published by hand - and whether it is on at start:

```
PORT       MAPPING    FROM  AT START  NOW  URL                    DESCRIPTION
storybook  6006       team  off       off  -                      Storybook, when you work on components
vite       5173:5173  team  on        on   http://127.0.0.1:5173  the dev server
```

When an env has several files, a later file's `.unibox.yaml` wins for a port
both name, as its sbxenv file does.

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

## What a sandbox may reach

A sandbox reaches only the hosts sbx's network policy allows: the
defaults, the hosts the env's kits add, and rules of your own. unibox
changes those rules with sbx, for one sandbox or with `--global` for every
one:

```sh
unibox network ls -s dev                    # the rules the sandbox has
unibox network allow jitpack.io -s dev
unibox network deny gist.github.com --global
unibox network rm jitpack.io -s dev         # take a rule of yours away
unibox network log -s dev                   # the latest requests, and what was decided
```

A host is as sbx takes it: a name, a wildcard such as `*.example.com`, an
IP address or CIDR prefix, each with an optional port (`example.com:443`). The defaults' and the kits' rules are not to be removed. A rule the
whole team needs belongs in a kit, where it is reviewed with the env.
