package main

// Contact sheets for reading a grid by eye.
//
// Reading whole rows off a scan is where transcription goes wrong: not
// the glyphs, but the counting. A digit read correctly and placed one
// column too far left costs the same as a misread one, and it is much
// harder to notice.
//
// The mask already answers *where* the clues are - it comes from the
// line geometry, independent of what the cells contain. So the reading
// only has to supply *which* glyph, and the position never has to be
// counted at all: this cuts out only the cells the mask marks, lays them
// out in a numbered sheet, and rebuilds the grid from the sequence of
// glyphs read back. A miscount is then impossible by construction, and
// the count of glyphs is checked against the count of clue cells.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

const (
	sheetCols = 8  // cells per row on the contact sheet
	sheetGap  = 14 // white space between cells
	sheetRows = 8  // rows per sheet file
)

// writeCells writes contact sheets of the clue cells and returns the
// cell order, as "r<row>c<col>" keys.
func writeCells(det *detection, dir, key string) ([]string, error) {
	bm, vg, hg := det.bm, det.vg, det.hg
	var order []string
	type cell struct{ x0, y0, x1, y1 int }
	var cells []cell
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if !det.m.given[r][c] {
				continue
			}
			// a little past the cell edges, so a glyph touching a line
			// is still complete
			cells = append(cells, cell{
				int(vg[c]) + 1, int(hg[r]) + 1,
				int(vg[c+1]), int(hg[r+1]),
			})
			order = append(order, fmt.Sprintf("r%dc%d", r+1, c+1))
		}
	}
	if len(cells) == 0 {
		return nil, fmt.Errorf("mask marks no clue cells")
	}

	cw, ch := 0, 0
	for _, c := range cells {
		cw, ch = max(cw, c.x1-c.x0), max(ch, c.y1-c.y0)
	}
	// scale small cells up: a glyph under ~40 px is hard to judge
	scale := 1
	for cw*scale < 90 {
		scale++
	}

	perSheet := sheetCols * sheetRows
	for s := 0; s*perSheet < len(cells); s++ {
		lo := s * perSheet
		hi := min(lo+perSheet, len(cells))
		rows := (hi - lo + sheetCols - 1) / sheetCols
		w := sheetCols*(cw*scale+sheetGap) + sheetGap
		h := rows*(ch*scale+sheetGap) + sheetGap
		img := image.NewGray(image.Rect(0, 0, w, h))
		for i := range img.Pix {
			img.Pix[i] = 235 // grey ground, so each cell's white shows
		}
		for i := lo; i < hi; i++ {
			c := cells[i]
			gx := sheetGap + (i-lo)%sheetCols*(cw*scale+sheetGap)
			gy := sheetGap + (i-lo)/sheetCols*(ch*scale+sheetGap)
			for y := 0; y < ch*scale; y++ {
				for x := 0; x < cw*scale; x++ {
					v := uint8(255)
					sx, sy := c.x0+x/scale, c.y0+y/scale
					if sx < c.x1 && sy < c.y1 && sx < bm.w && sy < bm.h {
						v = bm.lum[sy*bm.w+sx]
					}
					img.SetGray(gx+x, gy+y, color.Gray{Y: v})
				}
			}
		}
		out := filepath.Join(dir, fmt.Sprintf("%s_cells%d.png", key, s+1))
		f, err := os.Create(out)
		if err != nil {
			return nil, err
		}
		err = png.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		fmt.Printf("wrote %s (cells %d-%d of %d, %d per row)\n",
			out, lo+1, hi, len(cells), sheetCols)
	}
	return order, nil
}

// gridFromCells rebuilds a puzzle from the glyphs read off the contact
// sheets, in the order writeCells laid them out.
func gridFromCells(order []string, glyphs string) (string, error) {
	var vals []byte
	for i := 0; i < len(glyphs); i++ {
		if isHex(glyphs[i]) {
			vals = append(vals, glyphs[i])
		}
	}
	if len(vals) != len(order) {
		return "", fmt.Errorf("read %d glyphs but the mask marks %d clue cells",
			len(vals), len(order))
	}
	var g [16][16]byte
	for r := range g {
		for c := range g[r] {
			g[r][c] = '.'
		}
	}
	for i, k := range order {
		var r, c int
		fmt.Sscanf(k, "r%dc%d", &r, &c)
		g[r-1][c-1] = vals[i]
	}
	var sb strings.Builder
	for _, row := range g {
		sb.Write(row[:])
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// exportCells renders a page, detects the lattice and writes the contact
// sheets plus an order file naming the cell behind every position.
func exportCells(pdfPath string, page int, pdftoppm, pdftotext, outDir string, dpi int) error {
	tmpDir, err := os.MkdirTemp("", "hexcells")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	var solBox *ptRect
	if pages, werr := pdfWords(pdftotext, pdfPath, tmpDir); werr == nil && page-1 < len(pages) {
		_, solBox = solutionFromWords(pages[page-1])
	}
	// Detection is not monotone in resolution: a hairline can vanish
	// between pixels at one setting and be solid at another, so try the
	// working resolutions rather than assuming the highest is best.
	var det *detection

	for _, d := range []int{dpi, 150, 200, 400} {
		img, rerr := renderPage(pdftoppm, pdfPath, page, d, tmpDir)
		if rerr != nil {
			continue
		}
		det, err = maskFromImage(img, d, solBox, true)
		os.Remove(img)
		if det != nil {
			fmt.Printf("lattice found at %d dpi: %d clues\n", d, det.m.filled)
			break
		}
	}
	if det == nil {
		if err == nil {
			err = fmt.Errorf("no lattice found at any resolution")
		}
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	key := issueKey(pdfPath)
	order, err := writeCells(det, outDir, key)
	if err != nil {
		return err
	}
	ord := filepath.Join(outDir, key+"_order.txt")
	if err := os.WriteFile(ord, []byte(strings.Join(order, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d clue cells, order written to %s\n", len(order), ord)
	return nil
}
