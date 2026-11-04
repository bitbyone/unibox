# Installation

## Requirements

- macOS, Linux or Windows
- [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`)
  0.50.0 or newer; `unibox init` signs it in to Docker if it is not
- git
- the address of your team's repository of sbxenv files and kits

## Install unibox

Homebrew, from the tap in unibox's repository; it installs the release's
binary for your system, so it needs no Go:

```sh
brew tap bitbyone/unibox https://github.com/bitbyone/unibox
brew trust bitbyone/unibox     # Homebrew loads a third-party tap only once trusted
brew install unibox
```

Scoop, on Windows:

```powershell
scoop bucket add unibox https://github.com/bitbyone/unibox
scoop install unibox
```

A release archive from
[Releases](https://github.com/bitbyone/unibox/releases) - Linux, macOS and
Windows, amd64 and arm64 - holds one binary to put on your `PATH`.

From source, with Go 1.26 or newer:

```sh
git clone https://github.com/bitbyone/unibox
cd unibox && make install     # into ~/.local/bin
```

On Windows that is `%USERPROFILE%\.local\bin\unibox.exe`; `make` there
needs a POSIX `sh` on `PATH`, such as Git's. Put the directory on your `PATH` ahead of Scoop's, or
`make install` says which unibox runs instead.

```term
$ unibox version
0.2
```

Upgrade with `brew upgrade unibox` or `scoop update unibox`. `make
uninstall` removes a binary `make install` put there, as when moving to
brew; your config, state and sandboxes stay.

## Nightly

A nightly is a prerelease, tagged `vX.Y-pre.N` when something is to be
tried before the next release. It has a formula and a manifest of its own,
`unibox-nightly`, which install the same `unibox` command, so one channel
is taken out before the other goes in:

```sh
brew uninstall unibox && brew install unibox-nightly
scoop uninstall unibox; scoop install unibox-nightly
```

`brew upgrade` and `scoop update` then take the next nightly, and the way
back to stable is the same the other way round. Your config, state and
sandboxes stay either way.

## Set it up

```sh
unibox init https://github.com/acme/agent-sandboxes
```

[Setting up: init](/guide/init) describes what it asks. Run it again at
any time; it adds what is new and skips what is done.
