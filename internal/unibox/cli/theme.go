package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The lists are drawn beside the questions huh asks, in its Charm theme's
// green for what is on and red for errors, with titles and the cursor in a
// light blue. Keys are told
// apart from what they do in a dark yellow. Each colour has a shade for a
// light terminal too.
var (
	themeBlue  = lipgloss.AdaptiveColor{Light: "#0A75C2", Dark: "#7CC4FA"}
	themeGreen = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	themeRed   = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	themeDim   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	themeKey   = lipgloss.AdaptiveColor{Light: "136", Dark: "178"}
)

var (
	listTitle  = lipgloss.NewStyle().Foreground(themeBlue).Bold(true)
	listCursor = lipgloss.NewStyle().Foreground(themeBlue).Bold(true)
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
