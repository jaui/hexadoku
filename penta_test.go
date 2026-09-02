package main

import (
	"math/bits"
	"math/rand/v2"
	"strings"
	"testing"
)

// The EUC penta-hexadoku is the 2009 pinwheel plus the chequerboard, so
// its geometry must be identical to the hexamurai's and only the per-cell
// masks may differ.
func TestPentaGeometry(t *testing.T) {
	p, h := pentaVariant(), hexamuraiVariant()
	if p.ncells != h.ncells || p.nunits != h.nunits {
		t.Fatalf("penta has %d cells / %d units, hexamurai %d / %d",
			p.ncells, p.nunits, h.ncells, h.nunits)
	}
	for k := 0; k < p.ncells; k++ {
		if p.pos[k] != h.pos[k] || p.cellUnits[k] != h.cellUnits[k] {
			t.Fatalf("cell %d sits differently than in the hexamurai", k)
		}
	}

	// coloured cells (row+column even) take the even values, white ones
	// the odd values, and every unit must hold eight of each - otherwise
	// the rule could not be satisfied at all
	for k := 0; k < p.ncells; k++ {
		pos := int(p.pos[k])
		want := uint16(oddValues)
		if (pos/p.width+pos%p.width)%2 == 0 {
			want = evenValues
		}
		if p.allow[k] != want {
			t.Fatalf("cell %d at raster %d allows %016b, want %016b",
				k, pos, p.allow[k], want)
		}
	}
	for u := 0; u < p.nunits; u++ {
		even := 0
		for _, k := range p.unitCells[u][:p.nvals] {
			if p.allow[k] == evenValues {
				even++
			}
		}
		if even != p.nvals/2 {
			t.Fatalf("unit %d has %d coloured cells for %d even values",
				u, even, p.nvals/2)
		}
	}
	if bits.OnesCount16(evenValues) != 8 || evenValues&oddValues != 0 ||
		evenValues|oddValues != p.all {
		t.Fatal("the two parity masks do not partition the values")
	}
}

func TestPentaSolve(t *testing.T) {
	v := pentaVariant()
	rng := rand.New(rand.NewPCG(5, 0x9e3779b97f4a7c15))
	full := v.newBoard()
	if !v.fillRand(full, rng) {
		t.Fatal("could not fill the pentagrid at random")
	}
	checkFull(t, v, full)
	for k := 0; k < v.ncells; k++ {
		if 1<<full.grid[k]&v.allow[k] == 0 {
			t.Fatalf("cell %d holds %c on the wrong colour",
				k, charOf(v.nvals, full.grid[k]))
		}
	}

	pz := v.newBoard()
	for k := 0; k < v.ncells; k++ {
		if rng.IntN(4) == 0 {
			v.assign(pz, k, full.grid[k])
		}
	}
	work := *pz
	if !v.solve(&work) {
		t.Fatal("no solution for a pentagrid built from a valid one")
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

// A clue of the wrong parity is not a mere conflict but a broken rule, and
// the message must name the chequerboard so the reading can be corrected.
func TestPentaRejectsWrongColour(t *testing.T) {
	v := pentaVariant()
	lines := strings.Split(strings.TrimRight(v.render(v.newBoard()), "\n"), "\n")
	// raster (0,8) is the top left cell of the first grid, row+col even,
	// so it is coloured and only an even value may stand there
	lines[0] = lines[0][:8] + "3" + lines[0][9:]
	_, err := v.parse(strings.Join(lines, "\n"))
	if err == nil || !strings.Contains(err.Error(), "chequerboard") {
		t.Fatalf("odd clue on a coloured cell: got %v, want a chequerboard error", err)
	}
	lines[0] = lines[0][:8] + "4" + lines[0][9:]
	if _, err := v.parse(strings.Join(lines, "\n")); err != nil {
		t.Fatalf("even clue on a coloured cell: %v", err)
	}
}

// A "# variant:" header must override the cell count, which is the only
// thing that can tell the penta-hexadoku from the hexamurai it shares its
// geometry with.
func TestVariantHeaderWins(t *testing.T) {
	blank := pentaVariant()
	text := "# variant: penta-hexadoku\n" + blank.render(blank.newBoard())
	v := variantForText(text)
	if v == nil || v.name != blank.name {
		t.Fatalf("header picked %v, want %s", v, blank.name)
	}
	if v := variantForText(strings.TrimPrefix(text, "# variant: penta-hexadoku\n")); v == nil ||
		v.name != hexamuraiVariant().name {
		t.Fatalf("without the header the cell count must pick the hexamurai, got %v", v)
	}
}
