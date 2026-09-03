// alphaextract reads one of the two wide Elektor puzzles out of the text
// layer of a PDF page: the Alphanumski of 7-8/2007 (36x36 over 0-9 and
// A-Z) and the AlphaSudoku of 7-8/2008 (25x25 over 1-9 and A-P).
//
// Like the Hexamurai pages, these carry every clue as a real text glyph,
// so the transcription is exact instead of read off a scan. The lattice
// fitting is shared with cmd/muraiextract in internal/pdftext.
//
// The Alphanumski is printed across a double page, eighteen columns on
// each side, so it needs two pages that are fitted separately and joined
// left to right:
//
//	alphaextract.exe -variant alphanumski -page 140 -page2 141 ^
//	                 -o puzzles\elektor\2007-07_08.txt ^
//	                 elektor_pdfs\Elektornonlinear.ir2007-07_08.pdf
//	alphaextract.exe -variant alphasudoku -page 118 ^
//	                 -o puzzles\elektor\2008-07_08.txt ^
//	                 elektor_pdfs\Elektornonlinear.ir2008-07_08.pdf
//
// Glyphs are accepted by shape only - any digit or capital letter - and
// never checked against the alphabet of the puzzle. Over-accepting is
// harmless, because a character that is not on the fitted lattice is
// dropped as running text anyway, while a character that *is* on the
// lattice and outside the alphabet is a real finding: the solver reports
// it by name when it parses the file, which is where that knowledge
// belongs.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hexadoku/internal/pdftext"
)

type spec struct {
	title   string // how the magazine spells it
	variant string // the "# variant:" name the solver knows
	n       int    // grid side
	// columns printed on each page, in printing order. A puzzle that
	// spans a double page need not be halved evenly: the Alphanumski is
	// 18 + 18, but the Alfadoku is 16 + 9.
	cols     []int
	minClues int // a sanity floor per page
}

var specs = map[string]spec{
	"alphanumski": {"Alphanumski", "alphanumski", 36, []int{18, 18}, 150},
	"alphasudoku": {"AlphaSudoku", "alphasudoku", 25, []int{25}, 150},
	"alfadoku":    {"Alfadoku", "alfadoku", 25, []int{16, 9}, 80},
}

