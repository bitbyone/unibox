package unibox

import (
	"context"
	"fmt"
	"slices"
)

// pruneBuilds removes a profile's images but the one of its revision, after
// a build made that one: a sandbox does not need the image it was made
// from once it exists, and each build of a profile is gigabytes.
func (m *Manager) pruneBuilds(ctx context.Context, r Runtime, p Profile) {
	pruner, ok := r.(ImagePruner)
	if !ok {
		return
	}
	images, err := pruner.UniboxImages(ctx)
	if err != nil {
		fmt.Fprintf(m.Log, "old images of %s kept: %v\n", p.Name, err)
		return
	}
	for _, im := range images {
		if im.Profile == p.Name && im.Ref != imageTag(p) {
			m.removeImage(ctx, pruner, im.Ref)
		}
	}
}

// pruneKits removes the images the runtime made for a sandbox but the
// newest, which the sandbox was made from and a recreation reuses.
func (m *Manager) pruneKits(ctx context.Context, r Runtime, runtimeName string) {
	pruner, ok := r.(ImagePruner)
	if !ok {
		return
	}
	images, err := pruner.UniboxImages(ctx)
	if err != nil {
		return
	}
	for _, ref := range olderKits(images, runtimeName) {
		m.removeImage(ctx, pruner, ref)
	}
}

// olderKits are a sandbox's runtime-made images but the newest.
func olderKits(images []RuntimeImage, runtimeName string) []string {
	kits := []RuntimeImage{}
	for _, im := range images {
		if im.Sandbox == runtimeName {
			kits = append(kits, im)
		}
	}
	slices.SortFunc(kits, func(a, b RuntimeImage) int { return b.Created.Compare(a.Created) })
	out := []string{}
	for i, im := range kits {
		if i > 0 {
			out = append(out, im.Ref)
		}
	}
	return out
}

func (m *Manager) removeImage(ctx context.Context, pruner ImagePruner, ref string) bool {
	if err := pruner.RemoveImage(ctx, ref); err != nil {
		fmt.Fprintf(m.Log, "image %s kept: %v\n", ref, err)
		return false
	}
	fmt.Fprintf(m.Log, "removed image %s\n", ref)
	return true
}

// Prune removes, in every runtime, the images unibox no longer needs: the
// builds of a profile that is gone, and of one that is there but its
// current; the runtime-made images of a sandbox that is gone, and of one
// that is there but its newest. A profile that does not load keeps its
// builds, as which of them is current cannot be told.
func (m *Manager) Prune(ctx context.Context) ([]string, error) {
	names, err := ProfileNames(m.Config.Environments)
	if err != nil {
		return nil, err
	}
	current := map[string]string{}
	broken := map[string]bool{}
	for _, name := range names {
		if p, err := m.profile(name); err == nil {
			current[name] = imageTag(p)
		} else {
			broken[name] = true
		}
	}
	s, err := m.Paths.ReadState()
	if err != nil {
		return nil, err
	}
	sandboxes := map[string]bool{}
	for _, b := range s.Sandboxes {
		sandboxes[b.RuntimeName] = true
	}
	removed := []string{}
	for _, rt := range sortedKeys(m.Runtimes) {
		pruner, ok := m.Runtimes[rt].(ImagePruner)
		if !ok || m.Runtimes[rt].Available(ctx) != nil {
			continue
		}
		images, err := pruner.UniboxImages(ctx)
		if err != nil {
			return removed, err
		}
		stale := map[string]bool{}
		for _, im := range images {
			switch {
			case im.Profile != "" && !broken[im.Profile] && im.Ref != current[im.Profile]:
				stale[im.Ref] = true
			case im.Sandbox != "" && !sandboxes[im.Sandbox]:
				stale[im.Ref] = true
			}
		}
		for name := range sandboxes {
			for _, ref := range olderKits(images, name) {
				stale[ref] = true
			}
		}
		for _, ref := range sortedKeys(stale) {
			if m.removeImage(ctx, pruner, ref) {
				removed = append(removed, ref)
			}
		}
	}
	return removed, nil
}
