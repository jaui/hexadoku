// Geometry description for puzzles that are more than a single grid.
//
// Every sudoku-like puzzle is the same abstract problem: a set of cells,
// a set of values, and a set of *units* - groups of exactly nvals cells
// that must contain every value exactly once. For a plain grid the units
// are its rows, columns and boxes. A samurai layout is simply five grids
// whose units are declared over one shared cell pool, so the overlapping
// boxes are literally the same cells in the units of two grids at once.
//
// This description drives the general core in general.go. The single-grid
// sizes keep their hand-written cores in solver.go, where the geometry is
// a compile-time constant and roughly three times faster; nothing here is
// on their hot path.
package main

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxGCells = 1352 // hexadocube: 6*256 - 12*16 shared edges + 8 corners
	maxUnits  = 276  // hexadocube: 6*48 units - 12 shared boundary lines
	// A cell inside an overlap belongs to two rows, two columns and one
	// box (the box is shared by both grids, so it is a single unit). A
	// cube corner belongs to three faces, but the three lines meeting
	// there are shared pairwise, which again leaves six units.
	maxCellUnits = 6
)

type variant struct {
	name   string
	nvals  int    // 9 or 16
	all    uint16 // nvals low bits set
	ncells int
	nunits int

	unitCells [maxUnits][maxSize]uint16       // unit -> its nvals cells
	cellUnits [maxGCells][maxCellUnits]uint16 // cell -> the units containing it

	// Per-cell value restriction, all bits set unless a variant narrows
	// it (the chequerboard of the EUC penta-hexadoku does). It replaces
	// the constant "all" in cand, so the rule costs one load and not a
	// second mask operation.
	allow [maxGCells]uint16

	// touch[k] is the set of units whose candidate picture an assignment
	// to cell k can change: every unit of every cell that shares a unit
	// with k. assign ORs it into the board's dirty set, and propagate
	// looks only at dirty units - see general.go.
	touch [maxGCells]dirtySet

	// text layout: cells sit on a width x height character raster,
	// positions that are not part of any grid stay blank
	width, height int
	pos           [maxGCells]int32 // cell -> row*width + col
	at            []int32          // row*width + col -> cell+1, 0 = no cell
	// tokens: read the input as a plain sequence of cell characters and
	// ignore layout. Only correct where every raster position is a cell
	// (a single grid), but it tolerates separators and spacing.
	tokens bool

	// Hexadoku digest: sixteen blocks of five cells that take sixteen
	// given codes, one each. Empty for every other layout; see digest.go.
	blocks [][]int
	codes  [][]uint8

	builderr error // a header the layout could not be built from

	ncu  [maxGCells]uint8 // units registered so far, build time only
	seen map[string]int   // unit dedup by cell set, build time only
}

func newVariant(name string, nvals, width, height int) *variant {
	v := &variant{
		name:   name,
		nvals:  nvals,
		all:    uint16(1)<<nvals - 1,
		width:  width,
		height: height,
		at:     make([]int32, width*height),
		seen:   make(map[string]int),
	}
	return v
}

func (v *variant) cellAt(r, c int) int { return int(v.at[r*v.width+c]) - 1 }

// layout allocates the cells covered by the given grid origins in raster
// order and then declares the units of every grid. Overlapping grids share
// the cells they cover; identical units (the shared boxes) are declared once.
func (v *variant) layout(origins [][2]int, n, b int) {
	for _, o := range origins {
		for r := 0; r < n; r++ {
			for c := 0; c < n; c++ {
				v.at[(o[0]+r)*v.width+o[1]+c] = -1
			}
		}
	}
	for i, m := range v.at {
		if m == -1 {
			v.pos[v.ncells] = int32(i)
			v.ncells++
			v.at[i] = int32(v.ncells) // cell + 1
		}
	}
	for _, o := range origins {
		v.addGrid(o[0], o[1], n, b)
	}
	v.finish()
}

// addGrid declares the n rows, n columns and n boxes of one nxn grid
// whose top left corner sits at (r0, c0).
func (v *variant) addGrid(r0, c0, n, b int) {
	cells := make([]int, n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			cells[c] = v.cellAt(r0+r, c0+c)
		}
		v.addUnit(cells)
	}
	for c := 0; c < n; c++ {
		for r := 0; r < n; r++ {
			cells[r] = v.cellAt(r0+r, c0+c)
		}
		v.addUnit(cells)
	}
	for br := 0; br < n; br += b {
		for bc := 0; bc < n; bc += b {
			i := 0
			for r := 0; r < b; r++ {
				for c := 0; c < b; c++ {
					cells[i] = v.cellAt(r0+br+r, c0+bc+c)
					i++
				}
			}
			v.addUnit(cells)
		}
	}
}

