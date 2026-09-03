# End-to-end validation of reconstructed Elektor puzzles:
#  1. every puzzle must be uniquely solvable
#  2. its solution should equal a solution printed in a later issue.
#     Where that grid was too OCR-damaged to use for reconstruction, the
#     comparison still works as evidence: agreement far above chance
#     confirms both sides, and the few differing cells are the OCR slips.
#
# Deliberately sequential. Checking the puzzles side by side was tried and
# taken out again: most of a run sits in the uniqueness proof of the 2011
# hexamurai alone and most of the rest in the alfadoku, while the other
# 106 puzzles together need about four seconds. Best of three runs on
# twelve cores was 57.0 s sequential against 48.7 s parallel - a factor
# of 1.17, with a run-to-run spread nearly as large as the gain, and not
# worth the runspace machinery. What did help was the solver: -unique now
# searches the tree once instead of twice, and propagation looks only at
# units an assignment touched - a full run went from 55 s to 17 s.
param([string]$Dir = "puzzles\elektor")

function Get-GridLines([string]$path) {
    Get-Content $path | Where-Object { $_ -match '^[0-9A-F.#]{16}$' }
}

# index all printed solutions by exact content, and keep them for fuzzy
# comparison as well
$exact = @{}
$grids = @{}
foreach ($s in Get-ChildItem "$Dir\*_prevsolution.txt") {
    $lines = Get-GridLines $s.FullName
    $key = $s.BaseName -replace '_prevsolution', ''
    $grids[$key] = $lines
    $exact[($lines -join "`n")] = $key
}

function Best-Match([string[]]$mine) {
    $bestKey = ''; $bestHits = 0
    foreach ($k in $grids.Keys) {
        $g = $grids[$k]
        if ($g.Count -ne 16) { continue }
        $hits = 0
        for ($r = 0; $r -lt 16; $r++) {
            for ($c = 0; $c -lt 16; $c++) {
                if ($mine[$r][$c] -eq $g[$r][$c]) { $hits++ }
            }
        }
        if ($hits -gt $bestHits) { $bestHits = $hits; $bestKey = $k }
    }
    return @($bestKey, $bestHits)
}

$puzzles = Get-ChildItem "$Dir\20??-*.txt" |
    Where-Object { $_.Name -match '^\d{4}-[\d_]+\.txt$' } | Sort-Object Name
