# The msb Runtime

[microsandbox](https://microsandbox.dev) is unibox's built-in runtime: a new
sandbox goes to it when sbx is not installed, and unibox installs it - msb
and its kernel library, into `~/.microsandbox` - the first time it is
needed. It runs on Apple silicon Macs and on Linux with KVM. What follows was
verified against msb 0.7.8.

## What a profile becomes

unibox writes a configuration for each sandbox,
`~/.cache/unibox/msb/<sandbox>/sandbox.yaml`, and makes the sandbox with
`msb create --conf`:

| Profile | msb |
| --- | --- |
| the image | built with Docker and loaded with `msb image load` |
| `resources` | `cpus`, `memory`; the root disk may grow to 64 GiB |
| `network.allow`, `network.deny` | an allow list; deny wins |
| `network.ports` | published on the host's loopback when the sandbox is made |
| `env`, `caches` | the environment; caches mounted read-only at their paths |
| agents' `auth`, `services` | secrets: a placeholder in the sandbox, the value put in by msb's proxy |

## Secrets

msb keeps a secret's source, never its value: it reads a host variable when
it boots the sandbox. unibox boots it under a shell that fills each variable
with the output of the secret's command - `secret:<name>` from the
platform's store, or the profile's own command - so a value goes from the
command to msb and never through unibox. A sandbox does not start without
its secrets.

Before each session on a running sandbox unibox fetches the tokens again and
hands them to msb, which takes them without a restart; a rotated token
reaches the sandbox at the next session.

msb runs no OAuth flow and keeps no logins, so `oauth` is refused: claude
signs in by `token` (`claude setup-token`), codex by an API key, copilot by
a GitHub token.

## Network

msb would let a sandbox reach the whole internet. unibox gives it an allow
list instead: what sbx's default policy allows (AI services, package
registries, forges, container registries, certificate checks), the hosts of
the profile's services and its `allow`. HTTPS is intercepted, as secrets and
host rules need it; msb's certificate authority is trusted in the sandbox,
and node, git, curl and codex work through it.

## What needs a recreation

msb sets network rules, ports, mounts and secrets when it makes a sandbox
and cannot change them on a running one. A change of rules or ports is
offered as a recreation; `unibox port <sandbox> name:on` cannot switch a
port, and `--mount` is refused. A token that changes does not need one.

## Files

| Path | What it is |
| --- | --- |
| `~/.cache/unibox/msb/<sandbox>/sandbox.yaml` | the configuration the sandbox was made from |
| `~/.cache/unibox/msb/<sandbox>/secrets.json` | the commands that fetch its secrets |
| `~/.microsandbox/` | the runtime, its images and sandboxes |
| `~/.microsandbox/sandboxes/<name>/logs/` | msb's logs, including the output of commands run in the sandbox |
