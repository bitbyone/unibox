package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"gopkg.in/yaml.v3"
)

// The steps of an interactive init, in order. The first ones make the
// profile; from Agents on each can be skipped, or the rest left for later,
// which goes straight to the build.
const (
	stepRuntime    = "Runtime"
	stepRepository = "Repository"
	stepProfile    = "Profile"
	stepValues     = "Values"
	stepAgents     = "Agents"
	stepServices   = "Services"
	stepBuild      = "Build"
	stepRun        = "Run"
)

// wizard is the walk an interactive init takes: where the user is, what is
// done and what is left, and what they chose to do later.
type wizard struct {
	o      *options
	steps  []string
	state  map[string]string // done, skipped
	at     string
	rest   bool     // the rest is left for later
	later  []string // what to run then
	runDir string   // the repository the Run step offers to start in
}

func (o *options) newWizard(steps ...string) *wizard {
	return &wizard{o: o, steps: steps, state: map[string]string{}}
}

func (w *wizard) has(step string) bool { return slices.Contains(w.steps, step) }

// begin marks the step before as done and shows where the user is.
func (w *wizard) begin(step string) {
	if w.at != "" && w.state[w.at] == "" {
		w.state[w.at] = "done"
	}
	w.at = step
	parts := []string{}
	n := 0
	for i, s := range w.steps {
		mark := "·"
		switch {
		case s == step:
			mark, n = "▸", i+1
		case w.state[s] == "done":
			mark = "✓"
		case w.state[s] == "skipped":
			mark = "–"
		}
		parts = append(parts, mark+" "+s)
	}
	fmt.Fprintf(w.o.log, "\n%s   (%d/%d)\n", strings.Join(parts, "  "), n, len(w.steps))
}

// skip marks a step as left for later, with what to run then.
func (w *wizard) skip(step string, later ...string) {
	w.state[step] = "skipped"
	w.later = append(w.later, later...)
}

// offer asks whether to take a step that may be left for later: go on,
// skip this one, or leave the rest - but the build - for later.
func (w *wizard) offer(step, what string, later ...string) (bool, error) {
	if w.rest {
		w.skip(step, later...)
		return false, nil
	}
	choice, err := w.o.askPick(step, what, []pick{
		{"go", "Go on"},
		{"skip", "Skip this step - I will do it later"},
		{"rest", "Skip the rest - set up the profile as it is now, then build"},
	}, "go")
	if err != nil {
		return false, err
	}
	switch choice {
	case "go":
		return true, nil
	case "rest":
		w.rest = true
	}
	w.skip(step, later...)
	return false, nil
}

// finish says what was left for later.
func (w *wizard) finish() {
	if w.at != "" && w.state[w.at] == "" {
		w.state[w.at] = "done"
	}
	if len(w.later) == 0 {
		return
	}
	fmt.Fprintln(w.o.output, "left for later:")
	for _, line := range w.later {
		fmt.Fprintf(w.o.output, "  %s\n", line)
	}
}

// setupSteps are the steps from the profile on: its values, the agents,
// the services, the build and the first run.
func (o *options) setupSteps(cmd *cobra.Command, w *wizard, name string) error {
	if w.has(stepValues) {
		w.begin(stepValues)
		if ok, err := w.offer(stepValues, "The values and tokens the profile asks of you.", "unibox var ls -p "+name); err != nil {
			return err
		} else if ok {
			if err := o.askValues(cmd, name); err != nil {
				return err
			}
		}
	}
	w.begin(stepAgents)
	if ok, err := w.offer(stepAgents, "Sign the profile's agents in, as unibox agent login -i does.", "unibox agent login -i -p "+name); err != nil {
		return err
	} else if ok {
		if missing := o.runtimeMissing(); missing != "" {
			fmt.Fprintf(o.log, "the agents are signed in once a runtime is installed\n")
			w.skip(stepAgents, "unibox agent login -i -p "+name)
		} else if err := o.login(cmd, name, nil, false, false, true, "", ""); err != nil {
			return err
		}
	}
	w.begin(stepServices)
	if ok, err := w.offer(stepServices, "The forges your repositories live on, and services such as Artifactory your builds reach.", "unibox service add <name> -p "+name); err != nil {
		return err
	} else if ok {
		if err := o.askServices(cmd, name); err != nil {
			return err
		}
	}
	w.begin(stepBuild)
	built := false
	if o.runtimeMissing() != "" {
		w.skip(stepBuild, "unibox profile build "+name)
	} else if build, err := o.askYes("Build the image of profile "+name+" now?", true); err != nil {
		return err
	} else if build {
		if err := o.manager.Build(cmd.Context(), name, "", false); err != nil {
			return err
		}
		built = true
	} else {
		w.skip(stepBuild, "unibox profile build "+name)
	}
	if w.has(stepRun) {
		w.begin(stepRun)
		if err := o.offerRun(cmd, w, name, built); err != nil {
			return err
		}
	}
	w.finish()
	return nil
}

