// Wide layouts: the two Elektor summer puzzles whose alphabet is larger
// than a uint16 candidate mask.
//
//	Alphanumski, 7-8/2007, Gery Szcepanski
//	  one 36x36 grid in 6x6 boxes over 0-9 and A-Z. "All letters of the
//	  alphabet (A through Z) and all numerals 0 through 9 should occur
//	  only once in every line (1x36), every column (1x36) and every box
//	  (6x6)". No further rule - the chequerboard of the boxes is
//	  decoration, unlike the one of the 2012 penta-hexadoku.
//
//	AlphaSudoku, 7-8/2008, Claude Ghyselen
//	  one 25x25 grid in 5x5 boxes over 1-9 and A-P ("including the letter
//	  'O', hence the absence of the '0' in the numbers"), plus nine shaded
//	  3x3 blocks in the middle of the nine middle boxes: "the puzzle is
//	  essentially a (16x16) Alphadoku comprising nine classic (9x9)
//	  Sudokus using numbers 1 through 9".
//
// The geometry of both is the plainest of the whole series; what puts
// them outside general.go is the alphabet, which is a data type and not a
// layout. So this is that core once more over uint64 masks, with one
// genuine generalization: a unit carries its own size and its own value
// set. The nine blocks of the AlphaSudoku are units of nine cells over
// nine values inside a puzzle of twenty-five, and that is exactly the
// rule the magazine states - everything else about the embedded sudoku
// follows from it (see alphasudokuVariant).
//
// The specialized 9/16 cores in solver.go and the uint16 core in
// general.go are untouched: they are fast because their masks fit a
// register, and nothing here is on their path.
package main

import (
	"fmt"
	"math/bits"
	"math/rand/v2"
	"strings"
)

const (
	maxWideCells     = 36 * 36 // Alphanumski, the larger of the two
	maxWideUnits     = 120     // 3*36 = 108; 3*25 + 9 = 84
	maxWideUnit      = 36      // cells in the largest unit
	maxWideCellUnits = 4       // row, column, box, and for a shaded cell its block
)

// wboard is the whole solver state, flat and copyable like board and
// gboard: ~2.3 kB, so a branch node is one struct copy and the hot path
// never allocates.
type wboard struct {
	placed [maxWideUnits]uint64 // values already used per unit
	grid   [maxWideCells]uint8
	free   int16
	dirty  wideDirtySet // see gboard.dirty
}

type wideDirtySet [(maxWideUnits + 63) / 64]uint64

func (d *wideDirtySet) set(u int) { d[u>>6] |= 1 << (u & 63) }

func (d *wideDirtySet) or(o *wideDirtySet) {
	for i := range d {
		d[i] |= o[i]
	}
}

func (d *wideDirtySet) pop() int {
	for i := range d {
		if w := d[i]; w != 0 {
			d[i] = w & (w - 1)
			return i<<6 + bits.TrailingZeros64(w)
		}
	}
	return -1
}

type wideVariant struct {
	name     string
	n, box   int    // grid side and box side: 36/6 or 25/5
	nvals    int    // == len(alphabet)
	alphabet string // value v is alphabet[v]
	all      uint64 // nvals low bits set
	ncells   int
	nunits   int

	unitCells [maxWideUnits][maxWideUnit]uint16
	unitSize  [maxWideUnits]uint8
	unitAll   [maxWideUnits]uint64 // the values this unit holds once each

	cellUnits [maxWideCells][maxWideCellUnits]uint16
	allow     [maxWideCells]uint64       // per-cell value restriction, all unless narrowed
	touch     [maxWideCells]wideDirtySet // units an assignment to the cell can change

	// why allow is narrower than all, named in the parse error so a
	// misread clue points at the rule it breaks and not at "conflict"
	narrowed string

	// grey answer cells, in header order, and the code the magazine
	// announced for them if the file claims one
	grey     []int
	greyText string // how the header wrote the ranges, for the report
	greyCode string

	ncu      [maxWideCells]uint8 // units registered so far, build time only
	builderr error
}

