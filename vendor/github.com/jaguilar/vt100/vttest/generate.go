package vttest

import (
	"strings"
	"unicode/utf8"

	"github.com/jaguilar/vt100"
)

// FromLines generates a VT100 from content text.
// Each line must have the same number of runes.
func FromLines(s string) *vt100.VT100 {
	return FromLinesAndFormats(s, nil)
}

// FromLinesAndFormats generates a *VT100 whose state is set according
// to s (for content) and a (for attributes).
//
// Dimensions are set to the width of s' first line and the height of the
// number of lines in s.
//
// If a is nil, the default attributes are used.
func FromLinesAndFormats(s string, a [][]vt100.Format) *vt100.VT100 {
	lines := strings.Split(s, "\n")
	v := vt100.NewVT100(len(lines), utf8.RuneCountInString(lines[0]))
	for y := 0; y < v.Height; y++ {
		x := 0
		for _, r := range lines[y] {
			v.Content[y][x] = r
			if a != nil {
				v.Format[y][x] = a[y][x]
			}
			x++
		}
	}
	return v
}
