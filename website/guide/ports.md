# Ports

Ports of a sandbox reach your machine only when published, on
`127.0.0.1`.

## Open a port

```sh
unibox port ls                       # the ports, whether on, and their URLs
unibox port on vite                  # a named port
unibox port on vite=15173            # on another host port
unibox port on --map 8080:3000       # a port no file names
unibox port off vite
unibox port                          # switch them with the arrow keys or h and l, m for another host port
```

Add `-s dev` for another sandbox than the default. The sandbox must be
running. These switches last until the sandbox stops; at the next start
the files below decide again.

A mapping is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. `3000` alone
takes any free host port, a new one at each start; `8080:3000` uses host
port 8080.

## Named ports

The team can name ports in `unibox.yaml` at the root of the env repository
(every env) or beside the sbxenv file (`dev.unibox.yaml` for
`dev.sbxenv.yaml`). `unibox port ls` shows them.

Your own go in `~/.config/unibox/config.yaml`, for every env at the top or
for one env under it. Yours win over the team's:

```yaml
ports:                              # every env
  devtools: {port: '19222:9222', default: off}
envs:
  dev:
    ports:
      api: '8080:8080'              # on at every start
      vite: {port: '15173:5173'}    # the team's vite, on another host port
      storybook: {default: on}      # the team's storybook, on at every start
```

A port with `default: off` is published only by `unibox port on`. A
change is applied at the next `run`, `resume`, `sandbox shell` or
`port on|off`.

Whether a port is on at start is also set for you by a command, or `d`
in the `unibox port` list, which marks such a port `(yours)`:

```sh
unibox port default spring on       # on at each start of the env's sandboxes
unibox port default spring -        # as the team's files say again
```

Order, later wins: `unibox.yaml`, `<env>.unibox.yaml`, your top-level
`ports:`, your `envs.<env>.ports:`.
