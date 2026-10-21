package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// initFrom copies a template's profile into the repository and walks the
// user through what it needs of them.
func (o *options) initFrom(cmd *cobra.Command, root, previous, name, from, use, template string, interactive bool) error {
	ctx := cmd.Context()
	t, err := unibox.FetchTemplate(ctx, from)
	if err != nil {
		return err
	}
	defer t.Close()
	if template == "" {
		template = unibox.TemplateNameOf(t.Source)
	}
	profiles, err := t.Profiles()
	if err != nil {
		return err
	}
	if use == "" {
		use = name
	}
	if use == "" {
		if !interactive || len(profiles) == 0 {
			return fmt.Errorf("say which of template %s's profiles to copy: %s (--use, or the profile's name)", template, strings.Join(profiles, ", "))
		}
		picks := []pick{}
		for _, p := range profiles {
			picks = append(picks, pick{p, p})
		}
		if use, err = o.askPick("Profile to copy", "The profiles of template "+template+".", picks, profiles[0]); err != nil {
			return err
		}
	}
	if name == "" {
		name = use
		if interactive {
			if name, err = o.askText("Name of your copy", "The profile's name in your repository.", use, unibox.ValidName); err != nil {
				return err
			}
		}
	}
	if err = os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if _, err = t.CopyProfile(root, name, use, template); err != nil {
		return err
	}
	if root != previous {
		cfg := o.manager.Config
		cfg.Environments = root
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		o.manager.Config = cfg
	}
	if _, err := o.paths.EnsureVarsFile(); err != nil {
		return err
	}
	fmt.Fprintf(o.output, "copied profile %s of template %s as %s into %s\n", use, template, name, root)
	if err = o.completeProfile(cmd, name, interactive); err != nil {
		return err
	}
	return nil
}

