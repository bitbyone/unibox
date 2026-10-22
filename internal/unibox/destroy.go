package unibox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
	"gopkg.in/yaml.v3"
)

// DestroyOptions say how far unibox destroy goes. Without All it removes
// every sandbox and so every task; All removes what unibox ever made on
// the host besides; Secrets the tokens it keeps in the platform's store;
// MSB the microsandbox runtime. Force removes sandboxes with running
// sessions or unpushed work.
type DestroyOptions struct {
	All, Secrets, MSB, Force bool
}

// RuntimeCleaner is a runtime that keeps, outside its sandboxes, what
// unibox made in it - secrets, approvals - and can take it away.
type RuntimeCleaner interface {
	Clean(ctx context.Context, log io.Writer) error
}

// Destroy takes unibox back to where it was before it was used. Unless
// forced, it checks every sandbox before removing any, so work is never
// half lost: one with a running session or unpushed work stops it all.
func (m *Manager) Destroy(ctx context.Context, o DestroyOptions) error {
	if o.All && m.Config.Environments != "" {
		for _, dir := range []string{m.Paths.Config, m.Paths.Cache} {
			if rel, err := filepath.Rel(dir, m.Config.Environments); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
				return fmt.Errorf("the environments repository %s lies in %s, which --all removes; move it elsewhere first (unibox config set environments <path>)", m.Config.Environments, dir)
			}
		}
	}
	s, err := m.Paths.ReadState()
	if err != nil {
		return err
	}
	names := sortedKeys(s.Sandboxes)
	if !o.Force {
		for _, name := range names {
			if err := m.checkRemovable(ctx, s.Sandboxes[name]); err != nil {
				return fmt.Errorf("%w; nothing was removed: preserve the work, or pass --force to discard it", err)
			}
		}
	}
	for _, name := range names {
		b := s.Sandboxes[name]
		if _, err := m.Runtime(b.Runtime); err != nil {
			// The runtime is gone, and its sandboxes with it.
			fmt.Fprintf(m.Log, "sandbox %s: %v; forgetting it\n", name, err)
			if err := m.Paths.State(func(s *State) error { delete(s.Sandboxes, name); return nil }); err != nil {
				return err
			}
			continue
		}
		if err := m.remove(ctx, name, "", true); err != nil {
			return fmt.Errorf("sandbox %s: %w", name, err)
		}
		fmt.Fprintf(m.Log, "removed sandbox %s\n", name)
	}
	// Sandboxes of unibox's the state no longer knows.
	for _, rt := range sortedKeys(m.Runtimes) {
		r := m.Runtimes[rt]
		if !InUse(ctx, r) {
			continue
		}
		boxes, err := r.List(ctx)
		if err != nil {
			return err
		}
		for _, box := range boxes {
			if strings.HasPrefix(box.Name, "unibox-") {
				if err := r.Remove(ctx, box.Name); err != nil {
					return fmt.Errorf("%s sandbox %s: %w", rt, box.Name, err)
				}
				fmt.Fprintf(m.Log, "removed %s sandbox %s, which unibox no longer knew\n", rt, box.Name)
			}
		}
	}
	if o.All {
		for _, rt := range sortedKeys(m.Runtimes) {
			r := m.Runtimes[rt]
			if !InUse(ctx, r) {
				continue
			}
			if pruner, ok := r.(ImagePruner); ok {
				images, err := pruner.UniboxImages(ctx)
				if err != nil {
					return err
				}
				for _, im := range images {
					m.removeImage(ctx, pruner, im.Ref)
				}
			}
			if c, ok := r.(RuntimeCleaner); ok {
				if err := c.Clean(ctx, m.Log); err != nil {
					return fmt.Errorf("%s: %w", rt, err)
				}
			}
		}
		if _, err := os.Stat(m.pluginPath()); err == nil {
			if err := m.UninstallPlugin(ctx); err != nil {
				fmt.Fprintf(m.Log, "the herdr plugin stays linked (%v); herdr plugin unlink %s removes it\n", err, pluginID)
			}
		}
	}
	if o.Secrets {
		store, err := PlatformSecrets()
		if err != nil {
			return err
		}
		names, err := store.List(ctx)
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := store.Remove(ctx, name); err != nil {
				return fmt.Errorf("secret %s: %w", name, err)
			}
			fmt.Fprintf(m.Log, "removed secret %s from %s\n", name, store.Name())
		}
	}
	if o.MSB {
		if home, err := os.UserHomeDir(); err == nil {
			dir := filepath.Join(home, ".microsandbox")
			if _, err := os.Stat(dir); err == nil {
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
				fmt.Fprintf(m.Log, "removed %s\n", dir)
			}
		}
	}
	if o.All {
		// Last, as the locks live there: unibox's state, configuration and
		// the person's vars.toml, and what it cached. The environments
		// repository is the user's work and stays; only the configuration
		// naming it goes.
		for _, dir := range []string{m.Paths.Cache, m.Paths.Config} {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			fmt.Fprintf(m.Log, "removed %s\n", dir)
		}
	}
	return nil
}

