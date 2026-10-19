# The sbx Runtime

[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) is unibox's
supported runtime: one sbx sandbox is one micro-VM with its own kernel, its
own Docker Engine and its own disk, persistent until removed. The agent runs
in the VM as a non-root user with sudo.

This page collects what unibox relies on, as verified against sbx 0.47 on
macOS (Apple Silicon) on throwaway sandboxes. Where sbx's behaviour shows
through, it says so.

::: tip Before you start
`sbx login` once. `unibox runtime ls` shows whether sbx is installed and
available, and what it isolates.
:::

## How a profile becomes sbx

| Profile | sbx |
| --- | --- |
| `sandbox.dockerfile` | `docker build`, `docker save`, `sbx template load`; the template `unibox/<profile>:<tree>` |
| `image` | the template named |
| `resources` | `--cpus`, `--memory`; a share resolved by the adapter |
| `network.deny` | `--deny-network` at creation; `sbx policy` live |
| `network.allow` | `sbx policy allow network --sandbox`, live |
| `network.ports` | `sbx ports --publish` / `--unpublish`, live |
| `env` | `-e NAME` |
| `caches` | the host directory read-only as an extra workspace, and its path in the named variable |
| `start` | run by `sbx exec` when the VM's boot id is new |
| an agent session | `sbx exec -it -w /work/<task> <sandbox> <agent command>` |

unibox names its sbx sandboxes `unibox-<sandbox>`, and stops or removes only
those.

## Templates and kits

sbx has two words for shaping a sandbox:

- **A template** is the image a sandbox is made from, in sbx's own image
  store, apart from Docker's. A profile's Dockerfile becomes one: built,
  saved and loaded, tagged by the profile's tree.
- **A kit** is a declarative package laid over a sandbox: files, hooks,
  network rules, credential bindings. sbx's built-in agents are kits.

A profile with one agent uses that agent's built-in kind. A profile with
**several agents** is made from a generated **v3 kit**: a local kit directory
whose descriptor declares each agent's credentials, beside a Dockerfile that
is just `FROM` the profile's built image. unibox writes it under
`~/.cache/unibox/sbx/kits/`.

Facts that design turns on:

- **v3 kits cannot mix with v2 built-ins**, so several agents means a v3
  workload created from a local directory.
- **A kit's credential is injected only with a binding** in
  `~/.config/sbx/credentials.yaml`. sbx asks no question at creation, so
  unibox writes the bindings itself when it creates a kit sandbox - each
  agent's service with exactly the domains its declaration names - added and
  never removed.
- **sbx keeps one OAuth policy per sandbox**, taken from the first credential
  declaring OAuth. So the order of a profile's agents matters, two `oauth`
  agents are refused, and the kit declares first the services sbx holds an
  OAuth login for.
- **A host an agent calls that the kit does not declare gets the placeholder
  as it is.** Copilot calls its plan's API host - `api.individual`,
  `.business` or `.enterprise.githubcopilot.com` - besides
  `api.githubcopilot.com`, so the kit routes all of them.
- sbx ignores `apiKey.inject[].scheme` in v3 kits, which is why git over https
  is signed in by an `extraHeader` placeholder instead (below).

## Logins

sbx keeps the agents' logins on the host and its proxy signs requests to the
providers with them; the key never enters the VM.

- sbx stores locally only ChatGPT's OAuth login (codex). Claude's OAuth is a
  sign-in inside the sandbox, so claude beside another agent signs in by a
  long-lived `token` from `claude setup-token`: sbx stores the host command as
  a custom secret, claude sees a placeholder in `CLAUDE_CODE_OAUTH_TOKEN`, and
  the login outlives a recreated sandbox.
- Copilot reads its token from `COPILOT_GITHUB_TOKEN` and accepts a
  placeholder of the token's shape there, so it signs in without sbx's
  built-in `github` secret.
- **sbx's built-in `github` secret takes github.com over**: with it stored,
  the proxy sets `Authorization` on every request to github.com and
  overwrites any the sandbox sent. A git service on github.com never reaches
  GitHub while it is stored.

## Custom secrets and git

Services become sbx custom secrets, scoped to the sandbox:

- **A local custom secret substitutes its placeholder verbatim** in request
  headers, on requests to its hosts, and nowhere else.
- So git cannot use its own credential: it base64-encodes `user:password`,
  and the placeholder would never appear. unibox sets
  `http.https://<host>/.extraHeader` to `Authorization: <placeholder>`, and
  the stored value is the whole header, composed on the host.
- `http.<url>.extraHeader` is a list gathered from every matching URL, and an
  empty value resets it. unibox writes it so the most specific URL's header
  alone is sent - which is how one forge carries several owners' tokens.
- **A command secret is stored as its source**: the sbx daemon runs the command
  on the host at verification and whenever it refreshes, from a fresh
  temporary directory, with no terminal. A custom secret refreshes on demand
  unless a refresh interval is given.
- **Private hosts work.** The proxy resolves and connects from the host, so a
  host behind your VPN is reached once allowed.

## Network policy, live

- sbx refuses what its policy does not list (403 from the proxy) and already
  allows the usual registries.
- `sbx policy allow network --sandbox` and `sbx policy deny network
  --sandbox` change a running sandbox's rules at once, and removing a rule by
  id undoes either - no restart.
- sbx refuses a deny for a host an allow rule of the sandbox names exactly;
  the allow goes first. unibox applies the difference and removes only rules
  it added.

## Ports

- `sbx ports` publishes a port live on the host's loopback, tcp4 when no
  protocol is named. A host port already taken is refused.
- Published ports survive a stop: listed as none while stopped, back at the
  start, an ephemeral one with a new host port.
- `--unpublish` takes a port back only by its exact host port. So unibox
  records what it published by name and mapping, and leaves a port it did not
  publish alone.
- A start that finds a sandbox stopped drops the switches and publishes the
  profile's defaults; a switch on a stopped sandbox is refused, since
  publishing would start it.

## Mounts

`sbx mount HOST:TARGET` mounts into a running sandbox: live, writable, kept
across a stop, taken out by `sbx umount` leaving an empty mount point.
Removing the sandbox leaves the host's files alone. A recreated sandbox has
no mounts.

## Resources

- `--cpus 0` is all host CPUs; CPUs are shared by time, so all of them takes
  nothing from the host's own work but priority.
- **sbx refuses more than 75% of the host's memory.** unibox caps a share
  there and says so, rather than fail a start.

## Images

- `sbx template rm --force` removes a template a sandbox was made from, and
  the sandbox starts and runs as before: it needs its image only to be made.
  So a build removes the profile's older images, and making a sandbox removes
  the older images sbx made of its kit (`sbx-kit-src:...`), the newest kept
  for a recreation to reuse.

## Lifecycle

- `sbx exec` starts a stopped VM. A VM can be started by something other than
  unibox, so `start` is keyed to the kernel's boot id, not to unibox's own
  starts.
- unibox stops a sandbox when the last agent it started there ends, unless
  `runtimes.sbx.stop_idle` is `false`. The disk, caches and tasks stay.