// completeProfile asks for what a profile needs of the user before it can
// run - their own values and its tokens - and offers the build. Without a
// terminal it says what is missing and how to give it.
func (o *options) completeProfile(cmd *cobra.Command, name string, interactive bool) error {
	ctx := cmd.Context()
	p, err := o.manager.Profile(name)
	if err != nil {
		return err
	}
	todo := []string{}
	if missing := p.MissingVars(); len(missing) > 0 {
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		for _, v := range missing {
			if !interactive {
				todo = append(todo, fmt.Sprintf("unibox var set %s <value>   # %s", v, p.Vars[v]))
				continue
			}
			value, err := o.askText(v, p.Vars[v]+" - kept in your vars.toml; empty sets it later.", "", func(s string) error {
				if s == "" {
					return nil
				}
				return unibox.ValidVar(v, s)
			})
			if err != nil {
				return err
			}
			if value == "" {
				todo = append(todo, fmt.Sprintf("unibox var set %s <value>   # %s", v, p.Vars[v]))
				continue
			}
			u.Vars[v] = value
		}
		if err = o.paths.SaveVars(u); err != nil {
			return err
		}
	}
	// Secrets read by the person's own command are theirs to keep there.
	if refs := slices.DeleteFunc(p.SecretRefs(), func(n string) bool { return p.ReadBy(n) != "" }); len(refs) > 0 {
		store, err := o.secretStore()
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if ok, _ := store.Has(ctx, ref); ok {
				continue
			}
			what, explain, page := secretUse(p, ref)
			if !interactive {
				todo = append(todo, fmt.Sprintf("unibox secret set %s   # %s", ref, what))
				continue
			}
			fmt.Fprintf(o.log, "%s:\n", what)
			if err := o.ensureSecret(cmd, store, ref, false, explain, page); err != nil {
				return err
			}
		}
	}
	if len(todo) > 0 {
		fmt.Fprintf(o.output, "profile %s needs, before it runs:\n", name)
		for _, line := range todo {
			fmt.Fprintf(o.output, "  %s\n", line)
		}
		return nil
	}
	if interactive {
		if _, err := o.offerLogin(cmd, name); err != nil {
			return err
		}
		build, err := o.askYes("Build the image of profile "+name+" now?", true)
		if err != nil {
			return err
		}
		if build {
			if err := o.manager.Build(ctx, name, "", false); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(o.output, "profile %s is ready; start with unibox run --profile %s --agent <agent> in a repository\n", name, name)
	return nil
}

// secretUse says what a profile uses a secret for, and where a service's
// token is made when unibox knows the kind.
func secretUse(p unibox.Profile, ref string) (what string, explain []string, page string) {
	for _, svc := range sortedNames(p.Services) {
		s := p.Services[svc]
		if s.Command != unibox.SecretPrefix+ref {
			continue
		}
		for kind, k := range serviceKinds {
			if kind != "generic" && strings.HasPrefix(svc, kind) {
				explain = k.explain
				if k.page != nil && len(s.Host) > 0 {
					host, _, _ := strings.Cut(s.Host[0], "/")
					page = k.page(host, svc)
				}
			}
		}
		return "the token of service " + svc + " (" + strings.Join(s.ProxyHosts(), ", ") + ")", explain, page
	}
	for _, id := range p.Agents {
		if p.Auth[id].Command == unibox.SecretPrefix+ref {
			switch id {
			case "claude":
				explain = []string{"run claude setup-token where claude is installed; it prints a token for your subscription"}
			case "copilot":
				explain = []string{"a fine-grained GitHub token: no repositories, Account permissions > Copilot Requests: Read-only"}
				page = "https://github.com/settings/personal-access-tokens/new"
			}
			return "the " + p.Auth[id].Method + " login of agent " + id, explain, page
		}
	}
	return "secret " + ref, nil, ""
}

func sortedNames[V any](m map[string]V) []string { return sorted(m) }

// templateCommand lists the templates the repository keeps profiles from,
// updates their shared files and tells how a profile differs from its own.
func (o *options) templateCommand() *cobra.Command {
	c := &cobra.Command{Use: "template", Short: "The templates profiles were copied from: list them, update their shared files, compare a profile", Long: `A profile copied with unibox init --from is yours to change. What the
template's profiles share - its install scripts and instruction texts - is
kept under .templates/<template>, rewritten only from the template, between
your _shared, which wins, and unibox's .shared.`}
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the templates kept, where they come from and the profiles copied from each", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.manager.Environments()
		if err != nil {
			return err
		}
		rows := []map[string]any{}
		for _, name := range unibox.KeptTemplates(root) {
			src, _ := unibox.ReadTemplateSource(root, name)
			profiles := []string{}
			for _, p := range unibox.TemplateProfiles(root, name) {
				profiles = append(profiles, p.Name)
			}
			rows = append(rows, map[string]any{"name": name, "source": src.Source, "commit": src.Commit, "profiles": profiles})
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "templates": rows})
		}
		if len(rows) == 0 {
			fmt.Fprintln(o.output, "no templates; unibox init <profile> --from <repository> copies a profile from one")
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TEMPLATE\tSOURCE\tCOMMIT\tPROFILES")
		for _, r := range rows {
			commit := r["commit"].(string)
			if len(commit) > 12 {
				commit = commit[:12]
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r["name"], r["source"], commit, strings.Join(r["profiles"].([]string), ", "))
		}
		return w.Flush()
	}})
	var name string
	inspect := &cobra.Command{Use: "inspect source", Args: cobra.ExactArgs(1), Short: "Tell what a template holds - its profiles, what each needs, its shared files - writing nothing", Long: `Fetch a template - a git URL or a directory - and tell what it holds, writing
nothing: each profile's agents, its vars with what each is for, its services
and the tokens it needs, the variables its sessions get; the shared install
scripts and instruction texts; and how it stands to your repository - a
template of that name kept already, a profile of that name there already.
With --json it is the versioned contract a UI reads before it copies.`, RunE: func(cmd *cobra.Command, args []string) error {
		t, err := unibox.FetchTemplate(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		defer t.Close()
		if name == "" {
			name = unibox.TemplateNameOf(t.Source)
		}
		root := o.manager.Config.Environments
		if _, err := os.Stat(root); root == "" || err != nil {
			root = ""
		}
		info, err := t.Inspect(root, name)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(info)
		}
		fmt.Fprintf(o.output, "template %s from %s", info.Name, info.Source)
		if info.Commit != "" {
			fmt.Fprintf(o.output, " at %s", shortCommit(info.Commit))
		}
		fmt.Fprintln(o.output)
		if info.Kept != nil {
			if info.Kept.Source == info.Source {
				fmt.Fprintf(o.output, "  kept already, at %s, by profiles %s\n", shortCommit(info.Kept.Commit), strings.Join(info.Kept.Profiles, ", "))
			} else {
				fmt.Fprintf(o.output, "  the name is taken by a template from %s; copy with --template <another>\n", info.Kept.Source)
			}
		}
		if len(info.Shared) > 0 {
			fmt.Fprintf(o.output, "  shared: %s\n", strings.Join(info.Shared, ", "))
		}
		for _, p := range info.Profiles {
			fmt.Fprintf(o.output, "\nprofile %s", p.Name)
			if p.Exists {
				fmt.Fprint(o.output, " (you have a profile of this name; copy it under another)")
			}
			fmt.Fprintln(o.output)
			if p.Error != "" {
				fmt.Fprintf(o.output, "  does not load here: %s\n", p.Error)
				continue
			}
			fmt.Fprintf(o.output, "  agents: %s\n", strings.Join(p.Agents, ", "))
			for _, v := range sorted(p.Vars) {
				fmt.Fprintf(o.output, "  var %s: %s\n", v, p.Vars[v])
			}
			for _, s := range p.Services {
				kind := "a tool's token"
				if s.Git {
					kind = "git"
				}
				fmt.Fprintf(o.output, "  service %s: %s, %s\n", s.Name, strings.Join(s.Hosts, ", "), kind)
			}
			if len(p.Secrets) > 0 {
				fmt.Fprintf(o.output, "  tokens: %s\n", strings.Join(p.Secrets, ", "))
			}
			if len(p.Env) > 0 {
				fmt.Fprintf(o.output, "  env: %s\n", strings.Join(p.Env, ", "))
			}
		}
		return nil
	}}
	inspect.Flags().StringVar(&name, "template", "", "The name the template would be kept under (default: its repository's)")
	c.AddCommand(inspect)
	c.AddCommand(&cobra.Command{Use: "update template", Args: cobra.ExactArgs(1), Short: "Take a template anew: its shared files, and its changes merged into the profiles copied from it", Long: `Fetch the template again and merge it into the profiles copied from it, three
ways as git does: from the profile as it was copied, a file only the template
changed takes the template's, one only you changed stays yours, and one both
changed is merged where the changes do not meet. Its shared files are taken
anew too. All of it is written, or none: where both changed the same place,
nothing changes, the template's version and a merge with markers are left
under .templates/<template>/conflicts/<profile>/, and once your profile says
what you want, update again.`, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.manager.Environments()
		if err != nil {
			return err
		}
		name := args[0]
		src, err := unibox.ReadTemplateSource(root, name)
		if err != nil {
			return err
		}
		t, err := unibox.FetchTemplate(cmd.Context(), src.Source)
		if err != nil {
			return err
		}
		defer t.Close()
		plan, err := t.PlanUpdate(cmd.Context(), root, name)
		if err != nil {
			return err
		}
		changes := 0
		for _, f := range []struct {
			sign string
			list []string
		}{{"+", plan.Shared.Added}, {"~", plan.Shared.Changed}, {"-", plan.Shared.Removed}} {
			for _, rel := range f.list {
				fmt.Fprintf(o.output, "  %s .templates/%s/%s\n", f.sign, name, rel)
				changes++
			}
		}
		for _, m := range plan.Profiles {
			for _, rel := range sorted(m.Write) {
				what := "merged"
				if m.Write[rel] == nil {
					what = "removed, as the template did"
				}
				fmt.Fprintf(o.output, "  ~ profile %s: %s %s\n", m.Profile.Name, rel, what)
				changes++
			}
		}
		if plan.Conflicted() {
			written, err := plan.WriteConflicts(root)
			if err != nil {
				return err
			}
			for _, m := range plan.Profiles {
				for _, rel := range sorted(m.Conflicts) {
					why := "you and the template changed the same place"
					if m.NoBase {
						why = "it differs from the template's, and the profile was copied before unibox kept what it was copied as"
					}
					fmt.Fprintf(o.output, "  ! profile %s: %s: %s\n", m.Profile.Name, rel, why)
				}
			}
			for _, path := range written {
				fmt.Fprintf(o.output, "    %s\n", path)
			}
			return unibox.Fail(4, "template_conflict", "nothing was written: resolve each file into your profile - the template's version is the .template beside it, a merge with markers the .merge - then run unibox template update %s again", name)
		}
		if changes == 0 {
			fmt.Fprintf(o.output, "template %s and the profiles copied from it are as the template is\n", name)
			return nil
		}
		if err := o.confirm("Write these? Images of its profiles build again where they changed."); err != nil {
			return err
		}
		if err = plan.Apply(root, t); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("updated template %s to %s", name, shortCommit(t.Commit)))
	}})
	var profile string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "Show how a profile differs from the template profile it was copied from", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		if p.Template == "" {
			return fmt.Errorf("profile %s was not copied from a template", p.Name)
		}
		src, err := unibox.ReadTemplateSource(p.Root, p.TemplateName())
		if err != nil {
			return err
		}
		t, err := unibox.FetchTemplate(cmd.Context(), src.Source)
		if err != nil {
			return err
		}
		defer t.Close()
		// git's own diff, the template's side first: what was taken out
		// of it shows as removed, what the user added as added.
		diff := exec.CommandContext(cmd.Context(), "git", "diff", "--no-index", "--", filepath.Join(t.Dir, "sandboxes", p.TemplateProfile()), p.Dir)
		diff.Stdout, diff.Stderr = o.output, o.log
		if err := diff.Run(); err != nil {
			if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 1 {
				return nil
			}
			return err
		}
		fmt.Fprintf(o.output, "profile %s is as the template's %s\n", p.Name, p.TemplateProfile())
		return nil
	}}
	profileFlag(diff, &profile)
	c.AddCommand(diff)
	return c
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	if c == "" {
		return "its current state"
	}
	return c
}