func main() {
	pdftotext := flag.String("pdftotext", "pdftotext", "path to poppler's pdftotext")
	variant := flag.String("variant", "", "alphanumski, alphasudoku or alfadoku")
	page := flag.Int("page", 0, "page holding the puzzle, or its left half")
	page2 := flag.Int("page2", 0, "page holding the right half, for a puzzle across a double page")
	out := flag.String("o", "", "write the puzzle here (default: stdout)")
	verbose := flag.Bool("v", false, "report the fitted lattices")
	flag.Parse()

	sp, ok := specs[strings.ToLower(*variant)]
	if flag.NArg() != 1 || *page == 0 || !ok {
		fmt.Fprintln(os.Stderr, "usage: alphaextract -variant alphanumski|alphasudoku|alfadoku -page N [options] file.pdf")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if err := run(*pdftotext, flag.Arg(0), sp, *page, *page2, *out, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(pdftotext, path string, sp spec, page, page2 int, out string, verbose bool) error {
	pages := []int{page}
	if page2 != 0 {
		pages = append(pages, page2)
	}
	if len(pages) != len(sp.cols) {
		return fmt.Errorf("%s is printed on %d page(s), %d given", sp.title, len(sp.cols), len(pages))
	}
	total := 0
	for _, c := range sp.cols {
		total += c
	}
	if total != sp.n {
		return fmt.Errorf("%s: the pages hold %d columns, the grid is %d wide", sp.title, total, sp.n)
	}

	// one grid per page, sp.n rows by that page's share of the columns
	halves := make([][][]byte, len(pages))
	clues := 0
	for i, p := range pages {
		h, n, err := readPage(pdftotext, path, p, sp, sp.cols[i], verbose)
		if err != nil {
			return fmt.Errorf("page %d: %v", p, err)
		}
		halves[i], clues = h, clues+n
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Elektor %s from %s p. %s, %d clues\n",
		sp.title, filepath.Base(path), pageList(pages), clues)
	fmt.Fprintf(&sb, "# read from the PDF text layer by cmd/alphaextract (exact, not OCR)\n")
	fmt.Fprintf(&sb, "# variant: %s\n", sp.variant)
	for r := 0; r < sp.n; r++ {
		for _, h := range halves {
			sb.Write(h[r])
		}
		sb.WriteByte('\n')
	}

	if out == "" {
		fmt.Print(sb.String())
		return nil
	}
	return os.WriteFile(out, []byte(sb.String()), 0o644)
}

func pageList(pages []int) string {
	s := make([]string, len(pages))
	for i, p := range pages {
		s[i] = fmt.Sprint(p)
	}
	return strings.Join(s, "-")
}

// readPage returns the sp.n x cols block of characters printed on one
// page, and how many of them are clues.
func readPage(pdftotext, path string, page int, sp spec, cols int, verbose bool) ([][]byte, int, error) {
	glyphs, err := pdftext.ReadGlyphs(pdftotext, path, page, isAlnum)
	if err != nil {
		return nil, 0, err
	}
	if len(glyphs) < sp.minClues {
		return nil, 0, fmt.Errorf("only %d candidate glyphs - is this the puzzle page?", len(glyphs))
	}

	xs := make([]float64, len(glyphs))
	ys := make([]float64, len(glyphs))
	for i, g := range glyphs {
		xs[i], ys[i] = g.X, g.Y
	}
	ox, px, err := pdftext.FitAxis(xs)
	if err != nil {
		return nil, 0, fmt.Errorf("column lattice: %v", err)
	}
	oy, py, err := pdftext.FitAxis(ys)
	if err != nil {
		return nil, 0, fmt.Errorf("row lattice: %v", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "page %d lattice: columns at %.2f + k*%.3f, rows at %.2f + k*%.3f\n",
			page, ox, px, oy, py)
	}

	// index every glyph; anything off the lattice is running text
	var onLattice []pdftext.Glyph
	var rows, colIdx []int
	for _, g := range glyphs {
		c, okc := pdftext.Index(g.X, ox, px)
		r, okr := pdftext.Index(g.Y, oy, py)
		if !okc || !okr {
			if verbose {
				fmt.Fprintf(os.Stderr, "page %d: dropped %q at %.2f,%.2f (off the lattice)\n",
					page, g.Ch, g.X, g.Y)
			}
			continue
		}
		g.R, g.C = r, c
		onLattice = append(onLattice, g)
		rows = append(rows, r)
		colIdx = append(colIdx, c)
	}
	if len(onLattice) == 0 {
		return nil, 0, fmt.Errorf("no glyph sits on the fitted lattice")
	}

	// FitAxis anchors the lattice in the middle of the page, so half the
	// indices are negative; shift them to zero before looking for the
	// block, because Window counts lines from zero upwards.
	minR, minC := rows[0], colIdx[0]
	for i := range onLattice {
		minR, minC = min(minR, rows[i]), min(minC, colIdx[i])
	}
	for i := range onLattice {
		onLattice[i].R -= minR
		onLattice[i].C -= minC
		rows[i], colIdx[i] = onLattice[i].R, onLattice[i].C
	}

	// the puzzle is the densest block of the right shape; glyphs outside
	// it are text that happened to line up
	r0, r1 := pdftext.Window(rows, sp.n)
	c0, c1 := pdftext.Window(colIdx, cols)

	grid := make([][]byte, sp.n)
	for r := range grid {
		grid[r] = []byte(strings.Repeat(".", cols))
	}
	clues := 0
	for _, g := range onLattice {
		if g.R < r0 || g.R > r1 || g.C < c0 || g.C > c1 {
			if verbose {
				fmt.Fprintf(os.Stderr, "page %d: dropped %q outside the block at row %d, column %d\n",
					page, g.Ch, g.R, g.C)
			}
			continue
		}
		r, c := g.R-r0, g.C-c0
		if grid[r][c] != '.' {
			return nil, 0, fmt.Errorf("two clues %q and %q land on row %d, column %d",
				grid[r][c], g.Ch, r+1, c+1)
		}
		grid[r][c] = g.Ch
		clues++
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "page %d: %d clues in rows %d-%d, columns %d-%d\n",
			page, clues, r0, r1, c0, c1)
	}
	return grid, clues, nil
}

// isAlnum admits every character either alphabet can hold. The puzzle's
// own alphabet is checked by the solver when it reads the file.
func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z'
}
