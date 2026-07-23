# release.ps1 — Phat hanh ban cap nhat TU DONG cho TrafficTool.

param(
    [ValidateSet("patch", "minor", "major")]
    [string]$Bump,

    [string]$Tag,

    [string]$Dist = ".\release_dist",

    [switch]$SkipBuild,

    [switch]$NoGit
)

$ErrorActionPreference = "Stop"
$repo = "nguyenhung2203/ToolCatVideo"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

# --- 0. Doc phien ban hien tai trong update.go -------------------------------
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$updatePath = (Resolve-Path ".\update.go").Path
$updateGo = [System.IO.File]::ReadAllText($updatePath, [System.Text.Encoding]::UTF8)
$curVer = ""
if ($updateGo -match 'CurrentAppVersion\s*=\s*"([^"]+)"') { $curVer = $Matches[1] }

if (-not $curVer) {
    Write-Host "LOI: Khong doc duoc CurrentAppVersion trong update.go." -ForegroundColor Red
    exit 1
}

if ($Bump) {
    $numPart = $curVer.TrimStart('v', 'V')
    $parts = $numPart.Split('.')
    while ($parts.Count -lt 3) { $parts += "0" }
    [int]$maj = $parts[0]; [int]$min = $parts[1]; [int]$pat = $parts[2]
    switch ($Bump) {
        "major" { $maj++; $min = 0; $pat = 0 }
        "minor" { $min++; $pat = 0 }
        "patch" { $pat++ }
    }
    $Tag = "v$maj.$min.$pat"
    Write-Host "=== TrafficTool Release: $curVer -> $Tag (bump $Bump) ===" -ForegroundColor Cyan
}
elseif ($Tag) {
    Write-Host "=== TrafficTool Release: $Tag ===" -ForegroundColor Cyan
    if ($curVer -and $curVer -ne $Tag) {
        Write-Host "LOI: Tag ($Tag) KHAC CurrentAppVersion trong update.go ($curVer)." -ForegroundColor Red
        exit 1
    }
}
else {
    Write-Host "LOI: Phai truyen -Bump patch|minor|major HOAC -Tag vX.Y.Z." -ForegroundColor Red
    exit 1
}

# --- 1. KIEM TRA TIEN DIEU KIEN (Pre-flight Checks) - CHUA SUA CODE ------------
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Program Files\GitHub CLI\gh.exe") {
        $env:PATH += ";C:\Program Files\GitHub CLI"
    }
}
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    Write-Host "LOI: Chua cai GitHub CLI (gh). Tai tai https://cli.github.com/ roi chay 'gh auth login'." -ForegroundColor Red
    Write-Host "     (Phien ban trong code GIU NGUYEN $curVer do phat hanh chua thuc hien)." -ForegroundColor Yellow
    exit 1
}

if (-not (Test-Path $Dist)) {
    Write-Host "LOI: Khong tim thay thu muc ship '$Dist'." -ForegroundColor Red
    Write-Host "     Lan dau, hay tao no va dat vao DUNG bo app ship cho khach:" -ForegroundColor Yellow
    Write-Host "       $Dist\TrafficTool.exe" -ForegroundColor Yellow
    Write-Host "       $Dist\bin\ffmpeg.exe, ffprobe.exe, worker.exe, yt-dlp.exe" -ForegroundColor Yellow
    Write-Host "     (Phien ban trong code GIU NGUYEN $curVer do phat hanh chua thuc hien)." -ForegroundColor Yellow
    exit 1
}

# Ham khoi phuc lai version cu neu tien trinh phia sau loi
function Revert-VersionCode {
    if ($curVer -and ($curVer -ne $Tag)) {
        Write-Host "     Khoi phuc CurrentAppVersion = '$curVer' trong update.go..." -ForegroundColor Yellow
        $curContent = [System.IO.File]::ReadAllText($updatePath, [System.Text.Encoding]::UTF8)
        $revertContent = $curContent -replace '(CurrentAppVersion\s*=\s*")[^"]+(")', "`${1}$curVer`${2}"
        [System.IO.File]::WriteAllText($updatePath, $revertContent, $utf8NoBom)
    }
}