func (v *variant) addUnit(cells []int) {
	key := make([]int, len(cells))
	copy(key, cells)
	sort.Ints(key)
	ks := fmt.Sprint(key)
	if _, dup := v.seen[ks]; dup {
		return // shared box, already declared by the other grid
	}
	u := v.nunits
	if u >= maxUnits {
		panic("variant: too many units")
	}
	v.seen[ks] = u
	v.nunits++
	for i, k := range cells {
		v.unitCells[u][i] = uint16(k)
		if v.ncu[k] >= maxCellUnits {
			panic("variant: too many units per cell")
		}
		v.cellUnits[k][v.ncu[k]] = uint16(u)
		v.ncu[k]++
	}
}

// finish pads every cell's unit list to maxCellUnits by repeating its first
// unit. Both candidate lookup and assignment fold the list with OR, and OR
// is idempotent, so the duplicates cost nothing but remove the inner loop
// bound from the hot path.
func (v *variant) finish() {
	for k := 0; k < v.ncells; k++ {
		for i := int(v.ncu[k]); i < maxCellUnits; i++ {
			v.cellUnits[k][i] = v.cellUnits[k][0]
		}
		v.allow[k] = v.all
	}
	for k := 0; k < v.ncells; k++ {
		for _, u := range v.cellUnits[k] {
			for _, c := range v.unitCells[u][:v.nvals] {
				for _, w := range v.cellUnits[c] {
					v.touch[k].set(int(w))
				}
			}
		}
	}
	v.seen = nil
}

// ---------------- text I/O ----------------

// render prints the puzzle on its character raster: one line per row,
// exactly width characters, blanks where no grid covers the position.
// The output parses back in unchanged. Iterating over the raster and not
// over the cells matters for the hexadocube, where a cell on a cube edge
// is drawn once on each of the faces that share it.
func (v *variant) render(g *gboard) string {
	buf := make([]byte, v.width*v.height)
	for i := range buf {
		buf[i] = ' '
	}
	for i, m := range v.at {
		if m == 0 {
			continue
		}
		ch := byte('.')
		if k := int(m) - 1; g.grid[k] != empty {
			ch = charOf(v.nvals, g.grid[k])
		}
		buf[i] = ch
	}
	var sb strings.Builder
	for r := 0; r < v.height; r++ {
		sb.Write(buf[r*v.width : (r+1)*v.width])
		sb.WriteByte('\n')
	}
	return sb.String()
}

// parse reads a puzzle laid out on the variant's raster. Row and column of
// a character are its line and column number, so the blanks between the
// grids carry meaning and must be preserved.
func (v *variant) parse(text string) (*gboard, error) {
	if v.builderr != nil {
		return nil, v.builderr
	}
	g := v.newBoard()
	if v.tokens {
		k := 0
		s := stripComments(text)
		for i := 0; i < len(s); i++ {
			t := valueOf(v.nvals, s[i])
			if t == chNone {
				continue
			}
			if k >= v.ncells {
				return nil, fmt.Errorf("%s: more than %d cells", v.name, v.ncells)
			}
			if t != chEmpty {
				if err := v.place(g, k, uint8(t), k/v.width+1, k%v.width+1); err != nil {
					return nil, err
				}
			}
			k++
		}
		if k != v.ncells {
			return nil, fmt.Errorf("%s: expected %d cells, got %d", v.name, v.ncells, k)
		}
		return g, nil
	}
	row := 0
	for _, line := range strings.Split(stripComments(text), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue // blank separator lines are ignored, no grid row is blank
		}
		if row >= v.height {
			return nil, fmt.Errorf("%s: more than %d grid lines", v.name, v.height)
		}
		for c := 0; c < len(line); c++ {
			t := valueOf(v.nvals, line[c])
			if t == chNone {
				continue
			}
			if c >= v.width {
				return nil, fmt.Errorf("%s: line %d is wider than %d columns", v.name, row+1, v.width)
			}
			k := v.cellAt(row, c)
			if k < 0 {
				return nil, fmt.Errorf("%s: character %q at line %d column %d lies outside every grid",
					v.name, line[c], row+1, c+1)
			}
			if t == chEmpty {
				continue
			}
			if err := v.place(g, k, uint8(t), row+1, c+1); err != nil {
				return nil, err
			}
		}
		row++
	}
	return g, nil
}

func (v *variant) place(g *gboard, k int, val uint8, line, col int) error {
	// A cell drawn twice - the two sides of a cube edge - must carry the
	// same clue on both faces; that is the rule the magazine states, and
	// here it is simply the second copy agreeing with the first.
	if have := g.grid[k]; have != empty {
		if have == val {
			return nil
		}
		return fmt.Errorf("clue %c at line %d, column %d contradicts the %c "+
			"in the same cell on the neighbouring face",
			charOf(v.nvals, val), line, col, charOf(v.nvals, have))
	}
	if v.cand(g, k)&(1<<val) == 0 {
		if v.allow[k]&(1<<val) == 0 {
			return fmt.Errorf("clue %c at line %d, column %d is on the wrong "+
				"colour for the chequerboard", charOf(v.nvals, val), line, col)
		}
		return fmt.Errorf("conflicting clue %c at line %d, column %d",
			charOf(v.nvals, val), line, col)
	}
	v.assign(g, k, val)
	return nil
}
