# SPDX-License-Identifier: Apache-2.0
# Commit-pinned local-only Go source snapshot. No network/checkout mutations.
# Preview is read-only; snapshot creation requires independent script SHA-256.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [ValidatePattern('^[0-9a-f]{40}$')]
    [string]$ExpectedCommit,
    [switch]$CreateSnapshot,
    [switch]$RunTests,
    [string]$ApprovedScriptSha256 = '',
    [string]$ApprovedGoImage = ''
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ExpectedBranch = 'feature/localcode-path-guard-checkpoint-20261122'
$ExpectedGo = 'go1.26.6'
$Repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$Git = 'C:\Program Files\Git\cmd\git.exe'
$Docker = 'C:\Program Files\Docker\Docker\resources\bin\docker.exe'
$SnapshotBase = Join-Path (Split-Path -Parent $Repo) '_workspace\LocalCodePinnedSnapshots'
$NeedsSnapshot = ($CreateSnapshot.IsPresent -or $RunTests.IsPresent)

function Block([string]$Reason) { throw ('PINNED_GO_BLOCKED: ' + $Reason) }
function Assert-Plain([string]$Path,[bool]$Directory) {
    if ($Directory) {
        if (-not [IO.Directory]::Exists($Path)) { Block 'DIRECTORY_UNAVAILABLE' }
    } elseif (-not [IO.File]::Exists($Path)) { Block 'FILE_UNAVAILABLE' }
    $current = [IO.Path]::GetFullPath($Path)
    while ($true) {
        $entry = Get-Item -LiteralPath $current -Force
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Block 'REPARSE_PATH_REFUSED' }
        $parent = [IO.Directory]::GetParent($current)
        if ($null -eq $parent) { break }
        $current = $parent.FullName
    }
}
function GitCheck([string[]]$Arguments) {
    $out = & $Git -c ('safe.directory=' + $Repo.Replace('\','/')) -C $Repo @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) { Block 'GIT_PRECONDITION_FAILED' }
    return (@($out) -join [Environment]::NewLine).Trim()
}
function SourcePathOK([string]$Relative) {
    if ($Relative -cnotmatch '^src/(?:[A-Za-z0-9._-]+/)*[A-Za-z0-9._-]+$') { return $false }
    foreach ($part in ($Relative -split '/')) {
        if ($part -in @('.','..')) { return $false }
        if ($part -match '^(?i:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\..*)?$') { return $false }
        if ($part.EndsWith('.')) { return $false }
    }
    $ext = [IO.Path]::GetExtension($Relative).ToLowerInvariant()
    return $ext -in @('.go','.mod','.sum','.h','.c','.cc','.cpp','.syso',
        '.js','.css','.html','.svg','.png','.ico','.json','.xml','.md',
        '.txt','.toml','.yaml','.yml','.properties','.woff2','.wasm',
        '.webp','.jpeg','.jpg','.manifest','.rc')
}

Assert-Plain $Repo $true
Assert-Plain $Git $false
Assert-Plain $MyInvocation.MyCommand.Path $false
$initialHead = GitCheck @('rev-parse','--verify','HEAD')
if ($initialHead -cnotmatch '^[a-f0-9]{40}$') { Block 'CURRENT_HEAD_INVALID' }
if ((GitCheck @('symbolic-ref','--quiet','--short','HEAD')) -cne $ExpectedBranch) { Block 'BRANCH_CHANGED' }
if ((GitCheck @('cat-file','-t',$ExpectedCommit)) -cne 'commit') { Block 'PINNED_COMMIT_UNAVAILABLE' }
if ((GitCheck @('merge-base','--is-ancestor',$ExpectedCommit,$initialHead)) -ne '') {
    Block 'INVALID_GIT_ANCESTRY'
}
# merge-base exits 1 on a mismatch; GitCheck already treats that as BLOCKED.
$gateObject = GitCheck @('rev-parse','HEAD:scripts/Test-LocalCodePinnedGo.ps1')
if ($gateObject -cnotmatch '^[a-f0-9]{40,64}$') { Block 'GATE_NOT_COMMITTED' }
$gateWorking = GitCheck @('hash-object','--','scripts/Test-LocalCodePinnedGo.ps1')
if ($gateWorking -cne $gateObject) { Block 'GATE_SOURCE_CHANGED' }
if ($NeedsSnapshot) {
    if ($ApprovedScriptSha256 -cnotmatch '^[0-9a-f]{64}$') { Block 'INDEPENDENT_SCRIPT_HASH_REQUIRED' }
    $actualScriptHash = (Get-FileHash -LiteralPath $MyInvocation.MyCommand.Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualScriptHash -cne $ApprovedScriptSha256) { Block 'SCRIPT_HASH_MISMATCH' }
}

# Verify every object in the selected commit before archiving; NEVER read
# ignored/untracked host files, even to check their content.
$tree = GitCheck @('ls-tree','-r','-l',$ExpectedCommit,'--','src')
$entries = @($tree -split '[\r\n]+' | Where-Object { $_ })
if ($entries.Count -lt 1 -or $entries.Count -gt 5000) { Block 'SOURCE_MANIFEST_COUNT_INVALID' }
$manifest = @{}
$names = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
$totalBytes = [int64]0
foreach ($line in $entries) {
    if ($line -cnotmatch '^(100644|100755) blob ([a-f0-9]{40,64})[ ]+([0-9]+)\t(src/[^\r\n]+)$') {
        Block 'SOURCE_NONREGULAR_OR_INVALID_ENTRY'
    }
    $oid = $Matches[2]; $size = [int64]$Matches[3]; $rel = $Matches[4]
    if (-not (SourcePathOK $rel) -or $rel.Length -gt 240 -or -not $names.Add($rel)) {
        Block 'SOURCE_UNSAFE_PATH'
    }
    $totalBytes += $size
    if ($size -gt 10MB -or $totalBytes -gt 180MB) { Block 'SOURCE_SIZE_LIMIT' }
    $manifest[$rel] = $oid
}
$required = @('src/go.mod','src/agent_mission_telemetry_minimal.go',
    'src/agent_mission_telemetry_minimal_test.go','src/agent_mission_status.go')
foreach ($rel in $required) { if (-not $manifest.ContainsKey($rel)) { Block 'REQUIRED_SOURCE_MISSING' } }
# Detect strong private-key and API token signatures in COMMITTED text.
# Git -q discards matching text; no discovered value is ever printed.
# This scan is defense-in-depth, not a complete guarantee.
$credentialPattern = '(-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|ghp_[A-Za-z0-9]{25,}|sk-[A-Za-z0-9_-]{30,}|AIza[A-Za-z0-9_-]{30,})'
& $Git -c ('safe.directory=' + $Repo.Replace('\','/')) -C $Repo grep -I -E -q -e $credentialPattern $ExpectedCommit -- src *> $null
if ($LASTEXITCODE -eq 0) { Block 'CREDENTIAL_PATTERN_IN_COMMIT' }
if ($LASTEXITCODE -ne 1) { Block 'COMMIT_CREDENTIAL_SCAN_FAILED' }
Write-Output ('PINNED_GO_PREVIEW_PASS commit=' + $ExpectedCommit + ' files=' + $manifest.Count + ' mutations=0')
if (-not $NeedsSnapshot) { return }

$ParentBase = Split-Path -Parent $SnapshotBase
Assert-Plain $ParentBase $true
if (-not [IO.Directory]::Exists($SnapshotBase)) {
    [void][IO.Directory]::CreateDirectory($SnapshotBase)
}
Assert-Plain $SnapshotBase $true
$label = $ExpectedCommit.Substring(0,12) + '-' + [guid]::NewGuid().ToString('N').Substring(0,12)
$destination = Join-Path $SnapshotBase ('snapshot-' + $label)
$archive = Join-Path $SnapshotBase ('archive-' + $label + '.zip')
if ([IO.Directory]::Exists($destination) -or [IO.File]::Exists($archive)) {
    Block 'SNAPSHOT_DESTINATION_EXISTS'
}
try {
    & $Git -c ('safe.directory=' + $Repo.Replace('\','/')) -C $Repo archive --format=zip ('--output=' + $archive) $ExpectedCommit src 1>$null 2>$null
    if ($LASTEXITCODE -ne 0 -or -not [IO.File]::Exists($archive)) { Block 'ARCHIVE_FAILED' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($archive)
    try {
        $archiveFiles = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
        foreach ($item in $zip.Entries) {
            $rel = $item.FullName.TrimEnd('/')
            if ($item.FullName.EndsWith('/')) {
                if ($rel -ne 'src' -and -not (SourcePathOK ($rel + '/dummy.go'))) {
                    Block 'ARCHIVE_DIRECTORY_UNSAFE'
                }
                continue
            }
            if (-not (SourcePathOK $rel) -or -not $manifest.ContainsKey($rel) -or -not $archiveFiles.Add($rel)) {
                Block 'ARCHIVE_ENTRY_UNSAFE'
            }
        }
        if ($archiveFiles.Count -ne $manifest.Count) { Block 'ARCHIVE_CONTENT_COUNT_MISMATCH' }
    } finally { $zip.Dispose() }
    [void][IO.Directory]::CreateDirectory($destination)
    [IO.Compression.ZipFile]::ExtractToDirectory($archive,$destination)
} finally {
    if ([IO.File]::Exists($archive)) { [IO.File]::Delete($archive) }
}
$source = Join-Path $destination 'src'
Assert-Plain $source $true
$found = @(Get-ChildItem -LiteralPath $source -Recurse -File -Force)
if ($found.Count -ne $manifest.Count) { Block 'SNAPSHOT_CONTENT_COUNT_MISMATCH' }
foreach ($entry in $found) {
    Assert-Plain $entry.FullName $false
    $rel = $entry.FullName.Substring($destination.Length + 1).Replace('\','/')
    if (-not $manifest.ContainsKey($rel)) { Block 'SNAPSHOT_EXTRA_FILE' }
    $blob = & $Git hash-object -- $entry.FullName 2>$null
    if ($LASTEXITCODE -ne 0 -or (@($blob) -join '').Trim() -cne $manifest[$rel]) {
        Block 'SNAPSHOT_BLOB_ID_MISMATCH'
    }
}
if ((GitCheck @('rev-parse','--verify','HEAD')) -cne $initialHead -or
    (GitCheck @('hash-object','--','scripts/Test-LocalCodePinnedGo.ps1')) -cne $gateObject) {
    Block 'SOURCE_CHANGED_DURING_SNAPSHOT'
}
Write-Output ('PINNED_GO_SNAPSHOT_PASS commit=' + $ExpectedCommit + ' files=' + $manifest.Count + ' ignored_files_copied=false')
if (-not $RunTests.IsPresent) {
    Write-Output 'NO_TEST_RAN; NO_RELEASE_APPROVAL; NO_NETWORK_REQUEST'
    return
}

# Tests require a separately reviewed, locally PRESENT Go Docker image whose
# Go modules and compiler were installed in advance. Never pull/download.
if ($ApprovedGoImage -cnotmatch '^[a-z0-9][a-z0-9._/-]*(?::[a-z0-9._-]+)?@sha256:[a-f0-9]{64}$') {
    Block 'REVIEWED_DIGEST_IMAGE_REQUIRED'
}
Assert-Plain $Docker $false
$DockerHost = 'npipe:////./pipe/docker_engine'
$osType = & $Docker --host $DockerHost info --format '{{.OSType}}' 2>$null
if ($LASTEXITCODE -ne 0 -or (@($osType) -join '').Trim() -cne 'linux') {
    Block 'LOCAL_LINUX_DOCKER_UNAVAILABLE'
}
& $Docker --host $DockerHost image inspect $ApprovedGoImage 1>$null 2>$null
if ($LASTEXITCODE -ne 0) { Block 'PINNED_GO_IMAGE_NOT_LOCAL' }
$base = @(
    '--host',$DockerHost,'run','--rm','--pull=never','--network=none',
    '--read-only','--cap-drop=ALL','--security-opt=no-new-privileges',
    '--pids-limit=256','--memory=3g','--cpus=2','--user=65534:65534',
    '--tmpfs','/tmp:rw,exec,nosuid,nodev,size=1024m',
    '--env','HOME=/tmp','--env','GOCACHE=/tmp/gocache',
    '--env','GOTOOLCHAIN=local','--env','GOPROXY=off',
    '--env','GOSUMDB=off','--env','GOWORK=off',
    '--env','GIT_TERMINAL_PROMPT=0',
    '--mount',('type=bind,source=' + $source + ',target=/work,readonly'),
    '--workdir','/work','--entrypoint','go',$ApprovedGoImage
)
$version = & $Docker @base env GOVERSION 2>$null
if ($LASTEXITCODE -ne 0 -or (@($version) -join '').Trim() -cne $ExpectedGo) {
    Block 'GO_COMPILER_VERSION_MISMATCH'
}
& $Docker @base test '-count=1' '-timeout=240s' '-run=^TestAgentMinimalDesktop' '.' *> $null
if ($LASTEXITCODE -ne 0) { Block 'FOCUSED_GO_TEST_FAILED' }
Write-Output 'PINNED_GO_FOCUSED_PASS network=none'
& $Docker @base test '-count=1' '-timeout=300s' './...' *> $null
if ($LASTEXITCODE -ne 0) { Block 'FULL_GO_TEST_FAILED' }
Write-Output 'PINNED_GO_FULL_PASS network=none release_approved=false'
