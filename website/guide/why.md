# What unibox Is

unibox is a thin extension of Docker Sandboxes (`sbx`). sbx makes the
sandbox, isolates it, keeps the tokens on the host and enforces the network
policy; a team describes its sandbox in `*.sbxenv.yaml` files and kits, which
sbx reads. unibox has no format of its own. It takes any sbxenv file as it is
and adds only what sbx does not do:

- **Your own values.** An sbxenv file shared by a team declares args each
  person sets - a user name, a mirror, an amount of memory. unibox keeps
  yours in `vars.toml` and hands them to sbx at every create, so nobody types
  them twice.
- **Logins.** It reads which tokens the env's kits and bindings need, says
  which sbx has, and has sbx ask for the missing ones on your terminal.
- **Tasks.** Your repositories, mounted into the sandbox or cloned inside it,
  with an agent started in them and a record kept, so a task can be resumed,
  grown and removed safely.
- **Named ports.** Your own mapping of the sandbox's ports to the host's,
  published and taken back by name.
- **Doctor and drift.** It says what is missing, and when the env changed
  since its sandbox was made, so you can recreate it.

That is all it is. It is meant for local development, not for running
agents for other people, in CI or in the cloud. Anyone who would rather not
use it creates the same sandbox with `sbx env create` from the same files.

A coding agent is most useful when it may do anything: install a package,
run the tests, start a database. Doing that on your own machine puts your
home directory, your SSH keys and your logins within its reach; a sandbox
keeps them out, and unibox makes working in one quick.

## Why the team's files and not a format of unibox's

A sandbox is the team's business: which image, which tools, which hosts it
may reach, which services it signs in to. sbx already has the words for that
in sbxenv files and kits, and they are reviewed and versioned in the team's
repository like any other code. A format of unibox's on top would be a second
place to say the same things, one only unibox users could read. So unibox
reads the little it needs - the args, the kits, the bindings, the name - and
leaves every other field to sbx, which also means a newer sbx adding fields
does not break it.

What is the person's and not the team's stays out of the team's files:
values in `vars.toml`, ports in unibox's `config.yaml`, tokens in sbx's
store.

## The threat model, in plain words

Assume the agent may be wrong, confused or steered by something it read. What
can it reach?

**Not your machine.** sbx runs it in a micro-VM with its own kernel, its own
disk and its own Docker Engine. Your home, your keys and your other projects
are not there.

**Not your tokens.** Logins stay in sbx's store on the host. The sandbox gets
placeholders; sbx's proxy, on the host, puts the real value in place of the
placeholder on requests to the hosts a binding names. An agent that prints
its environment prints a placeholder.

**Not every host on the internet.** sbx's network policy refuses what it does
not list; `sbx policy` changes it.

**What you mount, it can change.** A task mounts your repository by default,
so the agent writes your files and your `.git` directly. A `.git` holds
commands your machine runs later - a hook, `core.fsmonitor`, a filter - and
editors, git GUIs and your shell prompt run git there all the time. unibox
says so once, the first time it mounts a directory. `--clone` keeps the agent
out of your checkout: it works on a clone of its own, and what comes back to
you is a `git fetch` from the forge, which carries objects and refs, never
config or hooks. [Mount or Clone](./mount) weighs the two.

**What it can reach is the forge**, with the token the env gives the
sandbox. It can push to any branch that token may push to, force-push and
delete. So the forge's branch protection is the real boundary: protect `main`
and release branches, and issue the token narrow.

::: tip
[Security](/reference/security) lists exactly what crosses between the host
and the sandbox.
:::

## Why a wrapper at all

sbx can be used directly, and the files are made for that. unibox saves the
steps around it that each person would otherwise repeat: the args typed at
every create, finding out which secrets a kit wants, mounting the right
directory at the right place, trusting it in the agent so it does not stop
to ask, keeping track of which sandbox holds which work, and noticing that a
teammate's change to the env needs a recreate. It also gives other programs,
such as [unagit](./unagit), a JSON contract to drive all of that.
