# Secrets & Agents

Every token a sandbox uses - the agent's login, the forge, a registry - is
an sbx secret, stored once on your machine and shared by every sandbox
that needs it. The env's kits say which tokens it needs.

## See what is missing

```sh
unibox secret ls
```

```
SERVICE    STORED  NEEDED    FOR
anthropic  global  yes       sbx-kit-claude
github     no      yes       the token git pushes with
npm        no      optional  ../kits/registry
```

## Store them

```sh
unibox secret set               # sbx asks for each missing token
unibox secret set --all         # the optional ones too
unibox secret set github        # one, by name
unibox secret set copilot -g    # open the page where a new one is made first
unibox secret                   # a list: enter stores, g generates, c stores a command
```

sbx asks on your terminal; the value goes from your keyboard to sbx and
never through unibox. Add `-e dev` for another env than the default.

Before sbx asks, unibox offers *I have a token* or *Generate a new token*
when it knows where the token is made: GitHub's fine-grained token page
for `github` and `copilot`, filled in with the permissions they need, and
any page the env's team names. A new token opens that page in your
browser; make it there and paste it at sbx's prompt.

To keep a token in another store, give sbx a command it runs on the host
whenever it needs the value:

```sh
unibox secret set github --command 'gh auth token'
unibox secret set npm --command 'security find-generic-password -s npm -w'
```

A rotated token is then picked up without storing it again. sbx runs the
command from a temporary directory with no terminal, so use a program on
your `PATH` or an absolute path, never one inside a mounted repository.

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