// checkRemovable says why a sandbox cannot be removed without losing
// something: a running session, or work not pushed.
func (m *Manager) checkRemovable(ctx context.Context, b api.Sandbox) error {
	u, err := m.usage(b.Name, false)
	if err != nil {
		return fmt.Errorf("sandbox %s has a running session", b.Name)
	}
	defer u.Close()
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return nil
	}
	if _, exists, err := m.findBox(ctx, r, b); err != nil || !exists {
		return err
	}
	return m.safeBox(ctx, b, r)
}

// Clean takes away what unibox made in sbx outside its sandboxes: the
// custom secrets carrying unibox's placeholders, a service's secret read
// by unibox's lookup, the approvals unibox added to credentials.yaml, and
// its generated kits. What the user made with sbx stays.
func (r *SBX) Clean(ctx context.Context, log io.Writer) error {
	b, err := commandOutput(hostCommand(ctx, "sbx", "secret", "ls", "--json"))
	if err != nil {
		return err
	}
	var list struct {
		Secrets []struct {
			Scope, Name, Type, Secret string
		} `json:"secrets"`
		Custom []struct {
			Scope       string `json:"scope"`
			Env         string `json:"env"`
			Placeholder string `json:"placeholder"`
		} `json:"custom_secrets"`
	}
	if err = json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("could not read sbx secret ls --json: %w", err)
	}
	for _, c := range list.Custom {
		if !strings.Contains(c.Placeholder, "unibox") {
			continue
		}
		args := []string{"secret", "rm", "--placeholder", c.Placeholder, "-f"}
		if c.Scope != "global" {
			args = append(args, "--sandbox", c.Scope)
		}
		if err := runQuiet(hostCommand(ctx, "sbx", args...)); err != nil {
			return fmt.Errorf("custom secret %s: %w", c.Env, err)
		}
		fmt.Fprintf(log, "removed sbx secret %s (%s)\n", c.Env, c.Scope)
	}
	for _, s := range list.Secrets {
		// Only a secret read by unibox's own lookup is known to be unibox's;
		// a login made with sbx is the user's.
		if !strings.HasPrefix(s.Secret, "command:") || !strings.Contains(s.Secret, "unibox") {
			continue
		}
		args := []string{"secret", "rm", s.Name, "-f"}
		if s.Scope != "global" {
			args = append(args, "--sandbox", s.Scope)
		}
		if err := runQuiet(hostCommand(ctx, "sbx", args...)); err != nil {
			return fmt.Errorf("sbx secret %s: %w", s.Name, err)
		}
		fmt.Fprintf(log, "removed sbx secret %s (%s)\n", s.Name, s.Scope)
	}
	if err := removeBindings(r.Bindings, log); err != nil {
		return err
	}
	return nil
}

// removeBindings takes out of sbx's credentials.yaml the domains unibox's
// kits approve, and what that leaves empty; others stay.
func removeBindings(file string, log io.Writer) error {
	if file == "" {
		return nil
	}
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	doc := map[string]any{}
	if err = yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("sbx credential bindings in %s are not valid YAML; repair the file: %w", file, err)
	}
	bindings, _ := doc["bindings"].(map[string]any)
	removed := 0
	for _, want := range sbxKitBindings {
		service, _ := bindings[want.Service].(map[string]any)
		if service == nil {
			continue
		}
		for mechanism, domains := range map[string][]string{"apiKey": want.APIKey, "oauth": want.OAuth} {
			entry, _ := service[mechanism].(map[string]any)
			list, _ := entry["domains"].([]any)
			kept := slices.DeleteFunc(slices.Clone(list), func(d any) bool { s, _ := d.(string); return slices.Contains(domains, s) })
			removed += len(list) - len(kept)
			if len(kept) == 0 && entry != nil {
				delete(entry, "domains")
			} else if entry != nil {
				entry["domains"] = kept
			}
			if entry != nil && len(entry) == 0 {
				delete(service, mechanism)
			}
		}
		if len(service) == 0 {
			delete(bindings, want.Service)
		}
	}
	if removed == 0 {
		return nil
	}
	if len(bindings) == 0 {
		delete(doc, "bindings")
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if len(doc) == 0 {
		out = nil
	}
	if err = AtomicWrite(file, out, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(log, "removed %d domain approvals unibox added from %s\n", removed, file)
	return nil
}
