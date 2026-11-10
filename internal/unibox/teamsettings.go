package unibox

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tobola/unibox/internal/version"
)

// TeamSettingKeys are the person's settings for the whole machine a team
// may give a default for under settings: in its unibox.yaml files, in the
// order init asks them. Each is the person's to change; the team's value
// is taken only while theirs is unsaid, and init asks again, the team's
// first, where theirs differs.
var TeamSettingKeys = []string{"ssh_agent", "ssh_sandbox", "remember_host_commands", "clipboard_text_copy", "clipboard_image_paste", "claude_remote_control"}

// checkTeamSettings refuses a key no person's setting has.
func checkTeamSettings(file string, settings map[string]bool) error {
	for key := range settings {
		if !slices.Contains(TeamSettingKeys, key) {
			return fmt.Errorf("%s: settings: %s is none of %v", file, key, TeamSettingKeys)
		}
	}
	return nil
}

// TeamSettings are the settings the team's files of the envs agree on,
// each env's a later file's over an earlier one's; differ names those
// the envs give different values, which none of them decides.
func TeamSettings(envs [][]string) (settings map[string]bool, differ []string, err error) {
	settings = map[string]bool{}
	for _, files := range envs {
		own := map[string]bool{}
		if err := readTeamFiles(files, func(name, _ string, doc teamDoc) error {
			if err := checkTeamSettings(name, doc.Settings); err != nil {
				return err
			}
			for k, v := range doc.Settings {
				own[k] = v
			}
			return nil
		}); err != nil {
			return nil, nil, err
		}
		for k, v := range own {
			if was, ok := settings[k]; ok && was != v || slices.Contains(differ, k) {
				delete(settings, k)
				if !slices.Contains(differ, k) {
					differ = append(differ, k)
				}
				continue
			}
			settings[k] = v
		}
	}
	slices.Sort(differ)
	return settings, differ, nil
}

// Setting is one of the person's settings for the whole machine by its
// key in config.yaml: nil when unsaid.
func (c *Config) Setting(key string) **bool {
	switch key {
	case "ssh_agent":
		return &c.SSHAgentOn
	case "ssh_sandbox":
		return &c.EditorSSHOn
	}
	return c.SBXSetting(key)
}

// adoptTeamSettings keeps the env's team's value of each setting the
// person has not said, so a sandbox starts as the team meant without a
// question; what the person said stays theirs. sbx is set to match by
// syncSBXSettings, but for its SSH agent forwarding, which is checked
// before then and so is set here.
func (m *Manager) adoptTeamSettings(ctx context.Context, env Env) error {
	adopted := []string{}
	for _, key := range TeamSettingKeys {
		if _, ok := env.Settings[key]; ok && *m.Config.Setting(key) == nil {
			adopted = append(adopted, key)
		}
	}
	if len(adopted) == 0 {
		return nil
	}
	c, err := m.Paths.LoadConfig()
	if err != nil {
		return err
	}
	for _, key := range adopted {
		v := env.Settings[key]
		*c.Setting(key) = &v
	}
	if err := m.Paths.SaveConfig(c); err != nil {
		return err
	}
	m.Config = c
	for _, key := range adopted {
		fmt.Fprintf(m.Log, "%s is %t, as env %s's team says; unibox config set %s changes it\n", key, env.Settings[key], env.Name, key)
	}
	if slices.Contains(adopted, "ssh_agent") {
		on := env.Settings["ssh_agent"]
		m.SBX.AllowSSHAgent(on)
		if now, err := m.SBX.SSHForwarding(ctx); err == nil && now != on {
			m.progress("setting sbx's SSH agent forwarding as the team says, restarting its daemon")
			return m.SBX.SetSSHForwarding(ctx, on)
		}
	}
	return nil
}

var uniboxRelease = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?`)

// UniboxAtLeast says whether a release of unibox is need or newer. A
// nightly counts as the release it leads to, and a build of no release, as
// a developer's, is never refused.
func UniboxAtLeast(have, need string) bool {
	a := uniboxRelease.FindStringSubmatch(have)
	if a == nil {
		return true
	}
	b := uniboxRelease.FindStringSubmatch(need)
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return true
}

// requireUnibox refuses a team's file that needs a newer unibox than this.
func requireUnibox(file, need string) error {
	if need == "" {
		return nil
	}
	if !uniboxRelease.MatchString(need) {
		return fmt.Errorf("%s: min_unibox is a release such as \"0.10\", not %q", file, need)
	}
	if have := version.String(); !UniboxAtLeast(have, need) {
		return Fail(4, "unibox_outdated", "%s needs unibox %s or newer, and this is %s; %s, then run it again", file, need, have, upgradeHint())
	}
	return nil
}

// upgradeHint says how this unibox is brought to the latest release, on
// its channel.
func upgradeHint() string {
	exe, _ := os.Executable()
	if hint := UpgradeHint(exe, version.String()); !strings.HasPrefix(hint, "https://") {
		return hint
	}
	return "https://github.com/bitbyone/unibox/releases/latest"
}
