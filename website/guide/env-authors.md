# For Env Authors

This site is for people using an env repository. If you maintain one:

- The sbxenv format and kits are sbx's: see the
  [Docker Sandboxes documentation](https://docs.docker.com/ai/sandboxes/).
  unibox reads an sbxenv file's `name`, `agent`, `args`, `kits` and
  `bindings` and passes the rest to sbx unchanged.
- Declare what each person must set as `args` with `required: true` and a
  `description`; `unibox init` asks for them by that description.
- Mark a kit's credential optional when the sandbox works without it;
  `unibox secret set` then skips it unless asked with `--all`.
- Name the team's ports in `unibox.yaml` at the repository root (every
  env) or `<env>.unibox.yaml` beside the sbxenv file:

  ```yaml
  ports:
    vite:
      port: '5173:5173'
      description: the dev server
      default: on
  ```

  sbx never reads these files.
- Say where each token is made in the same files, and unibox offers to
  open the page before sbx asks for the token:

  ```yaml
  secrets:
    gitlab:
      page: https://gitlab.example.com/-/user_settings/personal_access_tokens?name=unibox&scopes=read_repository,write_repository
      hint: a personal access token with read_repository and write_repository
  ```

  unibox knows GitHub's page for `github` and `copilot`; a team's entry
  replaces it.
- Name the repositories the env's tasks use, and have every task take the
  decisions and specifications (see [Tasks](/guide/tasks#repositories-by-name)):

  ```yaml
  repos:
    adrs: {url: https://gitlab.example.com/arch/adrs.git, ref: main, default: always}
    payments: https://gitlab.example.com/x/payments.git
  ```

- Give the env's sandboxes a default share of the machine in the same
  files; each person's `vars.toml` wins over it:

  ```yaml
  resources:
    cpus: 50%
    memory: 50%
  ```

  A share fits every machine; unibox converts it at each create and it
  wins over the sbxenv's own `sandboxOptions`.
- Tell the agents what they need under `instructions: |`, for the env at
  the top of the file and for each repository under `repos:`; unibox
  gathers them into the task's `AGENTS.md` (see
  [Tasks](/guide/tasks#what-the-agent-is-told)).
- Put hosts everyone needs in a kit, not in each person's
  `unibox network` rules.
- Say in the README which token each secret wants and where to create it.

[Files](/reference/files) and [What unibox Asks of sbx](/reference/sbx)
have the details.
