// muraiextract reads a samurai puzzle (Elektor's Hexamurai) out of the
// text layer of a PDF page.
//
// Unlike the ordinary Hexadoku pages, where the puzzle is a graphic and
// only the previous issue's solution is text, the Hexamurai pages of the
// double issues 7-8/2009 and 7-8/2011 carry every clue as a real text
// glyph. That makes an exact transcription possible instead of reading
// the scan: the glyph positions form one regular lattice across all five
// grids, so fitting that lattice maps each glyph to its cell.
//
//	muraiextract.exe -page 124 -o puzzles\elektor\2009-07_08.txt ^
//	                 elektor_pdfs\Elektornonlinear.ir2009-07_08.pdf
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"hexadoku/internal/pdftext"
)

// glyph, reading the text layer and fitting the lattice it sits on are
// shared with cmd/alphaextract, which does the same for the two wide
// puzzles of 7-8/2007 and 7-8/2008.
type glyph = pdftext.Glyph

func main() {
	pdftotext := flag.String("pdftotext", "pdftotext", "path to poppler's pdftotext")
	page := flag.Int("page", 0, "page number holding the puzzle")
	n := flag.Int("n", 16, "grid size: 16 (hexamurai) or 9 (samurai)")
	out := flag.String("o", "", "write the puzzle here (default: stdout)")
	solver := flag.String("solver", "", "solver binary used to pick between ambiguous lattice offsets")
	verbose := flag.Bool("v", false, "report the fitted lattice")
	flag.Parse()
	if flag.NArg() != 1 || *page == 0 {
		fmt.Fprintln(os.Stderr, "usage: muraiextract -page N [options] file.pdf")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if err := run(*pdftotext, flag.Arg(0), *page, *n, *out, *solver, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(pdftotext, path string, page, n int, out, solver string, verbose bool) error {
	glyphs, err := pdftext.ReadGlyphs(pdftotext, path, page, isHex)
	if err != nil {
		return err
	}
	if len(glyphs) < 100 {
		return fmt.Errorf("only %d hex glyphs on page %d - is this the puzzle page?", len(glyphs), page)
	}

	cands := layouts(n)
	span := 0
	for _, l := range cands {
		span = max(span, l.span)
	}

	xs := make([]float64, len(glyphs))
	ys := make([]float64, len(glyphs))
	for i, g := range glyphs {
		xs[i], ys[i] = g.X, g.Y
	}
	ox, px, err := pdftext.FitAxis(xs)
	if err != nil {
		return fmt.Errorf("column lattice: %v", err)
	}
	oy, py, err := pdftext.FitAxis(ys)
	if err != nil {
		return fmt.Errorf("row lattice: %v", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "lattice: columns at %.2f + k*%.3f, rows at %.2f + k*%.3f\n", ox, px, oy, py)
	}

	// index every glyph; anything off the lattice is running text
	var onLattice []glyph
	var rows, cols []int
	for _, g := range glyphs {
		c, okc := pdftext.Index(g.X, ox, px)
		r, okr := pdftext.Index(g.Y, oy, py)
		if !okc || !okr {
			if verbose {
				fmt.Fprintf(os.Stderr, "dropped %q at %.2f,%.2f (off the lattice)\n", g.Ch, g.X, g.Y)
			}
			continue
		}
		g.R, g.C = r, c
		onLattice = append(onLattice, g)
		rows, cols = append(rows, r), append(cols, c)
	}
	if len(onLattice) == 0 {
		return fmt.Errorf("no glyph sits on the fitted lattice")
	}

	// the lattice is anchored in the middle of the block, so indices start
	// out negative; move them to zero
	minR, minC := rows[0], cols[0]
	for i := range rows {
		minR, minC = min(minR, rows[i]), min(minC, cols[i])
	}
	for i := range onLattice {
		onLattice[i].R -= minR
		onLattice[i].C -= minC
		rows[i], cols[i] = onLattice[i].R, onLattice[i].C
	}

	// the puzzle occupies span consecutive lattice lines in each direction;
	// glyphs outside that block are text that happened to line up
	r0, r1 := pdftext.Window(rows, span)
	c0, c1 := pdftext.Window(cols, span)
	var kept []glyph
	maxR, maxC := 0, 0
	for _, g := range onLattice {
		if g.R < r0 || g.R > r1 || g.C < c0 || g.C > c1 {
			if verbose {
				fmt.Fprintf(os.Stderr, "dropped %q at row %d, column %d (outside the puzzle block)\n",
					g.Ch, g.R, g.C)
			}
			continue
		}
		g.R, g.C = g.R-r0, g.C-c0
		kept = append(kept, g)
		maxR, maxC = max(maxR, g.R), max(maxC, g.C)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "%d clues on a %dx%d block\n", len(kept), maxR+1, maxC+1)
		raw := make([][]byte, maxR+1)
		for r := range raw {
			raw[r] = []byte(strings.Repeat(".", maxC+1))
		}
		for _, g := range kept {
			raw[g.R][g.C] = g.Ch
		}
		for _, r := range raw {
			fmt.Fprintf(os.Stderr, "|%s|\n", r)
		}
	}

	// The block of clues is anchored at its first occupied row and column,
	// which need not be the first row and column of the layout. Try every
	// layout that is large enough and every shift that still fits, and keep
	// the ones where all clues land on real cells without breaking a rule.
	type fit struct {
		name   string
		dr, dc int
		text   string
	}
	var fits []fit
	for _, l := range cands {
		if l.span <= maxR || l.span <= maxC {
			continue // the clues do not fit into this arrangement
		}
		for dr := 0; dr <= l.span-1-maxR; dr++ {
			for dc := 0; dc <= l.span-1-maxC; dc++ {
				text, err := build(kept, l, dr, dc)
				if err != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "%s offset %d,%d rejected: %v\n", l.name, dr, dc, err)
					}
					continue
				}
				fits = append(fits, fit{l.name, dr, dc, text})
			}
		}
	}
	if len(fits) == 0 {
		return fmt.Errorf("no known arrangement places all %d clues on valid cells", len(kept))
	}
	if len(fits) > 1 && solver != "" {
		var uniq []fit
		for _, f := range fits {
			if uniquelySolvable(solver, f.text) {
				uniq = append(uniq, f)
			}
		}
		if len(uniq) > 0 {
			fits = uniq
		}
	}
	if len(fits) > 1 {
		return fmt.Errorf("%d arrangements/offsets are possible; none could be singled out", len(fits))
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "arrangement: %s, offset %d,%d\n", fits[0].name, fits[0].dr, fits[0].dc)
	}

	kind := "Hexamurai"
	if n == 9 {
		kind = "Samurai"
	}
	text := fmt.Sprintf("# Elektor %s from %s p. %d, %d clues, %s arrangement\n"+
		"# read from the PDF text layer by cmd/muraiextract (exact, not OCR)\n",
		kind, filepath.Base(path), page, len(kept), fits[0].name) + fits[0].text
	if out == "" {
		fmt.Print(text)
		return nil
	}
	return os.WriteFile(out, []byte(text), 0644)
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F'
}

