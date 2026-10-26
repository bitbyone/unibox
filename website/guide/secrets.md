# Secrets

A sandbox signs in to things: a model provider for the agent, a forge to push
to, a registry for images. The tokens for them are sbx's secrets. They are
stored once on your machine, in sbx's store (the keychain on macOS), and
never enter a sandbox: the sandbox sees a placeholder, and sbx's proxy, on
the host, puts the real token in its place on requests to the hosts the
env's bindings name.

unibox never reads a token. It works out which ones an env needs and has sbx
ask you for the missing ones.

## Which an env needs

The env's kits declare the services they take a token for; its bindings say
which hosts each may reach.

```sh
unibox secret ls -e dev
```

```
SERVICE    STORED  NEEDED    FOR
copilot    global  yes       sbx-kit-copilot
github     no      yes       the token git pushes with
npm        no      optional  ../kits/registry
```

`STORED` says whether sbx has it, stored globally or for the env's sandbox
alone, or not. A token a kit marks optional is one the sandbox works without.

The list is what the env's files declare, read through `sbx kit inspect` and
`sbx secret ls`; unibox never sees a value. So it cannot tell that a stored
token has expired or lacks a scope - the request it is put into says that -
and a service the agent calls that no kit nor binding names is not on it.

## Storing them

```sh
unibox secret set -e dev                # each missing required token
unibox secret set -e dev --all          # the optional ones too
unibox secret set github                # one, by name
```

sbx asks for each on your terminal; what you type goes from the keyboard to
sbx, never through unibox, and never into a file or the conversation with an
agent. Away from a terminal, one token can be piped in, which unibox hands
on to sbx unread:

```sh
security find-generic-password -s artifactory -w | unibox secret set artifactory
```

Rather than a value, sbx can keep a command it runs on the host whenever it
needs the token - a keychain read, a password manager, `gh auth token`:

```sh
unibox secret set copilot --command 'security find-generic-password -s copilot -w'
unibox secret set github --command 'gh auth token'
```

A rotated token is then read fresh, and none is copied anywhere.

## Why global

unibox stores every token globally: one secret serves every sandbox that
needs it. A token stored for one sandbox alone goes with that sandbox, so a
recreate would lose it and ask again. `unibox doctor` still counts one you
stored for the env's sandbox with `sbx secret set` yourself.

## Checking

```sh
unibox doctor -e dev
```

fails while a required token is missing and warns about an optional one,
naming the `unibox secret set` that fixes it. `unibox init` asks for the
missing ones as one of its steps.

The tokens are sbx's to manage further: `sbx secret ls` lists them, `sbx
secret rm` removes one. `unibox destroy --all` leaves them in place.
