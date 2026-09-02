// Samurai layouts: five grids arranged in a cross, each corner grid
// sharing one box with the grid in the middle.
//
//	+--------+     +--------+       origins are multiples of n-b, so all
//	|  TL    |     |   TR   |       five grids sit on one common box
//	|      +-+-----+-+      |       lattice; the shaded 4x4 (3x3) blocks
//	+------+-|     |-+------+       belong to two grids at once
//	       | |  C  | |
//	+------+-|     |-+------+
//	|      +-+-----+-+      |
//	|  BL    |     |   BR   |
//	+--------+     +--------+
//
// With 16 values this is Elektor's Hexamurai (5*256 - 4*16 = 1216 cells,
// 236 units); with 9 values the classic samurai sudoku (369 cells).
// Elektor states explicitly that the five grids cannot be solved
// separately - the middle grid on its own is ambiguous, and only the
// clues reaching it through the shared boxes pin it down. That is exactly
// what the general core does: a shared cell lies in the units of both
// grids, so propagation crosses the seam by itself.
package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"
)

// hexamuraiVariant is the layout Elektor actually printed in the double
// issues 7-8/2009 and 7-8/2011: not a symmetric cross but a pinwheel, and
// each outer grid shares a whole 4x12 strip - three boxes - with the one
// in the middle, not a single box. 5*256 - 4*48 = 1088 cells in 228 units.
// The grid corners below were read off the clue positions of the printed
// puzzle; the layout is confirmed by the puzzles solving uniquely.
//
//	         cols 0    8   12  16      24      39
//	row  0        +----A----+
//	row  8        |    |    +---B---+
//	row 12        | +--+----+       |
//	row 16   +----+-|  D    |       |
//	row 24   |  C   +-+--+--+---+---+
//	row 32   +------+ |  E      |
//	row 39            +---------+
func hexamuraiVariant() *variant {
	v := newVariant("hexamurai (pinwheel)", maxSize, 40, 40)
	v.layout(pinwheelOrigins, maxSize, 4)
	return v
}

// pinwheelOrigins are the five grid corners of that arrangement. The EUC
// penta-hexadoku of 7-8/2012 uses the very same ones - measured off its
// page, not assumed - so the two puzzles differ only in the extra rule.
var pinwheelOrigins = [][2]int{{0, 8}, {8, 24}, {16, 0}, {12, 12}, {24, 16}}

// Value parities as bitmasks: bit v is set for value v, and "even" means
// the hexadecimal digit is even (0, 2, 4, 6, 8, A, C, E).
const (
	evenValues = 0x5555
	oddValues  = 0xAAAA
)

// pentaVariant is the "EUC Penta-Hexadoku" of the double issue 7-8/2012,
// again by Claude Ghyselen: the pinwheel of five grids from 2009, drawn
// on a chequerboard, plus one rule on top of the hexadoku ones - an even
// value may only sit on a coloured cell, an odd one only on a white one.
// Or, as the article puts it, no two neighbouring cells may hold values
// of the same parity, which is what the colouring enforces; hence EUC,
// for even/uneven chequerboard.
//
// The colouring runs across the whole 40x40 raster and all five origins
// are even, so it is consistent over the overlaps: a cell is coloured
// exactly when row+column is even, and the sample box the magazine fills
// in "to illustrate the arrangement of the even/uneven chequerboard"
// agrees with that in all sixteen cells.
//
// The rule halves the candidates of every cell before any propagation,
// which is why this layout solves far faster than the plain hexamurai on
// the same geometry despite having fewer clues.
func pentaVariant() *variant {
	v := newVariant("EUC penta-hexadoku", maxSize, 40, 40)
	v.layout(pinwheelOrigins, maxSize, 4)
	for k := 0; k < v.ncells; k++ {
		p := int(v.pos[k])
		if (p/v.width+p%v.width)%2 == 0 {
			v.allow[k] = evenValues
		} else {
			v.allow[k] = oddValues
		}
	}
	return v
}

