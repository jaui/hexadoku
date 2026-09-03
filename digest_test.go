package main

import (
	"os"
	"strings"
	"testing"
)

const digestFile = "puzzles/elektor/2011-03.txt"

// The Hexadoku 'Digest' of 3/2011 is the test of the block search: with
// the sixteen prize codes it must have exactly one solution, without them
// it must not - the magazine says as much, and if the codes or the block
// positions were read wrongly, one of the two would fail.
func TestDigest(t *testing.T) {
	data, err := os.ReadFile(digestFile)
	if err != nil {
		t.Skipf("%s not available", digestFile)
	}
	text := string(data)

	v := variantForText(text)
	if v == nil || len(v.blocks) != 16 || len(v.codes) != 16 {
		t.Fatalf("the header must give 16 blocks and 16 codes, got %v", v)
	}
	g, err := v.parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if clues := v.ncells - int(g.free); clues != 73 {
		t.Fatalf("%d clues, want 73", clues)
	}

	work := *g
	if !v.solveWith(&work) {
		t.Fatal("no solution")
	}
	checkFull(t, v, &work)
	check := *g
	if n := v.countWith(&check, 2, nil); n != 1 {
		t.Fatalf("%d solutions, want exactly 1", n)
	}

	// every code used exactly once, and every block holding one
	seen := map[int]bool{}
	for b, c := range v.assignmentOf(&work) {
		if c < 0 {
			t.Fatalf("block %d holds no code", b+1)
		}
		if seen[c] {
			t.Fatalf("code %s used twice", v.codeString(c))
		}
		seen[c] = true
	}
	if len(seen) != 16 {
		t.Fatalf("%d codes used, want 16", len(seen))
	}

	// the same 73 clues without the block rule are ambiguous
	plain := gridVariant(maxSize)
	bare, err := plain.parse(stripComments(text))
	if err != nil {
		t.Fatal(err)
	}
	if n := plain.count(bare, 2, nil); n < 2 {
		t.Fatalf("the clues alone have %d solution(s); the digest is only "+
			"pinned down by the codes", n)
	}

	// and the result must be the grid Elektor printed two issues later,
	// up to the OCR damage in that scan
	if printed, err := os.ReadFile("puzzles/elektor/2011-05_prevsolution.txt"); err == nil {
		rows := []string{}
		for _, l := range strings.Split(strings.ReplaceAll(string(printed), "\r", ""), "\n") {
			if len(l) == 16 && !strings.HasPrefix(l, "#") {
				rows = append(rows, l)
			}
		}
		if len(rows) == 16 {
			same := 0
			for r := 0; r < 16; r++ {
				for c := 0; c < 16; c++ {
					if rows[r][c] == charOf(v.nvals, work.grid[r*16+c]) {
						same++
					}
				}
			}
			if same < 200 {
				t.Fatalf("only %d of 256 cells agree with the printed 3/2011 solution", same)
			}
		}
	}
}

// A header the digest cannot be built from must fail at parse time and
// not quietly fall back to solving the grid as a plain hexadoku.
func TestDigestRejectsBrokenHeader(t *testing.T) {
	blank := strings.Repeat(strings.Repeat(".", 16)+"\n", 16)
	for _, tc := range []struct{ name, header, want string }{
		{"no codes", "# variant: digest\n# block: r1c1-c5\n", "needs both"},
		{"count mismatch", "# variant: digest\n# codes: 4395C 3097D\n# block: r1c1-c5\n", "1 blocks but 2 codes"},
		{"block outside", "# variant: digest\n# codes: 4395C\n# block: r1c14-c18\n", "not inside"},
		{"bad code", "# variant: digest\n# codes: 4395Z\n# block: r1c1-c5\n", "not a hexadecimal"},
		{"wrong length", "# variant: digest\n# codes: 4395C\n# block: r1c1-c4\n", "4 cells"},
	} {
		v := variantForText(tc.header + blank)
		if v == nil {
			t.Fatalf("%s: no layout at all", tc.name)
		}
		_, err := v.parse(tc.header + blank)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: got %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}
