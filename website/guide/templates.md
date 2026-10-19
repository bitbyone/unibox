# Templates

A team keeps its profiles in one environments repository. Each person can
use it in either of two ways:

| | How | Fits when |
| --- | --- | --- |
| **As the configuration** | `unibox config set environments <clone>` | everyone runs the same profiles, and changes come with `git pull` |
| **As a template** | `unibox init <profile> --from <repository>` into an environments repository of your own | each person starts from the team's profile and changes it freely |

Nothing marks a repository as a template: any environments repository is one.
A profile meant for a team holds nothing personal - each person's values are
[vars](./profiles#vars-env-and-build-args), tokens are `secret:<name>` - so it
works either way.

## Looking before copying

```sh
unibox template inspect git@gitlab.example.com:team/agent-envs.git
```

fetches the template and tells what it holds, writing nothing: each profile's
agents, its vars with what each is for, its services and the tokens it needs,
the variables its sessions get, the shared install scripts and instruction
texts, and how it stands to your repository - a template of that name kept
already, a profile of that name you have. With `--json` it is part of the
versioned contract, for a UI to show before it copies.

A git template is cloned shallowly into a temporary directory, which goes
when the command ends; a directory is read where it is. Only what a copy
takes stays: the profile under `sandboxes/`, and the shared files with their
source under `.templates/<template>/`.

## Copying a profile

```sh
unibox init -i --from git@gitlab.example.com:team/agent-envs.git
```

asks which of the template's profiles to copy and what to name your copy,
then walks you through what it needs of you: each of its vars, with what the
profile says it is for, and each token it names that your platform's store
does not hold yet, with where it is made. Last it offers to build the image.

Without `-i` it copies and lists what is left to do:

```sh
unibox init dev --from ~/src/agent-envs --use team
```

| Flag | Meaning |
| --- | --- |
| `--from <source>` | a git URL, cloned with your own git, or a directory |
| `--use <profile>` | the template's profile to copy (default: the copy's name) |
| `--template <name>` | the name the template is kept under (default: its repository's) |

The copy is an ordinary profile of yours, as the template has it, with one
line added: `template: <template>/<profile>`. Change anything. unibox also
keeps the copy as it was written, under `.templates/<template>/bases/`, the
base an update merges from.

## What the template's profiles share

A template's `_shared/@install` scripts and `_shared/@instructions` texts are
kept apart from yours, under `.templates/<template>/` in your repository,
with where they come from in `source.yaml`. unibox writes that directory from
the template and replaces it on `unibox template update`; it never touches
your `_shared`. A profile copied from a template finds a script or text by
name in this order:

1. your `_shared` - so you override one of the template's by putting your own
   of the same name there;
2. its template's `.templates/<template>`;
3. unibox's `.shared`.

A profile sees only its own template's directory, so two templates never meet,
and a template's names never collide with yours.

## Keeping up with the template

```sh
unibox template ls                 # each template, where it comes from, its profiles
unibox template update agent-envs  # take its shared files anew
unibox template diff dev           # how your profile differs from the template's
```

`update` fetches the template again and merges it into every profile copied
from it, three ways as git does, from the profile as it was copied:

- a file only the template changed takes the template's version;
- a file only you changed stays yours;
- a file both changed is merged where the changes do not meet - your line
  added at the end of `sandbox.dockerfile` and the template's new base image
  both stay.

It lists what it will write - the shared files and each profile's files - and
asks; the images of the profiles that changed then build again.

**A conflict writes nothing.** Where you and the template changed the same
place, no profile and none of the template's shared files change, so every
profile keeps working as it was. Beside the template, under
`.templates/<template>/conflicts/<profile>/`, it leaves for each such file
the template's version as `<file>.template` and a merge with markers as
`<file>.merge`. Make your profile say what you want - from the `.merge`, say -
and run `update` again: it merges cleanly then, writes everything, and clears
what it left. A profile copied before unibox kept a base cannot tell whose a
difference is, so each file that differs from the template's is a conflict
once.

`diff` shows how a profile differs from the template's, the template's side
first.

## Writing a template

- Keep the profiles free of anything personal: what each person sets is a
  var, every token a `secret:<name>` that a service or an agent's `auth`
  names. `unibox var` and the copy's walkthrough ask for them.
- Put what several profiles share in the repository's `_shared/@install` and
  `_shared/@instructions`; a copy keeps them under the template's name, so
  no prefix is needed against the user's own.
- Describe each var for the person filling it in: the walkthrough shows the
  description as the question.
- Leave a place for each person's own tools outside the Dockerfile, so an
  update rarely conflicts: end it with `RUN @install custom`, which runs the
  user's `_shared/@install/custom.sh`, theirs to fill, and ship an empty
  `custom.sh` in the template's `_shared/@install` for those who have none.
  Their values belong in vars and `env`, what runs at each boot in `start`.
