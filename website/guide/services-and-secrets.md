# Services and Secrets

A **service** is a forge or another host service the sandbox authenticates
to: a GitHub or GitLab it pushes to, an Artifactory it downloads from,
anything the host signs in to and the agent must reach. Its token stays on
the host; the sandbox sees a placeholder the runtime's proxy swaps.

## Adding a service

```sh
unibox service add web github
```

shows where the token is made and what to grant, asks for it on a hidden
prompt, keeps it in the platform's store and writes the profile's `services`
entry:

```yaml
services:
  github: {git: true, host: github.com, user: x-access-token, command: secret:github}
```

unibox knows three kinds, from the name or from `--kind`:

| Kind | Defaults | The token |
| --- | --- | --- |
| `github` | git on `github.com`, user `x-access-token` | a fine-grained token with Contents read and write, or a classic one with `repo`; leave `workflow` unticked so the agent cannot change GitHub Actions |
| `gitlab` | git on `gitlab.com`, user `oauth2` | a personal access token with `read_repository` and `write_repository`, or a project or group access token |
| `artifactory` | scheme `bearer`, placeholder in `ARTIFACTORY_TOKEN` | an identity token from your Artifactory user profile |

Anything else is `--kind generic` with `--host` and either `--git` or `--env`.

```sh
unibox service add web gitlab-work --kind gitlab --host gitlab.example.com
unibox service add web registry --kind generic --host npm.example.com --env NPM_TOKEN --scheme bearer
unibox service ls web          # each service, and whether its token is stored
unibox service rm web registry # its token too, unless another profile names it
```

### Flags

| Flag | Meaning |
| --- | --- |
| `--kind` | `github`, `gitlab`, `artifactory` or `generic` |
| `--host` | a host, or `host/path` for one owner on a git forge (repeatable) |
| `--git` | wire the sandbox's git to the hosts (generic services) |
| `--env` | the variable a tool reads the placeholder from, instead of git |
| `--scheme` | `basic`, `bearer` or `raw` |
| `--user` | the user the forge pairs with the token (basic scheme) |
| `--author` | who commits under the service, as `Name <email>`; `--author` on a run wins |
| `--no-co-author` | no `Co-Authored-By` trailer for the agent in its repositories |
| `--command` | a host command printing the token, for a store unibox does not keep |
| `--gh` | sign a `github` service in through your gh login |
| `--refresh` | how often the runtime fetches the token again, e.g. `10m` |
| `--renew` | ask for the token again, replacing the stored one |

`service rm --keep-secret` leaves the stored token in place.

## How git is signed in

The proxy replaces a placeholder with the stored value, verbatim, on requests
to the service's hosts. Git's own credential would not do: git base64-encodes
`user:password`, so a placeholder password never appears in the header for the
proxy to find. So unibox writes an `http.<url>.extraHeader` into the
sandbox's git configuration carrying a placeholder for the whole
`Authorization` header, and stores the header composed on the host. The token
and its base64 never leave the host, and it works the same on any forge.

## Several owners on one forge

A fine-grained GitHub token covers one resource owner - you, or one
organization. Repositories of several owners need a token each, told apart by
path:

```sh
unibox service add web github                                   # github.com
unibox service add web github-acme --host github.com/acme       # github.com/acme/...
```

The most specific URL wins for each repository. One classic token for every
owner stays a single service on the bare host.

## Who commits

A git service may name the author and the co-author for the repositories under
its URLs:

```yaml
services:
  gitlab-work:
    git: true
    host: gitlab.example.com
    user: oauth2
    command: secret:gitlab-work
    author: Jane Doe <jane@example.com>
    co_author: false
```

The service's author is the default over your host git configuration; an
explicit `--author` on a run still wins. `co_author` wins over the profile's
`git.co_author`. Under several services, each field comes from the most
specific URL that sets it. A clone the agent makes itself in the sandbox gets
its service's author too.

## Your gh login

```sh
unibox service add web github --gh
```

writes a service with `command: gh auth token`. It reaches every owner your gh
login can at once - and with it, every repository and scope that login has.
At a terminal with gh signed in and nothing of the service's own stored,
unibox asks whether to use it.

## Secrets

`unibox secret` keeps any token in the platform's store - the login keychain
on macOS (`security`), the Secret Service on Linux (`secret-tool`) - for a
profile to name as `secret:<name>`:

```sh
unibox secret set artifactory     # hidden prompt, or read from stdin
unibox secret ls                  # names only, never values
unibox secret rm artifactory
```

The store stays the system's, so you see, rotate and delete a token with the
system's tools too. Within unibox a value passes through memory alone: never
a command's arguments, the log, `--json`, the state or the profile.

`secret:<name>` is portable: unibox turns it into the host command of the
platform it runs on, so a profile is shared between macOS and Linux.

A token stored this way is put into a profile by a service that names it,
instead of asking for it again - one token can serve several services or
profiles:

```sh
unibox secret set artifactory
unibox service add web artifactory --host artifactory.example.com --secret artifactory
```

`unibox service rm` removes a token it stored for the service itself, named
after it, unless another profile names it; a token stored on its own and
named with `--secret` stays until `unibox secret rm`.

## Stores unibox does not keep

A profile may name any host command instead - written by hand, or with
`--command`:

```yaml
services:
  gitlab:
    git: true
    host: gitlab.example.com
    user: oauth2
    command: vault kv get -field=token secret/ci/gitlab
  artifactory:
    host: [artifactory.example.com, "*.jfrog.io"]
    scheme: bearer
    env: ARTIFACTORY_TOKEN
    command: op read op://vault/artifactory/token
```

## Refreshing

The runtime stores a command as its source, not its value: it runs the
command on the host when the secret is verified and again whenever it
refreshes - on demand by default, or every `refresh` when the service sets
one. So the command runs outside the process that created the sandbox, at any
hour, with no terminal: it must reach its secret alone - a keychain, an agent
socket - not an environment variable of your shell. A short-lived token fits
well with a `refresh` shorter than its lifetime.

```sh
unibox service add web gitlab --command 'my-token-helper gitlab' --refresh 10m
```

## A host behind your VPN

The proxy resolves and connects from the host, so the host's VPN routes it.
A service's hosts join the sandbox's allow policy, so nothing more is needed
but the VPN being up when the sandbox reaches the host.

::: warning The token is the boundary
Issue it narrow - a GitLab project token, a GitHub fine-grained token on the
repositories concerned - and protect `main` and release branches on the
forge. The agent can push to any branch the token may push to.
:::
