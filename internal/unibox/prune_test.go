package unibox

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

// pruneRuntime is the fake runtime with images unibox can list and remove.
type pruneRuntime struct {
	*fakeRuntime
	mu     sync.Mutex
	images []RuntimeImage
}

func (p *pruneRuntime) UniboxImages(context.Context) ([]RuntimeImage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.images), nil
}

func (p *pruneRuntime) RemoveImage(_ context.Context, ref string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.images = slices.DeleteFunc(p.images, func(im RuntimeImage) bool { return im.Ref == ref })
	return nil
}

func (p *pruneRuntime) refs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []string{}
	for _, im := range p.images {
		out = append(out, im.Ref)
	}
	slices.Sort(out)
	return out
}

func TestOldImagesGoAfterABuildAndOnPrune(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pr := &pruneRuntime{fakeRuntime: f, images: []RuntimeImage{
		{Ref: "unibox/node:old", Profile: "node"},
		{Ref: "unibox/gone:abc", Profile: "gone"},
		{Ref: "sbx-kit-src:unibox-dev-aaaaaaaaaaaa", Sandbox: "unibox-dev", Created: day},
		{Ref: "sbx-kit-src:unibox-dev-bbbbbbbbbbbb", Sandbox: "unibox-dev", Created: day.Add(time.Hour)},
		{Ref: "sbx-kit-src:unibox-gone-cccccccccccc", Sandbox: "unibox-gone", Created: day},
	}}
	m.Runtimes = map[string]Runtime{"fake": pr}
	ctx := context.Background()
	if err := m.Build(ctx, "node", "fake", true); err != nil {
		t.Fatal(err)
	}
	p, _ := m.profile("node")
	pr.mu.Lock()
	pr.images = append(pr.images, RuntimeImage{Ref: imageTag(p), Profile: "node"})
	pr.mu.Unlock()
	// A build takes its profile's older builds with it, no other's.
	if err := m.Build(ctx, "node", "fake", true); err != nil {
		t.Fatal(err)
	}
	if got := pr.refs(); slices.Contains(got, "unibox/node:old") || !slices.Contains(got, imageTag(p)) || !slices.Contains(got, "unibox/gone:abc") {
		t.Fatalf("%v", got)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	// Making a sandbox takes its older kit images with it.
	if got := pr.refs(); slices.Contains(got, "sbx-kit-src:unibox-dev-aaaaaaaaaaaa") || !slices.Contains(got, "sbx-kit-src:unibox-dev-bbbbbbbbbbbb") {
		t.Fatalf("%v", got)
	}
	// prune takes what belongs to profiles and sandboxes that are gone.
	removed, err := m.Prune(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, []string{"sbx-kit-src:unibox-gone-cccccccccccc", "unibox/gone:abc"}) {
		t.Fatalf("%v", removed)
	}
	if got := pr.refs(); !slices.Equal(got, []string{"sbx-kit-src:unibox-dev-bbbbbbbbbbbb", imageTag(p)}) {
		t.Fatalf("%v", got)
	}
}

func TestSbxKitTagsNameTheirSandbox(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]string{
		"unibox-dev-fdd02543f015":    "unibox-dev",
		"unibox-dev-be-0123456789ab": "unibox-dev-be",
		"personal-kit-7ed50e732a0e":  "",
		"unibox-dev-short":           "",
	} {
		got := ""
		if m := sbxKitTag.FindStringSubmatch(tag); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s: %q", tag, got)
		}
	}
}
