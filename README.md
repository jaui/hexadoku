hexadoku
========

Hexadoku (16x16 Sudoku) solver in Go — solves classic 9x9 Sudoku, the
five-grid samurai layouts of both sizes (Elektor's *Hexamurai*), the six
grids on the faces of Elektor's *Hexadocube*, and the chequerboard rule of
its *EUC Penta-Hexadoku* as well.
Hexadokus were published as reader puzzles in the Elektor magazine:
each row, column and 4x4 box must contain the hex digits 0-F exactly once.

The original C++ brute-force/constraint solver is kept for reference in
`cpp/hexadoku.cpp`.

A write-up of every problem hit while building this and how it was
solved is in [`docs/werkstattbericht.html`](docs/werkstattbericht.html)
(German): solver data layout, the PDF toolchain dead end, grid detection
in scans, how the puzzle/solution pairing was derived from the data, and
the two Hexamurai arrangements.

Five more pages in `docs/` (German) visualise the special puzzles Elektor
printed instead of, or alongside, a plain hexadoku — the geometry of each
one drawn from the very files in `puzzles/` that the solver reads:
[`hexamurai.html`](docs/hexamurai.html) (the two five-grid arrangements),
[`hexadocube.html`](docs/hexadocube.html) (six faces, with a fold slider
from the printed net to the cube), [`penta-hexadoku.html`](docs/penta-hexadoku.html)
(the chequerboard rule), [`digest.html`](docs/digest.html) (the sixteen
prize codes and where they land) and
[`fremdformate.html`](docs/fremdformate.html) (the 36x36 Alphanumski and
the 25x25 AlphaSudoku, both out of reach). [`docs/index.html`](docs/index.html)
links them all.

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
| `murai.go` | the five-grid arrangements, the chequerboard, CLI glue |
| `cube.go` | the six faces of the hexadocube, folded up in 3D |
| `digest.go` | the block/code assignment layer of the 2011 digest |
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

Hexadocube
----------

The double issue 7-8/2010 printed a **Hexadocube** (same designer): six
hexadokus on the faces of a cube, drawn unfolded across pages 60-61 — the
article calls it the development.

                                +----+
                                | 4  |     the four grids of the middle
       +----+----+----+----+----+----+     row wrap around the cube,
       | 0  | 1  | 2  | 3  |               face 4 is its top and face 5
       +----+----+----+----+----+          its bottom
                                | 5  |
                                +----+

The faces are linked by the cells lying on the cube edge between them —
`cases` in the designer's French, which the English text renders as
"boxes". Such a cell is one cell seen from two sides and is drawn once on
each face, so both drawings must carry the same value; the magazine
states the rule that way ("a character found on face 1 along the edge
with face 2 will have to be copied across into the box on face 2 on the
other side of the boundary"). At each of the eight cube corners three
faces meet and three drawn cells coincide. The whole border ring is left
empty in the printed puzzle, which is why a single face is not a hexadoku
with 84 clues but a fragment.

Rather than tabulate those twelve identifications and the orientation of
each, `cube.go` folds the net up: every face gets an origin and two axes
as integer 3D vectors, propagated from its neighbour in the development
by a quarter turn about the shared edge, and cell `(r, c)` of a face
lands on the lattice point `O + r*V + c*U` on the surface of a cube of
side 15. Cells that end up at the same point *are* the same cell — the
identification falls out of the coordinates, in the right orientation,
with nothing to get wrong per edge:

| | cells | drawn | units |
| --- | --- | --- | --- |
| interior of a face | 6·14·14 = 1176 | once | 3 |
| on a cube edge | 12·14 = 168 | twice | 5 |
| a cube corner | 8 | three times | 6 |
| **total** | **1352** | 1536 characters | **276** |

The 288 units of six grids collapse to 276 for the same reason: a
boundary line of one face *is* a line of its neighbour, one unit declared
twice, and `addUnit` merges them — the same dedup that merges the shared
boxes of a samurai. A cube corner lies in six units and not nine, because
the three lines meeting there are each shared by two of the three faces,
so `maxCellUnits` stays at 6.

A generated hexadocube (`-gen hexadocube`, in
`puzzles/hexadocube_generated.txt`) has 431 clues and solves in 197 ms
over 10 844 branch nodes.

EUC Penta-Hexadoku
------------------

The double issue 7-8/2012 printed an **EUC Penta-Hexadoku**, again by
Ghyselen, and its geometry turns out to be nothing new: measured off the
page, the five grid corners are (0,8), (8,24), (16,0), (12,12), (24,16)
on a 40x40 raster — exactly the pinwheel of the 2009 Hexamurai. What is
new is one rule on top of the hexadoku ones, and it is not a unit at all:

> the even numbers (0,2,4,6,8,A,C,E) only appear on a coloured
> background, while the uneven numbers (1,3,5,7,9,B,D,F) only appear in
> white squares

— a chequerboard, hence EUC for even/uneven chequerboard. Put the other
way round, no two neighbouring cells may hold values of the same parity.

That is a *per-cell* restriction, so it does not fit the unit
abstraction. It became one array, `variant.allow`, which `cand` masks
with instead of the constant `all`; for every other layout the array
holds `all`, so the rule costs one load in place of one immediate and
nothing else. Every unit has eight coloured and eight white cells, so the
rule is satisfiable, and it halves the candidates of every cell before
any propagation runs.

The colouring is consistent over the overlaps because all five origins
are even: a cell is coloured exactly when row+column is even. That phase
was read off the page render — 465 coloured cells, no exception — and
agrees with the 4x4 box the article fills in "to illustrate the
arrangement of the even/uneven chequerboard".

Since the geometry is shared with the hexamurai, the cell count cannot
tell the two apart, and a file asks for the layout by name:

    # variant: penta-hexadoku

`puzzles/penta_generated.txt` shows what the rule is worth: its 193 clues
solve uniquely in 39 ms as a penta-hexadoku, and the same 193 clues read
as a plain hexamurai are ambiguous.

Hexadoku 'Digest'
-----------------

Issue 3/2011 printed a **Hexadoku 'Digest'**: an ordinary 16x16 grid with
73 clues and sixteen marked horizontal blocks of five cells, and one rule
that is not a rule of sudoku at all — the blocks take the prize codes of
sixteen earlier Hexadokus, one code each. The article names the set and
no more:

> Each block takes the solution of a Hexadoku puzzle that appeared in one
> of 16 editions of Elektor magazine in the period January 2009 to June
> 2010. The July & August 2009 double edition is excluded.

That double issue carried the Hexamurai, which has no five-cell code, so
sixteen monthly puzzles remain for sixteen blocks. Which block belongs to
which month is *not* given — the bijection is part of the puzzle, and the
extra knowledge is an unordered set of sixteen strings.

All sixteen are recoverable from the archive. Elektor announces each code
two issues later in the "Prize winners" box, and every one of them also
turns up as five consecutive cells in the solution this repo reconstructed
for that issue, which cross-checks the two readings against each other
(two announcements come out of the scan damaged — `DFBl 2` and `68310'` —
and the reconstructions settle them as `DFB12` and `6B310`).

A five-cell block matching a whole given string does not fit the unit
abstraction, so `digest.go` puts one more layer of search around the
ordinary one: pick the block with the fewest codes still fitting, try
each, and let propagation between two assignments do the rest — the same
MRV idea as the cell search, one level up. It is a strong filter, and the
puzzle falls in **16 branch nodes**.

Three things confirm the result:

- the same 73 clues *without* the block rule have more than one solution,
  exactly as the article claims;
- the solved grid agrees with the solution Elektor printed in 5/2011 in
  237 of 256 cells, the difference being two rows where the OCR of that
  scan dropped characters;
- the grey answer cells give `9302F`, and the printed grid shows `9302F`
  in the same place.

`puzzles/elektor/2011-03.txt` carries the whole thing in its header:

    # variant: digest
    # codes: 4395C 3097D 813D2 ... C81BA 6B310
    # block: r1c2-c6
    # block: r2c8-c12
    ...

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
    hexadoku.exe -gen penta            generate an EUC penta-hexadoku
    hexadoku.exe -gen hexadocube       generate a hexadocube

Puzzle file format (auto-detected):

- 16x16: hex digits `0-9a-fA-F`, empty cells `.` `*` `_`, other
  characters ignored, `#` starts a comment line
- 9x9: digits `1-9`, empty cells `0` `.` `*` `_` (81 cells total)
- collections: one complete puzzle per line (e.g. Norvig's `top95.txt`)
- 16x16 letter format `a-p` (some puzzle collections) is auto-detected
- samurai: 1216 cells (hexamurai) or 369 cells (9x9 samurai) on a
  character raster, blanks between the grids
- hexadocube: 1536 characters on a 64x48 raster — six 16x16 faces, the
  shared cells drawn once per face
- a `# variant: <name>` header line overrides the cell count, which is
  what tells an `euc-penta-hexadoku` from the `hexamurai` it shares its
  geometry with (`hexamurai`, `hexamurai-plus`, `samurai`,
  `hexa-samurai`, `penta-hexadoku`, `hexadocube`, `digest`)
- `digest` additionally reads `# codes:` and `# block: r<row>c<a>-c<b>`
  lines from the same header

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
  `samurai_generated.txt`, `hexasamurai_generated.txt`,
  `penta_generated.txt`, `hexadocube_generated.txt` — hard puzzles
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

Scans need four things a born-digital page does not:

- **Deskew** (`deskew.go`). Half a degree off square is invisible in
  print but fatal for a detector looking for the longest dark run inside
  one pixel row. The angle comes from the sharpness of the horizontal ink
  profile — scored *smoothed*, because only at zero degrees is the shift
  exactly nothing, and an unsmoothed score hands "do not rotate" a bonus
  no real angle can earn.
- **A rigid comb** (`bestComb`). Some of the 17 row lines print too faint
  for any per-line threshold. A hexadoku cell is square, so the row pitch
  must equal the column pitch of the lattice already found: place the 17
  rows as one comb and search only its offset, scored by its second
  weakest line.
- **Thresholds fitted to the page** (`raiseThresholds`, Otsu) when the
  fixed ones find no ink at all, and a **tiled retry** for pages so bowed
  that no single angle squares them.
- **`-page N`** to work from a page identified elsewhere, for scans whose
  text layer is too poor for the sweep.

Where automation still fails, `-crops` exports magnified bands for
reading by eye; five issues were recovered that way, and the masks
re-detected afterwards match those readings cell for cell.

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
solution actually printed in some issue. Current state over 110 PDFs:
**67 verified against a printed solution, 15 uniquely solvable but
without one to compare to, 4 documented special formats.**

Uniqueness alone is not enough to trust a reconstruction. A faint
printed *solution* grid can lose enough cells to the ink probe to pose as
a puzzle — and then validate, because a subset of the right solution is
uniquely solvable and matches what was printed. Two issues had passed
that way. Real Elektor puzzles hold 104 to 144 clues, so a mask past 160
is now rejected outright.

The July/August double issues are mostly not hexadokus at all, and each
is documented as such in `puzzles/elektor/<issue>.txt`:

| issue | what it prints |
| --- | --- |
| 7-8/2007 | *Alphanumski*, overlapping grids over a much larger symbol set |
| 7-8/2008 | *AlphaSudoku*, 25x25 |
| 7-8/2009 | *Hexamurai*, five grids in a pinwheel (solved, see above) |
| 7-8/2010 | *Hexadocube*, six grids on the faces of an unfolded cube |
| 7-8/2011 | *Hexamurai*, five grids in a plus (solved, see above) |
| 3/2011 | *Hexadoku Digest*, needs prize codes from 18 earlier issues |

Two English originals are damaged on archive.org and stay damaged after
re-downloading (6/2011 is mostly null bytes, 12/2014 has a broken xref).
The Dutch edition carries the same puzzle: `Elektuur2011-06.pdf` and
`Elektuur2014-12.pdf` were fetched from the `elektuur-572-2011-6_202005`
and `elektuur-614-2014-12` items and read cleanly.

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
