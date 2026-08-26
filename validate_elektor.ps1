# End-to-end validation of reconstructed Elektor puzzles:
#  1. every puzzle must be uniquely solvable
#  2. its solution should equal a solution printed in a later issue.
#     Where that grid was too OCR-damaged to use for reconstruction, the
#     comparison still works as evidence: agreement far above chance
#     confirms both sides, and the few differing cells are the OCR slips.
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
    if ((Get-Content $p.FullName -Raw) -match 'not a standalone puzzle') {
        Write-Host "SPEC $key (special format, not a single 16x16 grid)"
        $special++
        continue
    }
    $mine = .\hexadoku.exe -compact $p.FullName 2>$null | Where-Object { $_ -match '^[0-9A-F]{16}$' }
    if ($LASTEXITCODE -ne 0 -or $mine.Count -ne 16) {
        Write-Host "FAIL $key : not solvable"; $fail++; continue
    }
    if (.\hexadoku.exe -unique $p.FullName 2>$null | Select-String 'not unique') {
        Write-Host "FAIL $key : multiple solutions"; $fail++; continue
    }
    $joined = $mine -join "`n"
    if ($exact.ContainsKey($joined)) {
        Write-Host "PASS $key (unique, solution printed in issue $($exact[$joined]))"
        $pass++
        continue
    }
    $m = Best-Match $mine
    if ($m[1] -ge 200) {
        Write-Host ("PASS $key (unique; matches the OCR-damaged solution grid of {0} in {1}/256 cells)" -f $m[0], $m[1])
        $pass++
    } else {
        Write-Host "WARN $key (unique, but no printed solution matches - unverified)"
        $warn++
    }
}
Write-Host "validation: $pass verified, $warn unverified, $fail fail, $special special format"
