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

// coAuthorCommand switches whether agents name themselves co-author of
// their commits, for every env or one: on, the default, leaves it to each
// agent's own settings; off takes the trailer an agent adds of itself out.
func (o *options) coAuthorCommand() *cobra.Command {
	var envFlag string
	c := &cobra.Command{Use: "co-author [on|off|default]", Args: cobra.MaximumNArgs(1), ValidArgs: []string{"on", "off", "default"}, Short: "Show or switch whether agents name themselves co-author of commits", Long: `Whether an agent's commits in a sandbox name it co-author, for every
env, or with -e for one env over every env's, kept as co_author in
~/.config/unibox/vars.toml. On, the default, each agent does as its own
settings say; off, a commit hook takes out the Co-authored-by trailer an
agent adds of itself, whatever its settings say. default forgets the
env's own, or every env's without -e. It holds from the next session.`, Example: `  unibox co-author
  unibox co-author off
  unibox co-author on -e personal
  unibox co-author default -e personal`}
	c.Flags().StringVarP(&envFlag, "env", "e", "", "Env, over every env's (default: every env)")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		at := "every env"
		if envFlag != "" {
			env, err := o.manager.Config.LoadEnv(envFlag)
			if err != nil {
				return err
			}
			envFlag, at = env.Name, "env "+env.Name
		}
		if len(args) == 0 {
			on, from := v.CoAuthorFor(envFlag)
			if envFlag == "" && v.CoAuthor == nil {
				from = ""
			}
			state := map[bool]string{true: "on", false: "off"}[on]
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "env": envFlag, "co_author": on, "from": from})
			}
			if from == "" {
				from = "default"
			}
			if on {
				fmt.Fprintf(o.output, "%s for %s (%s): each agent names itself co-author of its commits as its settings say\n", state, at, from)
			} else {
				fmt.Fprintf(o.output, "%s for %s (%s): no agent's commit names it co-author\n", state, at, from)
			}
			return nil
		}
		var set *bool
		switch args[0] {
		case "on", "off":
			on := args[0] == "on"
			set = &on
		case "default":
		default:
			return fmt.Errorf("co-author is on, off or default, not %q", args[0])
		}
		switch {
		case envFlag == "":
			v.CoAuthor = set
		case set == nil:
			delete(v.EnvCoAuthors, envFlag)
		default:
			v.EnvCoAuthors[envFlag] = *set
		}
		if err = o.paths.SaveEnvValues(v); err != nil {
			return err
		}
		if set == nil {
			on, _ := v.CoAuthorFor(envFlag)
			return o.success(fmt.Sprintf("co-authors for %s are as by default, %s, from the next session", at, map[bool]string{true: "on", false: "off"}[on]))
		}
		return o.success(fmt.Sprintf("co-authors are %s for %s from the next session", args[0], at))
	}
	return c
}
