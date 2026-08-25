# End-to-end validation of reconstructed Elektor puzzles:
#  1. every puzzle must be uniquely solvable
#  2. the solver's solution should equal one of the solutions printed
#     in a later issue (closed loop; the pairing is not assumed but
#     found by comparing full grids)
param([string]$Dir = "puzzles\elektor")

function Get-GridText([string]$path) {
    (Get-Content $path | Where-Object { $_ -match '^[0-9A-F.#]{16}$' }) -join "`n"
}

# index all printed solutions by content
$solutions = @{}
foreach ($s in Get-ChildItem "$Dir\*_prevsolution.txt") {
    $solutions[(Get-GridText $s.FullName)] = $s.BaseName -replace '_prevsolution', ''
}

$puzzles = Get-ChildItem "$Dir\20??-*.txt" |
    Where-Object { $_.Name -match '^\d{4}-[\d_]+\.txt$' } | Sort-Object Name
$pass = 0; $warn = 0; $fail = 0
foreach ($p in $puzzles) {
    $key = $p.BaseName
    $solved = (.\hexadoku.exe -compact $p.FullName 2>$null | Where-Object { $_ -match '^[0-9A-F]{16}$' }) -join "`n"
    if ($LASTEXITCODE -ne 0 -or -not $solved) {
        Write-Host "FAIL $key : not solvable"; $fail++; continue
    }
    if (.\hexadoku.exe -unique $p.FullName 2>$null | Select-String 'not unique') {
        Write-Host "FAIL $key : multiple solutions"; $fail++; continue
    }
    if ($solutions.ContainsKey($solved)) {
        Write-Host "PASS $key (unique, solution printed in issue $($solutions[$solved]))"
        $pass++
    } else {
        Write-Host "WARN $key (unique, but no printed solution matches - unverified)"
        $warn++
    }
}
Write-Host "validation: $pass verified, $warn unverified, $fail fail"
