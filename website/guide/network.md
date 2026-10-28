# Network

A sandbox reaches the hosts sbx's policy allows, the hosts the env's kits
add, and the rules added for that sandbox. sbx enforces all of it. unibox
adds the list of what the sandbox was refused lately, rules per sandbox
kept across a recreate, and a way to hand rules back to the team's kits.

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

```term
$ unibox network ls
sandbox dev has no rules of its own; unibox network allow <host> adds one
the env's kits allow 29 hosts, sbx's defaults 195; --all lists them

refused lately - what the env may want to allow:
HOST              TIMES  LAST
jitpack.io        4      12m ago
repo.gradle.org   1      12m ago
unibox network export writes these and your rules for a kit of the env

$ unibox network allow jitpack.io
```

`network ls` lists the hosts the sandbox was refused recently, which is
usually the quickest way to see what to allow. `--all` adds the env's and
sbx's default hosts.

## Block or remove

```sh
unibox network deny gist.github.com  # even if the env allows it
unibox network rm jitpack.io         # remove your rule
unibox network                       # the same as a list
```

## Hand rules over to the env

What one sandbox needed is often what everyone does. `unibox network
export` writes your rules and those added with sbx, less the hosts the
env's kits allow already, as a kit's network-policy entry; the hosts
refused lately follow as lines to uncomment:

```sh
unibox network export -s dev
```

```yaml
- type: com.docker.sandbox/network-policy@2
  config:
    runtime:
      allow:
        - jitpack.io
        # - pypi.org  # refused 3 times, last 2h ago
      deny:
        - gist.github.com
```

Merge it into the network-policy entry of one of the env's kits (a kit
has one), then `unibox sandbox recreate dev` applies the kit and `unibox
network rm` takes your own rules away. A kit's host is any protocol, so a
udp rule is marked. The `unibox network` list does the same.

## Hosts

A host is a name, `*.name` for its subdomains, `**.name` for any depth, or
`name:port`. The protocol is tcp unless `--protocol` says `udp` or
`tcp,udp`. Add `-s dev` for another sandbox than the default.

Your rules are kept in `config.yaml` under the sandbox's name, applied
again after each recreate, and removed with the sandbox. A host the whole
team needs belongs in a kit: ask the env's maintainers.
