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
	vlines := lineCenters(bm.longestRun(true, 0, bm.h, thrInk), minVLen)
	fmt.Printf("vertical bold lines (minLen %d): %d at %.0f\n", minVLen, len(vlines), vlines)
	det, err := maskFromImage(imgPath, 150, nil)
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
