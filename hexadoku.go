// Hexadoku (16x16 Sudoku) solver — also solves classic 9x9 Sudoku.
//
// Rewrite of hexadoku.cpp in Go, optimized for amd64:
//   - candidate sets are uint16 bitmasks (bit v = value v is possible),
//     so all set operations are single register instructions
//   - math/bits.OnesCount16 / TrailingZeros16 compile to POPCNT / TZCNT
//   - the whole solver state is one flat ~350-byte struct; branching in the
//     search tree saves/restores it with a single struct copy (SSE moves),
//     no heap allocation in the hot path
//
// Search strategy (tree pruning instead of blind brute force):
//   1. constraint propagation to a fixpoint:
//        naked singles  - a cell with exactly one candidate is assigned
//        hidden singles - a value with exactly one possible cell in a
//                         row/column/box is assigned
//   2. backtracking with MRV heuristic: branch on the cell with the
//      fewest candidates, so the tree stays as narrow as possible
//
// The grid size is detected from the input: 256 cells -> 16x16 hexadoku
// (values 0-F), 81 cells -> 9x9 sudoku (values 1-9, '0' or '.' = empty).
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"
)

const (
	maxSize  = 16
	maxCells = maxSize * maxSize
	empty    = 0xFF
)

// geometry of the current puzzle size, set by initGeometry
// (the solver cores in solver.go are specialized per size and don't use these)
var (
	size    int    // 9 or 16
	box     int    // 3 or 4
	ncells  int    // size*size
	allMask uint16 // size low bits set
)

func initGeometry(n int) {
	size = n
	box = 3
	if n == maxSize {
		box = 4
	}
	ncells = n * n
	allMask = uint16(1)<<n - 1
}

// board is the complete solver state. It is deliberately small and flat:
// saving/restoring it during backtracking is a plain struct copy.
type board struct {
	rows  [maxSize]uint16 // values already used per row
	cols  [maxSize]uint16 // values already used per column
	boxes [maxSize]uint16 // values already used per box
	grid  [maxCells]uint8
	free  int16 // number of empty cells
}

func newBoard() *board {
	b := &board{free: int16(ncells)}
	for i := range b.grid {
		b.grid[i] = empty
	}
	return b
}

var nodes uint64 // branch counter for statistics

func boardFromClues(full *board, keep *[maxCells]bool) *board {
	b := newBoard()
	for k := 0; k < ncells; k++ {
		if keep[k] {
			b.assign(k, full.grid[k])
		}
	}
	return b
}

// generate creates a minimal puzzle: starting from a random full grid,
// clues are removed in random order as long as the solution stays unique.
// In the result every remaining clue is necessary - removing any single
// one yields multiple solutions ("gerade noch eindeutig lösbar").
// Returns the puzzle and its difficulty in branch nodes.
func generate(n int, rng *rand.Rand) (*board, uint64) {
	initGeometry(n)
	full := newBoard()
	fillRand(full, rng)

	order := rng.Perm(ncells)
	var keep [maxCells]bool
	for k := 0; k < ncells; k++ {
		keep[k] = true
	}
	// Removing clues can only ever increase the number of solutions, so a
	// clue that can't be removed now can't be removed later either: one
	// pass is enough for minimality.
	for _, k := range order {
		keep[k] = false
		check := boardFromClues(full, &keep)
		if countSolutions(check, 2, nil) != 1 {
			keep[k] = true
		}
	}

	pz := boardFromClues(full, &keep)
	nodes = 0
	work := *pz
	solve(&work)
	return pz, nodes
}

