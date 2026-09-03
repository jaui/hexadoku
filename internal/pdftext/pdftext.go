// Package pdftext reads the clues of a puzzle out of the text layer of a
// PDF page and fits the lattice they sit on.
//
// Most Elektor puzzle pages are graphics, but a few carry every clue as a
// real text glyph: the Hexamurai of the double issues 7-8/2009 and
// 7-8/2011, the Alphanumski of 7-8/2007 and the AlphaSudoku of 7-8/2008.
// For those an exact transcription is possible instead of reading the
// scan, because the glyph positions form one regular lattice and fitting
// it maps each glyph to its cell.
//
// The page also carries running text, and "0 to F" or "4x4" in it look
// exactly like clues, so every step here is built to survive a minority
// of positions that do not belong to the grid.
package pdftext

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Glyph is one character of the text layer: the centre of its bounding
// box, the character itself, and the lattice line it was assigned to.
type Glyph struct {
	X, Y float64
	Ch   byte
	R, C int
}

var wordRe = regexp.MustCompile(
	`<word xMin="([0-9.]+)" yMin="([0-9.]+)" xMax="([0-9.]+)" yMax="([0-9.]+)">([^<]*)</word>`)

// ReadGlyphs returns every single-character word of the given page that
// accept admits, with the centre of its bounding box.
func ReadGlyphs(pdftotext, path string, page int, accept func(byte) bool) ([]Glyph, error) {
	tmp, err := os.MkdirTemp("", "pdftext")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	dst := filepath.Join(tmp, "page.html")
	cmd := exec.Command(pdftotext, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		"-bbox", "-q", path, dst)
	if o, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftotext: %v: %s", err, o)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		return nil, err
	}
	var gs []Glyph
	for _, m := range wordRe.FindAllStringSubmatch(string(data), -1) {
		s := strings.TrimSpace(m[5])
		if len(s) != 1 || !accept(s[0]) {
			continue
		}
		x0, _ := strconv.ParseFloat(m[1], 64)
		y0, _ := strconv.ParseFloat(m[2], 64)
		x1, _ := strconv.ParseFloat(m[3], 64)
		y1, _ := strconv.ParseFloat(m[4], 64)
		gs = append(gs, Glyph{X: (x0 + x1) / 2, Y: (y0 + y1) / 2, Ch: s[0]})
	}
	return gs, nil
}

// FitAxis recovers the lattice behind a set of glyph centres. Every step
// is robust against a minority of positions that do not belong to the
// grid: the pitch is the *median* distance between neighbouring
// positions, the anchor is the median position, and only positions close
// to the resulting lattice enter the final fit.
func FitAxis(vals []float64) (origin, pitch float64, err error) {
	v := append([]float64(nil), vals...)
	sort.Float64s(v)

	// distances between positions that are not the same lattice line
	var gaps []float64
	for i := 1; i < len(v); i++ {
		if d := v[i] - v[i-1]; d > 1 {
			gaps = append(gaps, d)
		}
	}
	if len(gaps) < 8 {
		return 0, 0, fmt.Errorf("too few distinct positions")
	}
	sort.Float64s(gaps)
	pitch = gaps[len(gaps)/2]
	anchor := v[len(v)/2]

	for round := 0; round < 3; round++ {
		var sx, sy, sxx, sxy, m float64
		for _, c := range v {
			k := math.Round((c - anchor) / pitch)
			if math.Abs(c-(anchor+k*pitch)) > pitch/4 {
				continue // not on this lattice: running text, not a clue
			}
			sx, sy, sxx, sxy, m = sx+k, sy+c, sxx+k*k, sxy+k*c, m+1
		}
		if m < 8 {
			return 0, 0, fmt.Errorf("only %.0f positions fit a lattice of pitch %.2f", m, pitch)
		}
		d := m*sxx - sx*sx
		if d == 0 {
			break
		}
		pitch = (m*sxy - sx*sy) / d
		anchor = (sy - pitch*sx) / m
	}
	return anchor, pitch, nil
}

// Window picks the run of span consecutive lattice lines holding the most
// glyphs. Everything outside it is running text that happened to land on
// the lattice.
func Window(idx []int, span int) (lo, hi int) {
	counts := map[int]int{}
	maxIdx := 0
	for _, i := range idx {
		counts[i]++
		maxIdx = max(maxIdx, i)
	}
	best, bestStart := -1, 0
	for s := 0; s <= maxIdx; s++ {
		n := 0
		for i := s; i < s+span; i++ {
			n += counts[i]
		}
		if n > best {
			best, bestStart = n, s
		}
	}
	return bestStart, bestStart + span - 1
}

// Index maps a coordinate to its lattice line, rejecting anything that
// does not sit close enough to one.
func Index(v, origin, pitch float64) (int, bool) {
	k := math.Round((v - origin) / pitch)
	if math.Abs(v-(origin+k*pitch)) > pitch/3 {
		return 0, false
	}
	return int(k), true // the anchor sits inside the block, so k may be negative
}
