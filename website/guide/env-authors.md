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
- Put hosts everyone needs in a kit, not in each person's
  `unibox network` rules.
- Say in the README which token each secret wants and where to create it.

[Files](/reference/files) and [What unibox Asks of sbx](/reference/sbx)
have the details.
