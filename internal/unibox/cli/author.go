package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// authorCommand keeps who a person's commits in a sandbox are by: theirs
// for every env, for one env, or for one sandbox, each winning over the
// one before, and asked for when an env's team requires one.
func (o *options) authorCommand() *cobra.Command {
	var envFlag, sandboxFlag, scopeFlag string
	c := &cobra.Command{Use: "author", Args: cobra.NoArgs, Short: "Show or keep who your commits in a sandbox are by", Long: `Who the commits made in a sandbox are by, as Name <email>: yours for
every env (--scope global), for one env (env, the default) or for one
sandbox (sandbox, -s), each winning over the one before, kept in
~/.config/unibox/vars.toml. Every session in the sandbox commits as it,
in each repository of its task and in the sandbox's global git
configuration. With none, a repository commits as its git configuration
on this computer says. An env whose team writes author: required in its
unibox.yaml asks for one before its first task; --author on a run wins
for that run.`, Example: `  unibox author
  unibox author set "Jane Doe <jane@acme.com>" -e work
  unibox author set "Jane Doe <jane@example.com>" --scope global
  unibox author set "Jane Doe <jane@example.com>" -s personal --scope sandbox
  unibox author rm -s personal --scope sandbox`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	c.PersistentFlags().StringVarP(&sandboxFlag, "sandbox", "s", "", "Sandbox (default: UNIBOX_SANDBOX or the context's, when of the env, else the env's)")
	target := func() (unibox.Env, string, error) {
		name, err := o.secretEnv(envFlag, sandboxFlag)
		if err != nil {
			return unibox.Env{}, "", err
		}
		env, err := o.manager.Config.LoadEnv(name)
		return env, o.secretSandbox(env, sandboxFlag), err
	}
	// keep sets author where --scope says, "" removing it, and says where.
	keep := func(author string) (string, error) {
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return "", err
		}
		var at string
		var had bool
		switch scopeFlag {
		case "global":
			at, had = "every env", v.Author != ""
			v.Author = author
		case "env", "sandbox":
			env, sandbox, err := target()
			if err != nil {
				return "", err
			}
			authors, key := v.EnvAuthors, env.Name
			at = "env " + env.Name
			if scopeFlag == "sandbox" {
				authors, key = v.SandboxAuthors, sandbox
				at = "sandbox " + sandbox
			}
			had = authors[key] != ""
			if author == "" {
				delete(authors, key)
			} else {
				authors[key] = author
			}
		default:
			return "", fmt.Errorf("--scope is global, env or sandbox, not %q", scopeFlag)
		}
		if author == "" && !had {
			return "", fmt.Errorf("you have no author for %s", at)
		}
		return at, o.paths.SaveEnvValues(v)
	}
	set := &cobra.Command{Use: "set author", Args: cobra.ExactArgs(1), Short: "Set who your commits are by, as Name <email>", RunE: func(cmd *cobra.Command, args []string) error {
		if err := unibox.ValidAuthor(args[0]); err != nil {
			return err
		}
		at, err := keep(args[0])
		if err != nil {
			return err
		}
		return o.success(fmt.Sprintf("commits in %s are by %s from the next session", at, args[0]))
	}}
	rm := &cobra.Command{Use: "rm", Args: cobra.NoArgs, Short: "Remove an author, so a wider one or the repository's git configuration says", RunE: func(cmd *cobra.Command, args []string) error {
		at, err := keep("")
		if err != nil {
			return err
		}
		return o.success("removed the author for " + at)
	}}
	for _, s := range []*cobra.Command{set, rm} {
		s.Flags().StringVar(&scopeFlag, "scope", "env", "global, for every env; env, the env's sandboxes'; sandbox, one sandbox's (-s)")
	}
	c.AddCommand(set, rm)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		env, sandbox, err := target()
		if err != nil {
			return err
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		author, from := v.AuthorFor(env, sandbox)
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": env.Name, "sandbox": sandbox, "author": author, "from": from, "required": env.AuthorRequired})
		}
		switch {
		case author != "":
			fmt.Fprintf(o.output, "commits in sandbox %s are by %s (%s)\n", sandbox, author, from)
		case env.AuthorRequired:
			fmt.Fprintf(o.output, "env %s requires a commit author of yours and you have none; unibox author set -e %s \"Name <email>\" sets it\n", env.Name, env.Name)
		default:
			fmt.Fprintf(o.output, "sandbox %s has no author of yours; each repository commits as its git configuration says\n", sandbox)
		}
		return nil
	}
	return c
}
