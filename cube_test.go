package main

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// The folded cube must come out with exactly the cell and unit counts the
// surface of a 15-cube has: 6 faces of 256 lattice points, the 12 edges of
// 16 counted twice and the 8 corners three times. Nothing here is written
// down per edge - it all follows from the fold in hexadocubeNet - so these
// numbers are what says the fold is right.
func TestHexadocubeGeometry(t *testing.T) {
	v := hexadocubeVariant()
	if want := 6*256 - 12*16 + 8; v.ncells != want { // 1352
		t.Fatalf("%d cells, want %d", v.ncells, want)
	}
	if want := 6*48 - 12; v.nunits != want { // 276
		t.Fatalf("%d units, want %d", v.nunits, want)
	}

	// How often each cell is drawn, and how many units it lies in: an
	// interior cell once and in 3, a cell on a cube edge twice and in 5
	// (the line along the edge is one unit shared by both faces), a
	// corner three times and in 6.
	drawn := make([]int, v.ncells)
	for _, m := range v.at {
		if m > 0 {
			drawn[m-1]++
		}
	}
	want := map[int][2]int{1: {3, 1176}, 2: {5, 168}, 3: {6, 8}}
	count := map[int]int{}
	for k := 0; k < v.ncells; k++ {
		w, ok := want[drawn[k]]
		if !ok {
			t.Fatalf("cell %d is drawn %d times", k, drawn[k])
		}
		if int(v.ncu[k]) != w[0] {
			t.Fatalf("cell %d is drawn %d times but lies in %d units, want %d",
				k, drawn[k], v.ncu[k], w[0])
		}
		count[drawn[k]]++
	}
	for d, w := range want {
		if count[d] != w[1] {
			t.Fatalf("%d cells are drawn %d times, want %d", count[d], d, w[1])
		}
	}

	// Every face must land on the surface of the cube, and the corners of
	// the cube must be exactly the eight points with all coordinates at an
	// extreme. This catches a fold that rotates a face the wrong way: the
	// counts above would still come out if two faces landed on each other.
	faces := hexadocubeNet()
	if len(faces) != 6 {
		t.Fatalf("%d faces", len(faces))
	}
	normals := map[vec3]bool{}
	for i, f := range faces {
		if !f.placed {
			t.Fatalf("face %d was never folded into place", i)
		}
		if n := f.normal(); normals[n] {
			t.Fatalf("face %d repeats the normal %v of an earlier face", i, n)
		} else {
			normals[n] = true
		}
		for r := 0; r < maxSize; r++ {
			for c := 0; c < maxSize; c++ {
				p := f.point(r, c)
				edge := 0
				for _, x := range []int8{p.x, p.y, p.z} {
					if x != 0 && x != side {
						continue
					}
					edge++
				}
				if x, y, z := p.x, p.y, p.z; x < 0 || x > side || y < 0 || y > side || z < 0 || z > side {
					t.Fatalf("face %d cell (%d,%d) at %v is off the cube", i, r, c, p)
				}
				if edge == 0 {
					t.Fatalf("face %d cell (%d,%d) at %v is not on the surface", i, r, c, p)
				}
			}
		}
	}
	if len(normals) != 6 {
		t.Fatalf("%d distinct face normals, want 6", len(normals))
	}
}

// A cell on a cube edge is one cell drawn on two faces. Writing it through
// one face must show up on the other, and a clue that contradicts its own
// copy must be refused with a message that says so.
func TestHexadocubeSharesEdgeCells(t *testing.T) {
	v := hexadocubeVariant()
	copies := map[int][]int{} // cell -> raster positions
	for i, m := range v.at {
		if m > 0 {
			copies[int(m)-1] = append(copies[int(m)-1], i)
		}
	}
	g := v.newBoard()
	shared := 0
	for k, pos := range copies {
		if len(pos) < 2 {
			continue
		}
		shared++
		v.assign(g, k, uint8(k%maxSize))
	}
	if shared != 168+8 {
		t.Fatalf("%d cells are drawn more than once, want %d", shared, 168+8)
	}
	out := strings.Split(v.render(g), "\n")
	for k, pos := range copies {
		if len(pos) < 2 {
			continue
		}
		ch := charOf(v.nvals, uint8(k%maxSize))
		for _, p := range pos {
			if got := out[p/v.width][p%v.width]; got != ch {
				t.Fatalf("cell %d reads %c at raster %d, want %c", k, got, p, ch)
			}
		}
	}

	// two different values in the same cell, written on two faces. The
	// blank board is rendered first so the raster keeps its shape - the
	// blanks between the faces are what says where a face begins.
	lines := strings.Split(strings.TrimRight(v.render(v.newBoard()), "\n"), "\n")
	put := func(p int, ch byte) {
		r, c := p/v.width, p%v.width
		lines[r] = lines[r][:c] + string(ch) + lines[r][c+1:]
	}
	for _, pos := range copies {
		if len(pos) < 2 {
			continue
		}
		put(pos[0], '0')
		put(pos[1], '1')
		break
	}
	_, err := v.parse(strings.Join(lines, "\n"))
	if err == nil || !strings.Contains(err.Error(), "neighbouring face") {
		t.Fatalf("contradicting copies of one cell: got %v, want a neighbouring-face error", err)
	}
}

// The border ring of every face is empty in the printed puzzle, so those
// values can only arrive through the neighbouring faces. Clue the interiors
// of a valid cube and leave every border cell blank: if the coupling works,
// the search still recovers the whole surface.
func TestHexadocubeSolvesAcrossFolds(t *testing.T) {
	v := hexadocubeVariant()
	rng := rand.New(rand.NewPCG(11, 0x9e3779b97f4a7c15))
	full := v.newBoard()
	if !v.fillRand(full, rng) {
		t.Fatal("could not fill the cube at random")
	}
	checkFull(t, v, full)

	drawn := make([]int, v.ncells)
	for _, m := range v.at {
		if m > 0 {
			drawn[m-1]++
		}
	}
	pz := v.newBoard()
	border := 0
	for k := 0; k < v.ncells; k++ {
		if drawn[k] > 1 { // on a cube edge: left empty, as in the magazine
			border++
			continue
		}
		if rng.IntN(2) == 0 {
			v.assign(pz, k, full.grid[k])
		}
	}
	if border != 176 {
		t.Fatalf("%d border cells, want 176", border)
	}
	work := *pz
	if !v.solve(&work) {
		t.Fatal("no solution for a cube built from a valid one")
	}
	checkFull(t, v, &work)

	back, err := v.parse(v.render(pz))
	if err != nil {
		t.Fatalf("render/parse round trip: %v", err)
	}
	if *back != *pz {
		t.Fatal("render/parse round trip changed the board")
	}
}