// askValues asks for the values and tokens a profile names that the user
// has not given yet.
func (o *options) askValues(cmd *cobra.Command, name string) error {
	p, err := o.manager.Profile(name)
	if err != nil {
		return err
	}
	if missing := p.MissingVars(); len(missing) > 0 {
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		for _, v := range missing {
			value, err := o.askText(v, p.Vars[v]+" - kept in your vars.toml; empty sets it later.", "", func(s string) error {
				if s == "" {
					return nil
				}
				return unibox.ValidVar(v, s)
			})
			if err != nil {
				return err
			}
			if value != "" {
				u.Vars[v] = value
			}
		}
		if err = o.paths.SaveVars(u); err != nil {
			return err
		}
	}
	refs := slices.DeleteFunc(p.SecretRefs(), func(n string) bool { return p.ReadBy(n) != "" })
	if len(refs) == 0 {
		return nil
	}
	store, err := o.secretStore()
	if err != nil {
		return err
	}
	o.useSecretsOf(name)
	for _, ref := range refs {
		if ok, _ := store.Has(cmd.Context(), ref); ok {
			continue
		}
		what, explain, page := secretUse(p, ref)
		fmt.Fprintf(o.log, "%s:\n", what)
		if err := o.ensureSecret(cmd, store, ref, false, explain, page); err != nil {
			return err
		}
	}
	return nil
}

// askServices offers the forges and services most profiles need, and adds
// each chosen through unibox service add. What is the user's own - their
// commit identity, their user name - goes into their vars.toml, the
// profile referring to it, so the profile stays shareable.
func (o *options) askServices(cmd *cobra.Command, name string) error {
	chosen, err := o.askChoices("Services", "Where your repositories live, and what else your builds reach. Others: unibox service add later.", []string{"GitHub", "GitLab (gitlab.com)", "A self-hosted GitLab", "Artifactory"}, nil, nil)
	if err != nil {
		return err
	}
	for _, c := range chosen {
		var service, kind, host string
		switch c {
		case "GitHub":
			service, kind, host = "github", "github", "github.com"
		case "GitLab (gitlab.com)":
			service, kind, host = "gitlab", "gitlab", "gitlab.com"
		case "A self-hosted GitLab", "Artifactory":
			kind = "gitlab"
			if c == "Artifactory" {
				kind = "artifactory"
			}
			if host, err = o.askText(c+": host", "Its host name, such as "+map[string]string{"gitlab": "gitlab.example.com", "artifactory": "artifactory.example.com"}[kind]+".", "", func(s string) error {
				if strings.TrimSpace(s) == "" || strings.ContainsAny(s, " /:") {
					return fmt.Errorf("a host name, with no scheme or path")
				}
				return nil
			}); err != nil {
				return err
			}
			if service, err = o.askText(c+": name", "What the profile calls it.", serviceName(kind, host), unibox.ValidName); err != nil {
				return err
			}
		}
		args := []string{"add", service, "-p", name, "--kind", kind, "--host", host}
		if kind == "artifactory" {
			user, err := o.askText(service+": your user name", "Kept in your vars.toml; the profile refers to it. Empty leaves it out.", "", nil)
			if err != nil {
				return err
			}
			if user != "" {
				variable := service + "-user"
				env := strings.ToUpper(strings.ReplaceAll(service, "-", "_")) + "_USER"
				if err := o.personalValue(name, variable, "your user name on "+host, user, func(doc *yaml.Node) {
					unibox.SetEnvValue(doc, env, "${vars."+variable+"}")
				}); err != nil {
					return err
				}
			}
		} else {
			author, err := o.askText(service+": your commit identity", "Name <email> you commit as on "+host+"; kept in your vars.toml. Empty takes your git configuration.", "", func(s string) error {
				if s == "" {
					return nil
				}
				_, _, err := unibox.AuthorParts(s)
				return err
			})
			if err != nil {
				return err
			}
			if author != "" {
				variable := service + "-author"
				if err := o.personalValue(name, variable, "you on "+host+", as Name <email>", author, nil); err != nil {
					return err
				}
				args = append(args, "--author", "${vars."+variable+"}")
			}
		}
		svc := o.serviceCommand()
		svc.SetArgs(args)
		svc.SetIn(o.input)
		svc.SetOut(o.output)
		svc.SetErr(o.log)
		if err := svc.ExecuteContext(cmd.Context()); err != nil {
			return fmt.Errorf("service %s: %w", service, err)
		}
	}
	return nil
}

