// Specialized solver cores for 16x16 (hexadoku) and 9x9 (sudoku).
//
// The hot routines exist twice on purpose: with the grid size as a
// compile-time constant the compiler can use shifts/masks (16x16:
// row = k>>4, col = k&15), strength-reduce the constant division
// (9x9: k/9 becomes a multiply) and eliminate bounds checks, which a
// single size-generic implementation would prevent.
package main

import (
	"math/bits"
	"math/rand/v2"
)

const (
	size16  = 16
	cells16 = 256
	mask16  = 0xFFFF

	size9  = 9
	cells9 = 81
	mask9  = 0x1FF
)

var (
	box16   [cells16]uint8
	units16 [3 * size16][size16]uint8
	box9    [cells9]uint8
	units9  [3 * size9][size9]uint8
)

func init() {
	for r := 0; r < size16; r++ {
		for c := 0; c < size16; c++ {
			k := r*size16 + c
			box16[k] = uint8((r/4)*4 + c/4)
			units16[r][c] = uint8(k)
			units16[size16+c][r] = uint8(k)
			units16[2*size16+box16[k]][(r%4)*4+c%4] = uint8(k)
		}
	}
	for r := 0; r < size9; r++ {
		for c := 0; c < size9; c++ {
			k := r*size9 + c
			box9[k] = uint8((r/3)*3 + c/3)
			units9[r][c] = uint8(k)
			units9[size9+c][r] = uint8(k)
			units9[2*size9+box9[k]][(r%3)*3+c%3] = uint8(k)
		}
	}
}

// dispatchers (cold paths like parsing use these; the recursive cores
// below call their specialized versions directly)

func (b *board) cand(k int) uint16 {
	if size == size16 {
		return b.cand16(k)
	}
	return b.cand9(k)
}

func (b *board) assign(k int, v uint8) {
	if size == size16 {
		b.assign16(k, v)
	} else {
		b.assign9(k, v)
	}
}

func solve(b *board) bool {
	if size == size16 {
		return solve16(b)
	}
	return solve9(b)
}

// countSolutions counts solutions up to limit. The first one found is
// left in first, if that is not nil - a uniqueness proof yields the
// solution on its way, so nobody has to search the tree twice.
func countSolutions(b *board, limit int, first *board) int {
	if size == size16 {
		return count16(b, limit, first)
	}
	return count9(b, limit, first)
}

func fillRand(b *board, rng *rand.Rand) bool {
	if size == size16 {
		return fillRand16(b, rng)
	}
	return fillRand9(b, rng)
}

// ---------------- 16x16 core ----------------

func (b *board) cand16(k int) uint16 {
	return ^(b.rows[k>>4] | b.cols[k&15] | b.boxes[box16[k]]) & mask16
}

func (b *board) assign16(k int, v uint8) {
	m := uint16(1) << v
	b.grid[k] = v
	b.rows[k>>4] |= m
	b.cols[k&15] |= m
	b.boxes[box16[k]] |= m
	b.free--
}

func (b *board) unitPlaced16(u int) uint16 {
	switch {
	case u < size16:
		return b.rows[u]
	case u < 2*size16:
		return b.cols[u-size16]
	default:
		return b.boxes[u-2*size16]
	}
}

func (b *board) propagate16() bool {
	for {
		changed := false
		for k := 0; k < cells16; k++ {
			if b.grid[k] != empty {
				continue
			}
			m := b.cand16(k)
			if m == 0 {
				return false
			}
			if m&(m-1) == 0 {
				b.assign16(k, uint8(bits.TrailingZeros16(m)))
				changed = true
			}
		}
		for u := 0; u < 3*size16; u++ {
			var once, twice uint16
			for _, k := range units16[u] {
				if b.grid[k] == empty {
					m := b.cand16(int(k))
					twice |= once & m
					once |= m
				}
			}
			if once|b.unitPlaced16(u) != mask16 {
				return false
			}
			unique := once &^ twice
			if unique == 0 {
				continue
			}
			for _, k := range units16[u] {
				if b.grid[k] != empty {
					continue
				}
				m := b.cand16(int(k)) & unique
				if m == 0 {
					continue
				}
				if m&(m-1) != 0 {
					return false
				}
				b.assign16(int(k), uint8(bits.TrailingZeros16(m)))
				changed = true
				unique &^= m
			}
		}
		if !changed {
			return true
		}
	}
}

func (b *board) mrv16() int {
	best, bestCnt := -1, size16+1
	for k := 0; k < cells16; k++ {
		if b.grid[k] != empty {
			continue
		}
		if c := bits.OnesCount16(b.cand16(k)); c < bestCnt {
			best, bestCnt = k, c
			if c == 2 {
				break
			}
		}
	}
	return best
}

func solve16(b *board) bool {
	if !b.propagate16() {
		return false
	}
	if b.free == 0 {
		return true
	}
	k := b.mrv16()
	m := b.cand16(k)
	save := *b
	for m != 0 {
		v := uint8(bits.TrailingZeros16(m))
		m &= m - 1
		nodes++
		b.assign16(k, v)
		if solve16(b) {
			return true
		}
		*b = save
	}
	return false
}

