package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The lists are drawn in the colours of the questions huh asks, its Charm
// theme, so the walk and the lists read as one tool: titles and the cursor
// in its indigo, what is on in its green, errors in its red. Keys are told
// apart from what they do in a dark yellow. Each colour has a shade for a
// light terminal too.
var (
	themeIndigo = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	themeGreen  = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	themeRed    = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	themeDim    = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	themeKey    = lipgloss.AdaptiveColor{Light: "136", Dark: "178"}
)

var (
	listTitle  = lipgloss.NewStyle().Foreground(themeIndigo).Bold(true)
	listCursor = lipgloss.NewStyle().Foreground(themeIndigo).Bold(true)
	portOn     = lipgloss.NewStyle().Foreground(themeGreen).Bold(true)
	portOff    = lipgloss.NewStyle().Faint(true)
	portCursor = lipgloss.NewStyle().Bold(true)
	portHelp   = lipgloss.NewStyle().Foreground(themeDim)
	portErr    = lipgloss.NewStyle().Foreground(themeRed)
	helpKey    = lipgloss.NewStyle().Foreground(themeKey)
)

// cursorMark marks the row under the cursor.
func cursorMark() string { return listCursor.Render(">") + " " }

// keyHelp draws a list's keys from pairs of a key and what it does, the
// key in its colour and the rest dimmed.
func keyHelp(pairs ...string) string {
	parts := []string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, helpKey.Render(pairs[i])+" "+portHelp.Render(pairs[i+1]))
	}
	return strings.Join(parts, portHelp.Render(" · "))
}
