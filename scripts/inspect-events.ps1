$ErrorActionPreference = 'Stop'
[System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }

$pairing = Invoke-RestMethod -Uri 'http://127.0.0.1:32145/api/remote/pairing' -Method Post
$code = $pairing.code
$remoteTarget = if ($pairing.remote_urls -and $pairing.remote_urls.Count -gt 0) { $pairing.remote_urls[0] } else { "https://127.0.0.1:32146/remote" }
$remoteApiBase = $remoteTarget.TrimEnd('/')

$pairRes = Invoke-RestMethod -Uri "$remoteApiBase/api/pair" -Method Post -Body (@{ code = "$code"; device_name = 'Inspector' } | ConvertTo-Json) -ContentType 'application/json'
$headers = @{ 'X-LocalCode-Remote-Token' = "$($pairRes.token)" }

$snap = Invoke-RestMethod -Uri "$remoteApiBase/api/snapshot?thread_id=b0dae2494f26c1f1" -Headers $headers
Write-Host "Thread: $($snap.current_thread), Running: $($snap.running), Phase: $($snap.run_phase), Events: $($snap.events.Count)"
foreach ($ev in $snap.events) {
    Write-Host "----------------------------------"
    Write-Host "Type: $($ev.type) | Action: $($ev.action) | Path: $($ev.path)"
    Write-Host "Message: $($ev.message)"
}
