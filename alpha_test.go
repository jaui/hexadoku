package main

import (
	"math/bits"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

// The wide core is a second implementation of the same search, so the
// strongest test available is to point it at puzzles whose solution is
// already known: every reconstructed Elektor hexadoku, solved once by the
// specialized 16x16 core and once by the uint64 one over the same
// alphabet, cell by cell.
func TestWideCoreMatchesSpecialized(t *testing.T) {
	files := elektorHexadokus(t)
	v := gridWideVariant(16)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		b, err := parse(text)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		g, err := v.parse(text)
		if err != nil {
			t.Fatalf("%s: wide parse: %v", f, err)
		}
		work := *b
		if !solve(&work) {
			t.Fatalf("%s: specialized core found no solution", f)
		}
		if !v.solve(g) {
			t.Fatalf("%s: wide core found no solution", f)
		}
		checkWideFull(t, v, g)
		for k := 0; k < 256; k++ {
			if work.grid[k] != g.grid[k] {
				t.Fatalf("%s: cores disagree at cell %d", f, k)
			}
		}
	}
	if len(files) < 50 {
		t.Fatalf("only %d hexadokus cross-checked", len(files))
	}
	t.Logf("%d hexadokus solved identically by both cores", len(files))
}

func TestAlphanumskiGeometry(t *testing.T) {
	v := alphanumskiVariant()
	if v.ncells != 36*36 || v.nunits != 3*36 {
		t.Fatalf("%d cells / %d units, want 1296 / 108", v.ncells, v.nunits)
	}
	// no extra rule: every cell takes every value and sits in exactly a
	// row, a column and a box
	for k := 0; k < v.ncells; k++ {
		if v.allow[k] != v.all {
			t.Fatalf("cell %d is restricted, the alphanumski has no such rule", k)
		}
		if v.ncu[k] != 3 {
			t.Fatalf("cell %d lies in %d units, want 3", k, v.ncu[k])
		}
	}
	for u := 0; u < v.nunits; u++ {
		if v.unitSize[u] != 36 || v.unitAll[u] != v.all {
			t.Fatalf("unit %d holds %d cells over %d values, want 36 over 36",
				u, v.unitSize[u], bits.OnesCount64(v.unitAll[u]))
		}
	}
}

// The alfadoku is a plain grid over an unusual alphabet: no restricted
// cells and no extra units, which is exactly what has to be true of it.
func TestAlfadokuGeometry(t *testing.T) {
	v := alfadokuVariant()
	if v.ncells != 25*25 || v.nunits != 3*25 {
		t.Fatalf("%d cells / %d units, want 625 / 75", v.ncells, v.nunits)
	}
	if v.alphabet != "ABCDEFGHIJKLMNOPQRSTUVWXY" || strings.ContainsRune(v.alphabet, 'Z') {
		t.Fatalf("alphabet %q, want A-Y without Z", v.alphabet)
	}
	for k := 0; k < v.ncells; k++ {
		if v.allow[k] != v.all || v.ncu[k] != 3 {
			t.Fatalf("cell %d is restricted or sits in %d units", k, v.ncu[k])
		}
	}
	for u := 0; u < v.nunits; u++ {
		if v.unitSize[u] != 25 || v.unitAll[u] != v.all {
			t.Fatalf("unit %d holds %d cells, want 25 over the whole alphabet", u, v.unitSize[u])
		}
	}
}

func TestAlphasudokuGeometry(t *testing.T) {
	v := alphasudokuVariant()
	if v.ncells != 25*25 || v.nunits != 3*25+9 {
		t.Fatalf("%d cells / %d units, want 625 / 84", v.ncells, v.nunits)
	}
	shaded := 0
	for r := 0; r < v.n; r++ {
		for c := 0; c < v.n; c++ {
			k := r*v.n + c
			want, units := v.all, uint8(3)
			if alsuShaded(r) && alsuShaded(c) {
				want, units = alsuDigits, 4
				shaded++
			}
			if v.allow[k] != want {
				t.Fatalf("cell r%dc%d allows %025b, want %025b", r+1, c+1, v.allow[k], want)
			}
			if v.ncu[k] != units {
				t.Fatalf("cell r%dc%d lies in %d units, want %d", r+1, c+1, v.ncu[k], units)
			}
		}
	}
	if shaded != 81 {
		t.Fatalf("%d shaded cells, want 81", shaded)
	}

	// Each block is nine cells over the nine digits, and each ordinary
	// unit must still be satisfiable: a row of 25 needs nine cells that
	// may hold a digit, and a shaded row has exactly nine of them.
	blocks := 0
	for u := 0; u < v.nunits; u++ {
		if v.unitAll[u] == alsuDigits {
			blocks++
			if v.unitSize[u] != 9 {
				t.Fatalf("block unit %d holds %d cells, want 9", u, v.unitSize[u])
			}
			continue
		}
		digitCells := 0
		for _, k := range v.unitCells[u][:v.unitSize[u]] {
			if v.allow[k]&alsuDigits != 0 {
				digitCells++
			}
		}
		if digitCells < 9 {
			t.Fatalf("unit %d has room for only %d of the nine digits", u, digitCells)
		}
	}
	if blocks != 9 {
		t.Fatalf("%d embedded blocks, want 9", blocks)
	}
}