// Known five-grid arrangements. Elektor used a different one in each of
// the two double issues, so the layout is not assumed but chosen: only
// the ones whose span matches the block of clues found on the page are
// tried, and each must place every clue on a real cell without breaking a
// rule.
type layout struct {
	name    string
	n, b    int
	span    int
	origins [][2]int
}

func layouts(n int) []layout {
	if n == maxN {
		return []layout{
			// 7-8/2009: pinwheel, every outer grid shares a 4x12 strip
			// (three boxes) with the middle grid - 1088 cells
			{"pinwheel", 16, 4, 40, [][2]int{{0, 8}, {8, 24}, {16, 0}, {12, 12}, {24, 16}}},
			// 7-8/2011: plus, every outer grid shares a whole 8x16 half
			// with the middle grid - 768 cells
			{"plus", 16, 4, 32, [][2]int{{0, 8}, {8, 0}, {8, 8}, {8, 16}, {16, 8}}},
		}
	}
	return []layout{
		{"cross", 9, 3, 21, [][2]int{{0, 0}, {0, 12}, {6, 6}, {12, 0}, {12, 12}}},
	}
}

const maxN = 16

// build places the clues on the samurai raster shifted by (dr, dc) and
// checks every rule of every grid.
func build(gs []glyph, l layout, dr, dc int) (string, error) {
	n, b, span, origins := l.n, l.b, l.span, l.origins
	covered := func(r, c int) bool {
		for _, o := range origins {
			if r >= o[0] && r < o[0]+n && c >= o[1] && c < o[1]+n {
				return true
			}
		}
		return false
	}

	cells := make([][]byte, span)
	for r := range cells {
		cells[r] = make([]byte, span)
		for c := range cells[r] {
			if covered(r, c) {
				cells[r][c] = '.'
			} else {
				cells[r][c] = ' '
			}
		}
	}
	for _, g := range gs {
		r, c := g.R+dr, g.C+dc
		if !covered(r, c) {
			return "", fmt.Errorf("clue %q lands outside every grid at row %d, column %d", g.Ch, r, c)
		}
		if cells[r][c] != '.' {
			return "", fmt.Errorf("two clues in row %d, column %d", r, c)
		}
		cells[r][c] = g.Ch
	}

	// no value twice in any row, column or box of any of the five grids
	for _, o := range origins {
		for i := 0; i < n; i++ {
			var row, col []byte
			for j := 0; j < n; j++ {
				row = append(row, cells[o[0]+i][o[1]+j])
				col = append(col, cells[o[0]+j][o[1]+i])
			}
			if err := distinct(row); err != nil {
				return "", fmt.Errorf("grid at %d,%d row %d: %v", o[0], o[1], i, err)
			}
			if err := distinct(col); err != nil {
				return "", fmt.Errorf("grid at %d,%d column %d: %v", o[0], o[1], i, err)
			}
		}
		for br := 0; br < n; br += b {
			for bc := 0; bc < n; bc += b {
				var box []byte
				for r := 0; r < b; r++ {
					for c := 0; c < b; c++ {
						box = append(box, cells[o[0]+br+r][o[1]+bc+c])
					}
				}
				if err := distinct(box); err != nil {
					return "", fmt.Errorf("grid at %d,%d box %d,%d: %v", o[0], o[1], br, bc, err)
				}
			}
		}
	}

	var sb strings.Builder
	for _, row := range cells {
		sb.Write(row)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func distinct(cells []byte) error {
	var seen [256]bool
	for _, c := range cells {
		if c == '.' || c == ' ' {
			continue
		}
		if seen[c] {
			return fmt.Errorf("value %q twice", c)
		}
		seen[c] = true
	}
	return nil
}

func uniquelySolvable(solver, text string) bool {
	tmp, err := os.CreateTemp("", "murai*.txt")
	if err != nil {
		return false
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(text)
	tmp.Close()
	out, _ := exec.Command(solver, "-unique", tmp.Name()).CombinedOutput()
	return strings.Contains(string(out), "solution is unique")
}
