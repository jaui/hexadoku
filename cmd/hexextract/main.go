// hexextract pulls the Hexadoku puzzles out of Elektor magazine PDFs.
//
// The magazines print two grids on the Hexadoku page:
//   - the current puzzle: an embedded graphic (NOT text in the PDF)
//   - the solution of the PREVIOUS issue: real text glyphs
//
// This allows a nearly OCR-free chain reconstruction:
//
//  phase A (default mode, per PDF):
//    1. find the page mentioning "Hexadoku"
//    2. extract the printed previous-issue solution from the text glyphs
//       (16x16 lattice of hex digits) -> <key>_prevsolution.txt
//    3. render the page with pdftoppm and detect the puzzle grid's
//       *mask* - which cells contain ink - by finding the 17+17 grid
//       lines and probing each cell -> <key>_mask.txt
//
//  phase B (-chain):
//    the puzzle of issue N is mask(N) filled with digits from
//    prevsolution(N+1) -> <key>.txt
//
// Every reconstructed puzzle can then be validated end-to-end with the
// solver: it must be uniquely solvable and reproduce the printed
// solution. Issues without a successor solution (gaps, last issue) are
// reported and left for manual/vision transcription.
package main

import (
	"flag"
	"fmt"
	"html"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------- shared helpers ----------

type glyph struct {
	x, y float64
	c    byte
	row  int
}

type cluster struct {
	center float64
	count  int
}

func isHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'A' && b <= 'F'
}

func clusterVals(vals []float64, tol float64) []cluster {
	if len(vals) == 0 {
		return nil
	}
	sort.Float64s(vals)
	var out []cluster
	sum, n := vals[0], 1
	for _, v := range vals[1:] {
		if v-sum/float64(n) <= tol {
			sum += v
			n++
		} else {
			out = append(out, cluster{sum / float64(n), n})
			sum, n = v, 1
		}
	}
	return append(out, cluster{sum / float64(n), n})
}

func findRun(cs []cluster, minCount int, minPitch, maxPitch, tol float64, want int) ([]float64, float64) {
	var centers []float64
	for _, c := range cs {
		if c.count >= minCount {
			centers = append(centers, c.center)
		}
	}
	for i := 0; i+want <= len(centers); i++ {
		pitch := centers[i+1] - centers[i]
		if pitch < minPitch || pitch > maxPitch {
			continue
		}
		ok := true
		for k := i + 1; k < i+want-1; k++ {
			if math.Abs((centers[k+1]-centers[k])-pitch) > tol {
				ok = false
				break
			}
		}
		if ok {
			return centers[i : i+want], pitch
		}
	}
	return nil, 0
}

type grid struct {
	cells  [16][16]byte // 0 = empty
	filled int
}

