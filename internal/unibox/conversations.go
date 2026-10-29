package unibox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

// An agent keeps its conversations in its home in the sandbox, which goes
// with the sandbox: after a recreate, or sbx losing it, a resumed task's
// agent would have nothing to continue. So a task's conversations - those
// whose directory is the task's or one of its repositories' - are kept on
// the host after each of its sessions and before a recreate, and put back
// when the task is prepared in a sandbox made anew. Only the task's own are
// kept and put back, found as each agent's resume finds them.

// conversationsFile is where the host keeps a task's conversations.
func (p Paths) conversationsFile(task string) string {
	return filepath.Join(p.Data, "conversations", task+".tar.gz")
}

// conversationDirs are the directories a task's agent may work in: the
// task's, and each repository's in it. They are compared whole, never as a
// prefix, so the task oauth is not given oauth-2's.
func conversationDirs(t api.Task) []string {
	dirs := []string{t.Directory}
	for _, r := range t.Repositories {
		dirs = append(dirs, path.Join(t.Directory, r.Name))
	}
	return dirs
}

// conversationsScript writes to its output, as a gzipped tar of absolute
// paths, the conversations claude, codex and copilot had in dirs, and
// nothing when they had none.
func conversationsScript(dirs []string) string {
	return "set -u\nfor dir in " + quoteArgs(dirs) + `; do
c="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects/$(printf %s "$dir" | sed 's/[^A-Za-z0-9]/-/g')"
[ ! -d "$c" ] || printf '%s\n' "$c"
grep -rlsF --include='rollout-*.jsonl' "\"cwd\":\"$dir\"" "${CODEX_HOME:-$HOME/.codex}/sessions"
for w in "${COPILOT_HOME:-$HOME/.copilot}"/session-state/*/workspace.yaml; do
grep -qxF "cwd: $dir" "$w" 2>/dev/null && printf '%s\n' "${w%/workspace.yaml}"
done
done | sort -u > /tmp/unibox-conversations.$$
if [ -s /tmp/unibox-conversations.$$ ]; then tar -czPf - -T /tmp/unibox-conversations.$$; fi
status=$?
rm -f /tmp/unibox-conversations.$$
exit $status
`
}

// restoreConversationsScript unpacks kept conversations where they were,
// leaving a file the sandbox has newer as it is; tar says so of each, to
// the operation log.
const restoreConversationsScript = `tar -xzPf - --keep-newer-files`

// keepConversations copies a task's conversations from its sandbox to the
// host. A failure is told and nothing more: the session itself is over.
func (m *Manager) keepConversations(ctx context.Context, b api.Sandbox, t api.Task) {
	if m.Paths.Data == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	out, err := commandOutput(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", conversationsScript(conversationDirs(t))}}))
	if err == nil && len(out) > 0 {
		err = writePrivate(m.Paths.conversationsFile(t.ID), out)
	}
	if err != nil {
		fmt.Fprintf(m.Log, "unibox: the conversations of task %s were not kept on this computer: %v\n", t.ID, err)
	}
}

// restoreConversations puts a task's kept conversations back into its
// sandbox, so its agent continues where it was.
func (m *Manager) restoreConversations(ctx context.Context, b api.Sandbox, t api.Task) {
	if m.Paths.Data == "" {
		return
	}
	kept, err := os.ReadFile(m.Paths.conversationsFile(t.ID))
	if err != nil {
		return
	}
	if err = runLogged(m.SBX.Exec(ctx, b.Name, Command{Args: []string{"bash", "-c", restoreConversationsScript}, Input: bytes.NewReader(kept)}), m.Log); err != nil {
		fmt.Fprintf(m.Log, "unibox: the conversations of task %s were not put back; its agent starts afresh: %v\n", t.ID, err)
	}
}

// forgetConversations removes what the host kept of tasks that are gone.
func (m *Manager) forgetConversations(tasks ...api.Task) {
	if m.Paths.Data == "" {
		return
	}
	for _, t := range tasks {
		if err := os.Remove(m.Paths.conversationsFile(t.ID)); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(m.Log, "unibox: %v\n", err)
		}
	}
}

// writePrivate replaces a file only its owner reads, whole or not at all.
func writePrivate(file string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+strings.TrimSuffix(filepath.Base(file), ".tar.gz")+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
