package main

import "fmt"

// debugMask runs mask detection on a rendered PNG with diagnostics.
func debugMask(imgPath string) error {
	bm, err := loadBitmap(imgPath)
	if err != nil {
		return err
	}
	fmt.Printf("image %dx%d\n", bm.w, bm.h)
	minVLen := bm.h / 5
	vlines := lineCenters(bm.longestRun(true, 0, bm.h, bm.ink), minVLen)
	fmt.Printf("vertical bold lines (minLen %d): %d at %.0f\n", minVLen, len(vlines), vlines)
	skew := bm.deskew()
	fmt.Printf("deskew: %+.2f deg\n", skew)
	if skew != 0 {
		vlines = lineCenters(bm.longestRun(true, 0, bm.h, bm.ink), minVLen)
		fmt.Printf("after deskew: %d vertical bold lines at %.0f\n", len(vlines), vlines)
	}
	hlines := lineCenters(bm.longestRun(false, 0, bm.w, bm.ink), bm.w/5)
	fmt.Printf("horizontal bold lines (minLen %d): %d at %.0f\n", bm.w/5, len(hlines), hlines)
	colDark := make([]int, bm.w)
	for x := 0; x < bm.w; x++ {
		n := 0
		for y := 0; y < bm.h; y++ {
			if bm.at(x, y, bm.line) {
				n++
			}
		}
		colDark[x] = n
	}
	for _, frac := range []int{4, 5, 6, 8} {
		cs := lineCenters(colDark, bm.h/frac)
		fmt.Printf("columns by darkness count >= h/%d (%d): %d, even windows: %d\n",
			frac, bm.h/frac, len(cs), len(evenWindows(cs)))
	}
	if bm.raiseThresholds() {
		fmt.Printf("thresholds refitted to this page: ink %d, line %d\n", bm.ink, bm.line)
		for x := 0; x < bm.w; x++ {
			n := 0
			for y := 0; y < bm.h; y++ {
				if bm.at(x, y, bm.line) {
					n++
				}
			}
			colDark[x] = n
		}
		for _, frac := range []int{4, 5, 6} {
			cs := lineCenters(colDark, bm.h/frac)
			fmt.Printf("  columns >= h/%d: %d, even windows: %d\n",
				frac, len(cs), len(evenWindows(cs)))
		}
		vl := lineCenters(bm.longestRun(true, 0, bm.h, bm.ink), minVLen)
		fmt.Printf("  vertical bold lines: %d at %.0f\n", len(vl), vl)
	} else {
		fmt.Printf("thresholds not refitted (page is not pale)\n")
	}
	// how many rows a per-line threshold finds inside each column window -
	// on scans this is where the lattice is usually lost, which is what
	// the rigid comb of pass 2b exists for
	for _, vg := range evenWindows(vlines) {
		x0, x1 := int(vg[0]), int(vg[16])
		rowDark := make([]int, bm.h)
		for y := 0; y < bm.h; y++ {
			n := 0
			for x := x0; x <= x1 && x < bm.w; x++ {
				if bm.at(x, y, bm.line) {
					n++
				}
			}
			rowDark[y] = n
		}
		fmt.Printf("window x %d-%d: rows by darkness count at 90/80/70/60%%: %d/%d/%d/%d",
			x0, x1,
			len(lineCenters(rowDark, (x1-x0)*9/10)), len(lineCenters(rowDark, (x1-x0)*8/10)),
			len(lineCenters(rowDark, (x1-x0)*7/10)), len(lineCenters(rowDark, (x1-x0)*6/10)))
		if hg := bestComb(rowDark, (vg[16]-vg[0])/16, (x1-x0)*6/10); hg != nil {
			fmt.Printf("; comb y %.0f-%.0f, real=%v", hg[0], hg[16], latticeIsReal(bm, vg, hg))
		} else {
			fmt.Printf("; no comb")
		}
		fmt.Println()
	}
	det, err := maskFromImage(imgPath, 150, nil, true)
	if err != nil {
		return fmt.Errorf("maskFromImage: %v", err)
	}
	fmt.Printf("maskFromImage: %d clues at x %.0f-%.0f, y %.0f-%.0f\n%s",
		det.m.filled, det.vg[0], det.vg[16], det.hg[0], det.hg[16], det.m)
	if det.dense != nil {
		fmt.Printf("dense: %d cells at x %.0f-%.0f, y %.0f-%.0f\n",
			det.dense.m.filled, det.dense.vg[0], det.dense.vg[16], det.dense.hg[0], det.dense.hg[16])
	}
	return nil
}

func maybeDebug(args []string) bool {
	if len(args) == 2 && args[0] == "-maskdebug" {
		if err := debugMask(args[1]); err != nil {
			fmt.Println(err)
		}
		return true
	}
	return false
}