func (g *grid) String() string {
	var sb strings.Builder
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if g.cells[r][c] == 0 {
				sb.WriteByte('.')
			} else {
				sb.WriteByte(g.cells[r][c])
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ---------- issue keys and ordering ----------

var keyRe = regexp.MustCompile(`(20\d\d)[-_ ]?([0-9][0-9,_]*)`)

func issueKey(path string) string {
	m := keyRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	}
	return m[1] + "-" + strings.ReplaceAll(m[2], ",", "_")
}

type issueDate struct {
	year, m1, m2 int
}

func parseKey(key string) (issueDate, bool) {
	m := regexp.MustCompile(`^(20\d\d)-(\d+)(?:_(\d+))?$`).FindStringSubmatch(key)
	if m == nil {
		return issueDate{}, false
	}
	y, _ := strconv.Atoi(m[1])
	m1, _ := strconv.Atoi(m[2])
	m2 := m1
	if m[3] != "" {
		m2, _ = strconv.Atoi(m[3])
	}
	return issueDate{y, m1, m2}, true
}


// ---------- phase A: page text via poppler pdftotext -bbox ----------

// ptRect is a bounding box in PDF points, origin top-left (pdftotext
// -bbox coordinates).
type ptRect struct {
	x0, y0, x1, y1 float64
}

type word struct {
	x, y float64 // center
	s    string
}

var wordRe = regexp.MustCompile(`<word xMin="([0-9.]+)" yMin="([0-9.]+)" xMax="([0-9.]+)" yMax="([0-9.]+)">([^<]*)</word>`)

// pdfWords extracts per-word text with coordinates for every page.
func pdfWords(pdftotext, path, tmpDir string) ([][]word, error) {
	out := filepath.Join(tmpDir, "text.html")
	cmd := exec.Command(pdftotext, "-bbox", "-q", path, out)
	if o, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftotext: %v: %s", err, o)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	var pages [][]word
	for _, pageStr := range strings.Split(string(data), "<page ")[1:] {
		var ws []word
		for _, m := range wordRe.FindAllStringSubmatch(pageStr, -1) {
			x0, _ := strconv.ParseFloat(m[1], 64)
			y0, _ := strconv.ParseFloat(m[2], 64)
			x1, _ := strconv.ParseFloat(m[3], 64)
			y1, _ := strconv.ParseFloat(m[4], 64)
			ws = append(ws, word{(x0 + x1) / 2, (y0 + y1) / 2, html.UnescapeString(m[5])})
		}
		pages = append(pages, ws)
	}
	return pages, nil
}

// solutionFromWords finds a dense 16x16 lattice of hex-digit words and
// returns it with its bounding box (top-left origin, points).
func solutionFromWords(words []word) (*grid, *ptRect) {
	var glyphs []glyph
	for _, w := range words {
		s := strings.TrimSpace(w.s)
		if len(s) == 1 && isHex(s[0]) {
			glyphs = append(glyphs, glyph{w.x, w.y, s[0], -1})
		}
	}
	if len(glyphs) < 256 {
		return nil, nil
	}
	var ys []float64
	for _, g := range glyphs {
		ys = append(ys, g.y)
	}
	rows, pitchY := findRun(clusterVals(ys, 2.0), 10, 8, 34, 2.0, 16)
	if rows == nil {
		return nil, nil
	}
	var onGrid []glyph
	for _, g := range glyphs {
		for k, ry := range rows {
			if math.Abs(g.y-ry) < pitchY/3 {
				g.row = k // y grows downward: first cluster = top row
				onGrid = append(onGrid, g)
				break
			}
		}
	}
	var xs []float64
	for _, g := range onGrid {
		xs = append(xs, g.x)
	}
	cols, pitchX := findRun(clusterVals(xs, 3.5), 10, 8, 34, 3.0, 16)
	if cols == nil {
		return nil, nil
	}
	g := &grid{}
	for _, gl := range onGrid {
		for k, cx := range cols {
			if math.Abs(gl.x-cx) < pitchX/2.5 {
				if g.cells[gl.row][k] != 0 {
					return nil, nil
				}
				g.cells[gl.row][k] = gl.c
				g.filled++
				break
			}
		}
	}
	if g.filled != 256 {
		return nil, nil
	}
	// cols/rows are cell centers, so the grid edge lies half a cell out
	bbox := &ptRect{
		cols[0] - pitchX/2, rows[0] - pitchY/2,
		cols[15] + pitchX/2, rows[15] + pitchY/2,
	}
	return g, bbox
}

// ---------- phase A: puzzle mask from rendered page ----------

type mask struct {
	given  [16][16]bool
	filled int
}

func (m *mask) String() string {
	var sb strings.Builder
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if m.given[r][c] {
				sb.WriteByte('#')
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func renderPage(pdftoppm, pdfPath string, page, dpi int, tmpDir string) (string, error) {
	prefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command(pdftoppm, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		"-r", strconv.Itoa(dpi), "-png", pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm: %v: %s", err, out)
	}
	matches, _ := filepath.Glob(prefix + "*.png")
	if len(matches) == 0 {
		return "", fmt.Errorf("pdftoppm produced no output")
	}
	return matches[0], nil
}

// thresholds: grid lines can render as light anti-aliased gray, digits
// and bold lines are much darker
const (
	thrInk  = 160 // digits, bold lines
	thrLine = 205 // faint hairline grid lines
)

type bitmap struct {
	w, h int
	lum  []uint8
}

func loadBitmap(path string) (*bitmap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	bm := &bitmap{w: b.Dx(), h: b.Dy(), lum: make([]uint8, b.Dx()*b.Dy())}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			bm.lum[(y-b.Min.Y)*bm.w+(x-b.Min.X)] = uint8((299*r + 587*g + 114*bl) / 1000 >> 8)
		}
	}
	return bm, nil
}

func (bm *bitmap) at(x, y int, thr uint8) bool {
	if x < 0 || y < 0 || x >= bm.w || y >= bm.h {
		return false
	}
	return bm.lum[y*bm.w+x] < thr
}

// longestRun returns, for every column (or row), the longest dark run.
func (bm *bitmap) longestRun(vertical bool, lo, hi int, thr uint8) []int {
	n := bm.w
	if !vertical {
		n = bm.h
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		run, best := 0, 0
		for j := lo; j < hi; j++ {
			var d bool
			if vertical {
				d = bm.at(i, j, thr)
			} else {
				d = bm.at(j, i, thr)
			}
			if d {
				run++
				if run > best {
					best = run
				}
			} else {
				run = 0
			}
		}
		out[i] = best
	}
	return out
}

// lineCenters clusters positions whose run length exceeds minLen.
func lineCenters(runs []int, minLen int) []float64 {
	var vals []float64
	for i, r := range runs {
		if r >= minLen {
			vals = append(vals, float64(i))
		}
	}
	var out []float64
	for _, c := range clusterVals(vals, 4.5) {
		out = append(out, c.center)
	}
	return out
}

// splitGroups splits sorted line positions where the gap is much larger
// than the median gap.
func splitGroups(lines []float64) [][]float64 {
	if len(lines) < 2 {
		return nil
	}
	var gaps []float64
	for i := 1; i < len(lines); i++ {
		gaps = append(gaps, lines[i]-lines[i-1])
	}
	sorted := append([]float64(nil), gaps...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	var groups [][]float64
	cur := []float64{lines[0]}
	for i := 1; i < len(lines); i++ {
		if lines[i]-lines[i-1] > 1.45*median {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, lines[i])
	}
	return append(groups, cur)
}

func evenSpaced(lines []float64, tolFrac float64) bool {
	if len(lines) != 17 {
		return false
	}
	pitch := (lines[16] - lines[0]) / 16
	for i := 1; i < 17; i++ {
		if math.Abs((lines[i]-lines[i-1])-pitch) > tolFrac*pitch {
			return false
		}
	}
	return true
}

// evenWindows returns every 17-line window with uniform spacing. The
// two grids can sit so close together that their line sequences merge
// into one group, so fixed-size grouping is not enough.
func evenWindows(lines []float64) [][]float64 {
	var out [][]float64
	for i := 0; i+17 <= len(lines); i++ {
		w := lines[i : i+17]
		if evenSpaced(w, 0.25) {
			out = append(out, w)
		}
	}
	return out
}

// maskFromImage finds 17x17-line lattices and returns the sparse one.
//
// Pass 1 finds any clean lattice via long contiguous dark runs - the
// boldly printed solution grid is always found this way. Its y-band
// then anchors pass 2: within that band, faint (anti-aliased) grid
// lines of the puzzle graphic are recovered by counting dark pixels
// per column/row instead of requiring contiguous runs.
// detection is a successfully located puzzle lattice on a rendered page.
type detection struct {
	m      *mask
	vg, hg []float64
	bm     *bitmap
	dense  *detection // the nearly full lattice on the same page, if any
}

func maskFromImage(imgPath string, dpi int, solBox *ptRect) (*detection, error) {
	bm, err := loadBitmap(imgPath)
	if err != nil {
		return nil, err
	}
	// the printed solution grid's position is known exactly from the
	// PDF text; windows overlapping it are never the puzzle
	var excl *ptRect // in image pixels
	if solBox != nil {
		scale := float64(dpi) / 72
		excl = &ptRect{
			solBox.x0 * scale, solBox.y0 * scale,
			solBox.x1 * scale, solBox.y1 * scale,
		}
	}
	overlapsSolution := func(vg, hg []float64) bool {
		if excl == nil {
			return false
		}
		ox := math.Min(vg[16], excl.x1) - math.Max(vg[0], excl.x0)
		oy := math.Min(hg[16], excl.y1) - math.Max(hg[0], excl.y0)
		cell := (vg[16] - vg[0]) / 16
		return ox > cell && oy > cell
	}

	minVLen := bm.h / 5
	vlines := lineCenters(bm.longestRun(true, 0, bm.h, thrInk), minVLen)

	var best, dense *detection
	var bandY0, bandY1 int
	// Among plausible windows keep the one seeing the MOST clues: a
	// misaligned window (the grids sit about one pitch apart, so
	// shifted windows are equally uniform) always loses an edge
	// column. Windows overlapping the dense solution grid are ruled
	// out by the full-row/column check - no real puzzle has 16 givens
	// in one line.
	tryLattice := func(vg, hg []float64) {
		if !latticeIsReal(bm, vg, hg) {
			return
		}
		if bandY0 == 0 {
			bandY0, bandY1 = int(hg[0]), int(hg[16])
		}
		m := probeCells(bm, vg, hg)
		// a nearly full lattice is the printed previous-issue solution
		// (interesting on scanned issues, where it exists only as an
		// image and is later read via OCR)
		if m.filled >= 230 {
			if dense == nil || m.filled > dense.m.filled {
				dense = &detection{m: m, vg: vg, hg: hg, bm: bm}
			}
			return
		}
		if overlapsSolution(vg, hg) {
			return
		}
		if m.filled < 60 || m.filled > 200 {
			return
		}
		if !plausibleMask(m) {
			return
		}
		if best == nil || m.filled > best.m.filled {
			best = &detection{m: m, vg: vg, hg: hg, bm: bm}
		}
	}

	for _, vg := range evenWindows(vlines) {
		x0, x1 := int(vg[0]), int(vg[16])
		minHLen := (x1 - x0) * 8 / 10
		for _, hg := range evenWindows(lineCenters(bm.longestRun(false, x0, x1+1, thrInk), minHLen)) {
			tryLattice(vg, hg)
		}
	}
	if best != nil {
		best.dense = dense
		return best, nil
	}
	savedDense := dense

	// Pass 2a: scanned pages have broken, speckled grid lines that no
	// contiguous run survives. Counting dark pixels per column/row over
	// the whole page finds them anyway.
	colDark := make([]int, bm.w)
	for x := 0; x < bm.w; x++ {
		n := 0
		for y := 0; y < bm.h; y++ {
			if bm.at(x, y, thrLine) {
				n++
			}
		}
		colDark[x] = n
	}
	for _, vg := range evenWindows(lineCenters(colDark, bm.h/4)) {
		x0, x1 := int(vg[0]), int(vg[16])
		rowDark := make([]int, bm.h)
		for y := 0; y < bm.h; y++ {
			n := 0
			for x := x0; x <= x1 && x < bm.w; x++ {
				if bm.at(x, y, thrLine) {
					n++
				}
			}
			rowDark[y] = n
		}
		for _, hg := range evenWindows(lineCenters(rowDark, (x1-x0)*7/10)) {
			tryLattice(vg, hg)
		}
	}
	if best != nil {
		if dense == nil {
			dense = savedDense
		}
		best.dense = dense
		return best, nil
	}
	if bandY0 == 0 {
		return nil, fmt.Errorf("no 17x17 lattice found at all")
	}

	// pass 2: darkness-count line detection inside the known y-band,
	// with the low threshold that catches anti-aliased hairlines
	h := bandY1 - bandY0
	counts := make([]int, bm.w)
	for x := 0; x < bm.w; x++ {
		n := 0
		for y := bandY0; y <= bandY1; y++ {
			if bm.at(x, y, thrLine) {
				n++
			}
		}
		counts[x] = n
	}
	for _, vg := range evenWindows(lineCenters(counts, h*6/10)) {
		x0, x1 := int(vg[0]), int(vg[16])
		w := x1 - x0
		hcounts := make([]int, bm.h)
		for y := max(0, bandY0-h/10); y <= bandY1+h/10 && y < bm.h; y++ {
			n := 0
			for x := x0; x <= x1; x++ {
				if bm.at(x, y, thrLine) {
					n++
				}
			}
			hcounts[y] = n
		}
		for _, hg := range evenWindows(lineCenters(hcounts, w*6/10)) {
			tryLattice(vg, hg)
		}
	}
	if best == nil && excl != nil {
		// Pass 3: some layouts draw the puzzle with thin, uniform lines
		// that no line search picks up reliably. But both grids are
		// printed side by side at the same size, and the solution grid's
		// geometry is known exactly from the text layer - so mirror it:
		// keep pitch and row lines, and slide the column block left
		// until the lines sit on actual ink.
		pitch := (excl.x1 - excl.x0) / 16
		hg := make([]float64, 17)
		for i := range hg {
			hg[i] = excl.y0 + float64(i)*(excl.y1-excl.y0)/16
		}
		// Score by the SECOND weakest of the 17 lines, not the median: a
		// window shifted by a whole cell still has 16 lines sitting on
		// ink and only one hanging outside the grid, which a median
		// happily ignores - and the result is a grid short one column.
		// The outermost border can be printed lighter, hence second
		// weakest rather than weakest. Cell pitch is searched too: the
		// puzzle grid is not always exactly the size of the solution.
		score := func(vg []float64) float64 {
			fs := make([]float64, 0, 17)
			for _, x := range vg {
				n, dark := 0, 0
				for y := int(hg[0]); y <= int(hg[16]); y++ {
					n++
					if bm.at(int(x)-1, y, thrLine) || bm.at(int(x), y, thrLine) || bm.at(int(x)+1, y, thrLine) {
						dark++
					}
				}
				if n > 0 {
					fs = append(fs, float64(dark)/float64(n))
				}
			}
			sort.Float64s(fs)
			if len(fs) < 2 {
				return 0
			}
			return fs[1]
		}
		// Keep the best-scoring window that also yields a plausible clue
		// pattern - the highest score alone can be a window slid one
		// cell off the grid, whose outermost column stays empty.
		bestScore := 0.0
		for k := -12; k <= 12; k++ {
			p := pitch * (1 + float64(k)*0.0025)
			for x := 0.0; x+16*p < excl.x0; x++ {
				vg := make([]float64, 17)
				for i := range vg {
					vg[i] = x + float64(i)*p
				}
				s := score(vg)
				if s <= bestScore {
					continue
				}
				m := probeCells(bm, vg, hg)
				if m.filled < 60 || m.filled > 200 || !plausibleMask(m) {
					continue
				}
				bestScore = s
				best = &detection{m: m, vg: vg, hg: hg, bm: bm}
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no sparse 17x17 lattice found")
	}
	if dense == nil {
		dense = savedDense
	}
	best.dense = dense
	return best, nil
}

// plausibleMask rejects clue patterns no published puzzle has. A fully
// occupied line means the window has slid into the solution grid; an
// empty line means it has slid off the puzzle, so one of its 16 columns
// sits outside the grid and stays blank.
func plausibleMask(m *mask) bool {
	for i := 0; i < 16; i++ {
		nr, nc := 0, 0
		for j := 0; j < 16; j++ {
			if m.given[i][j] {
				nr++
			}
			if m.given[j][i] {
				nc++
			}
		}
		if nr == 16 || nc == 16 || nr == 0 || nc == 0 {
			return false
		}
	}
	return true
}

// latticeIsReal checks that the candidate's 17+17 positions carry
// actual lines: at a real grid line most pixels along it are dark,
// while a lattice accidentally fitted over text has no such contrast.
func latticeIsReal(bm *bitmap, vlines, hlines []float64) bool {
	frac := func(vertical bool, at float64, lo, hi float64) float64 {
		p := int(at)
		n, dark := 0, 0
		for q := int(lo); q <= int(hi); q++ {
			n++
			// allow one pixel of jitter for scanned, slightly skewed lines
			hit := false
			for d := -1; d <= 1 && !hit; d++ {
				if vertical {
					hit = bm.at(p+d, q, thrLine)
				} else {
					hit = bm.at(q, p+d, thrLine)
				}
			}
			if hit {
				dark++
			}
		}
		if n == 0 {
			return 0
		}
		return float64(dark) / float64(n)
	}
	medianOK := func(fs []float64) bool {
		sort.Float64s(fs)
		return fs[len(fs)/2] >= 0.7
	}
	var vf, hf []float64
	for _, x := range vlines {
		vf = append(vf, frac(true, x, hlines[0], hlines[16]))
	}
	for _, y := range hlines {
		hf = append(hf, frac(false, y, vlines[0], vlines[16]))
	}
	return medianOK(vf) && medianOK(hf)
}

func probeCells(bm *bitmap, vlines, hlines []float64) *mask {
	m := &mask{}
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			cx0, cx1 := vlines[c], vlines[c+1]
			cy0, cy1 := hlines[r], hlines[r+1]
			// probe the central area, away from the grid lines
			px0 := int(cx0 + (cx1-cx0)*0.28)
			px1 := int(cx0 + (cx1-cx0)*0.72)
			py0 := int(cy0 + (cy1-cy0)*0.22)
			py1 := int(cy0 + (cy1-cy0)*0.78)
			ink := 0
			for y := py0; y <= py1; y++ {
				for x := px0; x <= px1; x++ {
					if bm.at(x, y, thrInk) {
						ink++
					}
				}
			}
			area := (px1 - px0 + 1) * (py1 - py0 + 1)
			// a digit covers a few percent of the cell; near-total
			// coverage is a shaded (always empty) prize cell
			if ink*100 >= area*3 && ink*100 <= area*45 && ink >= 6 {
				m.given[r][c] = true
				m.filled++
			}
		}
	}
	return m
}

// ---------- phase A driver ----------

func processPDF(path, outDir, pdftoppm, pdftotext, tesseract string, dpi int, verbose bool) error {
	key := issueKey(path)
	tmpDir, err := os.MkdirTemp("", "hexextract")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	pages, err := pdfWords(pdftotext, path, tmpDir)
	if err != nil {
		return err
	}

	// Which pages to examine: normally the ones naming the puzzle.
	// Pure image scans have no text layer at all, so every page is a
	// candidate - swept from the back, where the Hexadoku always sits.
	var candidates []int
	for i, words := range pages {
		var all strings.Builder
		for _, w := range words {
			all.WriteString(w.s)
		}
		if strings.Contains(strings.ToLower(all.String()), "hexadoku") {
			candidates = append(candidates, i+1)
		}
	}
	sweep := len(candidates) == 0
	if sweep {
		for p := len(pages); p >= 1; p-- {
			candidates = append(candidates, p)
		}
		if verbose {
			fmt.Printf("  %s: no text layer, sweeping %d pages\n", key, len(candidates))
		}
	}

	var bestSol *grid
	var bestSolBox *ptRect
	var bestDet *detection
	bestPage := 0
	for _, pageNo := range candidates {
		words := pages[pageNo-1]
		sol, solBox := solutionFromWords(words)
		var det *detection
		if img, rerr := renderPage(pdftoppm, path, pageNo, dpi, tmpDir); rerr == nil {
			var merr error
			det, merr = maskFromImage(img, dpi, solBox)
			if merr != nil && verbose {
				fmt.Printf("  %s p.%d mask: %v\n", key, pageNo, merr)
			}
			os.Remove(img)
		} else if verbose {
			fmt.Printf("  %s p.%d render: %v\n", key, pageNo, rerr)
		}
		if verbose {
			fmt.Printf("  %s p.%d: solution=%v mask=%v\n", key, pageNo, sol != nil, det != nil)
		}
		if sweep {
			// Without a text layer the page has to be identified from
			// the image alone. A 16x16 lattice is already distinctive;
			// OCR confirms the cells really hold hex characters, which
			// rules out tables and coupon forms.
			if det == nil {
				continue
			}
			if tesseract != "" {
				g, oerr := ocrGrid(det, tesseract, tmpDir)
				if oerr != nil || g.filled < det.m.filled/2 {
					if verbose {
						fmt.Printf("  %s p.%d: lattice not confirmed by OCR (%v)\n", key, pageNo, oerr)
					}
					continue
				}
			}
			bestSol, bestSolBox, bestDet, bestPage = sol, solBox, det, pageNo
			break
		}
		// The Hexadoku always sits in the back of the magazine, while
		// the contents page up front mentions it too and can carry a
		// table that passes for a lattice. So a later page always beats
		// an earlier one, and a page with both grids ends the search.
		if sol != nil || det != nil {
			bestSol, bestSolBox, bestDet, bestPage = sol, solBox, det, pageNo
			if sol != nil && det != nil {
				break
			}
		}
	}
	if bestSol == nil && bestDet == nil {
		return fmt.Errorf("no hexadoku grids found")
	}

	// The page is located at the lower render resolution, but faint
	// digits can drop out of the ink test there. Re-detect the mask on
	// the 300 dpi render used for OCR and keep that one - it sees the
	// same grid with more pixels per cell.
	var ocrDet *detection
	const ocrDpi = 300
	if bestDet != nil {
		if img, rerr := renderPage(pdftoppm, path, bestPage, ocrDpi, tmpDir); rerr == nil {
			if d, merr := maskFromImage(img, ocrDpi, bestSolBox); merr == nil {
				// Always prefer the high-resolution detection. Comparing
				// clue counts across resolutions is wrong: a lattice
				// fitted to the wrong thing can carry MORE "clues" than
				// the real grid and would win on count alone.
				if verbose && d.m.filled != bestDet.m.filled {
					fmt.Printf("  %s: mask %d clues at %d dpi -> %d clues at %d dpi\n",
						key, bestDet.m.filled, dpi, d.m.filled, ocrDpi)
				}
				ocrDet = d
				bestDet = d
			}
			defer os.Remove(img)
		}
	}

	parts := []string{}
	if bestSol != nil {
		sf := filepath.Join(outDir, key+"_prevsolution.txt")
		head := fmt.Sprintf("# solution of the previous issue's Hexadoku, printed in Elektor %s (PDF page %d)\n", key, bestPage)
		if err := os.WriteFile(sf, []byte(head+bestSol.String()), 0o644); err != nil {
			return err
		}
		parts = append(parts, "solution")
	}
	if bestDet != nil {
		mf := filepath.Join(outDir, key+"_mask.txt")
		head := fmt.Sprintf("# clue mask of the Elektor %s Hexadoku (PDF page %d), %d clues\n", key, bestPage, bestDet.m.filled)
		if err := os.WriteFile(mf, []byte(head+bestDet.m.String()), 0o644); err != nil {
			return err
		}
		parts = append(parts, fmt.Sprintf("mask(%d clues)", bestDet.m.filled))
	}

	// independent digit OCR on the same high-resolution detection
	if ocrDet != nil && tesseract != "" {
		if og, oerr := ocrGrid(ocrDet, tesseract, tmpDir); oerr == nil {
			of := filepath.Join(outDir, key+"_ocr.txt")
			head := fmt.Sprintf("# Elektor Hexadoku %s, digits read by tesseract OCR (PDF page %d), %d cells\n", key, bestPage, og.filled)
			if err := os.WriteFile(of, []byte(head+og.String()), 0o644); err != nil {
				return err
			}
			parts = append(parts, fmt.Sprintf("ocr(%d cells)", og.filled))
		} else if verbose {
			fmt.Printf("  %s ocr: %v\n", key, oerr)
		}
		// scanned issues have no text layer: read the printed
		// previous-issue solution from the dense lattice
		if bestSol == nil && ocrDet.dense != nil {
			if sg, oerr := ocrGrid(ocrDet.dense, tesseract, tmpDir); oerr == nil && sg.filled >= 240 {
				sf := filepath.Join(outDir, key+"_prevsolution.txt")
				head := fmt.Sprintf("# solution of a previous issue's Hexadoku, printed in Elektor %s (PDF page %d)\n# read by tesseract OCR from the scan, %d of 256 cells\n", key, bestPage, sg.filled)
				if err := os.WriteFile(sf, []byte(head+sg.String()), 0o644); err != nil {
					return err
				}
				parts = append(parts, fmt.Sprintf("solution-ocr(%d cells)", sg.filled))
			} else if verbose {
				fmt.Printf("  %s solution ocr: %v\n", key, oerr)
			}
		}
	}
	fmt.Printf("ok   %s: p.%d %s\n", key, bestPage, strings.Join(parts, " + "))
	return nil
}

// ---------- phase B: chain ----------

func readGridFile(path string, wantHash bool) ([16][16]byte, error) {
	var g [16][16]byte
	data, err := os.ReadFile(path)
	if err != nil {
		return g, err
	}
	row := 0
	isGridLine := func(s string) bool {
		if len(s) != 16 {
			return false
		}
		for i := 0; i < 16; i++ {
			if !isHex(s[i]) && s[i] != '.' && s[i] != '#' {
				return false
			}
		}
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !isGridLine(line) {
			continue
		}
		if row >= 16 {
			return g, fmt.Errorf("%s: more than 16 grid rows", path)
		}
		for c := 0; c < 16; c++ {
			g[row][c] = line[c]
		}
		row++
	}
	if row != 16 {
		return g, fmt.Errorf("%s: got %d rows", path, row)
	}
	return g, nil
}

// chain matches each puzzle mask with the printed solution that belongs
// to it. Elektor prints a puzzle's solution a few issues later (about
// three months for the prize draw), and the lag varies between the
// monthly and bimonthly eras - so solutions are matched by CONTENT:
// the tesseract OCR digits of a puzzle identify its printed solution
// (>=85% agreement on the clue cells). The typical lag learned from
// those matches is then applied to masks without usable OCR.
func chain(outDir, solver string) error {
	loadSet := func(suffix string) map[string][16][16]byte {
		out := map[string][16][16]byte{}
		files, _ := filepath.Glob(filepath.Join(outDir, "*"+suffix))
		for _, f := range files {
			key := strings.TrimSuffix(filepath.Base(f), suffix)
			if _, ok := parseKey(key); !ok {
				continue
			}
			if g, err := readGridFile(f, false); err == nil {
				out[key] = g
			}
		}
		return out
	}
	masks := loadSet("_mask.txt")
	sols := loadSet("_prevsolution.txt")
	ocrs := loadSet("_ocr.txt")

	// OCR'd solutions (scanned issues) may carry misread characters.
	// Those break the hexadoku property, so the affected cells can be
	// blanked and recomputed by the solver - provably correct whenever
	// the remainder is uniquely solvable.
	if solver != "" {
		tmpDir, err := os.MkdirTemp("", "hexrepair")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		repaired, dropped := 0, 0
		for k, g := range sols {
			fixed, n, err := repairSolution(g, solver, tmpDir)
			if n == 0 {
				continue
			}
			if err != nil {
				fmt.Printf("drop %s solution: %v\n", k, err)
				delete(sols, k)
				dropped++
				continue
			}
			sols[k] = fixed
			p := filepath.Join(outDir, k+"_prevsolution.txt")
			head := fmt.Sprintf("# solution printed in Elektor %s\n# %d OCR-damaged cells repaired via the hexadoku constraints\n", k, n)
			if err := os.WriteFile(p, []byte(head+gridString(fixed)), 0o644); err != nil {
				return err
			}
			repaired++
		}
		if repaired+dropped > 0 {
			fmt.Printf("solutions: %d repaired, %d dropped as unrecoverable\n", repaired, dropped)
		}
	}

	// timeline over all known issue keys
	seen := map[string]bool{}
	var timeline []string
	for k := range masks {
		seen[k] = true
	}
	for k := range sols {
		seen[k] = true
	}
	for k := range seen {
		timeline = append(timeline, k)
	}
	sort.Slice(timeline, func(i, j int) bool {
		a, _ := parseKey(timeline[i])
		b, _ := parseKey(timeline[j])
		if a.year != b.year {
			return a.year < b.year
		}
		return a.m1 < b.m1
	})
	pos := map[string]int{}
	for i, k := range timeline {
		pos[k] = i
	}

	// OCR-based matching
	agreement := func(maskKey, solKey string) (float64, int) {
		m, o, s := masks[maskKey], ocrs[maskKey], sols[solKey]
		match, n := 0, 0
		for r := 0; r < 16; r++ {
			for c := 0; c < 16; c++ {
				if m[r][c] == '#' && o[r][c] != '.' && o[r][c] != 0 {
					n++
					if o[r][c] == s[r][c] {
						match++
					}
				}
			}
		}
		if n < 30 {
			return 0, n
		}
		return float64(match) / float64(n), n
	}

	assigned := map[string]string{} // maskKey -> solKey
	var lags []int
	var maskKeys []string
	for k := range masks {
		maskKeys = append(maskKeys, k)
	}
	sort.Slice(maskKeys, func(i, j int) bool { return pos[maskKeys[i]] < pos[maskKeys[j]] })

	for _, mk := range maskKeys {
		if _, ok := ocrs[mk]; !ok {
			continue
		}
		bestKey, bestScore := "", 0.0
		for sk := range sols {
			d := pos[sk] - pos[mk]
			if d < 1 || d > 6 {
				continue
			}
			if sc, _ := agreement(mk, sk); sc > bestScore {
				bestKey, bestScore = sk, sc
			}
		}
		if bestScore >= 0.85 {
			assigned[mk] = bestKey
			lags = append(lags, pos[bestKey]-pos[mk])
		}
	}
	lagMode := 0
	if len(lags) > 0 {
		cnt := map[int]int{}
		for _, l := range lags {
			cnt[l]++
		}
		for l, c := range cnt {
			if c > cnt[lagMode] {
				lagMode = l
			}
		}
		fmt.Printf("solution lag: mode %d issue(s), from %d OCR matches\n", lagMode, len(lags))
	}

	built, skipped := 0, 0
	for _, mk := range maskKeys {
		// never overwrite a hand-checked transcription
		pf := filepath.Join(outDir, mk+".txt")
		if b, err := os.ReadFile(pf); err == nil && strings.Contains(string(b), "transcribed visually") {
			fmt.Printf("keep %s: visually transcribed, left untouched\n", mk)
			continue
		}
		mask := masks[mk]
		solKey, matched := assigned[mk], true
		if solKey == "" {
			matched = false
			if lagMode > 0 && pos[mk]+lagMode < len(timeline) {
				cand := timeline[pos[mk]+lagMode]
				if _, ok := sols[cand]; ok {
					solKey = cand
				}
			}
		}

		// A pairing is only trustworthy if the resulting puzzle comes
		// out uniquely solvable. Where OCR could not confirm one, try
		// the candidates in order of plausibility and let the solver
		// decide - a wrong pairing practically never solves uniquely.
		note := ""
		if solver != "" && !matched {
			var cands []string
			if solKey != "" {
				cands = append(cands, solKey)
			}
			for d := 1; d <= 6 && pos[mk]+d < len(timeline); d++ {
				c := timeline[pos[mk]+d]
				if _, ok := sols[c]; ok && c != solKey {
					cands = append(cands, c)
				}
			}
			solKey = ""
			for _, c := range cands {
				if puzzleIsUnique(buildPuzzle(mask, sols[c]), solver, outDir) {
					solKey = c
					note = fmt.Sprintf("digits from %s_prevsolution (pairing confirmed by unique solvability)", c)
					break
				}
			}
		}

		var sb strings.Builder
		clues := 0
		if solKey != "" {
			sol := sols[solKey]
			for r := 0; r < 16; r++ {
				for c := 0; c < 16; c++ {
					if mask[r][c] == '#' {
						sb.WriteByte(sol[r][c])
						clues++
					} else {
						sb.WriteByte('.')
					}
				}
				sb.WriteByte('\n')
			}
			if note == "" {
				sc, n := agreement(mk, solKey)
				note = fmt.Sprintf("digits from %s_prevsolution (OCR match %.0f%% of %d cells)", solKey, sc*100, n)
			}
		} else if o, ok := ocrs[mk]; ok {
			for r := 0; r < 16; r++ {
				for c := 0; c < 16; c++ {
					if mask[r][c] == '#' && o[r][c] != '.' && o[r][c] != 0 {
						sb.WriteByte(o[r][c])
						clues++
					} else {
						sb.WriteByte('.')
					}
				}
				sb.WriteByte('\n')
			}
			note = "digits from tesseract OCR only (no matching printed solution found)"
			if solver != "" {
				var pg [16][16]byte
				for r := 0; r < 16; r++ {
					copy(pg[r][:], sb.String()[r*17:r*17+16])
				}
				tmpDir, err := os.MkdirTemp("", "hexrepair")
				if err != nil {
					return err
				}
				fixed, nbad, rerr := repairPuzzle(pg, solver, tmpDir)
				os.RemoveAll(tmpDir)
				if nbad > 0 {
					if rerr != nil {
						note += fmt.Sprintf("; %d OCR conflicts NOT repairable (%v)", nbad, rerr)
					} else {
						sb.Reset()
						sb.WriteString(gridString(fixed))
						note += fmt.Sprintf("; %d OCR-damaged cells repaired via the hexadoku constraints", nbad)
					}
				}
			}
		} else {
			// Nothing to build from. Remove a puzzle left over from an
			// earlier run so it cannot be mistaken for current output.
			if err := os.Remove(pf); err == nil {
				fmt.Printf("skip %s: no matching solution and no OCR digits (stale puzzle removed)\n", mk)
			} else {
				fmt.Printf("skip %s: no matching solution and no OCR digits\n", mk)
			}
			skipped++
			continue
		}

		head := fmt.Sprintf("# Elektor Hexadoku %s (%d clues)\n# %s\n", mk, clues, note)
		if err := os.WriteFile(pf, []byte(head+sb.String()), 0o644); err != nil {
			return err
		}
		fmt.Printf("ok   %s: %d clues (%s)\n", mk, clues, note)
		built++
	}
	fmt.Printf("chain: %d puzzles built, %d skipped\n", built, skipped)
	return nil
}

// ---------- main ----------

func main() {
	if maybeDebug(os.Args[1:]) {
		return
	}
	outDir := flag.String("out", "puzzles/elektor", "output directory")
	doChain := flag.Bool("chain", false, "combine masks and next-issue solutions into puzzles")
	crops := flag.Int("crops", 0, "export grid crops of this PDF page as PNG for visual reading (-1 = auto-detect page)")
	bands := flag.Int("bands", 4, "with -crops: split each grid into this many horizontal bands")
	dpi := flag.Int("dpi", 150, "render resolution for mask detection")
	pdftoppm := flag.String("pdftoppm", "pdftoppm", "path to poppler's pdftoppm")
	pdftotext := flag.String("pdftotext", "", "path to poppler's pdftotext (default: next to pdftoppm)")
	tesseract := flag.String("tesseract", "", "path to tesseract for independent digit OCR (empty = skip)")
	solver := flag.String("solver", "./hexadoku.exe", "path to the hexadoku solver, used to repair OCR damage")
	verbose := flag.Bool("v", false, "verbose diagnostics")
	flag.Parse()
	if *pdftotext == "" {
		*pdftotext = filepath.Join(filepath.Dir(*pdftoppm), "pdftotext"+filepath.Ext(*pdftoppm))
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *doChain {
		if err := chain(*outDir, *solver); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *crops != 0 {
		if flag.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "-crops needs exactly one PDF")
			os.Exit(2)
		}
		if err := exportCrops(flag.Arg(0), *crops, *pdftoppm, *pdftotext, *outDir, 300, *bands); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ok, fail := 0, 0
	for _, pattern := range flag.Args() {
		files, _ := filepath.Glob(pattern)
		if files == nil {
			files = []string{pattern}
		}
		for _, f := range files {
			if err := processPDF(f, *outDir, *pdftoppm, *pdftotext, *tesseract, *dpi, *verbose); err != nil {
				fail++
				fmt.Printf("FAIL %s: %v\n", filepath.Base(f), err)
			} else {
				ok++
			}
		}
	}
	fmt.Printf("%d processed, %d failed\n", ok, fail)
}
