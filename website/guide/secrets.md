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

## Signing in with a subscription

A kit may let its service be signed in to with a subscription instead of a
token - Claude's, ChatGPT's for Codex. unibox then asks which:

```term
$ unibox secret set openai
openai: OpenAI API access
[blue]?[/] [b]The openai token[/]  Sign in with my subscription
[dim](sbx signs you in, in the browser)[/]
sbx has stored openai
```

sbx 0.49 signs in to `openai` from the host. Another service's agent signs
in itself at its first start, with `/login`, and sbx keeps that sign-in:
`unibox secret ls` says `sign-in` for it, and nothing is asked before.

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
