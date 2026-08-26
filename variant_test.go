package main

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The general core and the specialized 16x16 core describe the same
// puzzle, so they must produce the same solutions. Every hexadoku in
// puzzles/ is solved twice and the results compared.
func TestGeneralCoreMatchesSpecialized(t *testing.T) {
	files, err := filepath.Glob("puzzles/elektor/20??-*.txt")
	if err != nil || len(files) == 0 {
		t.Skip("no elektor puzzles available")
	}
	// only the reconstructed puzzles, not the raw OCR / mask side files
	name := regexp.MustCompile(`^\d{4}-\d\d(_\d\d)?\.txt$`)
	v := gridVariant(16)
	checked := 0
	for _, f := range files {
		if !name.MatchString(filepath.Base(f)) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if countAllTokens(text) != 256 {
			continue // marker file, or one of the samurai layouts
		}
		if strings.Contains(text, "NOT repairable") {
			// the pipeline itself flags this reconstruction as broken;
			// it is kept as raw material for a visual reading, not as a
			// puzzle. Core equivalence is not what it would test.
			continue
		}
		b, err := parse(text)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		g, err := v.parse(text)
		if err != nil {
			t.Fatalf("%s: general parse: %v", f, err)
		}
		work := *b
		if !solve(&work) {
			t.Fatalf("%s: specialized core found no solution", f)
		}
		if !v.solve(g) {
			t.Fatalf("%s: general core found no solution", f)
		}
		checkFull(t, v, g)
		for k := 0; k < 256; k++ {
			if work.grid[k] != g.grid[k] {
				t.Fatalf("%s: cores disagree at cell %d", f, k)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no hexadoku puzzles found")
	}
	t.Logf("%d puzzles solved identically by both cores", checked)
}

func benchCores(b *testing.B, text string) {
	pz, err := parse(text)
	if err != nil {
		b.Fatal(err)
	}
	v := gridVariant(16)
	gpz, err := v.parse(text)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("specialized", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			work := *pz
			solve(&work)
		}
	})
	b.Run("general", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			work := *gpz
			v.solve(&work)
		}
	})
}

func BenchmarkHexadokuCores(b *testing.B) { benchCores(b, defaultPuzzle) }

func BenchmarkHexamurai(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("puzzles", "elektor", "2009-07_08.txt"))
	if err != nil {
		b.Skip("no hexamurai puzzle available")
	}
	v := variantFor(countAllTokens(string(data)))
	if v == nil {
		b.Fatal("no layout matches the puzzle")
	}
	pz, err := v.parse(string(data))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		work := *pz
		v.solve(&work)
	}
}

func TestSamuraiGeometry(t *testing.T) {
	for _, tc := range []struct {
		v            *variant
		b            int
		cells, units int
		shared       int // cells belonging to more than one grid
	}{
		// classic cross: each corner grid shares one box with the middle
		{samuraiVariant(9), 3, 5*81 - 4*9, 5*27 - 4, 4 * 9},
		{samuraiVariant(16), 4, 5*256 - 4*16, 5*48 - 4, 4 * 16},
		// Elektor 7-8/2009: pinwheel, a 4x12 strip (three boxes) shared
		{hexamuraiVariant(), 4, 5*256 - 4*48, 5*48 - 4*3, 4 * 48},
		// Elektor 7-8/2011: plus, the middle grid shares an 8x16 half with
		// each outer grid. Its rows, columns and boxes then coincide with
		// units of the outer grids, so it adds no unit of its own: 176 is
		// exactly what the four outer grids contribute.
		{hexamuraiPlusVariant(), 4, 5*256 - 4*128, 4*48 - 4*4, 256},
	} {
		v := tc.v
		if v.ncells != tc.cells || v.nunits != tc.units {
			t.Fatalf("%s: %d cells / %d units, want %d / %d", v.name, v.ncells, v.nunits, tc.cells, tc.units)
		}
		// every unit holds nvals distinct cells
		for u := 0; u < v.nunits; u++ {
			seen := map[uint16]bool{}
			for _, k := range v.unitCells[u][:v.nvals] {
				if seen[k] {
					t.Fatalf("%s: unit %d contains cell %d twice", v.name, u, k)
				}
				seen[k] = true
			}
		}
		// a plain cell lies in row, column and box; a cell in an overlap
		// lies in two rows, two columns and the one shared box
		shared := 0
		for k := 0; k < v.ncells; k++ {
			switch v.ncu[k] {
			case 3:
			case 5:
				shared++
			default:
				t.Fatalf("%s: cell %d lies in %d units", v.name, k, v.ncu[k])
			}
		}
		if shared != tc.shared {
			t.Fatalf("%s: %d shared cells, want %d", v.name, shared, tc.shared)
		}
	}
}

// checkFull verifies that every unit of a completed board is a permutation
// of all values.
func checkFull(t *testing.T, v *variant, g *gboard) {
	t.Helper()
	if g.free != 0 {
		t.Fatalf("%s: %d cells left empty", v.name, g.free)
	}
	for u := 0; u < v.nunits; u++ {
		var m uint16
		for _, k := range v.unitCells[u][:v.nvals] {
			b := uint16(1) << g.grid[k]
			if m&b != 0 {
				t.Fatalf("%s: unit %d contains value %c twice", v.name, u, charOf(v.nvals, g.grid[k]))
			}
			m |= b
		}
		if m != v.all {
			t.Fatalf("%s: unit %d is not complete (%016b)", v.name, u, m)
		}
	}
}

