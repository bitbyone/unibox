# Ports

A sandbox's ports reach your machine only where they are published, on the
host's loopback. Ports are named in files, by hand - the team's beside its
sbxenv files, yours in `config.yaml` - and each says whether a sandbox
starts with it published. `unibox port on` and `off` only publish and
unpublish them in a running sandbox.

## Your ports

```yaml
# ~/.config/unibox/config.yaml
envs:
  dev:
    files: [/home/me/team/sandboxes/dev.sbxenv.yaml]
    ports:
      fe: '3000:3000'                               # host 3000 to sandbox 3000
      api: '8080'                                   # sandbox 8080 on any free host port
      devtools: {port: '9333:9222', default: off}   # published only when asked to
```

The mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, as Docker
says it. A port is published whenever the env's sandbox starts unless it
says `default: off`, and a running sandbox gets a change of the
files at the next `unibox run`, `resume`, `sandbox shell` or `port on|off`. A port without a host
port gets a free one, and another one at each start: `unibox port ls` shows
where it is now. Give a host port for a page you keep open.

`unibox env show` lists an env's ports with the rest of it.

## The team's ports

The ports a whole team uses - the dev server, Storybook, a debugger - can be
named once, in files sbx does not read: `unibox.yaml` at the root of the
repository for every env in it, and beside an sbxenv file for one env -
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

Each person changes the team's ports for themselves in their
`config.yaml`, which wins over the team's file:

```yaml
envs:
  dev:
    ports:
      storybook: {default: on}               # on at every start
      vite: {port: '15173:5173'}             # on another host port, for you alone
```

`unibox port ls`
says of each where it is named - `repo`, `env`, `yours-all`, `yours`, or
`-` for one published by hand - and whether it is on at start:

```
PORT       MAPPING    FROM  AT START  NOW  URL                    DESCRIPTION
storybook  6006       repo  off       off  -                      Storybook, when you work on components
vite       5173:5173  env   on        on   http://127.0.0.1:5173  the dev server
```

## Which wins

A port named in more than one place is taken from each in turn, the later
over the earlier:

1. `unibox.yaml` at the root of the team's repository, for every env in it;
2. `<env>.unibox.yaml` beside the env's sbxenv file - of an env's several
   files, a later one's, as its sbxenv does;
3. yours for every env, `ports:` at the top of `config.yaml`
4. yours for the env, under it in `config.yaml`.

A later place may give a port only another mapping or switch it on or off;
its description stays the team's. A switch of yours for every env that an
env does not name is left out of that env.

```yaml
# config.yaml
ports:
  devtools: {port: '19222:9222', default: off}   # every env, on another host port
  vite: {default: on}                            # the team's vite, on wherever it is named
envs:
  dev:
    ports:
      vite: {default: off}                       # but not in dev
```

## Switching ports on a running sandbox

```sh
unibox port ls -s dev                     # its ports, with their URLs
unibox port on devtools -s dev            # by name
unibox port on vite=15173 -s dev          # a named port on another host port
unibox port off fe -s dev
unibox port on --map 8080:5173 -s dev     # a mapping no file names
```

`on` and `off` change the running sandbox and no file; the sandbox's next
start goes back to what the files say. `name=HOST_PORT` keeps the port's
sandbox port, host address and protocol. A switch on a stopped sandbox is
refused, as publishing would start it.

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
