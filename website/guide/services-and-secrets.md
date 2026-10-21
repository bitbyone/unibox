# Services and Secrets

A **service** is a forge or another host service the sandbox authenticates
to: a GitHub or GitLab it pushes to, an Artifactory it downloads from,
anything the host signs in to and the agent must reach. Its token stays on
the host; the sandbox sees a placeholder the runtime's proxy swaps.

## Adding a service

```sh
unibox service add github -p web
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
| `artifactory` | placeholder in `ARTIFACTORY_TOKEN` | an identity token from your Artifactory user profile |

Anything else is `--kind generic` with `--host`, and `--git` for a forge or
`--env` for a tool that reads the placeholder from a variable.

```sh
unibox service add gitlab-work -p web --kind gitlab --host gitlab.example.com
unibox service add registry -p web --kind generic --host npm.example.com --env NPM_TOKEN
unibox service ls -p web           # each service, and whether its token is stored
unibox service rm registry -p web  # its token too, unless another profile names it
```

### Flags

| Flag | Meaning |
| --- | --- |
| `--kind` | `github`, `gitlab`, `artifactory` or `generic` |
| `--host` | a host, or `host/path` for one owner on a git forge (repeatable) |
| `--git` | wire the sandbox's git to the hosts (generic services) |
| `--env` | the variable a tool reads the placeholder from: written into the profile's `env` as `NAME: ${services.<name>}` |
| `--scheme` | a git service's header: `basic`, `bearer` or `raw` |
| `--user` | the user a git forge pairs with the token (basic scheme) |
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
unibox service add github -p web                                # github.com
unibox service add github-acme -p web --host github.com/acme    # github.com/acme/...
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
unibox service add github -p web --gh
```

writes a service with `command: secret:github` and has `gh auth token` read
it, in your `vars.toml` under `[secrets.names]` - the profile stays as anyone
can share it. It reaches every owner your gh login can at once - and with it,
every repository and scope that login has. At a terminal with gh signed in
and nothing of the service's own stored, unibox asks whether to use it.

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

### Your own store for `secret:<name>`

Where your tokens live is yours to say, not the profile's. A profile from a
template keeps `secret:<name>`, and a command of yours reads it instead of
the platform's store - `{name}` stands for the secret's name:

```sh
unibox secret source 'op read op://Work/{name}/credential'           # every profile
unibox secret source 'vault kv get -field=token secret/{name}' -p dev  # one profile, over it
unibox secret source -p dev                                            # which one a profile uses
unibox secret source --reset                                           # the platform's store again
```

It is kept in your `~/.config/unibox/vars.toml`, beside your values, so
`sandbox.yaml` stays as the template has it:

```toml
[secrets]
command = "op read op://Work/{name}/credential"

[secrets.names]                 # one secret by its own command, over the above
copilot = "gh auth token"

[profiles.dev.secrets]
command = "vault kv get -field=token secret/{name}"
```

A secret's own command wins, the profile's before the global one; then the
command for every secret, the profile's first; then the platform's store.
`unibox secret source 'gh auth token' --name copilot` sets one.

The runtime runs the command on the host, as it would the store's; unibox
never sees a value. Being yours rather than the profile's, it asks for no
`unibox approve`; a sandbox gets the new source at its next start. With it,
`agent login` and `service add` ask for no token into the platform's store,
and `service ls` shows the token as read by your command.

A token stored this way is put into a profile by a service that names it,
instead of asking for it again - one token can serve several services or
profiles:

```sh
unibox secret set artifactory
unibox service add artifactory -p web --host artifactory.example.com --secret artifactory
```

`unibox service rm` removes a token it stored for the service itself, named
after it, unless another profile names it; a token stored on its own and
named with `--secret` stays until `unibox secret rm`.

## How a tool gets the placeholder

A service holds one token for HTTP requests to its hosts. git signs in
through a git service by itself: unibox puts its placeholder into the
sandbox's git configuration. Any other tool finds the placeholder in a
variable the profile's `env` sets to `${services.<name>}`:

```yaml
services:
  artifactory:
    host: artifactory.example.com
    command: secret:artifactory
env:
  - ARTIFACTORY_TOKEN: ${services.artifactory}
```

The tool sends it in a header as it likes - `Authorization: Bearer`, its own
header, or Basic authentication - and the proxy puts the token in its place
on requests to the service's hosts. A token in a URL or a body, a request
the tool signs with the token, and protocols other than HTTP are not served.

## A build's own credentials

A tool that takes a user and a password, as Gradle's `credentials { username;
password }` does, sends them as Basic authentication. The proxy finds the
placeholder inside it too (checked with sbx), so the password can be the
service's placeholder and the user each person's own value. Gradle reads a
project property from `ORG_GRADLE_PROJECT_<property>`, so for a build that
names `artifactoryUser2N` and `artifactoryPassword2N`:

```yaml
vars:
  artifactory-user: your Artifactory user name
services:
  artifactory-2n:
    host: artifactory.example.com
    command: secret:artifactory-2n
env:
  - ORG_GRADLE_PROJECT_artifactoryUser2N: ${vars.artifactory-user}
  - ORG_GRADLE_PROJECT_artifactoryPassword2N: ${services.artifactory-2n}
```

```sh
unibox secret set artifactory-2n          # the password, or an identity token
unibox var set artifactory-user tobola    # each person their own
```

The repository's URL is the team's, so it can sit in the image's
`~/.gradle/gradle.properties` or be a var too.

## Where a token's command goes

A profile says **what** the sandbox needs: a token for copilot, one for the
github service, each named `secret:<name>`. Where that token comes from - the
keychain, 1Password, your gh login - is **yours**, and may differ between
people and machines. So a command that reads a token goes in one of two
places:

| | your `vars.toml` (the default) | the profile (`--in-profile`) |
| --- | --- | --- |
| In git, with the repository | no - it stays on this machine | yes |
| On another machine of yours | set again there | there already |
| Someone copying your profile with `--from` | gets a clean profile and sets their own | gets your command, which may not work for them |
| A template update | never meets it | merges it, and may conflict |

The rule is one for everyone: **your own store goes in `vars.toml`, a source
everyone reads alike goes in the profile.** A team's Vault, read the same by
every person and in CI, belongs in the profile; your 1Password vault or your
gh login does not. `agent login --command`, `service add --command` and
`--gh` write to `vars.toml`, with the profile naming `secret:<name>`;
`--in-profile` writes the command into the profile instead.

```sh
unibox service add artifactory -p web --host artifactory.example.com --command 'op read op://Me/artifactory/token'
#   profile: command: secret:artifactory   vars.toml: artifactory = "op read op://Me/artifactory/token"
unibox service add vault-git -p web --kind generic --git --host git.example.com --command 'vault kv get -field=token team/git' --in-profile
#   profile: command: vault kv get -field=token team/git
```

This holds whether or not the profile came from a template: an environments
repository can become anyone's template at any time, and a profile kept
clean of your own sources goes with you to another machine as it is.
`unibox template diff` points out a command of your own written into a
profile copied from a template, and how to move it into `vars.toml`.

## Stores unibox does not keep

A profile may name any host command, shared by everyone who uses it - written
by hand, or with `--command --in-profile`:

```yaml
services:
  gitlab:
    git: true
    host: gitlab.example.com
    user: oauth2
    command: vault kv get -field=token secret/ci/gitlab
  artifactory:
    host: [artifactory.example.com, "*.jfrog.io"]
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
unibox service add gitlab -p web --command 'my-token-helper gitlab' --refresh 10m
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