# --- 2. GHI VERSION MOI VAO update.go ------------------------------------
if ($curVer -ne $Tag) {
    $newContent = $updateGo -replace '(CurrentAppVersion\s*=\s*")[^"]+(")', "`${1}$Tag`${2}"
    [System.IO.File]::WriteAllText($updatePath, $newContent, $utf8NoBom)
    Write-Host "      Da cap nhat CurrentAppVersion = '$Tag' vao update.go" -ForegroundColor Gray
}

# --- 3. Build ung dung ----------------------------------------------------
if (-not $SkipBuild) {
    Write-Host "`n[1/6] Dang build ung dung (wails build)..." -ForegroundColor Green
    wails build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "LOI: wails build that bai." -ForegroundColor Red
        Revert-VersionCode
        exit 1
    }
}

# --- 4. Chuan bi file ship TrafficTool.exe ------------------------------
$builtExe = ".\build\bin\TrafficTool.exe"
if (Test-Path $builtExe) {
    try {
        Copy-Item $builtExe (Join-Path $Dist "TrafficTool.exe") -Force -ErrorAction Stop
    } catch {
        Get-Process TrafficTool -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500
        Copy-Item $builtExe (Join-Path $Dist "TrafficTool.exe") -Force
    }
    Write-Host "[2/6] Da chep TrafficTool.exe moi vao $Dist" -ForegroundColor Green
} elseif (-not $SkipBuild) {
    Write-Host "LOI: Khong thay $builtExe sau khi build." -ForegroundColor Red
    Revert-VersionCode
    exit 1
}

# --- 5. Quet de quy Dist, tinh SHA256 ------------------------------------
Write-Host "`n[3/6] Dang quet thu muc ship va tinh SHA256..." -ForegroundColor Green
$distFull = (Resolve-Path $Dist).Path
$files = Get-ChildItem -Path $distFull -Recurse -File | Where-Object {
    # Bo qua file rac cap nhat + manifest.json (chinh script ghi vao Dist o buoc sau;
    # neu khong loai, lan chay thu 2 se quet nham manifest cu thanh 1 file cua app).
    $_.Name -notlike "*.old" -and $_.Name -notlike "*.new" -and $_.Name -ne "manifest.json"
}

