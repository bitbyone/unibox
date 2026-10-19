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
line added: `template: <template>/<profile>`. Change anything; nothing is
written over it later.

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

`update` fetches the template again, lists what changes in its shared files
and asks before writing them; the images of its profiles then build again, as
their scripts changed. It does not change your profiles: it says, for each
copied from the template, which files now differ from the template's profile,
and `diff` shows how - the template's side first - so you take what you want
by hand.

## Writing a template

- Keep the profiles free of anything personal: what each person sets is a
  var, every token a `secret:<name>` that a service or an agent's `auth`
  names. `unibox var` and the copy's walkthrough ask for them.
- Put what several profiles share in the repository's `_shared/@install` and
  `_shared/@instructions`; a copy keeps them under the template's name, so
  no prefix is needed against the user's own.
- Describe each var for the person filling it in: the walkthrough shows the
  description as the question.
