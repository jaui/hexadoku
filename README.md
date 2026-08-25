hexadoku
========

Hexadoku (16x16 Sudoku) solver in Go — solves classic 9x9 Sudoku too.
Hexadokus were published as reader puzzles in the Elektor magazine:
each row, column and 4x4 box must contain the hex digits 0-F exactly once.

The original C++ brute-force/constraint solver is kept for reference in
`cpp/hexadoku.cpp`.

A write-up of every problem hit while building this and how it was
solved is in [`docs/werkstattbericht.html`](docs/werkstattbericht.html)
(German): solver data layout, the PDF toolchain dead end, grid detection
in scans, and how the puzzle/solution pairing was derived from the data.

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

Puzzle file format (auto-detected):

- 16x16: hex digits `0-9a-fA-F`, empty cells `.` `*` `_`, other
  characters ignored, `#` starts a comment line
- 9x9: digits `1-9`, empty cells `0` `.` `*` `_` (81 cells total)
- collections: one complete puzzle per line (e.g. Norvig's `top95.txt`)
- 16x16 letter format `a-p` (some puzzle collections) is auto-detected

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
- `generated_sudoku.txt`, `generated_hexadoku.txt` — hard minimal
  puzzles produced by `-gen` (this solver)
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
