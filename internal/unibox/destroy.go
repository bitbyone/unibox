package unibox

import (
	"context"
	"fmt"
	"os"

	"github.com/tobola/unibox/internal/unibox/api"
)

// DestroyOptions say how far unibox destroy goes. Without All it removes
// every sandbox unibox created and so every task; All removes what unibox
// keeps on the host besides. Force removes sandboxes with running sessions
// or unpushed work. The env files, the kits and the secrets in sbx are
// the team's and the user's, and stay.
type DestroyOptions struct {
	All, Force bool
}

// Destroy takes unibox back to where it was before it was used. Unless
// forced, it checks every sandbox before removing any, so work is never
// half lost: one with a running session or unpushed work stops it all.
func (m *Manager) Destroy(ctx context.Context, o DestroyOptions) error {
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
		if s.Sandboxes[name].Adopted {
			// An adopted sandbox was the user's before unibox knew it, and
			// stays theirs.
			if err := m.Paths.State(func(s *State) error { delete(s.Sandboxes, name); return nil }); err != nil {
				return err
			}
			fmt.Fprintf(m.Log, "forgot sandbox %s, which unibox adopted; sbx rm %s removes it\n", name, name)
			continue
		}
		if err := m.remove(ctx, name, "", true); err != nil {
			return fmt.Errorf("sandbox %s: %w", name, err)
		}
		fmt.Fprintf(m.Log, "removed sandbox %s\n", name)
	}
	if o.All {
		if _, err := os.Stat(m.pluginPath()); err == nil {
			if err := m.UninstallPlugin(ctx); err != nil {
				fmt.Fprintf(m.Log, "the herdr plugin stays linked (%v); herdr plugin unlink %s removes it\n", err, pluginID)
			}
		}
		// Last, as the locks live there: unibox's state, configuration and
		// the person's args, and what it cached.
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
	if _, exists, err := m.findBox(ctx, b); err != nil || !exists {
		return err
	}
	return m.safeBox(ctx, b)
}

// PruneMounts unmounts what a running sandbox has mounted under /work
// that no task of it has any longer, as a task removed outside unibox
// leaves behind, each as "sandbox: host at target". A sandbox another
// command is busy with is left for the next prune and named in busy.
func (m *Manager) PruneMounts(ctx context.Context) (unmounted, busy []string, err error) {
	boxes, err := m.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, b := range boxes {
		if b.State != "running" {
			continue
		}
		guard, err := m.guard(b.Name)
		if err != nil {
			busy = append(busy, b.Name)
			continue
		}
		err = func() error {
			defer guard.Close()
			current, err := m.sandbox(b.Name)
			if err != nil {
				return err
			}
			_, stray, err := m.MountDrift(ctx, current)
			if err != nil {
				return err
			}
			for _, mt := range stray {
				if err := m.SBX.Unmount(ctx, b.Name, mt[0], mt[1]); err != nil {
					return err
				}
				unmounted = append(unmounted, fmt.Sprintf("%s: %s at %s", b.Name, mt[0], mt[1]))
			}
			return nil
		}()
		if err != nil {
			return unmounted, busy, err
		}
	}
	return unmounted, busy, nil
}

// Prune forgets the sandboxes sbx no longer has, removed with sbx itself,
// and says which.
func (m *Manager) Prune(ctx context.Context) ([]string, error) {
	boxes, err := m.SBX.List(ctx)
	if err != nil {
		return nil, err
	}
	gone := []string{}
	err = m.Paths.State(func(s *State) error {
		for _, name := range sortedKeys(s.Sandboxes) {
			found := false
			for _, v := range boxes {
				found = found || v.Name == name
			}
			if !found && len(s.Sandboxes[name].Runs) == 0 {
				delete(s.Sandboxes, name)
				gone = append(gone, name)
			}
		}
		return nil
	})
	return gone, err
}
