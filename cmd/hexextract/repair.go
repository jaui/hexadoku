package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// conflicts returns the cells taking part in a row/column/box clash.
// A correct 16x16 solution has none; an OCR slip almost always creates
// one, which makes the property a sharp error detector.
// With requireFull set, cells that are simply empty count as damaged
// too (used for solution grids, which must be complete).
func conflicts(g [16][16]byte, requireFull bool) [16][16]bool {
	var bad [16][16]bool
	mark := func(cells [][2]int) {
		byVal := map[byte][][2]int{}
		for _, c := range cells {
			v := g[c[0]][c[1]]
			if !isHex(v) {
				if requireFull {
					bad[c[0]][c[1]] = true
				}
				continue // blanks never clash in a puzzle grid
			}
			byVal[v] = append(byVal[v], c)
		}
		for _, cs := range byVal {
			if len(cs) > 1 {
				for _, c := range cs {
					bad[c[0]][c[1]] = true
				}
			}
		}
	}
	for i := 0; i < 16; i++ {
		var row, col [][2]int
		for j := 0; j < 16; j++ {
			row = append(row, [2]int{i, j})
			col = append(col, [2]int{j, i})
		}
		mark(row)
		mark(col)
	}
	for b := 0; b < 16; b++ {
		var box [][2]int
		for r := (b / 4) * 4; r < (b/4)*4+4; r++ {
			for c := (b % 4) * 4; c < (b%4)*4+4; c++ {
				box = append(box, [2]int{r, c})
			}
		}
		mark(box)
	}
	return bad
}

func countBad(bad [16][16]bool) int {
	n := 0
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if bad[r][c] {
				n++
			}
		}
	}
	return n
}

// buildPuzzle keeps the clue positions of the mask and takes their
// values from a solution grid.
func buildPuzzle(mask, sol [16][16]byte) [16][16]byte {
	var p [16][16]byte
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if mask[r][c] == '#' {
				p[r][c] = sol[r][c]
			} else {
				p[r][c] = '.'
			}
		}
	}
	return p
}

// puzzleIsUnique reports whether the solver finds exactly one solution.
// Proving a wrong pairing unsolvable can take arbitrarily long, so the
// call is capped - a timeout counts as "not this one".
func puzzleIsUnique(g [16][16]byte, solver, tmpDir string) bool {
	f := filepath.Join(tmpDir, "pairing.txt")
	if err := os.WriteFile(f, []byte(gridString(g)), 0o644); err != nil {
		return false
	}
	defer os.Remove(f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, solver, "-unique", f).Output()
	return strings.Contains(string(out), "solution is unique")
}

func gridString(g [16][16]byte) string {
	var sb strings.Builder
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if g[r][c] == 0 {
				sb.WriteByte('.')
			} else {
				sb.WriteByte(g[r][c])
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// repairSolution blanks every cell involved in a conflict and lets the
// solver fill them back in. If the remaining clues determine the grid
// uniquely, the result is provably the printed solution - regardless of
// how many characters the OCR got wrong.
func repairSolution(g [16][16]byte, solver, tmpDir string) ([16][16]byte, int, error) {
	return repair(g, solver, tmpDir, true)
}

// repairPuzzle does the same for a sparse puzzle grid: the clue
// positions are kept, only misread values are recomputed.
func repairPuzzle(g [16][16]byte, solver, tmpDir string) ([16][16]byte, int, error) {
	fixed, n, err := repair(g, solver, tmpDir, false)
	if err != nil || n == 0 {
		return g, n, err
	}
	out := g
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if isHex(g[r][c]) {
				out[r][c] = fixed[r][c]
			}
		}
	}
	return out, n, nil
}

func repair(g [16][16]byte, solver, tmpDir string, requireFull bool) ([16][16]byte, int, error) {
	bad := conflicts(g, requireFull)
	n := countBad(bad)
	if n == 0 {
		return g, 0, nil
	}
	if solver == "" {
		return g, n, fmt.Errorf("%d damaged cells and no solver configured", n)
	}
	blanked := g
	for r := 0; r < 16; r++ {
		for c := 0; c < 16; c++ {
			if bad[r][c] || !isHex(blanked[r][c]) {
				blanked[r][c] = '.'
			}
		}
	}
	f := filepath.Join(tmpDir, "repair.txt")
	if err := os.WriteFile(f, []byte(gridString(blanked)), 0o644); err != nil {
		return g, n, err
	}
	// the solver exits non-zero on unsolvable input, which is a valid
	// answer here rather than a failure - inspect its output instead
	run := func(args ...string) string {
		out, _ := exec.Command(solver, append(args, f)...).Output()
		return string(out)
	}
	if !strings.Contains(run("-unique"), "solution is unique") {
		return g, n, fmt.Errorf("%d damaged cells, remainder not uniquely solvable", n)
	}
	out := run("-compact")
	var fixed [16][16]byte
	row := 0
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if len(line) != 16 || row >= 16 {
			continue
		}
		for c := 0; c < 16; c++ {
			fixed[row][c] = line[c]
		}
		row++
	}
	if row != 16 {
		return g, n, fmt.Errorf("solver returned %d rows", row)
	}
	return fixed, n, nil
}
