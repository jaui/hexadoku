hexadoku
========

Hexadoku (16x16 Sudoku) solver in Go — solves classic 9x9 Sudoku, and the
five-grid samurai layouts of both sizes (Elektor's *Hexamurai*) as well.
Hexadokus were published as reader puzzles in the Elektor magazine:
each row, column and 4x4 box must contain the hex digits 0-F exactly once.

The original C++ brute-force/constraint solver is kept for reference in
`cpp/hexadoku.cpp`.

A write-up of every problem hit while building this and how it was
solved is in [`docs/werkstattbericht.html`](docs/werkstattbericht.html)
(German): solver data layout, the PDF toolchain dead end, grid detection
in scans, how the puzzle/solution pairing was derived from the data, and
the two Hexamurai arrangements.

[`docs/suchbaum.html`](docs/suchbaum.html) (German) steps through the
search interactively: candidates per cell, which cell falls as a naked
and which as a hidden single, the MRV cell picked as the branch point
with the candidate histogram that explains the choice, contradictions,
and the decision path with its discarded branches. Same algorithm as
`solver.go`, reimplemented in JavaScript; the branch counts match the Go
version on all four built-in puzzles. Open it from a local web server
(`file://` works in most browsers but Chrome is restrictive).

Algorithm
---------

Instead of blind brute force the search tree is pruned:

1. **Constraint propagation** to a fixpoint:
   - *naked singles*: a cell with exactly one candidate is assigned
   - *hidden singles*: a value with exactly one possible cell in a
     row/column/box is assigned
2. **Backtracking with MRV heuristic** (minimum remaining values):
   branch on the cell with the fewest candidates, so the tree stays as
   narrow as possible. This is the Norvig-style approach the C++ version
   already used, plus hidden singles.

amd64 optimizations
-------------------

- Candidate sets are `uint16` bitmasks (bit *v* = value *v* possible),
  so eliminate/count/pick are single register instructions.
- `math/bits.OnesCount16` / `TrailingZeros16` compile to `POPCNT` / `TZCNT`.
- The complete solver state is one flat ~350-byte struct
  (`[16]uint16` row/col/box masks + `[256]uint8` grid). Backtracking
  saves/restores it with a plain struct copy (vectorized moves) —
  no heap allocation in the hot path. The C++ version allocated a full
  board object with 256 `vector<bool>` cells per search node.

Typical numbers (same machine): built-in Elektor puzzle — old C++ solver
~600 ms process runtime, Go solver ~32 µs per solve; hardest downloaded
hexadoku (`extra3.txt`) ~3.8 ms; Norvig's `top95` hard-sudoku collection
~85 µs per puzzle. The hot routines are specialized separately for 9x9
and 16x16 (`solver.go`), which gains another ~13% on hard hexadokus
compared to one size-generic core.

Samurai layouts (Hexamurai)
---------------------------

Elektor's double issues 7-8/2009 and 7-8/2011 printed a **Hexamurai**
(designer Claude Ghyselen): five 16x16 grids overlapping in a cross. The
magazine states the grids cannot be solved separately, and that is
literally true — the middle grid on its own is ambiguous, and only what
reaches it through the shared cells pins it down.

The two issues do not use the same arrangement, and neither is the
textbook cross. Both were read off the printed clue positions and are
confirmed by the puzzles solving uniquely:

| arrangement | raster | shared per pair | cells | units | clues | solves in |
| --- | --- | --- | --- | --- | --- | --- |
| 7-8/2009, pinwheel | 40×40 | 4×12 (three boxes) | 1088 | 228 | 409 | 2.2 ms, 65 nodes |
| 7-8/2011, plus | 32×32 | 8×16 (half a grid) | 768 | 176 | 231 | 43 s, 3 364 151 nodes |
| classic cross (`samurai`) | 21×21 / 40×40 | one box | 369 / 1216 | 131 / 236 | — | — |

The 2011 layout has a curious property the unit table shows by itself:
every row, column and box of its middle grid coincides with a unit of an
outer grid, so declaring five grids yields 176 distinct units — exactly
what the four outer grids contribute on their own. The middle grid adds
no constraint, which leaves four only pairwise coupled grids at 30% clue
density, and that is why it costs five orders of magnitude more search
than the 2009 one.

All of them are handled by the same code, with the grid corners as the
only difference:

| file | role |
| --- | --- |
| `chars.go` | value ↔ character encoding, shared by every variant |
| `variant.go` | geometry: cells, units, and the text raster they sit on |
| `general.go` | solver core driven by a `variant` (any unit list) |
| `murai.go` | the five-grid arrangements, single-grid variant, CLI glue |
| `solver.go` | the hand-written 9x9 / 16x16 cores (unchanged) |

The abstraction is the *unit*: a group of `n` cells that must contain
every value exactly once. Rows, columns and boxes of a plain grid are
units; in a samurai the units of all five grids are declared over one
shared cell pool, so a cell in an overlap simply belongs to two rows, two
columns and one (shared) box. Units with identical cell sets are merged,
which is what makes the redundancy of the 2011 middle grid visible.
Propagation crosses the seams by itself — there is no special case for
the overlaps anywhere in the solver.

The general core costs about 17% over the specialized 16x16 core on a
plain hexadoku (`go test -bench Cores`), which is why the single-grid
sizes keep theirs. `go test` cross-checks the two cores against each
other on all 54 reconstructed Elektor puzzles.

Samurai puzzles are recognized by their cell count and are written on a
character raster — one line per row, blanks where no grid covers the
position, so the blanks carry meaning:

    hexadoku.exe puzzles\elektor\2009-07_08.txt
    hexadoku.exe -gen hexamurai -count 1     # minimal, takes a while
    hexadoku.exe -gen samurai                # 9x9 version, ~0.3 s

**`cmd/muraiextract`** reads a Hexamurai out of a PDF. Unlike the normal
Hexadoku pages, where the puzzle is a graphic, these two pages carry
every clue as a text glyph, so the transcription is exact rather than
OCR: the glyph positions form one regular lattice across all five grids,
and fitting that lattice maps each glyph to its cell. The fit has to be
robust — the article text on the same page contains "0 to F" and "4x4",
which look exactly like clues — so the pitch is taken as the *median*
neighbour distance, the anchor as the median position, and only the block
of consecutive lattice lines holding the most glyphs is kept. The
arrangement itself is not assumed: every known one is tried at every
offset that fits, and one must place all clues on real cells without
breaking a rule.

    go build -o muraiextract.exe ./cmd/muraiextract
    muraiextract.exe -pdftotext <poppler>\pdftotext.exe -page 124 ^
                     -solver hexadoku.exe -o puzzles\elektor\2009-07_08.txt ^
                     elektor_pdfs\Elektornonlinear.ir2009-07_08.pdf

Build
-----

Requires Go (any recent version, amd64 recommended):

    go build -o hexadoku.exe .

Optionally `set GOAMD64=v3` before building to allow BMI/AVX codegen.

Reference build of the old solver: `g++ -O2 --std=c++11 cpp/hexadoku.cpp -o cpp/hexadoku_cpp.exe`

Usage
-----

    hexadoku.exe                       solve the built-in Elektor puzzle
    hexadoku.exe puzzles\extra3.txt    solve puzzle file(s)
    hexadoku.exe -unique file          also check solution uniqueness
    hexadoku.exe -bench 1000 file      timing statistics
    hexadoku.exe -gen 16 -count 3      generate minimal hexadoku puzzles
    hexadoku.exe -gen 9 -tries 300     generate hard minimal sudokus
    hexadoku.exe -gen samurai          generate a 9x9 samurai
    hexadoku.exe -gen hexamurai        generate a 16x16 hexamurai

Puzzle file format (auto-detected):

- 16x16: hex digits `0-9a-fA-F`, empty cells `.` `*` `_`, other
  characters ignored, `#` starts a comment line
- 9x9: digits `1-9`, empty cells `0` `.` `*` `_` (81 cells total)
- collections: one complete puzzle per line (e.g. Norvig's `top95.txt`)
- 16x16 letter format `a-p` (some puzzle collections) is auto-detected
- samurai: 1216 cells (hexamurai) or 369 cells (9x9 samurai) on a
  character raster, blanks between the grids

The generator (`-gen`) produces *minimal* puzzles: clues are removed in
random order while the solution stays unique, so in the result removing
any single clue would lose uniqueness. `-tries N` generates N candidates
and keeps the hardest (measured in branch nodes); `-seed` makes runs
reproducible.

Puzzles
-------

`puzzles/` contains test data:

- `easy/medium/extreme.txt` — from [fiskmoz/Hexadoku-Solver](https://github.com/fiskmoz/Hexadoku-Solver)
  (note: `medium.txt` has multiple solutions)
- `extra1-3.txt` — hard instances from [fedorkanin/hexadoku](https://github.com/fedorkanin/hexadoku)
- `supersudoku.txt` — from [MarcoVad/anydoku-sudoku-solver](https://github.com/MarcoVad/anydoku-sudoku-solver)
- `top95.txt`, `hardest.txt` — 9x9 benchmark collections from [norvig.com](https://norvig.com/sudoku.html)
- `inkala2012.txt` — Arto Inkala's "world's hardest sudoku" (2012)
- `generated_sudoku.txt`, `generated_hexadoku.txt`,
  `samurai_generated.txt`, `hexasamurai_generated.txt` — hard puzzles
  produced by `-gen` (this solver)
- `elektor/2009-07_08.txt`, `elektor/2011-07_08.txt` — the two genuine
  Elektor Hexamurai, read exactly from the PDF text layer
- `elektor_2012_06.txt` — the genuine Elektor Hexadoku from the
  June 2012 issue (p. 76), transcribed from the archive.org scan
  ([ElektorMagazine collection](https://archive.org/details/ElektorMagazine));
  `elektor_2012_03_solution.txt` is the March 2012 solution grid printed
  on the same page (transcription cross-checked: the grey cells spell
  the published prize key 78BE0)

More hexadokus: Elektor publishes one in every issue
(https://www.elektormagazine.com, search "Hexadoku").

Elektor archive tools
---------------------

Old Elektor issues (2006-2016, the Hexadoku era) are preserved on
archive.org. Two helper tools fetch and mine them:

**`cmd/elektordl`** downloads issues as PDF into `elektor_pdfs/`
(git-ignored, kept for manual checking):

    go build -o elektordl.exe ./cmd/elektordl
    elektordl.exe            # fetch 2006+ from the known archive.org items
    elektordl.exe -dry       # only list
    elektordl.exe -from 1980 -to 1985   # other years, -all for everything

**`cmd/hexextract`** pulls the puzzles out of the PDFs. The magazines
print the current puzzle as a *graphic* but the previous issue's
solution as *text*, so extraction works in two phases:

    hexextract.exe -pdftoppm <poppler>\pdftoppm.exe ^
                   -tesseract "C:\Program Files\Tesseract-OCR\tesseract.exe" ^
                   "elektor_pdfs\*.pdf"     # phase A, per issue
    hexextract.exe -chain                   # phase B, build puzzles

Phase A finds the Hexadoku page, renders it (poppler pdftoppm) and
detects the 17x17 grid lines geometrically, storing which cells carry a
clue (the *mask*). The printed previous-issue solution comes from the
PDF text layer on born-digital issues; on scanned ones the same lattice
detection finds the dense grid and tesseract reads it.

Phase B pairs each mask with the solution that belongs to it. Elektor
prints a puzzle's solution about two issues later, but the lag varies,
so the pairing is found by *content*: the tesseract reading of a puzzle
identifies its solution by agreement, and the lag learned that way is
applied to the rest. OCR damage in a solution grid is repaired
automatically - misread characters violate the hexadoku constraints, so
the affected cells are blanked and recomputed by the solver, which is
provably correct whenever the remainder is uniquely solvable.

Validation is end-to-end (`validate_elektor.ps1`): every reconstructed
puzzle must be uniquely solvable *and* its solution must equal a
solution actually printed in some issue.

For anything the automation cannot resolve, `-crops` exports the
detected grids as readable PNG bands for visual transcription -
`crops.ps1` wraps it and looks the page number up in `extract.log`:

    .\crops.ps1 2009-01 2009-02 -Bands 8 -Out crops

Read the bands, write the grid into `puzzles/elektor/<issue>.txt` with
the marker `transcribed visually` in a comment line, and check it with
`hexadoku.exe -unique`. Files carrying that marker are never overwritten
by `-chain`. Two independent checks make such a transcription
trustworthy: a genuine magazine puzzle solves with zero branch nodes,
and the clue count should match the geometrically detected mask.

`status_elektor.ps1` prints what exists per issue and how each
reconstructed puzzle fared in validation. See `puzzles/elektor/`.
