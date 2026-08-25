# Reports, per Elektor issue, what was extracted and whether the
# reconstructed puzzle passes validation. Used to pick the cases that
# still need visual transcription.
param([string]$Dir = "puzzles\elektor")

$pages = @{}
foreach ($l in Get-Content "$Dir\extract.log") {
    if ($l -match '^ok\s+(\S+):\s+p\.(\d+)') { $pages[$Matches[1]] = [int]$Matches[2] }
}
$dropped = @{}
foreach ($l in Get-Content "$Dir\chain.log" -ErrorAction SilentlyContinue) {
    if ($l -match '^drop (\S+) solution: (\d+) damaged') { $dropped[$Matches[1]] = [int]$Matches[2] }
}
$verdict = @{}
foreach ($l in Get-Content "$Dir\validate.log" -ErrorAction SilentlyContinue) {
    if ($l -match '^(PASS|WARN|FAIL) (\S+)') { $verdict[$Matches[2]] = $Matches[1] }
}

$keys = Get-ChildItem "$Dir\*_mask.txt", "$Dir\*_prevsolution.txt", "$Dir\*_ocr.txt" -ErrorAction SilentlyContinue |
    ForEach-Object { $_.BaseName -replace '_(mask|prevsolution|ocr)$', '' } | Sort-Object -Unique

$rows = foreach ($k in $keys) {
    [pscustomobject]@{
        Issue    = $k
        Page     = $(if ($pages.ContainsKey($k)) { $pages[$k] } else { '' })
        Mask     = $(if (Test-Path "$Dir\${k}_mask.txt") { 'x' } else { '' })
        Solution = $(if ($dropped.ContainsKey($k)) { "drop($($dropped[$k]))" } elseif (Test-Path "$Dir\${k}_prevsolution.txt") { 'x' } else { '' })
        Ocr      = $(if (Test-Path "$Dir\${k}_ocr.txt") { 'x' } else { '' })
        Puzzle   = $(if ($verdict.ContainsKey($k)) { $verdict[$k] } elseif (Test-Path "$Dir\$k.txt") { '?' } else { '-' })
    }
}
$rows | Format-Table -AutoSize
Write-Host ""
Write-Host ("Hefte: {0} | Masken: {1} | Lösungen ok: {2} | Lösungen verworfen: {3}" -f `
    $rows.Count, ($rows | Where-Object Mask).Count, ($rows | Where-Object { $_.Solution -eq 'x' }).Count, $dropped.Count)
Write-Host ("Rätsel: PASS {0} | WARN {1} | FAIL {2} | kein Rätsel {3}" -f `
    ($rows | Where-Object { $_.Puzzle -eq 'PASS' }).Count, ($rows | Where-Object { $_.Puzzle -eq 'WARN' }).Count,
    ($rows | Where-Object { $_.Puzzle -eq 'FAIL' }).Count, ($rows | Where-Object { $_.Puzzle -eq '-' }).Count)
