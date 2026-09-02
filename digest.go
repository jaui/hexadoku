// Hexadoku 'Digest' (Elektor 3/2011): a plain 16x16 grid with 73 clues
// and one rule that is not a rule of sudoku at all.
//
// Sixteen marked horizontal blocks of five cells each must be filled with
// the prize codes of sixteen earlier Hexadokus - January 2009 to June
// 2010, the July-August 2009 double issue excluded, because it carried
// the Hexamurai and has no five-cell code. The article says which sixteen
// puzzles, and nothing more: which block belongs to which month is part
// of the puzzle. Without those codes the grid is deliberately ambiguous.
//
// So the extra knowledge is an unordered set of sixteen strings, and the
// extra rule is that the sixteen blocks take them one each - a bijection
// between blocks and codes on top of the hexadoku. That does not fit the
// unit abstraction the other layouts use: a unit says "these n cells hold
// every value once", and here it is five *cells at a time* that have to
// match a whole given string.
//
// It is one more layer of search around the ordinary one. Assign codes to
// blocks with the same MRV idea the cell search uses - always take the
// block with the fewest codes still fitting - and let propagation between
// two assignments do the work. A code fits a block when each of its five
// characters is still a candidate in the corresponding cell, which after
// a couple of blocks is a strong filter: the first block placed cuts the
// possibilities for every block that shares a row band or a column with
// it.
package main

import (
	"fmt"
	"math/bits"
	"strings"
)

// parseDigest reads the "# codes:" and "# block:" header lines of a
// digest puzzle. Blocks are written the way this repo writes the grey
// prize cells elsewhere, r<row>c<first>-c<last>, one-based.
func digestVariant(text string) *variant {
	v := gridVariant(maxSize)
	v.name = "hexadoku digest"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		if _, rest, ok := strings.Cut(line, "codes:"); ok {
			for _, f := range strings.Fields(rest) {
				code := make([]uint8, 0, len(f))
				for i := 0; i < len(f); i++ {
					t := valueOf(v.nvals, f[i])
					if t < 0 {
						v.builderr = fmt.Errorf("digest: %q is not a hexadecimal code", f)
						return v
					}
					code = append(code, uint8(t))
				}
				v.codes = append(v.codes, code)
			}
			continue
		}
		if _, rest, ok := strings.Cut(line, "block:"); ok {
			var row, c0, c1 int
			if _, err := fmt.Sscanf(strings.TrimSpace(rest), "r%dc%d-c%d", &row, &c0, &c1); err != nil {
				v.builderr = fmt.Errorf("digest: cannot read the block %q", strings.TrimSpace(rest))
				return v
			}
			if row < 1 || row > maxSize || c0 < 1 || c1 > maxSize || c1 < c0 {
				v.builderr = fmt.Errorf("digest: block r%dc%d-c%d is not inside the grid", row, c0, c1)
				return v
			}
			cells := make([]int, 0, c1-c0+1)
			for c := c0; c <= c1; c++ {
				cells = append(cells, (row-1)*maxSize+c-1)
			}
			v.blocks = append(v.blocks, cells)
		}
	}
	switch {
	case len(v.blocks) == 0 || len(v.codes) == 0:
		v.builderr = fmt.Errorf("digest: the header needs both \"# codes:\" and \"# block:\" lines")
	case len(v.blocks) != len(v.codes):
		v.builderr = fmt.Errorf("digest: %d blocks but %d codes", len(v.blocks), len(v.codes))
	case len(v.blocks) > 16:
		v.builderr = fmt.Errorf("digest: %d blocks, at most 16 fit in a bitmask", len(v.blocks))
	}
	for i, b := range v.blocks {
		if len(v.codes) > i && len(b) != len(v.codes[i]) {
			// only a length mismatch of the whole set matters; codes are
			// interchangeable, so compare against the first block
			if len(b) != len(v.codes[0]) {
				v.builderr = fmt.Errorf("digest: block %d holds %d cells, the codes are %d long",
					i+1, len(b), len(v.codes[0]))
			}
		}
	}
	return v
}

