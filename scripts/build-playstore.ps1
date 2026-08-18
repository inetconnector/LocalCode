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
    [pscustomobject]@{
        type   = $artifact.Extension.TrimStart('.').ToLowerInvariant()
        path   = $artifact.FullName
        bytes  = $artifact.Length
        sha256 = $hash
    }
}

Write-Host ""
Write-Host 'Release artifacts:'
$result | Format-Table -AutoSize
Write-Host ""
Write-Host 'PLAY_STORE_BUILD_RESULT_JSON'
$result | ConvertTo-Json -Depth 4 -Compress
