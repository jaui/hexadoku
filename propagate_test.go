package main

import (
	"math/bits"
	"os"
	"path/filepath"
	"testing"
)

// The propagation as it stood before it became incremental, kept as the
// oracle: naked and hidden singles are monotone, so their least fixpoint
// is unique and the order in which it is reached cannot matter. The new
// propagate must therefore land on exactly the same board at every node,
// make the same MRV choice, and so walk exactly the same tree - which the
// branch-node count states precisely. The reference sweeps every cell and
// every unit until nothing changes any more.

func (v *variant) propagateRef(g *gboard) bool {
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
			if m&(m-1) == 0 {
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
				return false
			}
			unique := once &^ twice
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
					return false
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

func (v *variant) solveRef(g *gboard) bool {
	if !v.propagateRef(g) {
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
		if v.solveRef(g) {
			return true
		}
		*g = save
	}
	return false
}

func (v *wideVariant) propagateRef(g *wboard) bool {
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
			if m&(m-1) == 0 {
				v.assign(g, k, uint8(bits.TrailingZeros64(m)))
				changed = true
			}
		}
		for u := 0; u < v.nunits; u++ {
			cells := v.unitCells[u][:v.unitSize[u]]
			var once, twice uint64
			for _, k := range cells {
				if g.grid[k] == empty {
					m := v.cand(g, int(k))
					twice |= once & m
					once |= m
				}
			}
			if once|g.placed[u] != v.unitAll[u] {
				return false
			}
			unique := once &^ twice
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
					return false
				}
				v.assign(g, int(k), uint8(bits.TrailingZeros64(m)))
				changed = true
				unique &^= m
			}
		}
		if !changed {
			return true
		}
	}
}

func (v *wideVariant) solveRef(g *wboard) bool {
	if !v.propagateRef(g) {
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
		if v.solveRef(g) {
			return true
		}
		*g = save
	}
	return false
}

// Branch-node counts of the unchanged code, recorded before propagate was
// touched. They pin the tree beyond the oracle comparison: should both
// implementations ever drift together, these would still notice.
var knownNodes = map[string]uint64{
	"2009-07_08.txt":            65,
	"2011-07_08.txt":            3364151,
	"2006-07_08.txt":            454055,
	"2007-07_08.txt":            1,
	"2008-07_08.txt":            0,
	"hexadocube_generated.txt":  10844,
	"hexasamurai_generated.txt": 18171,
	"penta_generated.txt":       1550,
	"samurai_generated.txt":     359,
}

func TestPropagateSameTree(t *testing.T) {
	files := append(elektorHexadokus(t),
		"puzzles/elektor/2009-07_08.txt",
		"puzzles/elektor/2011-07_08.txt",
		"puzzles/elektor/2011-03.txt",
		"puzzles/hexadocube_generated.txt",
		"puzzles/hexasamurai_generated.txt",
		"puzzles/penta_generated.txt",
		"puzzles/samurai_generated.txt",
		"puzzles/elektor/2006-07_08.txt",
		"puzzles/elektor/2007-07_08.txt",
		"puzzles/elektor/2008-07_08.txt",
	)
	checked := 0
	for _, f := range files {
		base := filepath.Base(f)
		if base == "2011-07_08.txt" && testing.Short() {
			continue // ~22 s per solve
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)

		var refNodes, newNodes uint64
		if wv := wideVariantForText(text); wv != nil {
			g, err := wv.parse(text)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			ref, work := *g, *g
			nodes = 0
			if !wv.solveRef(&ref) {
				t.Fatalf("%s: reference found no solution", f)
			}
			refNodes = nodes
			nodes = 0
			if !wv.solve(&work) {
				t.Fatalf("%s: no solution", f)
			}
			newNodes = nodes
			if ref.grid != work.grid {
				t.Fatalf("%s: the two propagations reach different solutions", f)
			}
		} else {
			v := variantForText(text)
			if v == nil {
				v = gridVariant(16)
			}
			g, err := v.parse(text)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			ref, work := *g, *g
			nodes = 0
			if !v.solveRef(&ref) {
				t.Fatalf("%s: reference found no solution", f)
			}
			refNodes = nodes
			nodes = 0
			if !v.solve(&work) {
				t.Fatalf("%s: no solution", f)
			}
			newNodes = nodes
			if ref.grid != work.grid {
				t.Fatalf("%s: the two propagations reach different solutions", f)
			}
		}
		if refNodes != newNodes {
			t.Fatalf("%s: %d branch nodes, the reference propagation takes %d - not the same tree",
				f, newNodes, refNodes)
		}
		if want, ok := knownNodes[base]; ok && newNodes != want {
			t.Fatalf("%s: %d branch nodes, recorded %d", f, newNodes, want)
		}
		checked++
	}
	t.Logf("%d puzzles walk the same tree under both propagations", checked)
}