$pass = 0; $warn = 0; $fail = 0; $special = 0
foreach ($p in $puzzles) {
    $key = $p.BaseName
    $raw = Get-Content $p.FullName -Raw
    if ($raw -match 'not a standalone puzzle') {
        Write-Host "SPEC $key (special format, not a single 16x16 grid)"
        $special++
        continue
    }
    # The wide layouts of the summer double issues 7-8/2006, 7-8/2007 and
    # 7-8/2008: 25 and 36 values, so a solver core of their own, and no
    # printed solution grid anywhere to compare against. What settles them
    # is the code the magazine announced for the grey answer cells, and
    # the solver checks that itself - it knows the alphabet, and the codes
    # are six and seven characters over it rather than five hex digits, so
    # the claim regex further down cannot read them.
    if ($raw -match '(?m)^\s*#\s*variant:\s*(alphanumski|alphasudoku|alfadoku|alphadoku)') {
        $layout = $Matches[1]
        $u = .\hexadoku.exe -unique $p.FullName 2>$null
        $hit = $u | Select-String 'grey cells .* give (\S+), the code the magazine announced'
        if (-not $hit) {
            Write-Host "FAIL $key : $layout does not reproduce the announced code"
            $fail++; continue
        }
        $code = $hit.Matches[0].Groups[1].Value
        if ($u | Select-String 'solution is unique') {
            Write-Host "PASS $key ($layout, uniquely solvable; the grey cells give $code, the code the magazine announced)"
            $pass++; continue
        }
        # The Alphanumski has two solutions as printed. They differ in
        # four cells, none of them grey, so the answer is still the one
        # Elektor announced - which is what the solver reports here.
        if ($u | Select-String 'grey cells are the same in every solution') {
            Write-Host "PASS $key ($layout, more than one solution as printed, but every one gives $code in the grey cells - the code the magazine announced)"
            $pass++; continue
        }
        Write-Host "FAIL $key : $layout leaves the grey cells ambiguous"
        $fail++; continue
    }

    # More than one grid on a wide raster - hexamurai, hexadocube, penta.
    # There is no printed solution grid to compare against, so uniqueness
    # is the whole check: a wrong arrangement would not be uniquely
    # solvable. (The 2011 hexamurai needs some ten seconds for the
    # proof.) The test is the cell count the solver itself goes by, not
    # the word "hexamurai" appearing somewhere in the header - 2009-10 is
    # a plain hexadoku whose comment only mentions the hexamurai printed
    # beside it, and used to be counted as one.
    $cells = 0
    foreach ($l in Get-Content $p.FullName) {
        if ($l -match '^\s*#') { continue }
        $cells += ([regex]::Matches($l, '[0-9A-Fa-f.*_]')).Count
    }
    if ($cells -gt 256) {
        $u = .\hexadoku.exe -unique $p.FullName 2>$null
        $layout = 'multi-grid'
        if (($u -join ' ') -match '\(([^,]+), \d+ cells') { $layout = $Matches[1] }
        if ($u | Select-String 'solution is unique') {
            Write-Host "PASS $key ($layout, uniquely solvable)"
            $pass++
        } else {
            Write-Host "FAIL $key : $layout not uniquely solvable"
            $fail++
        }
        continue
    }
    $mine = .\hexadoku.exe -compact $p.FullName 2>$null | Where-Object { $_ -match '^[0-9A-F]{16}$' }
    if ($LASTEXITCODE -ne 0 -or $mine.Count -ne 16) {
        Write-Host "FAIL $key : not solvable"; $fail++; continue
    }
    if (.\hexadoku.exe -unique $p.FullName 2>$null | Select-String 'not unique') {
        Write-Host "FAIL $key : multiple solutions"; $fail++; continue
    }
    # Before 11/2007 no solution grid was printed at all, only the five
    # characters of the grey answer cells, announced two issues later.
    # Where the reading reproduces that code it is proven just as firmly
    # as by a solution grid - the code names five specific cells out of
    # 256, so agreement by chance is about one in a million.
    # The claim may be worded freely, as long as the code follows the cell
    # range within 80 characters - it often sits on the next comment line.
    # Only the comment lines are searched, and only upper case counts, so
    # neither the grid below nor a lower case word such as "faded" can
    # pose as a code. A claim is checked whenever one is present, even if
    # a solution grid already settles the puzzle: the two corroborate each
    # other, and a typo in the header stays visible either way.
    $header = (Get-Content $p.FullName |
        Where-Object { $_ -match '^\s*#' }) -join ' '
    $claim = $false; $code = ''; $got = ''; $headerError = ''
    if ($header -cmatch 'r(\d+)c(\d+)\s*-\s*c(\d+).{0,80}?\b([0-9A-F]{5})\b') {
        $claim = $true
        $row = [int]$Matches[1]; $col = [int]$Matches[2]
        $end = [int]$Matches[3]; $code = $Matches[4]
        if ($row -lt 1 -or $row -gt 16 -or $col -lt 1 -or $end -gt 16 -or
            $end -ne $col + 4) {
            $headerError = "the claimed range r${row}c$col-c$end is not five cells inside the grid"
        } else {
            $got = $mine[$row - 1].Substring($col - 1, 5)
        }
    }
    if ($headerError) {
        Write-Host "FAIL $key : $headerError"; $fail++; continue
    }
    if ($claim -and $got -ne $code) {
        Write-Host "FAIL $key : grey cells r${row}c$col give $got, but the header claims the announced code is $code"
        $fail++; continue
    }
    $confirm = ''
    if ($claim) {
        $confirm = "; the grey cells give $code, the prize code the magazine announced"
    }
    $joined = $mine -join "`n"
    if ($exact.ContainsKey($joined)) {
        Write-Host "PASS $key (unique, solution printed in issue $($exact[$joined])$confirm)"
        $pass++
        continue
    }
    $m = Best-Match $mine
    if ($m[1] -ge 200) {
        Write-Host ("PASS $key (unique; matches the OCR-damaged solution grid of {0} in {1}/256 cells{2})" -f $m[0], $m[1], $confirm)
        $pass++
        continue
    }
    if ($claim) {
        Write-Host "PASS $key (unique$confirm)"
        $pass++
        continue
    }
    Write-Host "WARN $key (unique, but no printed solution matches - unverified)"
    $warn++
}
Write-Host "validation: $pass verified, $warn unverified, $fail fail, $special special format"