// ---------------- layout ----------------

// newWide builds a plain n x n grid in boxes of b over the given alphabet:
// its n rows, n columns and n boxes, each holding every value once.
func newWide(name string, n, b int, alphabet string) *wideVariant {
	v := &wideVariant{
		name:     name,
		n:        n,
		box:      b,
		nvals:    len(alphabet),
		alphabet: alphabet,
		ncells:   n * n,
	}
	v.all = uint64(1)<<v.nvals - 1
	for k := 0; k < v.ncells; k++ {
		v.allow[k] = v.all
	}
	cells := make([]int, n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			cells[c] = r*n + c
		}
		v.addUnit(cells, v.all)
	}
	for c := 0; c < n; c++ {
		for r := 0; r < n; r++ {
			cells[r] = r*n + c
		}
		v.addUnit(cells, v.all)
	}
	for br := 0; br < n; br += b {
		for bc := 0; bc < n; bc += b {
			i := 0
			for r := 0; r < b; r++ {
				for c := 0; c < b; c++ {
					cells[i] = (br+r)*n + bc + c
					i++
				}
			}
			v.addUnit(cells, v.all)
		}
	}
	return v
}

// addUnit declares that the given cells hold each value of vals exactly
// once. vals is the whole alphabet for an ordinary row, column or box,
// and the nine digits for a block of the embedded sudoku.
func (v *wideVariant) addUnit(cells []int, vals uint64) {
	if v.nunits >= maxWideUnits || len(cells) > maxWideUnit {
		panic("wide: too many units")
	}
	u := v.nunits
	v.nunits++
	v.unitSize[u] = uint8(len(cells))
	v.unitAll[u] = vals
	for i, k := range cells {
		v.unitCells[u][i] = uint16(k)
		if v.ncu[k] >= maxWideCellUnits {
			panic("wide: too many units per cell")
		}
		v.cellUnits[k][v.ncu[k]] = uint16(u)
		v.ncu[k]++
	}
}

// finish pads every cell's unit list to maxWideCellUnits by repeating its
// first unit. cand and assign fold the list with OR, and OR is
// idempotent, so the duplicates cost nothing and remove the loop bound
// from the hot path - the same trick as variant.finish.
func (v *wideVariant) finish() {
	for k := 0; k < v.ncells; k++ {
		for i := int(v.ncu[k]); i < maxWideCellUnits; i++ {
			v.cellUnits[k][i] = v.cellUnits[k][0]
		}
	}
	for k := 0; k < v.ncells; k++ {
		for _, u := range v.cellUnits[k] {
			for _, c := range v.unitCells[u][:v.unitSize[u]] {
				for _, w := range v.cellUnits[c] {
					v.touch[k].set(int(w))
				}
			}
		}
	}
}

// alphanumskiVariant is the 36x36 grid of 7-8/2007: rows, columns and
// 6x6 boxes over the 36 characters 0-9 and A-Z, and nothing else.
func alphanumskiVariant() *wideVariant {
	v := newWide("alphanumski", 36, 6, alnum36)
	v.finish()
	return v
}

// alsuBlockLines are the rows - and, mirrored, the columns - covered by
// the nine shaded blocks of the AlphaSudoku, zero based. Each 5x5 box of
// the grid spans five lines and the block takes its middle three, so the
// nine middle boxes carry one block each and the outer ring of sixteen
// boxes stays clear. Measured off the page render of 7-8/2008 p. 118.
var alsuBlockLines = [3][3]int{{6, 7, 8}, {11, 12, 13}, {16, 17, 18}}

// alsuDigits are the values 1-9 in the alsu25 alphabet: its first nine
// characters, so bits 0 to 8.
const alsuDigits = 1<<9 - 1