// hexamuraiPlusVariant is the arrangement of 7-8/2011: a plus, with the
// middle grid overlapping each of the four outer ones by a whole 8x16
// half. 5*256 - 4*128 = 768 cells. The middle grid adds no constraint of
// its own - each of its rows, columns and boxes is already a unit of an
// outer grid - which the unit table shows by itself: declaring five grids
// yields 176 distinct units, exactly what the four outer ones contribute.
func hexamuraiPlusVariant() *variant {
	v := newVariant("hexamurai (plus)", maxSize, 32, 32)
	v.layout([][2]int{{0, 8}, {8, 0}, {8, 8}, {8, 16}, {16, 8}}, maxSize, 4)
	return v
}

// samuraiVariant builds the classic samurai cross, where each corner grid
// shares exactly one box with the middle one: n=9 is the usual samurai
// sudoku, n=16 its 16-valued counterpart.
func samuraiVariant(n int) *variant {
	b, name := 3, "samurai"
	if n == maxSize {
		b, name = 4, "hexa-samurai"
	}
	d := n - b // grid origins step by one grid minus the shared box
	span := 2*d + n
	origins := [][2]int{{0, 0}, {0, 2 * d}, {d, d}, {2 * d, 0}, {2 * d, 2 * d}}
	v := newVariant(name, n, span, span)
	v.layout(origins, n, b)
	return v
}

// gridVariant describes a single nxn grid. The specialized cores in
// solver.go handle these much faster and are what the CLI uses; this
// exists so the general core can be cross-checked against them.
func gridVariant(n int) *variant {
	b, name := 3, "sudoku"
	if n == maxSize {
		b, name = 4, "hexadoku"
	}
	v := newVariant(name, n, n, n)
	v.layout([][2]int{{0, 0}}, n, b)
	v.tokens = true // a plain grid may be written with separators
	return v
}

// namedVariants are the layouts a puzzle file can ask for by name in a
// "# variant: ..." header line. That is needed where the cell count is
// not decisive: the EUC penta-hexadoku sits on exactly the same pinwheel
// as the 2009 hexamurai and only the extra rule tells them apart.
var namedVariants = map[string]func() *variant{
	"hexamurai":          hexamuraiVariant,
	"hexamurai-plus":     hexamuraiPlusVariant,
	"samurai":            func() *variant { return samuraiVariant(9) },
	"hexa-samurai":       func() *variant { return samuraiVariant(maxSize) },
	"penta":              pentaVariant,
	"penta-hexadoku":     pentaVariant,
	"euc-penta-hexadoku": pentaVariant,
	"hexadocube":         hexadocubeVariant,
	// "digest" is handled in variantForText: it is built from the header
}

// variantFor returns the layout whose cell count matches n, or nil. The
// count is of characters on the raster, so the hexadocube counts its
// shared cells once per face, as the development draws them.
func variantFor(ncells int) *variant {
	switch ncells {
	case 5*maxSize*maxSize - 4*128: // 768, Elektor's plus
		return hexamuraiPlusVariant()
	case 5*maxSize*maxSize - 4*3*4*4: // 1088, Elektor's pinwheel
		return hexamuraiVariant()
	case 5*maxSize*maxSize - 4*4*4: // 1216, symmetric cross
		return samuraiVariant(maxSize)
	case 6 * maxSize * maxSize: // 1536 drawn cells, 1352 on the cube
		return hexadocubeVariant()
	case 5*9*9 - 4*3*3: // 369
		return samuraiVariant(9)
	}
	return nil
}

// variantForText picks the layout for a puzzle file: an explicit
// "# variant: name" header wins, otherwise the cell count decides.
func variantForText(text string) *variant {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		_, rest, ok := strings.Cut(line, "variant:")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(fields[0])
		// the digest needs its blocks and codes out of the same header,
		// so it is built from the text rather than from a bare name
		if name == "digest" || name == "hexadoku-digest" {
			return digestVariant(text)
		}
		if build, ok := namedVariants[name]; ok {
			return build()
		}
	}
	return variantFor(countAllTokens(text))
}

