// Reading and running the wide layouts of alpha.go from a puzzle file.
//
// Neither cell count can pick these two out on its own: countTokens knows
// only 0-9A-F, so it cannot even count the cells of a 36-value grid. A
// file therefore names its layout, the way the EUC penta-hexadoku has to
// because it shares its geometry with the hexamurai:
//
//	# variant: alphasudoku
//	# grey: r20c6-c11
//	# code: HKCEAO
//
// The grey cells are the ones the magazine printed shaded and asked its
// readers to send in; the code is what it announced two issues later.
// Where a file claims one, solving it checks the claim - six or seven
// named cells out of 625 or 1296 cannot agree by accident, so the code
// confirms the transcription, the rule model and the solver at once.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// wideVariants are the layouts a "# variant:" header can ask for.
var wideVariants = map[string]func() *wideVariant{
	"alphanumski": alphanumskiVariant,
	"alphasudoku": alphasudokuVariant,
	"alfadoku":    alfadokuVariant,
	"alphadoku":   alfadokuVariant, // how the 2007 article spells the 2006 puzzle
}

// wideVariantForText returns the wide layout a file asks for by name,
// with its grey cells read from the same header, or nil.
func wideVariantForText(text string) *wideVariant {
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
		build, ok := wideVariants[strings.ToLower(fields[0])]
		if !ok {
			return nil
		}
		v := build()
		v.readGrey(text)
		return v
	}
	return nil
}

// readGrey collects the "# grey:" ranges and the "# code:" claim. Ranges
// are written r<row>c<first>-c<last>, one-based, the same notation the
// digest uses for its blocks; several lines are concatenated in order,
// because the seven answer cells of the Alphanumski may well be split by
// the gutter of the double page.
func (v *wideVariant) readGrey(text string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		if _, rest, ok := strings.Cut(line, "grey:"); ok {
			var row, c0, c1 int
			if _, err := fmt.Sscanf(strings.TrimSpace(rest), "r%dc%d-c%d", &row, &c0, &c1); err != nil {
				v.builderr = fmt.Errorf("%s: cannot read the grey range %q",
					v.name, strings.TrimSpace(rest))
				return
			}
			if row < 1 || row > v.n || c0 < 1 || c1 > v.n || c1 < c0 {
				v.builderr = fmt.Errorf("%s: the grey range r%dc%d-c%d is not inside the grid",
					v.name, row, c0, c1)
				return
			}
			for c := c0; c <= c1; c++ {
				v.grey = append(v.grey, (row-1)*v.n+c-1)
			}
			if v.greyText != "" {
				v.greyText += " "
			}
			v.greyText += fmt.Sprintf("r%dc%d-c%d", row, c0, c1)
			continue
		}
		if _, rest, ok := strings.Cut(line, "code:"); ok {
			if f := strings.Fields(rest); len(f) > 0 {
				v.greyCode = f[0]
			}
		}
	}
	if v.greyCode != "" && len(v.greyCode) != len(v.grey) {
		v.builderr = fmt.Errorf("%s: the header claims the %d character code %s "+
			"for %d grey cells", v.name, len(v.greyCode), v.greyCode, len(v.grey))
	}
}

// runWide is runVariant for the wide core: same report, plus the grey
// cells and the check against the announced code.
func runWide(name string, v *wideVariant, text string, bench int, checkUnique bool) bool {
	g, err := v.parse(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return false
	}
	if !compact {
		fmt.Printf("=== %s (%s, %dx%d over %d values, %d clues)\n%s\n",
			name, v.name, v.n, v.n, v.nvals, v.ncells-int(g.free), v.render(g))
	}

	// with -unique the uniqueness proof finds the solution on its way,
	// so the tree is searched once and not twice (see count)
	nodes = 0
	work := *g
	start := time.Now()
	var ok bool
	nsol := 0
	if checkUnique {
		check := *g
		nsol = v.count(&check, 2, &work)
		ok = nsol > 0
	} else {
		ok = v.solve(&work)
	}
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

	if len(v.grey) > 0 {
		got := v.greyString(&work)
		switch {
		case v.greyCode == "":
			fmt.Printf("grey cells %s give %s\n", v.greyText, got)
		case got == v.greyCode:
			fmt.Printf("grey cells %s give %s, the code the magazine announced\n",
				v.greyText, got)
		default:
			fmt.Fprintf(os.Stderr, "%s: grey cells %s give %s, but the header claims "+
				"the announced code is %s\n", name, v.greyText, got, v.greyCode)
			return false
		}
	}

	if checkUnique {
		if nsol > 1 {
			fmt.Println("warning: solution is not unique")
			if len(v.grey) > 0 && v.greyPinned(g, &work) {
				fmt.Println("the grey cells are the same in every solution, " +
					"so the answer is unambiguous")
			}
		} else {
			fmt.Println("solution is unique")
		}
	}

	if bench > 0 {
		start = time.Now()
		for i := 0; i < bench; i++ {
			work = *g
			v.solve(&work)
		}
		elapsed = time.Since(start)
		fmt.Printf("bench: %d runs, %v total, %v per solve\n", bench, elapsed, elapsed/time.Duration(bench))
	}
	fmt.Println()
	return true
}
