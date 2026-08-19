param(
    [string]$ProjectDirectory = ".",
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$project = (Resolve-Path -LiteralPath $ProjectDirectory).Path
$gradlew = Join-Path $project 'gradlew.bat'
if (-not (Test-Path -LiteralPath $gradlew -PathType Leaf)) {
    throw "Gradle wrapper not found: $gradlew"
}

function Invoke-GradleTask {
    param([Parameter(Mandatory=$true)][string]$Task)
    Write-Host "==> gradlew $Task"
    Push-Location $project
    try {
        & $gradlew --no-daemon --console=plain $Task
        if ($LASTEXITCODE -ne 0) {
            throw "Gradle task '$Task' failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
}

function Get-GradleTaskNames {
    Push-Location $project
    try {
        $lines = & $gradlew --no-daemon --console=plain tasks --all 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "Could not enumerate Gradle tasks (exit $LASTEXITCODE)"
        }
        $names = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        foreach ($line in $lines) {
            if ($line -match '^([A-Za-z0-9:_-]+)\s+-\s+') {
                [void]$names.Add($matches[1])
            }
        }
        return ,$names
    } finally {
        Pop-Location
    }
}

function Find-ReleaseArtifacts {
    $patterns = @('*.aab', '*.apk')
    $artifacts = @()
    foreach ($pattern in $patterns) {
        $artifacts += Get-ChildItem -LiteralPath $project -Recurse -File -Filter $pattern -ErrorAction SilentlyContinue |
            Where-Object {
                $_.FullName -match '[\\/]build[\\/]outputs[\\/]' -and
                $_.Name -notmatch '(?i)debug|androidTest|unaligned|unsigned'
            }
    }
    return @($artifacts | Sort-Object FullName -Unique)
}

function Get-CommandPath {
    param([Parameter(Mandatory=$true)][string[]]$Names)
    foreach ($name in $Names) {
        $command = Get-Command $name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $command) {
            if ($command.Source) { return $command.Source }
            if ($command.Path) { return $command.Path }
        }
    }
    return ''
}

function Test-AabSignature {
    param([Parameter(Mandatory=$true)][string]$Path)
    $jarsigner = Get-CommandPath -Names @('jarsigner.exe', 'jarsigner')
    if (-not $jarsigner) {
        throw 'jarsigner was not found. The release AAB cannot be signature-verified, so this Play Store build is not accepted as complete.'
    }
    $output = & $jarsigner '-J-Duser.language=en' '-J-Duser.country=US' -verify -verbose -certs $Path 2>&1
    $exitCode = $LASTEXITCODE
    $text = ($output | Out-String)
    if ($exitCode -ne 0 -or $text -notmatch '(?i)jar verified') {
        throw "Release AAB signature verification failed: $Path"
    }
    return 'verified:jarsigner'
}

function Test-ApkSignatureIfAvailable {
    param([Parameter(Mandatory=$true)][string]$Path)
    $apksigner = Get-CommandPath -Names @('apksigner.bat', 'apksigner.exe', 'apksigner')
    if (-not $apksigner) {
        return 'not-checked:apksigner-not-found'
    }
    & $apksigner verify --verbose $Path | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Release APK signature verification failed: $Path"
    }
    return 'verified:apksigner'
}

$taskNames = Get-GradleTaskNames
$planned = [System.Collections.Generic.List[string]]::new()
if (-not $SkipTests) {
    foreach ($candidate in @('test', 'lint')) {
        if ($taskNames.Contains($candidate)) {
            [void]$planned.Add($candidate)
        }
    }
}
foreach ($candidate in @('bundleRelease', 'assembleRelease')) {
    if ($taskNames.Contains($candidate)) {
        [void]$planned.Add($candidate)
    }
}
if (-not $taskNames.Contains('bundleRelease')) {
    throw "This project does not expose bundleRelease. A Play Store Android App Bundle (.aab) cannot be produced by the current Gradle project."
}

Write-Host "Project: $project"
Write-Host "Tasks: $($planned -join ', ')"
Write-Host "Safety: this helper never creates/replaces keystores, never prints signing secrets, and never uploads or publishes."

foreach ($task in $planned) {
    Invoke-GradleTask -Task $task
}

$artifacts = Find-ReleaseArtifacts
$aabs = @($artifacts | Where-Object Extension -EQ '.aab')
if ($aabs.Count -eq 0) {
    throw 'bundleRelease finished but no release .aab was found below a build/outputs directory.'
}

$result = foreach ($artifact in $artifacts) {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $artifact.FullName).Hash.ToLowerInvariant()
    $signature = if ($artifact.Extension -ieq '.aab') {
        Test-AabSignature -Path $artifact.FullName
    } else {
        Test-ApkSignatureIfAvailable -Path $artifact.FullName
    }
    [pscustomobject]@{
        type             = $artifact.Extension.TrimStart('.').ToLowerInvariant()
        path             = $artifact.FullName
        bytes            = $artifact.Length
        sha256           = $hash
        signature_status = $signature
    }
}

Write-Host ""
Write-Host 'Release artifacts:'
$result | Format-Table -AutoSize
Write-Host ""
Write-Host 'PLAY_STORE_BUILD_RESULT_JSON'
$result | ConvertTo-Json -Depth 4 -Compress
