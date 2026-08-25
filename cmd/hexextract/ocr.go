package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ocrGrid reads the puzzle digits with tesseract. The puzzle region is
// cropped, binarized and the grid lines are whitened, then tesseract
// runs once over the whole region; its TSV output carries bounding
// boxes, which map every recognized character back to its cell.
func ocrGrid(det *detection, tesseract, tmpDir string) (*grid, error) {
	bm, vg, hg := det.bm, det.vg, det.hg
	x0, y0 := int(vg[0])-2, int(hg[0])-2
	x1, y1 := int(vg[16])+3, int(hg[16])+3
	w, h := x1-x0, y1-y0
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(255)
			if bm.at(x0+x, y0+y, thrInk) {
				v = 0
			}
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	// whiten the grid lines so tesseract sees clean glyphs only
	line := int(float64(w) / 16 * 0.14)
	for _, lx := range vg {
		for dx := -line; dx <= line; dx++ {
			x := int(lx) - x0 + dx
			for y := 0; y < h; y++ {
				if x >= 0 && x < w {
					img.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
	}
	for _, ly := range hg {
		for dy := -line; dy <= line; dy++ {
			y := int(ly) - y0 + dy
			for x := 0; x < w; x++ {
				if y >= 0 && y < h {
					img.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
	}

	crop := filepath.Join(tmpDir, "ocr.png")
	f, err := os.Create(crop)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	cmd := exec.Command(tesseract, crop, "stdout",
		"--psm", "6", "-c", "tessedit_char_whitelist=0123456789ABCDEF", "tsv")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tesseract: %v", err)
	}

	g := &grid{}
	conf := [16][16]float64{}
	for _, lineStr := range strings.Split(string(out), "\n") {
		f := strings.Split(strings.TrimRight(lineStr, "\r"), "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		left, _ := strconv.Atoi(f[6])
		top, _ := strconv.Atoi(f[7])
		width, _ := strconv.Atoi(f[8])
		height, _ := strconv.Atoi(f[9])
		cf, _ := strconv.ParseFloat(f[10], 64)
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		for i := 0; i < len(text); i++ {
			if !isHex(text[i]) {
				continue
			}
			cx := float64(x0+left) + float64(width)*(float64(i)+0.5)/float64(len(text))
			cy := float64(y0+top) + float64(height)/2
			col, row := -1, -1
			for k := 0; k < 16; k++ {
				if cx >= vg[k] && cx < vg[k+1] {
					col = k
				}
				if cy >= hg[k] && cy < hg[k+1] {
					row = k
				}
			}
			if col < 0 || row < 0 {
				continue
			}
			if g.cells[row][col] == 0 {
				g.filled++
			}
			if g.cells[row][col] == 0 || cf > conf[row][col] {
				g.cells[row][col] = text[i]
				conf[row][col] = cf
			}
		}
	}
	if g.filled < 40 {
		return nil, fmt.Errorf("only %d characters recognized", g.filled)
	}
	return g, nil
}
