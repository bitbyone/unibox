# Setting Up: init

`unibox init` takes a person from an installed sbx to a sandbox ready for
work, from the team's repository of sbxenv files and kits. Everything it
sets up is also a command of its own, which `init` runs; `unibox doctor`
checks the same things afterwards.

## Where it starts

```sh
unibox init https://github.com/acme/agent-sandboxes   # cloned with your own git
unibox init ~/code/agent-sandboxes                      # a checkout of yours, used in place
cd ~/code/agent-sandboxes && unibox init                # the current repository
```

An address is cloned with your git and credentials into
`~/.local/share/unibox/repos/<host>/<path>`, which `unibox env update`
keeps current. A checkout of yours is pulled only while it is clean and
follows a remote branch.

## The five steps

```term ~
$ unibox init https://github.com/acme/agent-sandboxes
cloned https://github.com/acme/agent-sandboxes into /Users/jdoe/.local/share/unibox/repos/github.com/acme/agent-sandboxes

[b]1/5 sbx[/]
[blue]?[/] [b]SSH agent in sandboxes[/]  No - agents reach git through the services' tokens alone (recommended)
[blue]?[/] [b]Editors over SSH[/]  Yes - editors connect over SSH
[blue]?[/] [b]Host commands[/]  No - ask each time (recommended)
  installed and signed in
[blue]?[/] [b]The network policy every sandbox starts from[/]  balanced - the AI services and package registries of typical development (recommended)

[b]2/5 Envs[/]
[blue]?[/] [b]Envs to set up[/]  [ok]✓[/] sandboxes/dev.sbxenv.yaml
  env dev, sandboxes/dev.sbxenv.yaml: added
[blue]?[/] [b]Claude remote control[/]  Yes - allow /remote-control

[b]3/5 Values[/]
env dev asks for values of your own, kept for it in vars.toml:
[blue]?[/] [b]registryUser[/]  [dim]Your user name at the package registry[/]  jdoe

[b]4/5 Secrets[/]
  sbx has no token for anthropic, github of env dev
  sbx asks for each on this terminal, and the value goes to it alone
[blue]?[/] [b]Store them now?[/]  Yes
anthropic: the Claude API key
[dim](sbx's prompt for the value)[/]
github: the token git pushes with
[blue]?[/] [b]The github token[/]  Generate a new token
  the browser opens https://github.com/settings/personal-access-tokens/new?name=...; make the token there, then paste it here
[dim](sbx's prompt for the value)[/]

[b]5/5 Sandboxes[/]
[blue]?[/] [b]Create sandbox dev of env dev now? sbx shows what it makes and asks to approve it[/]  Yes
creating sandbox dev from env dev
[dim]... sbx shows its plan and asks to approve the env and its host commands ...[/]

[ok]set up; start a task with unibox run <directory>, or unibox run --shell for a shell[/]
```

1. **sbx** - checks that sbx is installed and signed in to Docker, and
   offers `sbx login` when it is not. Then three questions of yours: whether
   sandboxes may use your SSH agent (sbx forwards it by default, which lets
   an agent sign with every key you have), whether editors such as VS
   Code or Cursor may reach a sandbox over SSH, and whether sbx may run an
   env's host commands you approved once again without asking. Once per machine it asks
   for the [network policy](./network) every sandbox starts from.
2. **Envs** - offers every sbxenv file of the repository. Each one picked
   becomes an env, named as the file names its sandbox. With several, one
   becomes the default. When an env runs Claude Code, it asks whether its
   `/remote-control` may work in sandboxes.
3. **Values** - asks for the args the env declares for each person, such
   as a registry user name, by the description the team wrote. They go to
   `~/.config/unibox/vars.toml`, never into the team's repository.
4. **Secrets** - reads which tokens the env's kits need and which sbx has.
   Where the team or unibox knows the page that makes a token, it offers
   to open it. The value goes from your keyboard to sbx alone; see
   [Secrets](./secrets).
5. **Sandboxes** - creates the env's sandbox with `sbx env create`. sbx
   shows what it will make and asks you to approve the env and its host
   commands.

## Running it again

`init` is safe to run any number of times. A step that is done says so and
moves on; a new sbxenv file, a value the env now asks for or a token not
yet stored is picked up.

```term ~
$ unibox init
[b]1/5 sbx[/]
  installed and signed in

[b]2/5 Envs[/]
  env dev, sandboxes/dev.sbxenv.yaml: set up already

[b]3/5 Values[/]
  env dev has every value it needs

[b]4/5 Secrets[/]
  sbx has every token env dev needs

[b]5/5 Sandboxes[/]
  sandbox dev is there

[ok]set up; start a task with unibox run <directory>, or unibox run --shell for a shell[/]
```

## Several teams

Run `init` once for each team's repository. The envs live side by side,
and `unibox env ls` says which repository each comes from:

```term
$ unibox env ls
ENV            SANDBOX  FILES  REPO
dev (default)  dev      1      github.com/acme/agent-sandboxes
web            web-dev  1      gitlab.example.org/web/agent-sandboxes
```

When a second team's env or sandbox has a name taken already, `init` asks
for another, and the env keeps it. Without a terminal it names the command
that does the same: `unibox env add web <file> --name web-dev`.

## Without a terminal

With `--no-input`, or without a terminal, `init` asks nothing. What it
would ask comes from flags, and a missing answer stops it with the flag to
give:

```sh
unibox init https://github.com/acme/agent-sandboxes --no-input \
  --file sandboxes/dev.sbxenv.yaml --var registryUser=jdoe
```

Tokens are not among the flags: store them with `unibox secret set`, which
hands them to sbx on its standard input.
