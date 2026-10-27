# Network

A sandbox reaches only the hosts sbx allows by default, the hosts the env's
kits add, and the rules you add for that sandbox.

## The policy every sandbox starts from

sbx creates no sandbox until you choose, once on this machine, what every
sandbox may reach to begin with. `unibox init` asks, and so does the first
`unibox sandbox create` or `unibox run` at a terminal:

```sh
unibox network init             # asks
unibox network init balanced    # or allow-all, deny-all
```

`balanced`, recommended, lets a sandbox reach the AI services and package
registries of typical development; `allow-all` every host; `deny-all` only
what the env's kits and your rules allow. The rules below are added to it.

## Something is blocked

```sh
unibox network ls                    # your rules, and hosts refused lately
unibox network allow jitpack.io
unibox network allow '*.npmjs.org' registry.npmjs.org
unibox network allow stun.example.com:3478 --protocol udp
```

`network ls` lists the hosts the sandbox was refused recently, which is
usually the quickest way to see what to allow. `--all` adds the env's and
sbx's default hosts.

## Block or remove

```sh
unibox network deny gist.github.com  # even if the env allows it
unibox network rm jitpack.io         # remove your rule
unibox network                       # a list: a allows, n denies, d removes
```

A host is a name, `*.name` for its subdomains, `**.name` for any depth, or
`name:port`. The protocol is tcp unless `--protocol` says `udp` or
`tcp,udp`. Add `-s dev` for another sandbox than the default.

Your rules are kept in `config.yaml` under the sandbox's name, applied
again after each recreate, and removed with the sandbox. A host the whole
team needs belongs in a kit: ask the env's maintainers.