$manifestFiles = @()
foreach ($f in $files) {
    $rel = $f.FullName.Substring($distFull.Length).TrimStart('\', '/') -replace '\\', '/'
    $hash = (Get-FileHash -Algorithm SHA256 -Path $f.FullName).Hash.ToLower()
    $flatAsset = $rel -replace '/', '__'
    $manifestFiles += [PSCustomObject]@{
        path   = $rel
        sha256 = $hash
        size   = $f.Length
        url    = ""
        asset  = $flatAsset
    }
}
Write-Host "      Tim thay $($manifestFiles.Count) file trong bo ship." -ForegroundColor Gray

# --- 6. Doi chieu voi manifest truoc -------------------------------------
Write-Host "`n[4/6] Dang doi chieu voi ban phat hanh truoc..." -ForegroundColor Green
$prevFilesByHash = @{}
try {
    $tmpPrev = New-TemporaryFile
    gh release download --repo $repo --pattern "manifest.json" --output $tmpPrev.FullName --clobber 2>$null
    if ((Test-Path $tmpPrev.FullName) -and ((Get-Item $tmpPrev.FullName).Length -gt 0)) {
        $prev = Get-Content $tmpPrev.FullName -Raw | ConvertFrom-Json
        foreach ($pf in $prev.files) {
            $prevFilesByHash[$pf.path + "|" + $pf.sha256] = $pf.url
        }
    }
    Remove-Item $tmpPrev.FullName -Force -ErrorAction SilentlyContinue
} catch {
    Write-Host "      (Khong doc duoc manifest cu - se upload toan bo)." -ForegroundColor Gray
}

$toUpload = @()
$downloadBase = "https://github.com/" + $repo + "/releases/download/" + $Tag
foreach ($mf in $manifestFiles) {
    $key = $mf.path + "|" + $mf.sha256
    if ($prevFilesByHash.ContainsKey($key)) {
        $mf.url = $prevFilesByHash[$key]
    } else {
        $mf.url = $downloadBase + "/" + $mf.asset
        $toUpload += $mf
    }
}

$uploadMB = [math]::Round((($toUpload | Measure-Object -Property size -Sum).Sum) / 1MB, 1)
Write-Host "      Can upload $($toUpload.Count)/$($manifestFiles.Count) file ($uploadMB MB)." -ForegroundColor Gray

# --- 7. Ghi manifest.json ------------------------------------------------
$notes = "TrafficTool " + $Tag
$manifestObj = [PSCustomObject]@{
    version = $Tag
    notes   = $notes
    files   = $manifestFiles | ForEach-Object {
        [PSCustomObject]@{ path = $_.path; sha256 = $_.sha256; url = $_.url; size = $_.size }
    }
}
$manifestPath = Join-Path $distFull "manifest.json"
$jsonManifest = $manifestObj | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($manifestPath, $jsonManifest, (New-Object System.Text.UTF8Encoding $false))
Write-Host "`n[5/6] Da tao manifest.json" -ForegroundColor Green

# --- 8. Tao GitHub Release + upload --------------------------------------
Write-Host "`n[6/6] Dang tao Release $Tag va upload..." -ForegroundColor Green

$uploadArgs = @()
$stageDir = Join-Path $env:TEMP ("trafficool_release_" + $Tag)
if (Test-Path $stageDir) { Remove-Item $stageDir -Recurse -Force }
New-Item -ItemType Directory -Path $stageDir | Out-Null

foreach ($mf in $toUpload) {
    $src = Join-Path $distFull ($mf.path -replace '/', '\')
    $dst = Join-Path $stageDir $mf.asset
    Copy-Item $src $dst -Force
    $uploadArgs += $dst
}
$uploadArgs += $manifestPath

$releaseExists = $false
try {
    $null = gh release view $Tag --repo $repo 2>&1
    if ($LASTEXITCODE -eq 0) {
        $releaseExists = $true
    }
} catch {
    $releaseExists = $false
}

if ($releaseExists) {
    Write-Host "      Release $Tag da ton tai - upload de asset..." -ForegroundColor Gray
    gh release upload $Tag $uploadArgs --repo $repo --clobber
} else {
    gh release create $Tag $uploadArgs --repo $repo --title ("TrafficTool " + $Tag) --notes $notes
}
if ($LASTEXITCODE -ne 0) {
    Write-Host "LOI: Tao/upload release that bai." -ForegroundColor Red
    Revert-VersionCode
    exit 1
}

Remove-Item $stageDir -Recurse -Force -ErrorAction SilentlyContinue

# --- 9. Commit + push version moi ----------------------------------------
if (-not $NoGit) {
    if (Get-Command git -ErrorAction SilentlyContinue) {
        Write-Host "`n[7/7] Dang commit + push version $Tag..." -ForegroundColor Green
        try {
            $prevEA = $ErrorActionPreference
            $ErrorActionPreference = "Continue"
            $null = git add update.go 2>&1
            $null = git commit -m ("release: " + $Tag) 2>&1
            if ($LASTEXITCODE -eq 0) {
                $null = git push 2>&1
            }
            $ErrorActionPreference = $prevEA
        } catch {}
    }
}

Write-Host "`n[OK] HOAN TAT! Da phat hanh $Tag thanh cong 100%." -ForegroundColor Cyan