func count16(b *board, limit int, first *board) int {
	if !b.propagate16() {
		return 0
	}
	if b.free == 0 {
		if first != nil && first.free != 0 {
			*first = *b
		}
		return 1
	}
	k := b.mrv16()
	m := b.cand16(k)
	save := *b
	n := 0
	for m != 0 && n < limit {
		v := uint8(bits.TrailingZeros16(m))
		m &= m - 1
		nodes++
		b.assign16(k, v)
		n += count16(b, limit-n, first)
		*b = save
	}
	return n
}

func fillRand16(b *board, rng *rand.Rand) bool {
	if !b.propagate16() {
		return false
	}
	if b.free == 0 {
		return true
	}
	k := b.mrv16()
	m := b.cand16(k)
	var vals [size16]uint8
	n := 0
	for m != 0 {
		vals[n] = uint8(bits.TrailingZeros16(m))
		m &= m - 1
		n++
	}
	rng.Shuffle(n, func(i, j int) { vals[i], vals[j] = vals[j], vals[i] })
	save := *b
	for i := 0; i < n; i++ {
		b.assign16(k, vals[i])
		if fillRand16(b, rng) {
			return true
		}
		*b = save
	}
	return false
}

// ---------------- 9x9 core ----------------

func (b *board) cand9(k int) uint16 {
	return ^(b.rows[k/size9] | b.cols[k%size9] | b.boxes[box9[k]]) & mask9
}

func (b *board) assign9(k int, v uint8) {
	m := uint16(1) << v
	b.grid[k] = v
	b.rows[k/size9] |= m
	b.cols[k%size9] |= m
	b.boxes[box9[k]] |= m
	b.free--
}

func (b *board) unitPlaced9(u int) uint16 {
	switch {
	case u < size9:
		return b.rows[u]
	case u < 2*size9:
		return b.cols[u-size9]
	default:
		return b.boxes[u-2*size9]
	}
}

func (b *board) propagate9() bool {
	for {
		changed := false
		for k := 0; k < cells9; k++ {
			if b.grid[k] != empty {
				continue
			}
			m := b.cand9(k)
			if m == 0 {
				return false
			}
			if m&(m-1) == 0 {
				b.assign9(k, uint8(bits.TrailingZeros16(m)))
				changed = true
			}
		}
		for u := 0; u < 3*size9; u++ {
			var once, twice uint16
			for _, k := range units9[u] {
				if b.grid[k] == empty {
					m := b.cand9(int(k))
					twice |= once & m
					once |= m
				}
			}
			if once|b.unitPlaced9(u) != mask9 {
				return false
			}
			unique := once &^ twice
			if unique == 0 {
				continue
			}
			for _, k := range units9[u] {
				if b.grid[k] != empty {
					continue
				}
				m := b.cand9(int(k)) & unique
				if m == 0 {
					continue
				}
				if m&(m-1) != 0 {
					return false
				}
				b.assign9(int(k), uint8(bits.TrailingZeros16(m)))
				changed = true
				unique &^= m
			}
		}
		if !changed {
			return true
		}
	}
}

func (b *board) mrv9() int {
	best, bestCnt := -1, size9+1
	for k := 0; k < cells9; k++ {
		if b.grid[k] != empty {
			continue
		}
		if c := bits.OnesCount16(b.cand9(k)); c < bestCnt {
			best, bestCnt = k, c
			if c == 2 {
				break
			}
		}
	}
	return best
}

func solve9(b *board) bool {
	if !b.propagate9() {
		return false
	}
	if b.free == 0 {
		return true
	}
	k := b.mrv9()
	m := b.cand9(k)
	save := *b
	for m != 0 {
		v := uint8(bits.TrailingZeros16(m))
		m &= m - 1
		nodes++
		b.assign9(k, v)
		if solve9(b) {
			return true
		}
		*b = save
	}
	return false
}

func count9(b *board, limit int, first *board) int {
	if !b.propagate9() {
		return 0
	}
	if b.free == 0 {
		if first != nil && first.free != 0 {
			*first = *b
		}
		return 1
	}
	k := b.mrv9()
	m := b.cand9(k)
	save := *b
	n := 0
	for m != 0 && n < limit {
		v := uint8(bits.TrailingZeros16(m))
		m &= m - 1
		nodes++
		b.assign9(k, v)
		n += count9(b, limit-n, first)
		*b = save
	}
	return n
}

func fillRand9(b *board, rng *rand.Rand) bool {
	if !b.propagate9() {
		return false
	}
	if b.free == 0 {
		return true
	}
	k := b.mrv9()
	m := b.cand9(k)
	var vals [size9]uint8
	n := 0
	for m != 0 {
		vals[n] = uint8(bits.TrailingZeros16(m))
		m &= m - 1
		n++
	}
	rng.Shuffle(n, func(i, j int) { vals[i], vals[j] = vals[j], vals[i] })
	save := *b
	for i := 0; i < n; i++ {
		b.assign9(k, vals[i])
		if fillRand9(b, rng) {
			return true
		}
		*b = save
	}
	return false
}
