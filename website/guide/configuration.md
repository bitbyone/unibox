# Team and Personal Configuration

The team's repository describes the sandbox; each person adds values of
their own. sbx has the first part in its sbxenv files and kits, and asks
each person for the env's args. unibox keeps what it adds the same way: the
team's in the repository, beside the sbx files, and yours on your machine,
where it wins over the team's.

## The team's, in the repository

```
agent-sandboxes/
├── unibox.yaml                    # unibox: every env of the repository
├── kits/
│   └── java/java.yaml             # sbx
└── sandboxes/
    ├── dev.sbxenv.yaml            # sbx: the env dev
    └── dev.unibox.yaml            # unibox: the env dev alone
```

| In `unibox.yaml` or `<env>.unibox.yaml` | |
| --- | --- |
| `ports` | named ports, off or on at start - [Ports](./ports) |
| `repos` | the repository index - [Repository index](./repos) |
| `instructions` | what the agent is told - [Agent instructions](./instructions) |
| `resources` | CPUs and memory, as a share of each machine - [Sandboxes](./sandboxes#cpus-and-memory) |
| `secrets` | where each token is made - [Secrets](./secrets) |

sbx does not read these files. A teammate who uses sbx alone works from
the same repository and is limited by nothing in them.

## Yours, on your machine

| File | Holds | Written by |
| --- | --- | --- |
| `~/.config/unibox/vars.toml` | your values of the env's args; your `[resources]` | `unibox var`, `init`; resources by hand |
| `~/.config/unibox/config.yaml` | your envs, ports, repositories, instructions, network rules, settings | `unibox repo`, `port`, `network`, `config`, `init`; or by hand |
| sbx's secret store | tokens | `unibox secret set`, through sbx |

Every key of `config.yaml` may be given for every env at the top, for
every env of one env repository under `env_repos.<repo>` - its host and
path, as `unibox env ls` shows it - or for one env under `envs.<env>`:

```yaml
ports:
  devtools: {port: '19222:9222', default: off}
repos:
  notes: https://github.com/jdoe/notes.git
instructions: |
  Answer in English, briefly.
env_repos:
  github.com/acme/agent-envs:          # every env of one team's repository
    repos:
      payments-spec: https://github.com/acme/payments-spec.git
    instructions: |
      Tickets are in Jira project PAY.
envs:
  dev:
    ports:
      vite: {port: '15173:5173'}       # the team's vite, on another host port
    repos:
      spec: {default: off}             # the team's spec, not picked to begin with
```

## Which wins

Later wins, entry by entry:

1. `unibox.yaml` at the repository's root
2. `<env>.unibox.yaml` beside the env's sbxenv file
3. your top-level keys in `config.yaml`
4. your `env_repos.<repo>` keys, for the repository the env's files are in
5. your `envs.<env>` keys

A port or repository is merged key by key: naming only `default` keeps
the team's address and description. The env's instructions are not
replaced but joined, in the same order. Resources come from the team's
files, then `vars.toml`, whose `[env_repos."<repo>"]` tables sit the same
way between `[args]` and `[envs.<env>]`.

The lists show where each entry comes from: `FROM` in `unibox port ls` and
`unibox repo ls`, `(yours)` beside a default you set over the team's.

```term
$ unibox port ls
PORT      MAPPING    FROM              AT START    NOW  URL                    DESCRIPTION
devtools  9222:9222  the repository's  off         off  -                      Chrome DevTools
spring    8080:8080  the repository's  on          on   http://127.0.0.1:8080  the Spring Boot application
vite      5173:5173  the repository's  on (yours)  off  -                      the Vite dev server
```

## Applied when

| Change | Reaches the sandbox |
| --- | --- |
| ports | at the next `run`, `resume` or `port on/off` |
| repositories, instructions | at the next task or session |
| network rules | at once |
| values, resources, the env's files and kits | at `unibox sandbox recreate`, which `doctor` and the next start point out |