// A full grid to work from. fillRand delivers one for the alphanumski in
// milliseconds; for the alphasudoku it does not finish at all (see
// alpha.go), so there the printed puzzle is solved instead.
func fullWideBoard(t *testing.T, v *wideVariant) *wboard {
	t.Helper()
	if v.name == "alphasudoku" {
		data, err := os.ReadFile("puzzles/elektor/2008-07_08.txt")
		if err != nil {
			t.Skip("no alphasudoku puzzle available")
		}
		g, err := v.parse(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if !v.solve(g) {
			t.Fatal("the printed alphasudoku has no solution")
		}
		return g
	}
	g := v.newBoard()
	if !v.fillRand(g, rand.New(rand.NewPCG(7, 0x9e3779b97f4a7c15))) {
		t.Fatalf("%s: could not fill a grid at random", v.name)
	}
	return g
}

// Punch holes into a valid grid and let the solver close them again.
//
// The number of holes is deliberately modest. Completing a *randomly*
// punched grid is hardest around half filled, and on this size that is a
// wall rather than a slope: with 550 of the 1296 alphanumski cells blank
// the search closes them in 6 branch nodes, with 600 it needs 206696, and
// at 648 - exactly half - it does not return. That is a property of
// random completion at this size, not of the core: on the same 16x16
// instances this core and the one in general.go branch exactly alike, and
// a 16x16 stays easy even with 240 of its 256 cells blank. A printed
// puzzle is nothing like a random hole pattern, which is why both real
// ones fall to propagation almost without branching.
func TestWideSolveRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		v     *wideVariant
		holes int
	}{
		{alphanumskiVariant(), 400},
		{alphasudokuVariant(), 120},
	} {
		v := tc.v
		full := fullWideBoard(t, v)
		checkWideFull(t, v, full)

		rng := rand.New(rand.NewPCG(11, 0x9e3779b97f4a7c15))
		pz := v.newBoard()
		for _, k := range rng.Perm(v.ncells)[tc.holes:] {
			v.assign(pz, k, full.grid[k])
		}
		work := *pz
		if !v.solve(&work) {
			t.Fatalf("%s: no solution for a grid built from a valid one", v.name)
		}
		checkWideFull(t, v, &work)

		back, err := v.parse(v.render(pz))
		if err != nil {
			t.Fatalf("%s: render/parse round trip: %v", v.name, err)
		}
		if *back != *pz {
			t.Fatalf("%s: render/parse round trip changed the board", v.name)
		}
	}
}

// A letter in a shaded cell is not a mere conflict but a broken rule, and
// the message must name the embedded sudoku so the reading can be
// corrected - the same demand TestPentaRejectsWrongColour makes of the
// chequerboard.
func TestAlphasudokuRejectsLetterInBlock(t *testing.T) {
	v := alphasudokuVariant()
	lines := strings.Split(strings.TrimRight(v.render(v.newBoard()), "\n"), "\n")
	// row 7, column 7 (one-based) is the top left cell of the first block
	set := func(ch string) string {
		out := append([]string(nil), lines...)
		out[6] = out[6][:6] + ch + out[6][7:]
		return strings.Join(out, "\n")
	}
	if _, err := v.parse(set("A")); err == nil ||
		!strings.Contains(err.Error(), "embedded sudoku") {
		t.Fatalf("letter in a shaded cell: got %v, want an embedded-sudoku error", err)
	}
	if _, err := v.parse(set("4")); err != nil {
		t.Fatalf("digit in a shaded cell: %v", err)
	}
}

// A file must ask for these layouts by name - the cell count cannot find
// them, because countTokens does not know their alphabets.
func TestWideVariantHeaderWins(t *testing.T) {
	v := alphasudokuVariant()
	grid := v.render(v.newBoard())
	if got := wideVariantForText("# variant: alphasudoku\n" + grid); got == nil || got.name != v.name {
		t.Fatalf("header picked %v, want %s", got, v.name)
	}
	if got := wideVariantForText(grid); got != nil {
		t.Fatalf("without a header the grid must not be recognised, got %s", got.name)
	}
	if got := wideVariantForText("# variant: hexamurai\n" + grid); got != nil {
		t.Fatalf("a narrow layout must not be answered by the wide core, got %s", got.name)
	}
}

