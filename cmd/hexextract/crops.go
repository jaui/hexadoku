package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// writeCrop saves the detected lattice as a PNG, optionally split into
// horizontal bands so each part is large enough to read reliably.
func writeCrop(det *detection, path string, bands int) error {
	bm, vg, hg := det.bm, det.vg, det.hg
	x0, x1 := int(vg[0])-3, int(vg[16])+4
	if x0 < 0 {
		x0 = 0
	}
	if x1 > bm.w {
		x1 = bm.w
	}
	rowsPer := 16 / bands
	for b := 0; b < bands; b++ {
		y0 := int(hg[b*rowsPer]) - 3
		y1 := int(hg[(b+1)*rowsPer]) + 4
		if y0 < 0 {
			y0 = 0
		}
		if y1 > bm.h {
			y1 = bm.h
		}
		w, h := x1-x0, y1-y0
		img := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetGray(x, y, color.Gray{Y: bm.lum[(y0+y)*bm.w+(x0+x)]})
			}
		}
		out := path
		if bands > 1 {
			ext := filepath.Ext(path)
			out = fmt.Sprintf("%s_band%d%s", path[:len(path)-len(ext)], b+1, ext)
		}
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		err = png.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		fmt.Println("wrote", out)
	}
	return nil
}

// exportCrops renders one page and writes crops of the puzzle grid and
// of the dense (solution) grid for visual reading.
func exportCrops(pdfPath string, page int, pdftoppm, pdftotext, outDir string, dpi, bands int) error {
	tmpDir, err := os.MkdirTemp("", "hexcrop")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// same solution-grid exclusion as the production path (born-digital
	// issues only; scanned ones fall back to the dense-lattice rule)
	var solBox *ptRect
	pages, werr := pdfWords(pdftotext, pdfPath, tmpDir)
	if page <= 0 {
		if werr != nil {
			return fmt.Errorf("cannot auto-detect page: %v", werr)
		}
		for i, words := range pages {
			var all strings.Builder
			for _, w := range words {
				all.WriteString(w.s)
			}
			if strings.Contains(strings.ToLower(all.String()), "hexadoku") {
				if img, err := renderPage(pdftoppm, pdfPath, i+1, 100, tmpDir); err == nil {
					_, sb := solutionFromWords(words)
					_, merr := maskFromImage(img, 100, sb, false)
					os.Remove(img)
					if merr == nil {
						page = i + 1
						break
					}
				}
			}
		}
		if page <= 0 {
			return fmt.Errorf("no hexadoku page with a detectable grid found")
		}
		fmt.Printf("auto-detected hexadoku on page %d\n", page)
	}
	if werr == nil && page-1 < len(pages) {
		_, solBox = solutionFromWords(pages[page-1])
	}

	img, err := renderPage(pdftoppm, pdfPath, page, dpi, tmpDir)
	if err != nil {
		return err
	}
	det, err := maskFromImage(img, dpi, solBox, true)
	if err != nil {
		return err
	}
	key := issueKey(pdfPath)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := writeCrop(det, filepath.Join(outDir, key+"_puzzle.png"), bands); err != nil {
		return err
	}
	fmt.Printf("puzzle lattice: %d clues detected at x %.0f-%.0f, y %.0f-%.0f\n%s",
		det.m.filled, det.vg[0], det.vg[16], det.hg[0], det.hg[16], det.m)
	if det.dense != nil {
		if err := writeCrop(det.dense, filepath.Join(outDir, key+"_solution.png"), bands); err != nil {
			return err
		}
		fmt.Printf("dense lattice: %d filled cells\n", det.dense.m.filled)
	}
	return nil
}