// Compact returns the puzzle as size lines of size characters.
func (b *board) Compact() string {
	var sb strings.Builder
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if v := b.grid[r*size+c]; v == empty {
				sb.WriteByte('.')
			} else {
				sb.WriteByte(valChar(v))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func valChar(v uint8) byte { return charOf(size, v) }

// parse reads a puzzle from text and auto-detects its size.
// Formats:
//   - 16x16: hex digits 0-9/a-f/A-F, empty cells '.', '*' or '_';
//     everything else (spaces, grid lines) is ignored
//   - 16x16 letter format a-p for 0-15 (used by some puzzle collections,
//     detected by the presence of letters g-p)
//   - 9x9: digits 1-9, empty cells '0', '.', '*' or '_' (81 cells total)
func parse(s string) (*board, error) {
	// tokens: 0-15 = value, -1 = empty; interpretation is decided
	// after counting, because '0' means "empty" in 9x9 but is a value in 16x16
	var toks []int8
	s = stripComments(s)

	if strings.ContainsAny(s, "ghijklmnopGHIJKLMNOP") {
		// letter format: grid rows start with '|', the value of cell i
		// sits at column 4*i+2, a space means empty
		for _, line := range strings.Split(s, "\n") {
			if !strings.HasPrefix(line, "|") {
				continue
			}
			for i := 0; i < maxSize; i++ {
				pos := 4*i + 2
				if pos >= len(line) {
					return nil, fmt.Errorf("malformed grid line: %q", line)
				}
				switch ch := line[pos]; {
				case ch >= 'a' && ch <= 'p':
					toks = append(toks, int8(ch-'a'))
				case ch >= 'A' && ch <= 'P':
					toks = append(toks, int8(ch-'A'))
				case ch == ' ' || ch == '.' || ch == '*':
					toks = append(toks, -1)
				default:
					return nil, fmt.Errorf("unexpected character %q in grid line", ch)
				}
			}
		}
	} else {
		// collect the cell characters first: '0' means "empty" in a 9x9
		// sudoku but is a value in a 16x16 hexadoku, so the size has to be
		// known before the characters can be decoded
		var chars []byte
		for i := 0; i < len(s); i++ {
			if valueOf(maxSize, s[i]) != chNone {
				chars = append(chars, s[i])
			}
		}
		switch len(chars) {
		case maxCells:
			initGeometry(maxSize)
		case 81:
			initGeometry(9)
		default:
			return nil, fmt.Errorf("expected 256 (hexadoku) or 81 (sudoku) cells, got %d", len(chars))
		}
		for _, ch := range chars {
			t := valueOf(size, ch)
			if t == chNone {
				return nil, fmt.Errorf("value %c is not valid in a %dx%d puzzle", ch, size, size)
			}
			toks = append(toks, int8(t))
		}
		return buildBoard(toks)
	}

	if len(toks) != maxCells {
		return nil, fmt.Errorf("expected 256 (hexadoku) cells, got %d", len(toks))
	}
	initGeometry(maxSize)

	return buildBoard(toks)
}

// buildBoard turns decoded tokens (value, or -1 for empty) into a board.
// initGeometry must have been called for the matching size.
func buildBoard(toks []int8) (*board, error) {
	b := newBoard()
	for k, t := range toks {
		if t < 0 {
			continue
		}
		v := uint8(t)
		if b.cand(k)&(1<<v) == 0 {
			return nil, fmt.Errorf("conflicting clue %c at row %d, column %d", valChar(v), k/size+1, k%size+1)
		}
		b.assign(k, v)
	}
	return b, nil
}

func (b *board) String() string {
	var sb strings.Builder
	segs := make([]string, box)
	for i := range segs {
		w := 2*box - 1 // block content width
		if i > 0 {
			w++
		}
		if i < box-1 {
			w++
		}
		segs[i] = strings.Repeat("-", w)
	}
	sep := strings.Join(segs, "+")
	for r := 0; r < size; r++ {
		if r > 0 && r%box == 0 {
			sb.WriteString(sep)
			sb.WriteByte('\n')
		}
		for c := 0; c < size; c++ {
			if c > 0 {
				if c%box == 0 {
					sb.WriteString(" | ")
				} else {
					sb.WriteByte(' ')
				}
			}
			if v := b.grid[r*size+c]; v == empty {
				sb.WriteByte('.')
			} else {
				sb.WriteByte(valChar(v))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// stripComments removes lines whose first non-space character is '#'.
func stripComments(s string) string {
	if !strings.Contains(s, "#") {
		return s
	}
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// splitBlocks splits text at blank lines into blocks that each contain
// a complete puzzle (81 or 256 cells).
func splitBlocks(text string) []string {
	var blocks []string
	var cur []string
	curTokens := 0
	flush := func() {
		if curTokens == 81 || curTokens == maxCells {
			blocks = append(blocks, strings.Join(cur, "\n"))
		}
		cur, curTokens = nil, 0
	}
	for _, l := range strings.Split(stripComments(text), "\n") {
		if strings.TrimSpace(l) == "" {
			flush()
			continue
		}
		cur = append(cur, l)
		curTokens += countTokens(l)
	}
	flush()
	return blocks
}

// countTokens counts puzzle cells in one line of text.
func countTokens(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		switch ch := line[i]; {
		case ch >= '0' && ch <= '9', ch >= 'a' && ch <= 'f', ch >= 'A' && ch <= 'F',
			ch == '.', ch == '*', ch == '_':
			n++
		}
	}
	return n
}

// runCollection handles files with one complete puzzle per line
// (e.g. Norvig's top95.txt): every puzzle is solved, only statistics
// are printed.
func runCollection(name string, lines []string) bool {
	fmt.Printf("=== %s (collection of %d puzzles)\n", name, len(lines))
	var totalNodes, maxNodes uint64
	var total time.Duration
	var maxTime time.Duration
	worst := -1
	for i, l := range lines {
		b, err := parse(l)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s line %d: %v\n", name, i+1, err)
			return false
		}
		nodes = 0
		start := time.Now()
		ok := solve(b)
		elapsed := time.Since(start)
		if !ok {
			fmt.Printf("puzzle %d: no solution!\n", i+1)
			return false
		}
		totalNodes += nodes
		total += elapsed
		if elapsed > maxTime {
			maxTime, maxNodes, worst = elapsed, nodes, i+1
		}
	}
	fmt.Printf("all %d solved in %v (avg %v/puzzle, %d branch nodes total)\n",
		len(lines), total, total/time.Duration(len(lines)), totalNodes)
	fmt.Printf("hardest: puzzle %d with %v, %d branch nodes\n\n", worst, maxTime, maxNodes)
	return true
}

// defaultPuzzle is the puzzle hardcoded in the original hexadoku.cpp.
const defaultPuzzle = "E......A...6.F8..65...E.18.F.0.A3.7B..65.D...2..8....." +
	"B..34...5....07...9.....2..B...2.0F7.8D..65.E...FBD16.C....D.6.3..2.0..A.7" +
	"....D.....C.287.73BE..9C0A82..6...A..1....7E.B9.29....0.64D...A.476..F...." +
	".A0..2B.C3A.5480...EF.DE9.0C2.4.F5..18F..5.B....19.4.."

var compact bool // -compact: print only the solution in compact form

func run(name, text string, bench int, checkUnique bool) bool {
	// alphanumski or alphasudoku? Their alphabets do not fit a uint16, so
	// they have a core of their own; the cell count cannot detect them
	// (countTokens knows only 0-9A-F), the header names them.
	if wv := wideVariantForText(text); wv != nil {
		return runWide(name, wv, text, bench, checkUnique)
	}

	// samurai, cube or penta layout? (named in a "# variant:" header or
	// detected by its total cell count; the cells sit on a character
	// raster, so it needs the geometry-driven core)
	if v := variantForText(text); v != nil {
		return runVariant(name, v, text, bench, checkUnique)
	}

	// collection file? (several lines that each hold a complete puzzle)
	var puzzleLines []string
	for _, l := range strings.Split(stripComments(text), "\n") {
		if n := countTokens(l); n == 81 || n == maxCells {
			puzzleLines = append(puzzleLines, l)
		}
	}
	if len(puzzleLines) > 1 {
		return runCollection(name, puzzleLines)
	}

	// several puzzles separated by blank lines?
	if blocks := splitBlocks(text); len(blocks) > 1 {
		ok := true
		for i, blk := range blocks {
			ok = run(fmt.Sprintf("%s #%d", name, i+1), blk, bench, checkUnique) && ok
		}
		return ok
	}

	b, err := parse(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return false
	}
	if !compact {
		fmt.Printf("=== %s (%dx%d, %d clues)\n%s\n", name, size, size, ncells-int(b.free), b)
	}

	// With -unique the tree is searched once: the proof that no second
	// solution exists finds the first one on its way, so there is no
	// separate solve before it. The branch-node count reported is then
	// the proof's, which covers the whole tree rather than the part up to
	// the first solution.
	nodes = 0
	work := *b
	start := time.Now()
	var ok bool
	nsol := 0
	if checkUnique {
		check := *b
		nsol = countSolutions(&check, 2, &work)
		ok = nsol > 0
	} else {
		ok = solve(&work)
	}
	elapsed := time.Since(start)

	if !ok {
		fmt.Printf("no solution (%v, %d branch nodes)\n\n", elapsed, nodes)
		return false
	}
	if compact {
		fmt.Print(work.Compact())
		return true
	}
	fmt.Printf("%s\nsolved in %v, %d branch nodes\n", &work, elapsed, nodes)

	if checkUnique {
		if nsol > 1 {
			fmt.Println("warning: solution is not unique")
		} else {
			fmt.Println("solution is unique")
		}
	}

	if bench > 0 {
		start = time.Now()
		for i := 0; i < bench; i++ {
			work = *b
			solve(&work)
		}
		elapsed = time.Since(start)
		fmt.Printf("bench: %d runs, %v total, %v per solve\n", bench, elapsed, elapsed/time.Duration(bench))
	}
	fmt.Println()
	return true
}

func main() {
	bench := flag.Int("bench", 0, "additionally solve n times and report the average time")
	unique := flag.Bool("unique", false, "check whether the solution is unique")
	flag.BoolVar(&compact, "compact", false, "print only the solution in compact 16-line form")
	gen := flag.String("gen", "", "generate puzzles instead of solving: 9, 16, samurai, hexamurai, penta or hexadocube")
	count := flag.Int("count", 3, "with -gen: number of puzzles to generate")
	tries := flag.Int("tries", 8, "with -gen: attempts per puzzle, the hardest is kept")
	seed := flag.Uint64("seed", 0, "with -gen: random seed (0 = time-based)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] [puzzle files...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "with no files, the built-in puzzle is solved\n\noptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *gen != "" {
		var v *variant
		var n int
		switch *gen {
		case "9":
			n = 9
		case "16":
			n = maxSize
		case "samurai":
			v = samuraiVariant(9)
		case "hexa-samurai":
			v = samuraiVariant(maxSize)
		case "hexamurai", "murai":
			v = hexamuraiVariant()
		case "penta", "penta-hexadoku":
			v = pentaVariant()
		case "hexadocube", "cube":
			v = hexadocubeVariant()
		default:
			fmt.Fprintln(os.Stderr, "-gen must be 9, 16, samurai, hexa-samurai, hexamurai, penta or hexadocube")
			os.Exit(2)
		}
		if *seed == 0 {
			*seed = uint64(time.Now().UnixNano())
		}
		rng := rand.New(rand.NewPCG(*seed, 0x9e3779b97f4a7c15))
		for i := 0; i < *count; i++ {
			if v != nil {
				pz, d := generateVariant(v, rng)
				fmt.Printf("# %s puzzle %d: %d clues, difficulty %d branch nodes, unique, minimal up to a %d node budget per removal, seed %d\n",
					v.name, i+1, v.ncells-int(pz.free), d, genBudget, *seed)
				fmt.Print(v.render(pz))
				fmt.Println()
				continue
			}
			var best *board
			var bestNodes uint64
			for t := 0; t < *tries; t++ {
				pz, d := generate(n, rng)
				if best == nil || d > bestNodes {
					best, bestNodes = pz, d
				}
			}
			fmt.Printf("# %dx%d puzzle %d: %d clues, difficulty %d branch nodes, minimal (removing any clue loses uniqueness), seed %d\n",
				n, n, i+1, ncells-int(best.free), bestNodes, *seed)
			fmt.Print(best.Compact())
			fmt.Println()
		}
		return
	}

	ok := true
	if flag.NArg() == 0 {
		ok = run("built-in", defaultPuzzle, *bench, *unique)
	} else {
		for _, f := range flag.Args() {
			data, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				ok = false
				continue
			}
			ok = run(f, string(data), *bench, *unique) && ok
		}
	}
	if !ok {
		os.Exit(1)
	}
}
