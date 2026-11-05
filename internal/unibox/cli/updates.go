package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/version"
	"golang.org/x/term"
)

// noticeStyle and noticeRun draw what is newer as an update is told in a
// terminal: a word in the update's colour, the command in bold.
var (
	noticeStyle = lipgloss.NewStyle().Foreground(themeMount)
	noticeRun   = lipgloss.NewStyle().Foreground(themeMount).Bold(true)
)

// noUpdateNotice are the commands after which nothing is told: doctor
// tells it itself, and the check is the command that finds it.
var noUpdateNotice = []string{"doctor", "update-check", "__complete", "__completeNoDesc", "completion"}

// atTerminal says whether a person reads what unibox writes: its input
// and its log are a terminal, and nothing asked for JSON.
func (o *options) atTerminal() bool {
	if !o.interactive || o.json {
		return false
	}
	f, ok := o.log.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// prepareUpdates starts a check of what is newer when one is due, in a
// process of its own that the command does not wait for, and sets the
// line every list shows from what the last one found.
func (o *options) prepareUpdates(cmd *cobra.Command) {
	if !o.atTerminal() || slices.Contains(noUpdateNotice, cmd.Name()) || !o.manager.Config.UpdateChecks() {
		return
	}
	if releases, repos, err := o.paths.ClaimCheck(time.Now()); err == nil && (releases || repos) {
		o.startUpdateCheck(releases, repos)
	}
	if notices := o.notices(); len(notices) > 0 {
		updateNotice = noticeText(notices[0])
		if len(notices) > 1 {
			updateNotice += noticeStyle.Render(fmt.Sprintf("  +%d more", len(notices)-1))
		}
	}
}

func (o *options) startUpdateCheck(releases, repos bool) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	args := []string{"update-check"}
	if releases {
		args = append(args, "--releases")
	}
	if repos {
		args = append(args, "--repos")
	}
	// Nothing of it reaches the terminal, nor does it hold the command up:
	// what it finds is shown by the commands after it.
	c := exec.Command(exe, args...)
	if c.Start() == nil {
		_ = c.Process.Release()
	}
}

func (o *options) notices() []unibox.Notice {
	exe, _ := os.Executable()
	sbx, _ := exec.LookPath("sbx")
	return o.paths.ReadUpdates().Notices(resolved(exe), resolved(sbx), o.manager.Config.ReposOfEnvs())
}

// resolved is a program's path past its links, where Homebrew's Cellar or
// Caskroom shows.
func resolved(path string) string {
	if path == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// tellUpdates writes what is newer after a command, a line each.
func (o *options) tellUpdates(cmd *cobra.Command) {
	if updateNotice == "" || slices.Contains(noUpdateNotice, cmd.Name()) {
		return
	}
	for _, n := range o.notices() {
		fmt.Fprintln(o.log, noticeText(n))
	}
}

// updateCheckCommand is the check itself, which a command starts in the
// background when one is due; hidden, as nobody runs it by hand.
func (o *options) updateCheckCommand() *cobra.Command {
	var releases, repos bool
	c := &cobra.Command{Use: "update-check", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		check := unibox.UpdateCheck{Unibox: version.String()}
		for repo := range o.manager.Config.ReposOfEnvs() {
			check.Repos = append(check.Repos, repo)
		}
		slices.Sort(check.Repos)
		o.paths.Check(cmd.Context(), o.manager.SBX, check, releases, repos)
		return nil
	}}
	c.Flags().BoolVar(&releases, "releases", false, "Ask GitHub for unibox's and sbx's newest releases")
	c.Flags().BoolVar(&repos, "repos", false, "Fetch the env repositories")
	return c
}

// noticeText is a notice as a terminal shows it.
func noticeText(n unibox.Notice) string {
	if strings.HasPrefix(n.Run, "https://") {
		return noticeStyle.Render(n.What+" · ") + noticeRun.Render(n.Run)
	}
	return noticeStyle.Render(n.What+" · run ") + noticeRun.Render(n.Run)
}

// noticeHint is a doctor check's hint for a notice: its command, or the
// page it names.
func noticeHint(n unibox.Notice) string {
	if strings.HasPrefix(n.Run, "https://") {
		return "update from " + n.Run
	}
	return n.Run
}

// doctorReleases asks GitHub, now rather than from the cache, whether a
// newer unibox or sbx is out, and keeps the answer for the commands after.
func (o *options) doctorReleases(ctx context.Context, add func(area, name, status, message, hint string)) {
	exe, _ := os.Executable()
	sbxPath, _ := exec.LookPath("sbx")
	var newUnibox, newSBX string
	var uniboxErr, sbxErr error
	_ = o.working("asking GitHub for the newest releases", func() error {
		newUnibox, uniboxErr = unibox.NewerUnibox(ctx, version.String())
		installed, err := o.manager.SBX.Version(ctx)
		if err != nil {
			sbxErr = err
			return nil
		}
		newSBX, sbxErr = unibox.NewerRelease(ctx, "docker/sbx-releases", installed)
		return nil
	})
	switch {
	case !unibox.IsRelease(version.String()):
		add("Updates", "unibox", "pass", version.String()+", a build of your own, compared with no release", "")
	case uniboxErr != nil:
		add("Updates", "unibox", "warn", "not checked: "+uniboxErr.Error(), "")
	case newUnibox != "":
		add("Updates", "unibox", "warn", newUnibox+" is out, this is "+version.String(), noticeHint(unibox.Notice{Run: unibox.UpgradeHint(resolved(exe), newUnibox)}))
	default:
		add("Updates", "unibox", "pass", version.String()+", the newest", "")
	}
	switch {
	case sbxErr != nil:
		add("Updates", "sbx", "warn", "not checked: "+sbxErr.Error(), "")
	case newSBX != "":
		add("Updates", "sbx", "warn", newSBX+" is out", noticeHint(unibox.Notice{Run: unibox.SBXUpgradeHint(resolved(sbxPath), newSBX)}))
	default:
		add("Updates", "sbx", "pass", "the newest", "")
	}
	if uniboxErr == nil && sbxErr == nil {
		o.paths.KeepReleases(time.Now(), newUnibox, newSBX)
	}
}
