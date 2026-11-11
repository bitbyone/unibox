package unibox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tobola/unibox/internal/testenv"
)

func TestReleasesAreOrderedWithAPrereleaseBeforeItsRelease(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.7", "0.7-pre.4", 1},
		{"0.7-pre.4", "0.7-pre.10", -1},
		{"0.8", "0.7", 1},
		{"v0.51.0", "0.50.1", 1},
		{"0.51.0", "0.51", 0},
	} {
		if got := CompareReleases(c.a, c.b); got != c.want {
			t.Errorf("CompareReleases(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// A newer release is offered on the channel installed: a stable unibox is
// never offered a nightly, a nightly one is offered either.
func TestANewerReleaseIsOfferedOnItsChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v0.9-pre.1"},{"tag_name":"v0.10","draft":true},{"tag_name":"v0.8"},{"tag_name":"v0.7"}]`))
	}))
	defer server.Close()
	was := releasesAPI
	releasesAPI = server.URL
	defer func() { releasesAPI = was }()
	ctx := context.Background()
	for installed, want := range map[string]string{"0.7": "0.8", "0.8": "", "0.8-pre.2": "0.9-pre.1", "dev (abc)": ""} {
		if got, err := NewerUnibox(ctx, installed); err != nil || got != want {
			t.Errorf("NewerUnibox(%s) = %q %v, want %q", installed, got, err, want)
		}
	}
}

func TestAnUpdateIsBroughtAsItWasInstalled(t *testing.T) {
	t.Parallel()
	for exe, want := range map[string]string{
		"/opt/homebrew/Cellar/unibox/0.7/bin/unibox":       "brew upgrade unibox",
		`C:\Users\me\scoop\apps\unibox\current\unibox.exe`: "scoop update unibox",
		"/home/me/.local/bin/unibox":                       "https://github.com/bitbyone/unibox/releases/tag/v0.8",
	} {
		if got := UpgradeHint(exe, "0.8"); got != want {
			t.Errorf("UpgradeHint(%s) = %q, want %q", exe, got, want)
		}
	}
	if got := UpgradeHint("/opt/homebrew/Cellar/unibox-nightly/0.8-pre.1/bin/unibox", "0.8-pre.2"); got != "brew upgrade unibox-nightly" {
		t.Error(got)
	}
	if got := SBXUpgradeHint("/opt/homebrew/Caskroom/sbx/0.51.0/Sbx.app/Contents/MacOS/sbx", "0.52.0"); got != "brew upgrade sbx" {
		t.Error(got)
	}
}

// A check is claimed once while it is due, so commands started meanwhile
// start no other, and what it found stays when a source cannot be
// reached the next time.
func TestACheckIsClaimedOnceAndKeepsWhatItCouldNotAskAgain(t *testing.T) {
	t.Parallel()
	p := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	now := time.Now()
	if releases, repos, err := p.ClaimCheck(now); err != nil || !releases || !repos {
		t.Fatalf("%v %v %v", releases, repos, err)
	}
	if releases, repos, _ := p.ClaimCheck(now.Add(time.Hour)); releases || repos {
		t.Fatalf("claimed again: %v %v", releases, repos)
	}
	if releases, repos, _ := p.ClaimCheck(now.Add(5 * time.Hour)); releases || !repos {
		t.Fatalf("%v %v", releases, repos)
	}
	gone := t.TempDir()
	p.keep(func(u *Updates) { u.Behind = map[string]int{gone: 3, "/no/longer/an/env": 1} })
	u := p.Check(context.Background(), nil, UpdateCheck{Repos: []string{gone}}, false, true)
	if u.Behind[gone] != 3 || len(u.Behind) != 1 {
		t.Fatalf("%v", u.Behind)
	}
	notices := u.Notices("", "", map[string][]string{gone: {"dev"}})
	if len(notices) != 1 || notices[0].Run != "unibox env update dev" {
		t.Fatalf("%+v", notices)
	}
	if slices.ContainsFunc(u.Notices("", "", nil), func(n Notice) bool { return n.Run != "" }) {
		t.Fatal("a repository no env is in was told of")
	}
}

// A newer release found is told only while what it was found newer than
// is still installed: after an upgrade unibox forgets it and asks again.
func TestAReleaseFoundIsForgottenOnceUpgraded(t *testing.T) {
	t.Parallel()
	p := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	sbx := filepath.Join(t.TempDir(), "sbx")
	if err := testenv.WriteExecutable(sbx, []byte("0.51")); err != nil {
		t.Fatal(err)
	}
	was := UpdateCheck{Unibox: "0.10-pre.3", SBX: BinaryStamp(sbx)}
	now := time.Now()
	p.KeepReleases(now, "0.10", "0.53.0", was)
	if u := p.Current(was); u.Unibox != "0.10" || u.SBX != "0.53.0" || !u.Released.Equal(now) {
		t.Fatalf("%+v", u)
	}
	if err := testenv.WriteExecutable(sbx, []byte("0.53.0")); err != nil {
		t.Fatal(err)
	}
	u := p.Current(UpdateCheck{Unibox: "0.10-pre.3", SBX: BinaryStamp(sbx)})
	if u.Unibox != "0.10" || u.SBX != "" || !u.Released.IsZero() {
		t.Fatalf("sbx upgraded: %+v", u)
	}
	if u = p.Current(UpdateCheck{Unibox: "0.10", SBX: BinaryStamp(sbx)}); u.Unibox != "" {
		t.Fatalf("unibox upgraded: %+v", u)
	}
	if releases, _ := u.Due(now); !releases {
		t.Fatal("the releases are not asked again")
	}
}