// alphasudokuVariant is the 25x25 grid of 7-8/2008 plus its extra rule.
//
// Every clue standing in a shaded cell on the printed page is a digit, so
// a shaded cell takes one of 1-9 and no letter: that is allow[k], the
// same per-cell mask the EUC penta-hexadoku uses for its chequerboard.
// On top of it each 3x3 block holds the nine digits once, which is nine
// more units - of nine cells over nine values, where every other unit of
// this puzzle has twenty-five of each.
//
// The rows and columns of the embedded sudoku need no units of their own.
// A shaded row crosses three blocks, so nine of its cells are shaded and
// take digits only; the 25-cell row already holds each digit exactly
// once, so those nine cells are 1-9 by themselves. Same for the columns.
// The block constraint is the one that does not follow, and it is the one
// declared here.
func alphasudokuVariant() *wideVariant {
	v := newWide("alphasudoku", 25, 5, alsu25)
	for r := 0; r < v.n; r++ {
		for c := 0; c < v.n; c++ {
			if alsuShaded(r) && alsuShaded(c) {
				v.allow[r*v.n+c] = alsuDigits
			}
		}
	}
	v.narrowed = "the shaded blocks of the embedded sudoku take the digits 1-9 only"
	cells := make([]int, 9)
	for _, rows := range alsuBlockLines {
		for _, cols := range alsuBlockLines {
			i := 0
			for _, r := range rows {
				for _, c := range cols {
					cells[i] = r*v.n + c
					i++
				}
			}
			v.addUnit(cells, alsuDigits)
		}
	}
	v.finish()
	return v
}

// alsuShaded reports whether row - or column - i is one of the nine lines
// covered by the shaded blocks.
func alsuShaded(i int) bool {
	for _, g := range alsuBlockLines {
		for _, l := range g {
			if l == i {
				return true
			}
		}
	}
	return false
}

// alfadokuVariant is the 25x25 grid of Elektuur 7-8/2006 - the "Alfadoku",
// and the puzzle the 2007 Alphanumski article means when it names its
// predecessor. Rows, columns and 5x5 boxes over the 25 letters A to Y,
// "alle letters van het alfabet, met uitzondering van de Z", no digits and
// no extra rule. Designed not by Ghyselen or Szcepanski but by a reader,
// S. Jobse, and printed only in the Dutch edition this archive could find.
//
// So of the three wide layouts this is the plainest: nothing but the
// alphabet sets it apart from an ordinary sudoku.
func alfadokuVariant() *wideVariant {
	v := newWide("alfadoku", 25, 5, alfa25)
	v.finish()
	return v
}

// gridWideVariant describes a plain grid the wide core can solve, so the
// core can be cross-checked against the specialized ones on puzzles with
// a known solution. Nothing but the tests uses it.
func gridWideVariant(n int) *wideVariant {
	b, alphabet := 3, alnum36[1:10] // 1-9, as in a printed sudoku
	if n == maxSize {
		b, alphabet = 4, alnum36[:16] // 0-F
	}
	v := newWide(fmt.Sprintf("%dx%d", n, n), n, b, alphabet)
	v.finish()
	return v
}

// ---------------- core ----------------

func (v *wideVariant) newBoard() *wboard {
	g := &wboard{free: int16(v.ncells)}
	for i := range g.grid {
		g.grid[i] = empty
	}
	for u := 0; u < v.nunits; u++ {
		g.dirty.set(u)
	}
	return g
}

func (v *wideVariant) cand(g *wboard, k int) uint64 {
	u := &v.cellUnits[k]
	return ^(g.placed[u[0]] | g.placed[u[1]] |
		g.placed[u[2]] | g.placed[u[3]]) & v.allow[k]
}

func (v *wideVariant) assign(g *wboard, k int, val uint8) {
	m := uint64(1) << val
	g.grid[k] = val
	u := &v.cellUnits[k]
	g.placed[u[0]] |= m
	g.placed[u[1]] |= m
	g.placed[u[2]] |= m
	g.placed[u[3]] |= m
	g.free--
	g.dirty.or(&v.touch[k])
}

