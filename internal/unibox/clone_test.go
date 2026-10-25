package unibox

import (
	"path/filepath"
	"testing"
)

func TestAGitAddressIsClonedUnderItsHostAndPath(t *testing.T) {
	t.Parallel()
	p := Paths{Config: "/c", Data: "/d"}
	for address, want := range map[string]string{
		"git@github.com:team/agent-sandboxes.git":  "/d/repos/github.com/team/agent-sandboxes",
		"https://gitlab.example.com/a/b/envs.git":  "/d/repos/gitlab.example.com/a/b/envs",
		"ssh://git@gitlab.example.com:2222/a/envs": "/d/repos/gitlab.example.com/a/envs",
		"file:///srv/git/envs.git":                 "/d/repos/local/srv/git/envs",
	} {
		if !IsGitURL(address) {
			t.Errorf("%s is not taken for an address", address)
		}
		if got, err := p.CloneDir(address); err != nil || got != filepath.FromSlash(want) {
			t.Errorf("%s: %s, %v", address, got, err)
		}
	}
	for _, path := range []string{".", "~/work/envs", "/srv/envs", "envs:dev", `C:\envs`} {
		if IsGitURL(path) {
			t.Errorf("%s is taken for an address", path)
		}
	}
	for _, address := range []string{"https://example.com/", "https://example.com/a/../b", "git@host:"} {
		if _, err := p.CloneDir(address); err == nil {
			t.Errorf("%s was given a directory", address)
		}
	}
}
