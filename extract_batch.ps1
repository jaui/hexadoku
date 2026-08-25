# Runs hexextract over every downloaded Elektor PDF, one process per
# file with a hard kill on timeout (some malformed PDFs drive the PDF
# parser into an allocation loop). Resumable: files whose outputs
# already exist are skipped.
param(
    [string]$PdfDir = "elektor_pdfs",
    [string]$OutDir = "puzzles\elektor",
    [string]$Log = "puzzles\elektor\extract.log",
    [int]$TimeoutSec = 90
)
$pp = "C:\Users\jaui\AppData\Local\Microsoft\WinGet\Packages\oschwartz10612.Poppler_Microsoft.Winget.Source_8wekyb3d8bbwe\poppler-25.07.0\Library\bin\pdftoppm.exe"
$tess = "C:\Program Files\Tesseract-OCR\tesseract.exe"
$env:GOMEMLIMIT = "1GiB"

foreach ($f in (Get-ChildItem "$PdfDir\*.pdf" | Sort-Object Name)) {
    $key = if ($f.Name -match '(20\d\d)[-_ ]?([0-9][0-9,_]*)') {
        "$($Matches[1])-$($Matches[2] -replace ',','_')"
    } else { $f.BaseName }
    if ((Test-Path "$OutDir\${key}_mask.txt") -or (Test-Path "$OutDir\${key}_prevsolution.txt")) {
        continue
    }
    $t = [datetime]::Now
    $out = "$env:TEMP\hexextract_out.txt"
    $p = Start-Process -FilePath ".\hexextract.exe" `
        -ArgumentList @("-pdftoppm", "`"$pp`"", "-tesseract", "`"$tess`"", "`"$($f.FullName)`"") `
        -NoNewWindow -PassThru -RedirectStandardOutput $out
    $deadline = [datetime]::Now.AddSeconds($TimeoutSec)
    while (-not $p.HasExited -and [datetime]::Now -lt $deadline) {
        Start-Sleep -Milliseconds 500
        # emergency brake well before the box starts swapping
        $p.Refresh()
        if ($p.WorkingSet64 -gt 3GB) { break }
    }
    if (-not $p.HasExited) {
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        "HANG $($f.Name) [killed]" | Add-Content $Log
        continue
    }
    $secs = [int]([datetime]::Now - $t).TotalSeconds
    Get-Content $out -ErrorAction SilentlyContinue | Where-Object { $_ -match '^(ok|FAIL)' } | ForEach-Object {
        "$_ [${secs}s]" | Add-Content $Log
    }
}
"BATCH-DONE" | Add-Content $Log