// propagate is the general core's, over uint64 masks and units of their
// own size and value set; see general.go for why looking only at dirty
// units reaches the very same fixpoint as the full sweep.
func (v *wideVariant) propagate(g *wboard) bool {
	for {
		u := g.dirty.pop()
		if u < 0 {
			return true
		}
		cells := v.unitCells[u][:v.unitSize[u]]
		var once, twice uint64
		for _, k := range cells {
			if g.grid[k] != empty {
				continue
			}
			m := v.cand(g, int(k))
			if m == 0 {
				return false
			}
			if m&(m-1) == 0 { // naked single
				v.assign(g, int(k), uint8(bits.TrailingZeros64(m)))
				continue
			}
			twice |= once & m
			once |= m
		}
		if once|g.placed[u] != v.unitAll[u] {
			return false // a value has nowhere left to go in this unit
		}
		unique := once &^ twice // hidden singles
		if unique == 0 {
			continue
		}
		for _, k := range cells {
			if g.grid[k] != empty {
				continue
			}
			m := v.cand(g, int(k)) & unique
			if m == 0 {
				continue
			}
			if m&(m-1) != 0 {
				return false // one cell is the only place for two values
			}
			v.assign(g, int(k), uint8(bits.TrailingZeros64(m)))
			unique &^= m
		}
	}
}

func (v *wideVariant) mrv(g *wboard) int {
	best, bestCnt := -1, v.nvals+1
	for k := 0; k < v.ncells; k++ {
		if g.grid[k] != empty {
			continue
		}
		if c := bits.OnesCount64(v.cand(g, k)); c < bestCnt {
			best, bestCnt = k, c
			if c == 2 {
				break
			}
		}
	}
	return best
}

func (v *wideVariant) solve(g *wboard) bool {
	if !v.propagate(g) {
		return false
	}
	if g.free == 0 {
		return true
	}
	k := v.mrv(g)
	m := v.cand(g, k)
	save := *g
	for m != 0 {
		val := uint8(bits.TrailingZeros64(m))
		m &= m - 1
		nodes++
		v.assign(g, k, val)
		if v.solve(g) {
			return true
		}
		*g = save
	}
	return false
}

// count counts solutions up to limit and leaves the first one found in
// first, if that is not nil - see the general core for why.
func (v *wideVariant) count(g *wboard, limit int, first *wboard) int {
	if !v.propagate(g) {
		return 0
	}
	if g.free == 0 {
		if first != nil && first.free != 0 {
			*first = *g
		}
		return 1
	}
	k := v.mrv(g)
	m := v.cand(g, k)
	save := *g
	n := 0
	for m != 0 && n < limit {
		val := uint8(bits.TrailingZeros64(m))
		m &= m - 1
		nodes++
		v.assign(g, k, val)
		n += v.count(g, limit-n, first)
		*g = save
	}
	return n
}

// fillRand fills the board at random. It serves the tests only, and it
// is usable for the alphanumski (a full 36x36 in some 15 ms) but not for
// the alphasudoku, where it does not finish: the 81 shaded cells are
// locked to nine of the twenty-five values and the nine blocks each need
// all nine digits, so a greedy random fill paints itself into a corner
// again and again. The extra rule that makes the puzzle easy to *solve* -
// it cuts the candidates of those cells by two thirds - is what makes a
// grid hard to *invent*. Where a full alphasudoku is needed, solve the
// printed one instead.
func (v *wideVariant) fillRand(g *wboard, rng *rand.Rand) bool {
	if !v.propagate(g) {
		return false
	}
	if g.free == 0 {
		return true
	}
	k := v.mrv(g)
	m := v.cand(g, k)
	var vals [maxWideUnit]uint8
	n := 0
	for m != 0 {
		vals[n] = uint8(bits.TrailingZeros64(m))
		m &= m - 1
		n++
	}
	rng.Shuffle(n, func(i, j int) { vals[i], vals[j] = vals[j], vals[i] })
	save := *g
	for i := 0; i < n; i++ {
		v.assign(g, k, vals[i])
		if v.fillRand(g, rng) {
			return true
		}
		*g = save
	}
	return false
}