// serviceName is what a service on a host is called by default: the kind,
// with the host's first label when that says more.
func serviceName(kind, host string) string {
	label, _, _ := strings.Cut(host, ".")
	if label == kind || label == "" {
		return kind
	}
	if parts := strings.Split(host, "."); len(parts) > 2 && parts[0] == kind {
		label = parts[1]
	}
	name := strings.ToLower(kind + "-" + label)
	if unibox.ValidName(name) != nil {
		return kind
	}
	return name
}

// personalValue declares a var in the profile, has it referred to as edit
// says, and keeps the user's value in their vars.toml.
func (o *options) personalValue(profile, variable, description, value string, edit func(doc *yaml.Node)) error {
	if err := o.manager.EditProfile(profile, func(doc *yaml.Node) error {
		if err := unibox.SetProfilePath(doc, []string{"vars", variable}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: description}); err != nil {
			return err
		}
		if edit != nil {
			edit(doc)
		}
		return nil
	}); err != nil {
		return err
	}
	u, err := o.paths.LoadVars()
	if err != nil {
		return err
	}
	u.Vars[variable] = value
	return o.paths.SaveVars(u)
}

// offerRun ends the walk by running an agent in the repository init was
// run from, if it is one.
func (o *options) offerRun(cmd *cobra.Command, w *wizard, name string, built bool) error {
	dir := w.runDir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil
		}
		out, err := exec.CommandContext(cmd.Context(), "git", "-C", wd, "rev-parse", "--show-toplevel").Output()
		if err != nil {
			w.skip(stepRun, "unibox run -p "+name+"   # in a repository")
			return nil
		}
		dir = strings.TrimSpace(string(out))
	}
	if root, _ := filepath.Abs(o.manager.Config.Environments); root == dir || o.runtimeMissing() != "" {
		w.skip(stepRun, "unibox run -p "+name+"   # in a repository")
		return nil
	}
	run, err := o.askYes("Run an agent in "+filepath.Base(dir)+" now?", built)
	if err != nil || !run {
		w.skip(stepRun, "unibox run -p "+name+"   # in "+dir)
		return err
	}
	p, err := o.manager.Profile(name)
	if err != nil {
		return err
	}
	agent := p.Agents[0]
	if len(p.Agents) > 1 {
		picks := []pick{}
		for _, a := range p.Agents {
			picks = append(picks, pick{a, a})
		}
		if agent, err = o.askPick("Agent", "The agent to run.", picks, agent); err != nil {
			return err
		}
	}
	w.finish()
	w.later = nil
	req := unibox.Request{Profile: name, Agent: agent, Repositories: []unibox.RepoInput{{Source: dir}}}
	if err := o.preflight(cmd, &req); err != nil {
		return err
	}
	return o.manager.Run(cmd.Context(), req, o.input, o.output, o.log, nil)
}
