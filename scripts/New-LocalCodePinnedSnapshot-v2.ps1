# SPDX-License-Identifier: Apache-2.0
# Local-only, commit-pinned source snapshot gate. This script never runs Go,
# installs tools, sends data, or alters a Git checkout.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [ValidatePattern('^[a-f0-9]{40}$')]
    [string]$ExpectedCommit,
    [switch]$CreateSnapshot,
    [string]$ApprovedScriptSha256 = ''
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$RequiredBranch = 'feature/localcode-path-guard-checkpoint-20261122'
$Git = 'C:\Program Files\Git\cmd\git.exe'
$Repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$SnapshotRoot = Join-Path (Split-Path -Parent $Repo) '_workspace\LocalCodePinnedSnapshots'
$RelativeScript = 'scripts/New-LocalCodePinnedSnapshot-v2.ps1'

function Block([string]$Code) { throw ('PINNED_SNAPSHOT_BLOCKED: ' + $Code) }
function CheckPlain([string]$Path,[bool]$IsDirectory) {
    if ($IsDirectory) {
        if (-not [IO.Directory]::Exists($Path)) { Block 'DIRECTORY_UNAVAILABLE' }
    } elseif (-not [IO.File]::Exists($Path)) { Block 'FILE_UNAVAILABLE' }
    $current = [IO.Path]::GetFullPath($Path)
    while ($true) {
        $item = Get-Item -LiteralPath $current -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Block 'REPARSE_REFUSED' }
        $parent = [IO.Directory]::GetParent($current)
        if ($null -eq $parent) { break }
        $current = $parent.FullName
    }
}
function GitChecked([string[]]$Arguments) {
    $result = & $Git -c ('safe.directory=' + $Repo.Replace('\','/')) -C $Repo @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) { Block 'GIT_CHECK_FAILED' }
    return (@($result) -join [Environment]::NewLine).Trim()
}
function SafeSourcePath([string]$Path) {
    if ($Path -cnotmatch '^src/(?:[A-Za-z0-9._-]+/)*[A-Za-z0-9._-]+$' -or $Path.Length -gt 240) { return $false }
    foreach ($part in ($Path -split '/')) {
        if ($part -in @('.','..','.git','.tools','data','private-state','node_modules')) { return $false }
        if ($part -match '^(?i:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\..*)?$') { return $false }
        if ($part.EndsWith('.')) { return $false }
    }
    if ($Path -match '(?i:(?:^|/)(?:\.env(?:\..*)?|id_rsa(?:\..*)?|credentials(?:\..*)?))$') { return $false }
    if ($Path -match '(?i:(?:^|/)[^/]*(?:secret|password|token|credential)[^/]*\.(?:json|toml|yaml|yml|ini|cfg|txt|env|pem|key|p12|pfx|kdbx))$') {
        return $false
    }
    $extension = [IO.Path]::GetExtension($Path).ToLowerInvariant()
    return $extension -in @(
        '.go','.mod','.sum','.h','.c','.cc','.cpp','.syso',
        '.js','.css','.html','.svg','.png','.ico','.json','.xml','.md',
        '.txt','.toml','.yaml','.yml','.properties','.woff2','.wasm',
        '.webp','.jpeg','.jpg','.manifest','.rc'
    )
}
CheckPlain $Repo $true
CheckPlain $Git $false
CheckPlain $MyInvocation.MyCommand.Path $false
$initialHead = GitChecked @('rev-parse','--verify','HEAD')
if ($initialHead -cnotmatch '^[0-9a-f]{40}$') { Block 'HEAD_INVALID' }
if ((GitChecked @('symbolic-ref','--quiet','--short','HEAD')) -cne $RequiredBranch) { Block 'BRANCH_MISMATCH' }
if ((GitChecked @('cat-file','-t',$ExpectedCommit)) -cne 'commit') { Block 'PINNED_COMMIT_MISSING' }
[void](GitChecked @('merge-base','--is-ancestor',$ExpectedCommit,$initialHead))
$sourceBlob = GitChecked @('rev-parse',('HEAD:' + $RelativeScript))
$workingBlob = GitChecked @('hash-object','--',$RelativeScript)
if ($sourceBlob -cnotmatch '^[0-9a-f]{40,64}$' -or $sourceBlob -cne $workingBlob) {
    Block 'GATE_NOT_COMMITTED_OR_CHANGED'
}
if ($CreateSnapshot) {
    if ($ApprovedScriptSha256 -cnotmatch '^[0-9a-f]{64}$') { Block 'EXTERNAL_SHA256_APPROVAL_REQUIRED' }
    if ((Get-FileHash -LiteralPath $MyInvocation.MyCommand.Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $ApprovedScriptSha256) {
        Block 'SCRIPT_SHA256_CHANGED'
    }
}

$rawTree = GitChecked @('ls-tree','-r','-l',$ExpectedCommit,'--','src')
$treeLines = @($rawTree -split '[\r\n]+' | Where-Object { $_ })
if ($treeLines.Count -lt 1 -or $treeLines.Count -gt 5000) { Block 'MANIFEST_COUNT_INVALID' }
$manifest = @{}
$unique = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
$total = [int64]0
foreach ($line in $treeLines) {
    if ($line -cnotmatch '^(100644|100755) blob ([a-f0-9]{40,64})[ ]+([0-9]+)\t(src/[^\r\n]+)$') {
        Block 'MANIFEST_UNSAFE_ENTRY'
    }
    $blob = $Matches[2]; $size = [int64]$Matches[3]; $name = $Matches[4]
    if (-not (SafeSourcePath $name) -or -not $unique.Add($name)) { Block 'MANIFEST_UNSAFE_PATH' }
    $total += $size
    if ($size -gt 10MB -or $total -gt 180MB) { Block 'MANIFEST_TOO_LARGE' }
    $manifest[$name] = $blob
}
foreach ($name in @('src/go.mod','src/agent_mission_telemetry_minimal.go','src/agent_mission_telemetry_minimal_test.go','src/agent_mission_status.go')) {
    if (-not $manifest.ContainsKey($name)) { Block 'SOURCE_REQUIRED_FILE_MISSING' }
}
Write-Output ('PINNED_SNAPSHOT_PREVIEW_PASS commit=' + $ExpectedCommit + ' files=' + $manifest.Count + ' mutations=0')
if (-not $CreateSnapshot) { return }

$parent = Split-Path -Parent $SnapshotRoot
CheckPlain $parent $true
if (-not [IO.Directory]::Exists($SnapshotRoot)) { [void][IO.Directory]::CreateDirectory($SnapshotRoot) }
CheckPlain $SnapshotRoot $true
$id = $ExpectedCommit.Substring(0,12) + '-' + [guid]::NewGuid().ToString('N').Substring(0,12)
$destination = Join-Path $SnapshotRoot ('snapshot-' + $id)
$zipPath = Join-Path $SnapshotRoot ('archive-' + $id + '.zip')
if ([IO.Directory]::Exists($destination) -or [IO.File]::Exists($zipPath)) { Block 'TARGET_EXISTS' }
try {
    & $Git -c ('safe.directory=' + $Repo.Replace('\','/')) -C $Repo archive --format=zip ('--output=' + $zipPath) $ExpectedCommit src 1>$null 2>$null
    if ($LASTEXITCODE -ne 0 -or -not [IO.File]::Exists($zipPath)) { Block 'ARCHIVE_FAILED' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
    try {
        $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
        foreach ($entry in $archive.Entries) {
            $name = $entry.FullName.TrimEnd('/')
            if ($entry.FullName.EndsWith('/')) {
                if ($name -ne 'src' -and -not (SafeSourcePath ($name + '/placeholder.go'))) { Block 'ARCHIVE_UNSAFE_DIRECTORY' }
                continue
            }
            if (-not (SafeSourcePath $name) -or -not $manifest.ContainsKey($name) -or -not $seen.Add($name)) {
                Block 'ARCHIVE_UNSAFE_FILE'
            }
        }
        if ($seen.Count -ne $manifest.Count) { Block 'ARCHIVE_MANIFEST_MISMATCH' }
    } finally { $archive.Dispose() }
    [void][IO.Directory]::CreateDirectory($destination)
    [IO.Compression.ZipFile]::ExtractToDirectory($zipPath,$destination)
} finally {
    if ([IO.File]::Exists($zipPath)) { [IO.File]::Delete($zipPath) }
}
$src = Join-Path $destination 'src'
CheckPlain $src $true
$files = @(Get-ChildItem -LiteralPath $src -Recurse -File -Force)
if ($files.Count -ne $manifest.Count) { Block 'EXTRACTED_FILE_COUNT_MISMATCH' }
foreach ($file in $files) {
    CheckPlain $file.FullName $false
    $name = $file.FullName.Substring($destination.Length + 1).Replace('\','/')
    if (-not $manifest.ContainsKey($name)) { Block 'EXTRACTED_FILE_NOT_COMMITTED' }
    $blob = & $Git hash-object -- $file.FullName 2>$null
    if ($LASTEXITCODE -ne 0 -or (@($blob) -join '').Trim() -cne $manifest[$name]) {
        Block 'EXTRACTED_BLOB_MISMATCH'
    }
}
if ((GitChecked @('rev-parse','--verify','HEAD')) -cne $initialHead -or
    (GitChecked @('hash-object','--',$RelativeScript)) -cne $sourceBlob) {
    Block 'HEAD_OR_SCRIPT_CHANGED_DURING_SNAPSHOT'
}
Write-Output ('PINNED_SNAPSHOT_PASS commit=' + $ExpectedCommit + ' files=' + $manifest.Count + ' ignored_files_copied=false')
Write-Output 'NO_TEST_RAN; NO_NETWORK_REQUEST; NO_RELEASE_APPROVAL'
