package unibox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// herdrHint names the agent a session's sbx exec runs. herdr recognises an
// agent by the processes of its pane's foreground job, which see only sbx,
// and takes HERDR_AGENT in the environment of one of them for the agent.
func herdrHint(agent string) []string {
	if agent == "" {
		return nil
	}
	return []string{"HERDR_AGENT=" + agent}
}

// herdrTerminalEnv tells Copilot in the sandbox that it draws in a herdr
// pane. Copilot knows herdr by HERDR_ENV, HERDR_PANE_ID or
// HERDR_SOCKET_PATH, which stay on the host - they name the host's socket
// - so it takes TERM_PROGRAM=herdr for a terminal it does not know and
// shows no images. COPILOT_MULTIPLEXER says herdr without them. Copilot
// shows images in herdr only when COPILOT_INLINE_IMAGES_HERDR=1, as it
// cannot see from inside a pane whether herdr draws them; unibox can, from
// herdr's config, and the host's own value of either variable wins.
func herdrTerminalEnv() []string {
	if os.Getenv("HERDR_ENV") == "" && os.Getenv("HERDR_PANE_ID") == "" && os.Getenv("HERDR_SOCKET_PATH") == "" {
		return nil
	}
	multiplexer := os.Getenv("COPILOT_MULTIPLEXER")
	if multiplexer == "" {
		multiplexer = "herdr"
	}
	env := []string{"COPILOT_MULTIPLEXER=" + multiplexer}
	images, set := os.LookupEnv("COPILOT_INLINE_IMAGES_HERDR")
	if !set && herdrDrawsImages() {
		images = "1"
	}
	if images != "" {
		env = append(env, "COPILOT_INLINE_IMAGES_HERDR="+images)
	}
	return env
}

// herdrDrawsImages reads terminal.kitty_graphics from herdr's config, or
// experimental.kitty_graphics it replaced. Unset, herdr draws images from
// 0.9.0 on and not before.
func herdrDrawsImages() bool {
	var config struct {
		Terminal struct {
			KittyGraphics *bool `toml:"kitty_graphics"`
		} `toml:"terminal"`
		Experimental struct {
			KittyGraphics *bool `toml:"kitty_graphics"`
		} `toml:"experimental"`
	}
	if b, err := os.ReadFile(herdrConfigPath()); err == nil {
		_ = toml.Unmarshal(b, &config)
	}
	if v := config.Terminal.KittyGraphics; v != nil {
		return *v
	}
	if v := config.Experimental.KittyGraphics; v != nil {
		return *v
	}
	return os.Getenv("TERM_PROGRAM") == "herdr" && VersionAtLeast(os.Getenv("TERM_PROGRAM_VERSION"), "0.9.0")
}

// herdrConfigPath is where herdr reads its config.toml.
func herdrConfigPath() string {
	if p := os.Getenv("HERDR_CONFIG_PATH"); p != "" {
		return p
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "herdr", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "config.toml")
}

// RemoveHerdrPlugin takes away the plugin earlier releases had herdr link,
// whose observer HERDR_AGENT replaced: left linked, herdr would run a
// command unibox no longer has at each pane it opens.
func (m *Manager) RemoveHerdrPlugin(ctx context.Context) {
	dir := filepath.Join(m.Paths.Config, "herdr-plugin")
	if _, err := os.Stat(dir); err != nil {
		return
	}
	binary := "herdr"
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		binary = p
	}
	cmd := hostCommand(ctx, binary, "plugin", "unlink", "unibox")
	if socket := os.Getenv("HERDR_SOCKET_PATH"); socket != "" {
		cmd.Env = append(cmd.Env, "HERDR_SOCKET_PATH="+socket)
	}
	if _, err := commandOutput(cmd); err != nil {
		fmt.Fprintln(m.Log, "unibox's old herdr plugin may stay linked; herdr plugin unlink unibox removes it")
	}
	_ = os.RemoveAll(dir)
}
