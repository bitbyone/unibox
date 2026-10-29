package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// A list is a part of the terminal below the prompt, not all of it: its
// table shows at most listMaxLines lines, fewer when the terminal leaves
// less beside the list's own, and never fewer than listMinLines, which
// a terminal too short shows by scrolling. Beyond them the rows scroll
// with the cursor.
const (
	listMaxLines = 20
	listMinLines = 3
)

// listRoom is how many lines a list's table gets on a terminal height
// lines high, others of them the list's own: its title, its keys. A
// height of 0 is a terminal that has not said it.
func listRoom(height, others int) int {
	if height <= 0 {
		return listMaxLines
	}
	return min(max(height-others, listMinLines), listMaxLines)
}

// rowWindow is the part of n rows of one line each that room shows, the
// cursor's in it: the first row and the one after the last.
func rowWindow(n, cursor, room int) (first, end int) {
	if n <= room {
		return 0, n
	}
	first = min(max(cursor-room/2, 0), n-room)
	return first, first + room
}

// blockWindow is the part of rows of several lines each that room lines
// show, gap lines apart, the cursor's row in it and as many about it as
// fit: the first row and the one after the last.
func blockWindow(heights []int, cursor, gap, room int) (first, end int) {
	if len(heights) == 0 {
		return 0, 0
	}
	cursor = min(max(cursor, 0), len(heights)-1)
	first, end = cursor, cursor+1
	used := heights[cursor]
	for grew := true; grew; {
		grew = false
		if end < len(heights) && used+gap+heights[end] <= room {
			used += gap + heights[end]
			end, grew = end+1, true
		}
		if first > 0 && used+gap+heights[first-1] <= room {
			used += gap + heights[first-1]
			first, grew = first-1, true
		}
	}
	return first, end
}

// moreAbove and moreBelow stand above and below the rows a list shows,
// where rows lie beyond them, so a list cut to a few rows does not read as
// a list of a few; "" where none lie. The one below says where the rows
// shown are among all of them.
func moreAbove(above int) string {
	if above <= 0 {
		return ""
	}
	return listCursor.Render("  ↑") + portHelp.Render(fmt.Sprintf(" %d more", above))
}

func moreBelow(first, end, n int) string {
	if first == 0 && end >= n {
		return ""
	}
	if end >= n {
		return portHelp.Render(fmt.Sprintf("  rows %d-%d of %d", first+1, end, n))
	}
	return listCursor.Render("  ↓") + portHelp.Render(fmt.Sprintf(" %d more · rows %d-%d of %d", n-end, first+1, end, n))
}

// scrolled puts the lines of how many more about the rows a list shows.
func scrolled(rows []string, first, end, n int) []string {
	out := []string{}
	if l := moreAbove(first); l != "" {
		out = append(out, l)
	}
	out = append(out, rows...)
	if l := moreBelow(first, end, n); l != "" {
		out = append(out, l)
	}
	return out
}

// titleLine is a list's title with corner at the right of the terminal,
// or a few spaces after the title on one that has not said its width or
// is too narrow for both.
func titleLine(title, corner string, width int) string {
	if corner == "" {
		return title
	}
	gap := 3
	if width > 0 {
		gap = max(width-1-lipgloss.Width(title)-lipgloss.Width(corner), 3)
	}
	return title + strings.Repeat(" ", gap) + corner
}