// countAllTokens counts the puzzle cells in a whole text.
func countAllTokens(text string) int {
	n := 0
	for _, l := range strings.Split(stripComments(text), "\n") {
		n += countTokens(l)
	}
	return n
}

func runVariant(name string, v *variant, text string, bench int, checkUnique bool) bool {
	g, err := v.parse(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return false
	}
	if !compact {
		fmt.Printf("=== %s (%s, %d cells, %d clues)\n%s\n",
			name, v.name, v.ncells, v.ncells-int(g.free), v.render(g))
	}

	nodes = 0
	work := *g
	start := time.Now()
	ok := v.solveWith(&work)
	elapsed := time.Since(start)

	if !ok {
		fmt.Printf("no solution (%v, %d branch nodes)\n\n", elapsed, nodes)
		return false
	}
	fmt.Print(v.render(&work))
	if compact {
		return true
	}
	fmt.Printf("solved in %v, %d branch nodes\n", elapsed, nodes)
	if len(v.blocks) > 0 {
		fmt.Println("blocks and the codes they took:")
		for b, c := range v.assignmentOf(&work) {
			k := v.blocks[b][0]
			name := "?"
			if c >= 0 {
				name = v.codeString(c)
			}
			fmt.Printf("  r%dc%d-c%d  %s\n", k/v.width+1, k%v.width+1,
				k%v.width+len(v.blocks[b]), name)
		}
	}

	if checkUnique {
		check := *g
		if n := v.countWith(&check, 2); n > 1 {
			fmt.Println("warning: solution is not unique")
		} else {
			fmt.Println("solution is unique")
		}
	}

	if bench > 0 {
		start = time.Now()
		for i := 0; i < bench; i++ {
			work = *g
			v.solveWith(&work)
		}
		elapsed = time.Since(start)
		fmt.Printf("bench: %d runs, %v total, %v per solve\n", bench, elapsed, elapsed/time.Duration(bench))
	}
	fmt.Println()
	return true
}

// budget per uniqueness proof while generating. Deciding that a samurai
// has a second solution can take far longer than solving it; a clue whose
// removal cannot be decided within the budget is kept, so the result stays
// a correct puzzle and is minimal up to that budget.
const genBudget = 20000

// generateVariant creates a minimal samurai puzzle the same way generate()
// does for a single grid: fill at random, then drop clues in random order
// as long as the solution stays unique.
func generateVariant(v *variant, rng *rand.Rand) (*gboard, uint64) {
	full := v.newBoard()
	v.fillRand(full, rng)

	keep := make([]bool, v.ncells)
	for k := range keep {
		keep[k] = true
	}
	build := func() *gboard {
		g := v.newBoard()
		for k := 0; k < v.ncells; k++ {
			if keep[k] {
				v.assign(g, k, full.grid[k])
			}
		}
		return g
	}
	start := time.Now()
	for i, k := range rng.Perm(v.ncells) {
		keep[k] = false
		if n, done := v.countBudget(build(), 2, genBudget); !done || n != 1 {
			keep[k] = true
		}
		if (i+1)%64 == 0 {
			left := 0
			for _, b := range keep {
				if b {
					left++
				}
			}
			fmt.Fprintf(os.Stderr, "\r%s: %d/%d cells tried, %d clues left, %v elapsed   ",
				v.name, i+1, v.ncells, left, time.Since(start).Round(time.Second))
		}
	}
	fmt.Fprintln(os.Stderr)

	pz := build()
	// The removal loop keeps uniqueness as an invariant, but a budget was
	// involved, so verify it once without one.
	check := *pz
	if n := v.count(&check, 2); n != 1 {
		fmt.Fprintf(os.Stderr, "%s: generated puzzle has %d solutions\n", v.name, n)
	}
	nodes = 0
	work := *pz
	v.solve(&work)
	return pz, nodes
}
