package unibox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
