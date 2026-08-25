# Exports the grid crops of one or more Elektor issues for visual
# transcription. The page number is taken from extract.log.
#   .\crops.ps1 2008-09 2009-01
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Issues,
      [int]$Bands = 4,
      [string]$Out = "crops")

$pp = "C:\Users\jaui\AppData\Local\Microsoft\WinGet\Packages\oschwartz10612.Poppler_Microsoft.Winget.Source_8wekyb3d8bbwe\poppler-25.07.0\Library\bin\pdftoppm.exe"

$pages = @{}
foreach ($l in Get-Content "puzzles\elektor\extract.log") {
    if ($l -match '^ok\s+(\S+):\s+p\.(\d+)') { $pages[$Matches[1]] = [int]$Matches[2] }
}

foreach ($issue in $Issues) {
    $pdf = Get-ChildItem "elektor_pdfs\*.pdf" | Where-Object {
        $k = if ($_.Name -match '(20\d\d)[-_ ]?([0-9][0-9,_]*)') { "$($Matches[1])-$($Matches[2] -replace ',','_')" } else { '' }
        $k -eq $issue
    } | Select-Object -First 1
    if (-not $pdf) { Write-Host "kein PDF für $issue"; continue }
    $page = if ($pages.ContainsKey($issue)) { $pages[$issue] } else { -1 }
    Remove-Item "$Out\$issue*" -ErrorAction SilentlyContinue
    & .\hexextract.exe -crops $page -bands $Bands -out $Out -pdftoppm $pp $pdf.FullName |
        Where-Object { $_ -match 'lattice|auto-detected|no hexadoku' } |
        ForEach-Object { "$issue (p.$page): $_" }
}