// codeFits reports whether every character of code c can still stand in
// the cells of block b, and place writes it. Cells already holding the
// right value are fine - propagation may have found one of them itself.
func (v *variant) codeFits(g *gboard, b, c int) bool {
	code := v.codes[c]
	for i, k := range v.blocks[b] {
		if g.grid[k] != empty {
			if g.grid[k] != code[i] {
				return false
			}
			continue
		}
		if v.cand(g, k)&(1<<code[i]) == 0 {
			return false
		}
	}
	return true
}

func (v *variant) placeCode(g *gboard, b, c int) bool {
	code := v.codes[c]
	for i, k := range v.blocks[b] {
		if g.grid[k] != empty {
			if g.grid[k] != code[i] {
				return false
			}
			continue
		}
		if v.cand(g, k)&(1<<code[i]) == 0 {
			return false
		}
		v.assign(g, k, code[i])
	}
	return true
}

// nextBlock returns the unassigned block with the fewest codes still
// fitting, together with those codes; ok is false when every block is
// assigned, and the mask is zero when some block has nothing left.
func (v *variant) nextBlock(g *gboard, done, used uint16) (b int, fits uint16, ok bool) {
	best, bestN := -1, len(v.codes)+1
	var bestFits uint16
	for i := range v.blocks {
		if done&(1<<i) != 0 {
			continue
		}
		var m uint16
		for c := range v.codes {
			if used&(1<<c) == 0 && v.codeFits(g, i, c) {
				m |= 1 << c
			}
		}
		n := bits.OnesCount16(m)
		if n == 0 {
			return i, 0, true
		}
		if n < bestN {
			best, bestN, bestFits = i, n, m
			if n == 1 {
				break
			}
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return best, bestFits, true
}

// solveAssign is solve() with the block/code layer around it: pick the
// most constrained block, try each code that still fits, and once every
// block is placed hand the rest to the ordinary search.
func (v *variant) solveAssign(g *gboard, done, used uint16) bool {
	if !v.propagate(g) {
		return false
	}
	b, fits, ok := v.nextBlock(g, done, used)
	if !ok {
		return v.solve(g)
	}
	save := *g
	for fits != 0 {
		c := bits.TrailingZeros16(fits)
		fits &= fits - 1
		nodes++
		if v.placeCode(g, b, c) && v.solveAssign(g, done|1<<b, used|1<<c) {
			return true
		}
		*g = save
	}
	return false
}

// countAssign is the same search, counting solutions up to limit.
func (v *variant) countAssign(g *gboard, limit int, done, used uint16) int {
	if !v.propagate(g) {
		return 0
	}
	b, fits, ok := v.nextBlock(g, done, used)
	if !ok {
		return v.count(g, limit)
	}
	save := *g
	n := 0
	for fits != 0 && n < limit {
		c := bits.TrailingZeros16(fits)
		fits &= fits - 1
		if v.placeCode(g, b, c) {
			n += v.countAssign(g, limit-n, done|1<<b, used|1<<c)
		}
		*g = save
	}
	return n
}

// assignmentOf reads back which code ended up in which block, for the
// report - the puzzle is as much about that bijection as about the grid.
func (v *variant) assignmentOf(g *gboard) []int {
	out := make([]int, len(v.blocks))
	for b := range v.blocks {
		out[b] = -1
		for c := range v.codes {
			hit := true
			for i, k := range v.blocks[b] {
				if g.grid[k] != v.codes[c][i] {
					hit = false
					break
				}
			}
			if hit {
				out[b] = c
				break
			}
		}
	}
	return out
}

func (v *variant) codeString(c int) string {
	var sb strings.Builder
	for _, val := range v.codes[c] {
		sb.WriteByte(charOf(v.nvals, val))
	}
	return sb.String()
}

// solveWith and countWith are what the runner calls: the block search
// where a layout has blocks, the plain one everywhere else.
func (v *variant) solveWith(g *gboard) bool {
	if len(v.blocks) == 0 {
		return v.solve(g)
	}
	return v.solveAssign(g, 0, 0)
}

func (v *variant) countWith(g *gboard, limit int) int {
	if len(v.blocks) == 0 {
		return v.count(g, limit)
	}
	return v.countAssign(g, limit, 0, 0)
}