// ---------------- text I/O ----------------

// render prints the grid as n lines of n characters, which parse back in
// unchanged.
func (v *wideVariant) render(g *wboard) string {
	var sb strings.Builder
	for r := 0; r < v.n; r++ {
		for c := 0; c < v.n; c++ {
			ch := byte('.')
			if val := g.grid[r*v.n+c]; val != empty {
				ch = charOfIn(v.alphabet, val)
			}
			sb.WriteByte(ch)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// parse reads exactly n grid lines of exactly n characters. That is
// stricter than variant.parse, which has to tolerate the blanks between
// the grids of a samurai raster; here a single grid is the whole file, so
// anything off the shape is a mistake in the transcription and is worth
// reporting rather than skipping.
func (v *wideVariant) parse(text string) (*wboard, error) {
	if v.builderr != nil {
		return nil, v.builderr
	}
	g := v.newBoard()
	row := 0
	for _, line := range strings.Split(stripComments(text), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		if row >= v.n {
			return nil, fmt.Errorf("%s: more than %d grid lines", v.name, v.n)
		}
		if len(line) != v.n {
			return nil, fmt.Errorf("%s: line %d holds %d characters, want %d",
				v.name, row+1, len(line), v.n)
		}
		for c := 0; c < v.n; c++ {
			t := valueOfIn(v.alphabet, line[c])
			if t == chEmpty {
				continue
			}
			if t == chNone {
				return nil, fmt.Errorf("%s: character %q at line %d, column %d is not one of %s",
					v.name, line[c], row+1, c+1, v.alphabet)
			}
			if err := v.place(g, row*v.n+c, uint8(t), row+1, c+1); err != nil {
				return nil, err
			}
		}
		row++
	}
	if row != v.n {
		return nil, fmt.Errorf("%s: %d grid lines, want %d", v.name, row, v.n)
	}
	return g, nil
}

func (v *wideVariant) place(g *wboard, k int, val uint8, line, col int) error {
	if v.cand(g, k)&(1<<val) == 0 {
		if v.allow[k]&(1<<val) == 0 {
			return fmt.Errorf("clue %c at line %d, column %d breaks the rule that %s",
				charOfIn(v.alphabet, val), line, col, v.narrowed)
		}
		return fmt.Errorf("conflicting clue %c at line %d, column %d",
			charOfIn(v.alphabet, val), line, col)
	}
	v.assign(g, k, val)
	return nil
}

// greyPinned reports whether every solution of the puzzle agrees with sol
// on all grey cells - that is, whether the answer the magazine asked for
// is unambiguous even where the grid is not.
//
// The Alphanumski of 7-8/2007 needs the distinction: as printed it has
// two solutions, which differ in four cells forming a rectangle (two
// values that can be swapped around it). None of the four is a grey cell,
// so the seven characters the readers had to send in are the same either
// way, and the code the magazine announced is still the only possible
// answer.
//
// The question is decided exactly and cheaply: a solution differing in a
// grey cell exists precisely when some grey cell can be forced to a value
// other than sol's and the puzzle still solves. That is at most one solve
// per (grey cell, value) pair.
func (v *wideVariant) greyPinned(g *wboard, sol *wboard) bool {
	for _, k := range v.grey {
		m := v.cand(g, k) &^ (1 << sol.grid[k])
		for m != 0 {
			val := uint8(bits.TrailingZeros64(m))
			m &= m - 1
			work := *g
			v.assign(&work, k, val)
			if v.solve(&work) {
				return false
			}
		}
	}
	return true
}

// greyString reads the answer cells out of a board, in header order.
func (v *wideVariant) greyString(g *wboard) string {
	var sb strings.Builder
	for _, k := range v.grey {
		if g.grid[k] == empty {
			sb.WriteByte('.')
			continue
		}
		sb.WriteByte(charOfIn(v.alphabet, g.grid[k]))
	}
	return sb.String()
}
