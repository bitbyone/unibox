package cli

import (
	"fmt"
	"strings"
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

// moreLine says how many rows lie above and below the part a list shows,
// "" when it shows them all.
func moreLine(above, below int) string {
	parts := []string{}
	if above > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d more", above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d more", below))
	}
	if len(parts) == 0 {
		return ""
	}
	return portHelp.Render("  " + strings.Join(parts, " · "))
}