// The two real puzzles, read out of the PDF text layer, against the codes
// Elektor announced two issues later. Seven or six named cells out of
// 1296 or 625 cannot agree by chance, so the codes confirm the
// transcription, the rule model and the solver at once.
func TestElektorAlphaPuzzles(t *testing.T) {
	for _, tc := range []struct {
		file   string
		layout string
		cells  int
		clues  int
		code   string
		unique bool
	}{
		{"puzzles/elektor/2008-07_08.txt", "alphasudoku", 625, 345, "HKCEAO", true},
		// Elektuur hedged in 10/2006 that this one might have several
		// solutions, because readers' programs kept hanging on it. It has
		// exactly one; what the programs hit was its difficulty - some
		// 450000 branch nodes, by far the most in this archive.
		{"puzzles/elektor/2006-07_08.txt", "alfadoku", 625, 267, "IDRFBV", true},
		// as printed the alphanumski has two solutions; they differ in
		// four cells, none of them a grey one, so the code is still the
		// only possible answer
		{"puzzles/elektor/2007-07_08.txt", "alphanumski", 1296, 820, "LVC4ZM1", false},
	} {
		// One subtest per puzzle, run in parallel. Splitting a single
		// search across cores is not worth it here - measured on twelve
		// cores it reaches about 4.5x, but even twelve *independent*
		// proofs with no coordination at all slow each other down by a
		// factor of 2.7 to 4, so the ceiling is the memory system and not
		// the scheduling. Solving whole puzzles side by side gets the same
		// ceiling for nothing.
		t.Run(tc.layout, func(t *testing.T) {
			t.Parallel()
			testElektorAlpha(t, tc.file, tc.layout, tc.cells, tc.clues, tc.code, tc.unique)
		})
	}
}

func testElektorAlpha(t *testing.T, file, layout string, cells, clues int, code string, wantUnique bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		t.Skipf("%s not available", file)
	}
	text := string(data)
	v := wideVariantForText(text)
	if v == nil || v.name != layout {
		t.Fatalf("%s: header does not select the %s layout", file, layout)
	}
	g, err := v.parse(text)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	if v.ncells != cells || v.ncells-int(g.free) != clues {
		t.Fatalf("%s: %d cells with %d clues, want %d with %d",
			file, v.ncells, v.ncells-int(g.free), cells, clues)
	}
	work := *g
	if !v.solve(&work) {
		t.Fatalf("%s: no solution", file)
	}
	checkWideFull(t, v, &work)
	if got := v.greyString(&work); got != code {
		t.Fatalf("%s: grey cells give %s, the magazine announced %s", file, got, code)
	}
	// Proving the alfadoku unique costs a good minute - it is by far
	// the hardest puzzle here - so the proof is skipped in short mode.
	if !testing.Short() {
		check := *g
		if unique := v.count(&check, 2, nil) == 1; unique != wantUnique {
			t.Fatalf("%s: uniquely solvable = %v, want %v", file, unique, wantUnique)
		}
	}
	// A unique solution pins the grey cells trivially; the question only
	// arises where a second solution exists that could differ in them.
	if !wantUnique && !v.greyPinned(g, &work) {
		t.Fatalf("%s: another solution puts a different code in the grey cells", file)
	}
}

// checkWideFull verifies a finished board against the rules themselves:
// every cell filled, every value allowed where it stands, and every unit
// holding its whole value set.
func checkWideFull(t *testing.T, v *wideVariant, g *wboard) {
	t.Helper()
	for k := 0; k < v.ncells; k++ {
		if g.grid[k] == empty {
			t.Fatalf("%s: cell %d is empty", v.name, k)
		}
		if v.allow[k]&(1<<g.grid[k]) == 0 {
			t.Fatalf("%s: cell %d holds %c, which is not allowed there",
				v.name, k, charOfIn(v.alphabet, g.grid[k]))
		}
	}
	for u := 0; u < v.nunits; u++ {
		var seen uint64
		for _, k := range v.unitCells[u][:v.unitSize[u]] {
			m := uint64(1) << g.grid[k]
			if seen&m != 0 {
				t.Fatalf("%s: unit %d holds %c twice", v.name, u, charOfIn(v.alphabet, g.grid[k]))
			}
			seen |= m
		}
		if seen != v.unitAll[u] {
			t.Fatalf("%s: unit %d holds %d values, want %d",
				v.name, u, bits.OnesCount64(seen), bits.OnesCount64(v.unitAll[u]))
		}
	}
}
