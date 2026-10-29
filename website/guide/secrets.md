# Secrets

Every token a sandbox uses - the agent's login, the forge, a registry - is
an sbx secret, kept by sbx on your machine and put into requests by its
proxy; a sandbox sees placeholders only. unibox adds which tokens an env
needs, read from its kits, and where each one is made. It never sees a
token's value: sbx asks for it.

## See what is missing

```sh
unibox secret ls
```

```term
$ unibox secret ls
SERVICE    STORED  NEEDED    FOR
anthropic  global  yes       the Claude API key
github     no      yes       the token git pushes with
npm        no      optional  the registry's token
```

## Store them

```sh
unibox secret set               # sbx asks for each missing token
unibox secret set --all         # the optional ones too
unibox secret set github        # one, by name
unibox secret set copilot -g    # open the page where a new one is made first
unibox secret                   # the same as a list
```

sbx asks on your terminal; the value goes from your keyboard to sbx and
never through unibox. Add `-e dev` for another env than the default.

Before sbx asks, unibox offers *I have a token* or *Generate a new token*
when it knows where the token is made: GitHub's fine-grained token page
for `github` and `copilot`, filled in with the permissions they need, and
any page the env's team names. A new token opens that page in your
browser; make it there and paste it at sbx's prompt.

```term
$ unibox secret set github
github: the token git pushes with
[blue]?[/] [b]The github token[/]  Generate a new token
  the browser opens https://github.com/settings/personal-access-tokens/new?name=...; make the token there, then paste it here
[dim](sbx's prompt for the value)[/]
```

The team names its own pages under `secrets:` in `unibox.yaml`:

```yaml
secrets:
  gitlab:
    page: https://gitlab.acme.com/-/user_settings/personal_access_tokens?scopes=read_repository,write_repository
    hint: a GitLab token with read_repository and write_repository, nothing else
```

A token made more ways than one lists them under `ways:`, and unibox asks
which. A way with a `command` names what to run on your machine; unibox
runs nothing itself, and you paste what the command prints at sbx's
prompt:

```yaml
secrets:
  anthropic:
    ways:
      - name: Claude subscription
        command: claude setup-token
        hint: it signs you in and prints a token
      - name: API key
        page: https://platform.claude.com/settings/keys
```

```term
$ unibox secret set anthropic
[blue]?[/] [b]The anthropic token[/]  Claude subscription
  it signs you in and prints a token
  run claude setup-token in another terminal, then paste the token it prints here
[dim](sbx's prompt for the value)[/]
```

To keep a token in another store, give sbx a command it runs on the host
whenever it needs the value:

```sh
unibox secret set github --command 'gh auth token'
unibox secret set npm --command 'security find-generic-password -s npm -w'
```

A rotated token is then picked up without storing it again. sbx runs the
command from a temporary directory with no terminal, so use a program on
your `PATH` or an absolute path, never one inside a mounted repository.

## A subscription or an API key

For Claude and Codex unibox asks which you have:

```term
$ unibox secret set anthropic openai
anthropic: Anthropic API access (API key or claude.ai OAuth)
[blue]?[/] [b]The anthropic token[/]  Claude subscription
  Claude Code signs in with your subscription itself: run /login in it the first time it starts; sbx keeps the sign-in
openai: OpenAI API access (API key or ChatGPT OAuth)
[blue]?[/] [b]The openai token[/]  ChatGPT subscription
  sbx signs you in with your ChatGPT account, in the browser
sbx has stored openai
```

An API key is typed at sbx's prompt. A ChatGPT subscription is signed in
by `sbx secret set openai --oauth`, which unibox runs. A Claude
subscription is signed in from Claude Code, with `/login`: Docker's Claude
kit manages api.anthropic.com and keeps the sign-in through sbx's proxy.
`unibox secret rm <service>` has sbx forget a service's token.

## A token of an env's or a sandbox's own

Tokens are global unless told otherwise: every sandbox takes the same. A
token can instead be one env's sandboxes' own - a second Copilot account
for work - or one sandbox's own, over its env's: a personal sandbox with
your own GitHub token beside the team's. The narrowest wins: the
sandbox's, then the env's, then the global one.

```sh
unibox secret set github -s personal --scope sandbox     # typed into the keychain
unibox secret set copilot -e my2n-dev --scope env --command 'pass copilot-2n'
unibox secret rm github -s personal --scope sandbox      # the env's or global one again
```

Typed in, the token goes to your system's keychain, asked for by its own
program on your terminal, never through unibox: `security` and the login
keychain on macOS, `secret-tool` and the Secret Service (GNOME Keyring,
KWallet) on Linux - `libsecret-tools` on Debian and Ubuntu - and on
Windows PowerShell, which encrypts it for your account alone (DPAPI). sbx
is given the command that reads it, so `--command` with a command of
your own works the same.

The command goes to `config.yaml`, under `envs.<env>.secrets` or
`sandboxes.<sandbox>.secrets`, where it can be edited by hand. unibox has
sbx keep it as the token of each sandbox it is for, set again at every
start: a recreate takes a sandbox's own tokens away, and a changed
command is applied. Removed with `rm --scope`, or with the sandbox, what
unibox stored in the keychain goes too.

In `unibox secret` at a terminal, `s` moves the token under the cursor:

```term
$ unibox secret -s personal
[dim]...[/]
[blue]?[/] [b]Where sandbox personal takes the github token from[/]  sandbox personal: its own, over its env's
[blue]?[/] [b]How the github token of sandbox personal is kept[/]  typed in; it goes into the macOS keychain
the macOS keychain asks for the github token of sandbox personal; it goes there, not through unibox
[dim](security's prompt for the value)[/]
```

Moved back to global, a token sbx has no global one of is asked for at
once.

## Which sandbox takes which

`unibox secret --all` lists every token kept, in every scope, with the
sandboxes that take it - and those a sandbox needs that are kept nowhere:

```term
$ unibox secret --all
SERVICE    SCOPE             KEPT      USED BY
anthropic  global            sbx       personal, team
github     global            sbx       team
github     sandbox personal  keychain  personal
gitlab     -                 missing   team (needed)
npm        global            sbx       -
```

A sandbox an env would make and has not yet is named with `(not made)`.
`--json` gives the same for a program.

## One name, two hosts

sbx keeps one token a service name. If two teams' kits both name a
service `gitlab` for different GitLab servers, the token stored is sent to
both. `unibox doctor` says so:

```term
$ unibox doctor
[dim]...[/]
[b]Tokens[/]
  [warn]![/] secret gitlab  env dev sends it to gitlab.acme.com; env web sends it to gitlab.example.org; sbx keeps one token for all of them
                  [dim]→ have one team's kit name its service apart, such as gitlab-<team>[/]
```

## Signing an agent in

The agent's login is one of the env's secrets, named by its kit -
`unibox secret ls` shows which. Store it like any other: with a token
from the provider, or a `--command` that prints one. The kit decides which
kinds of login it accepts; the env repository's README should say which
token to create.

`unibox secret set` asks for a token. To sign codex in with your ChatGPT
account instead, run sbx's browser login: `sbx secret set openai --oauth`.

## Good tokens

- Give the forge token the least it needs: the repositories you work on,
  contents write, no workflow scope.
- Protect `main` and release branches on the forge. The agent can push
  wherever the token can.

Tokens are sbx's to manage further: `sbx secret ls` lists them, `sbx
secret rm` removes one.
