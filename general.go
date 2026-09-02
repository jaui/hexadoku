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
}

func (v *variant) newBoard() *gboard {
	g := &gboard{free: int16(v.ncells)}
	for i := range g.grid {
		g.grid[i] = empty
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
}

func (v *variant) propagate(g *gboard) bool {
	for {
		changed := false
		for k := 0; k < v.ncells; k++ {
			if g.grid[k] != empty {
				continue
			}
			m := v.cand(g, k)
			if m == 0 {
				return false
			}
			if m&(m-1) == 0 { // naked single
				v.assign(g, k, uint8(bits.TrailingZeros16(m)))
				changed = true
			}
		}
		for u := 0; u < v.nunits; u++ {
			cells := v.unitCells[u][:v.nvals]
			var once, twice uint16
			for _, k := range cells {
				if g.grid[k] == empty {
					m := v.cand(g, int(k))
					twice |= once & m
					once |= m
				}
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
				changed = true
				unique &^= m
			}
		}
		if !changed {
			return true
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
func (v *variant) countBudget(g *gboard, limit int, budget int64) (int, bool) {
	budgetLeft, budgetHit = budget, false
	n := v.count(g, limit)
	hit := budgetHit
	budgetLeft, budgetHit = -1, false
	return n, !hit
}

func (v *variant) count(g *gboard, limit int) int {
	if !v.propagate(g) {
		return 0
	}
	if g.free == 0 {
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
		v.assign(g, k, val)
		n += v.count(g, limit-n)
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
