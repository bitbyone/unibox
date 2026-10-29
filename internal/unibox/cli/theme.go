package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The lists are drawn beside the questions huh asks, in its Charm theme's
// green for what is on and red for errors, with titles and the cursor in a
// light blue. Keys are told
// apart from what they do in a dark yellow, and what is mounted from the
// host, which the agent writes into, in orange, and what is cloned in two
// greys. Each colour has a shade for a
// light terminal too.
var (
	themeBlue  = lipgloss.AdaptiveColor{Light: "#0A75C2", Dark: "#7CC4FA"}
	themeGreen = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	themeRed   = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	themeDim   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	themeKey   = lipgloss.AdaptiveColor{Light: "136", Dark: "178"}
	themeMount = lipgloss.AdaptiveColor{Light: "#C4570A", Dark: "#FF9F43"}
	themePath  = lipgloss.AdaptiveColor{Light: "#D98A4A", Dark: "#FFC890"}
	themeFrom  = lipgloss.AdaptiveColor{Light: "240", Dark: "250"}
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
	repoMount  = lipgloss.NewStyle().Foreground(themeMount)
	mountPath  = lipgloss.NewStyle().Foreground(themePath)
	clonePath  = lipgloss.NewStyle().Foreground(themeFrom)
	taskName   = lipgloss.NewStyle().Foreground(themeBlue)
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

// helpLines are a list's keys as keyHelp draws them, wrapped between two
// keys where the terminal is narrower: a line longer than the terminal is
// cut, and the keys at its end would be lost. width 0 is a terminal that
// has not said its width.
func helpLines(width int, pairs ...string) []string {
	if width <= 0 {
		return []string{keyHelp(pairs...)}
	}
	sep := portHelp.Render(" · ")
	room := width - 1
	lines, line := []string{}, ""
	for i := 0; i+1 < len(pairs); i += 2 {
		part := keyHelp(pairs[i], pairs[i+1])
		switch {
		case line == "":
			line = part
		case lipgloss.Width(line+sep+part) <= room:
			line += sep + part
		default:
			lines, line = append(lines, line), part
		}
	}
	return append(lines, line)
}
