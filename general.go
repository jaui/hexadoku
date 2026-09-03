// General solver core, driven by a variant description.
//
// Same two ideas as the specialized cores in solver.go - constraint
// propagation to a fixpoint (naked and hidden singles), then backtracking
// on the cell with the fewest candidates - but over an arbitrary list of
// units instead of the fixed rows/columns/boxes of one grid. That is what
// makes the overlapping grids of a samurai layout work: a cell in an
// overlap simply appears in the units of both grids, so propagation
// carries information from one grid into the other on its own.
//
// The state is again one flat, copyable struct (~1.7 kB for a hexamurai),
// so a branch node is a struct copy and the hot path never allocates.
package main

import (
	"math/bits"
	"math/rand/v2"
)

type gboard struct {
	placed [maxUnits]uint16 // values already used per unit
	grid   [maxGCells]uint8
	free   int16 // number of empty cells

	// units that may hold a single propagate has not seen yet. A fresh
	// board has every unit dirty; assign adds the units an assignment
	// can affect; propagate clears what it examines and stops when
	// nothing is left.
	dirty dirtySet
}

// dirtySet is a bitset over units, sized for the largest layout.
type dirtySet [(maxUnits + 63) / 64]uint64

func (d *dirtySet) set(u int) { d[u>>6] |= 1 << (u & 63) }

func (d *dirtySet) or(o *dirtySet) {
	for i := range d {
		d[i] |= o[i]
	}
}

// pop removes and returns the lowest dirty unit, or -1.
func (d *dirtySet) pop() int {
	for i := range d {
		if w := d[i]; w != 0 {
			d[i] = w & (w - 1)
			return i<<6 + bits.TrailingZeros64(w)
		}
	}
	return -1
}

func (v *variant) newBoard() *gboard {
	g := &gboard{free: int16(v.ncells)}
	for i := range g.grid {
		g.grid[i] = empty
	}
	for u := 0; u < v.nunits; u++ {
		g.dirty.set(u)
	}
	return g
}

// cand masks with allow[k] rather than the constant all: for every variant
// but the EUC penta-hexadoku that array holds all, so the per-cell rule is
// one load in place of one immediate and costs nothing else.
func (v *variant) cand(g *gboard, k int) uint16 {
	u := &v.cellUnits[k]
	return ^(g.placed[u[0]] | g.placed[u[1]] | g.placed[u[2]] |
		g.placed[u[3]] | g.placed[u[4]] | g.placed[u[5]]) & v.allow[k]
}

func (v *variant) assign(g *gboard, k int, val uint8) {
	m := uint16(1) << val
	g.grid[k] = val
	u := &v.cellUnits[k]
	g.placed[u[0]] |= m
	g.placed[u[1]] |= m
	g.placed[u[2]] |= m
	g.placed[u[3]] |= m
	g.placed[u[4]] |= m
	g.placed[u[5]] |= m
	g.free--
	g.dirty.or(&v.touch[k])
}

// propagate runs naked and hidden singles to a fixpoint, looking only at
// units something has happened to.
//
// Both rules are monotone - a candidate, once gone, never returns - so
// their least fixpoint is unique and the order of applying them cannot
// matter. What matters is not to miss one, and that is what the dirty set
// guarantees: a unit's picture can only change through an assignment to
// a cell that shares a unit with one of its cells, and assign marks
// exactly those units. A unit that is not dirty has not changed since it
// was last examined and cannot hold a single. So every board that leaves
// here is the same board the full sweep would have produced - the test
// in propagate_test.go holds the two against each other, branch node for
// branch node, over every puzzle in the archive.
//
// Naked singles are found on the way: every cell lies in a unit, and the
// unit pass computes each empty cell's candidates anyway, so the separate
// sweep over all cells the old version began with is gone as well.
//
// For the 2011 hexamurai an assignment touches some 48 of the 176 units -
// the ones of its own grid and the shared strip - where the sweep would
// have visited all 176 plus 768 cells, several times per node. Measured,
// best of three: its uniqueness proof fell from 22.3 s to 10.4 s, and with
// the proof no longer preceded by a separate solve (see count) a
// hexadoku.exe -unique on it went from 46-59 s to 10 s. The alfadoku, one
// grid where nearly every unit meets every other, gains a third.
func (v *variant) propagate(g *gboard) bool {
	for {
		u := g.dirty.pop()
		if u < 0 {
			return true
		}
		cells := v.unitCells[u][:v.nvals]
		var once, twice uint16
		for _, k := range cells {
			if g.grid[k] != empty {
				continue
			}
			m := v.cand(g, int(k))
			if m == 0 {
				return false
			}
			if m&(m-1) == 0 { // naked single
				v.assign(g, int(k), uint8(bits.TrailingZeros16(m)))
				continue
			}
			twice |= once & m
			once |= m
		}
		if once|g.placed[u] != v.all {
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
			v.assign(g, int(k), uint8(bits.TrailingZeros16(m)))
			unique &^= m
		}
	}
}

func (v *variant) mrv(g *gboard) int {
	best, bestCnt := -1, v.nvals+1
	for k := 0; k < v.ncells; k++ {
		if g.grid[k] != empty {
			continue
		}
		if c := bits.OnesCount16(v.cand(g, k)); c < bestCnt {
			best, bestCnt = k, c
			if c == 2 {
				break
			}
		}
	}
	return best
}

func (v *variant) solve(g *gboard) bool {
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
		val := uint8(bits.TrailingZeros16(m))
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

// Search budget for count. Proving that a samurai has *two* solutions can
// take far longer than solving it, which matters when a generator runs one
// such proof per cell; -1 means unlimited.
var (
	budgetLeft int64 = -1
	budgetHit  bool
)

// countBudget counts solutions but gives up after budget branch nodes.
// The second result says whether the count is complete.
func (v *variant) countBudget(g *gboard, limit int, budget int64, first *gboard) (int, bool) {
	budgetLeft, budgetHit = budget, false
	n := v.count(g, limit, first)
	hit := budgetHit
	budgetLeft, budgetHit = -1, false
	return n, !hit
}

// count counts solutions up to limit. The first one found is left in
// first, if that is not nil: a uniqueness proof then yields the solution
// as a by-product, and the runner need not search the tree twice - for
// the 2011 hexamurai, whose first solution comes only after most of the
// tree, that is the difference between 22 and 44 seconds.
func (v *variant) count(g *gboard, limit int, first *gboard) int {
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
		if budgetLeft == 0 {
			budgetHit = true
			break
		}
		if budgetLeft > 0 {
			budgetLeft--
		}
		val := uint8(bits.TrailingZeros16(m))
		m &= m - 1
		nodes++
		v.assign(g, k, val)
		n += v.count(g, limit-n, first)
		*g = save
		if budgetHit {
			break
		}
	}
	return n
}

func (v *variant) fillRand(g *gboard, rng *rand.Rand) bool {
	if !v.propagate(g) {
		return false
	}
	if g.free == 0 {
		return true
	}
	k := v.mrv(g)
	m := v.cand(g, k)
	var vals [maxSize]uint8
	n := 0
	for m != 0 {
		vals[n] = uint8(bits.TrailingZeros16(m))
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