func TestSamuraiSolve(t *testing.T) {
	for _, n := range []int{9, 16} {
		v := samuraiVariant(n)
		rng := rand.New(rand.NewPCG(42, 0x9e3779b97f4a7c15))
		full := v.newBoard()
		if !v.fillRand(full, rng) {
			t.Fatalf("%s: could not fill a grid at random", v.name)
		}
		checkFull(t, v, full)

		// drop two thirds of the clues, solve what is left
		pz := v.newBoard()
		for k := 0; k < v.ncells; k++ {
			if rng.IntN(3) == 0 {
				v.assign(pz, k, full.grid[k])
			}
		}
		work := *pz
		if !v.solve(&work) {
			t.Fatalf("%s: no solution for a grid built from a valid one", v.name)
		}
		checkFull(t, v, &work)

		// the rendered puzzle must parse back into the same board
		back, err := v.parse(v.render(pz))
		if err != nil {
			t.Fatalf("%s: render/parse round trip: %v", v.name, err)
		}
		if *back != *pz {
			t.Fatalf("%s: render/parse round trip changed the board", v.name)
		}
	}
}

// The two Hexamurai puzzles Elektor actually printed, read out of the PDF
// text layer by cmd/muraiextract. Both must be uniquely solvable - that is
// what proves the arrangement was read correctly, since a wrong one would
// place clues into conflict or leave the puzzle ambiguous.
func TestElektorHexamurai(t *testing.T) {
	for _, tc := range []struct {
		file  string
		cells int
		clues int
		slow  bool
	}{
		{"puzzles/elektor/2009-07_08.txt", 1088, 409, false},
		{"puzzles/elektor/2011-07_08.txt", 768, 231, true}, // ~3.4M branch nodes
	} {
		data, err := os.ReadFile(tc.file)
		if err != nil {
			t.Skipf("%s not available", tc.file)
		}
		v := variantFor(countAllTokens(string(data)))
		if v == nil || v.ncells != tc.cells {
			t.Fatalf("%s: no layout matches the puzzle", tc.file)
		}
		g, err := v.parse(string(data))
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if clues := v.ncells - int(g.free); clues != tc.clues {
			t.Fatalf("%s: %d clues, want %d", tc.file, clues, tc.clues)
		}
		if tc.slow && testing.Short() {
			continue
		}
		work := *g
		if !v.solve(&work) {
			t.Fatalf("%s: no solution", tc.file)
		}
		checkFull(t, v, &work)
		work = *g
		if n := v.count(&work, 2); n != 1 {
			t.Fatalf("%s: %d solutions, want exactly one", tc.file, n)
		}
	}
}

// A generated puzzle must be uniquely solvable. The generator decides
// removals under a search budget, so this checks the result without one.
func TestGenerateSamurai(t *testing.T) {
	if testing.Short() {
		t.Skip("generation takes a few seconds")
	}
	v := samuraiVariant(9)
	rng := rand.New(rand.NewPCG(11, 0x9e3779b97f4a7c15))
	pz, _ := generateVariant(v, rng)
	work := *pz
	if n := v.count(&work, 2); n != 1 {
		t.Fatalf("generated samurai has %d solutions, want exactly one", n)
	}
	work = *pz
	if !v.solve(&work) {
		t.Fatal("generated samurai has no solution")
	}
	checkFull(t, v, &work)
}

// Elektor's rules say the five grids of a hexamurai cannot be solved
// separately. This makes that concrete: a hexamurai whose solution is
// unique stays ambiguous when its middle grid is cut out and solved as a
// plain hexadoku - the missing information travels through the four
// shared boxes, which only the coupled units carry.
func TestSamuraiCouplesGrids(t *testing.T) {
	v := samuraiVariant(16)
	rng := rand.New(rand.NewPCG(7, 0x9e3779b97f4a7c15))
	full := v.newBoard()
	if !v.fillRand(full, rng) {
		t.Fatal("could not fill a grid at random")
	}
	// middle grid coordinates on the raster
	midCell := func(k int) bool {
		p := int(v.pos[k])
		r, c := p/v.width, p%v.width
		return r >= 12 && r < 28 && c >= 12 && c < 28
	}

	// Clue the middle grid sparsely and the four outer grids generously,
	// then keep adding clues *outside* the middle until the layout as a
	// whole is unique. The middle therefore never gets enough clues of
	// its own to be solvable on its own.
	pz := v.newBoard()
	var outer, inner []int
	for k := 0; k < v.ncells; k++ {
		p := 6
		if midCell(k) {
			p, inner = 1, append(inner, k) // about 10% inside the middle grid
		} else {
			outer = append(outer, k)
		}
		if rng.IntN(10) < p {
			v.assign(pz, k, full.grid[k])
		}
	}
	shuffle := func(s []int) { rng.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] }) }
	shuffle(outer)
	shuffle(inner)
	order := append(outer, inner...)
	next := 0
	for {
		whole := *pz
		if v.count(&whole, 2) == 1 {
			break
		}
		for next < len(order) && pz.grid[order[next]] != empty {
			next++
		}
		if next == len(order) {
			t.Fatal("not unique even with every cell clued")
		}
		v.assign(pz, order[next], full.grid[order[next]])
		next++
	}

	// the same clues, but only those of the middle grid (rows/columns
	// 12..27 of the raster), solved as a stand-alone hexadoku
	initGeometry(maxSize)
	mid := newBoard()
	for k := 0; k < v.ncells; k++ {
		if pz.grid[k] == empty {
			continue
		}
		p := int(v.pos[k])
		r, c := p/v.width-12, p%v.width-12
		if r >= 0 && r < 16 && c >= 0 && c < 16 {
			mid.assign(r*16+c, pz.grid[k])
		}
	}
	if n := countSolutions(mid, 2); n < 2 {
		t.Fatalf("middle grid alone has %d solution(s); it should be ambiguous "+
			"without the constraints of the neighbouring grids", n)
	}
}
