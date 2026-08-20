$ErrorActionPreference = 'Stop'

$patchPath = Join-Path $PSScriptRoot 'temp-native-agent-teams-patch.ps1'
$text = [IO.File]::ReadAllText($patchPath)
$bad = @'
$newReliability = "Task: deterministicSubagentTaskPrefix +`n`t`t`t`t\"Analyze the requested change before any edit. Build a repository intelligence map, identify the most relevant implementation and test files, architecture invariants, likely failure modes, and the narrowest reliable verification plan. Do not modify anything. User task: \" + intent.OriginalTask,"
'@.Trim()
$good = @'
$newReliability = @'
Task: deterministicSubagentTaskPrefix +
    "Analyze the requested change before any edit. Build a repository intelligence map, identify the most relevant implementation and test files, architecture invariants, likely failure modes, and the narrowest reliable verification plan. Do not modify anything. User task: " + intent.OriginalTask,
'@.Trim()
'@.Trim()
if (-not $text.Contains($bad)) {
    throw 'Expected temporary PowerShell quoting anchor was not found.'
}
$text = $text.Replace($bad, $good)
[IO.File]::WriteAllText($patchPath, $text, [Text.UTF8Encoding]::new($false))

& $patchPath
if ($LASTEXITCODE -ne 0) { throw "Native Agent Teams patch failed with exit code $LASTEXITCODE" }
