# Network and Ports

What a sandbox may reach, and which of its ports your machine reaches, are
part of its profile - and both change on running sandboxes at once, with
nothing recreated.

## Network rules

The runtime has a policy of its own: sbx refuses what it does not list and
already allows the usual registries. A profile widens it with `allow` and
narrows it with `deny`:

```sh
unibox network allow web jitpack.io repo.example.com
unibox network deny web gist.github.com
unibox network rm web jitpack.io        # back to the runtime's policy
unibox network ls web                   # the rules, with the hosts the services need
```

```yaml
network:
  allow: [jitpack.io, repo.example.com]
  deny: [gist.github.com]
```

Each entry is one pattern: a host, `*.domain`, an IP or a CIDR, with an
optional port. **Deny wins** where both match.

The change is written into the profile and applied to the profile's running
sandboxes at once. A hand edit of the file is told and applied at the next
start. A service's hosts join the allow policy on their own; you do not list
them.

## Ports

A port is published on the host's loopback **by name**:

```sh
unibox network port web fe 3000:3000               # at every start
unibox network port web api 8080                   # any free host port
unibox network port web devtools 9333:9222 --off   # only when switched on
unibox network rm web devtools                     # take it out
```

```yaml
network:
  ports:
    fe: '3000:3000'                              # host 3000 to sandbox 3000
    api: '8080'                                  # sandbox 8080 on any free host port
    devtools: {port: '9333:9222', default: off}  # published only when switched on
```

The spec is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. A port is
published when a sandbox starts unless `--off`, and running sandboxes get a
change at once. A port without a host port gets a free one, and another one
at each start of the sandbox: `unibox port <sandbox>` shows where it is now.
Give a host port for a page you keep open.

### Switching ports on a running sandbox

```sh
unibox port web-1                          # its ports, with their URLs
unibox port web-1 fe:off devtools:on       # by the profile's names
unibox port web-1 8080:5173:on             # a spec the profile does not name
```

`unibox port` changes the running sandbox and leaves the profile as it is; the
sandbox's next start goes back to the profile's defaults. A switch on a stopped
sandbox is refused, as publishing would start it. A port published with sbx's
own command is left alone.

::: tip A browser for the agent
A browser an agent drives belongs in the image - Chrome for Testing and
chrome-devtools-mcp, say - with its DevTools port published for you to look
on. An agent driving your own browser would have its cookies, sessions and
files.
:::

## What is live and what is not

| Change | Applied |
| --- | --- |
| `network.allow`, `network.deny` | live, to running sandboxes |
| `network.ports` | live through `unibox network port`; at the next start after a hand edit |
| `unibox port <sandbox> ...` | live, until the sandbox next starts |
| `services`, an agent's `auth` | a recreation: sbx binds a token's placeholder when it makes the sandbox |
| `env`, `caches`, `agents`, the image | a recreation, told at the next start |
