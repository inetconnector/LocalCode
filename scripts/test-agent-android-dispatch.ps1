$ErrorActionPreference = 'Stop'
[System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }

$pairing = Invoke-RestMethod -Uri 'http://127.0.0.1:32145/api/remote/pairing' -Method Post
$code = $pairing.code
$remoteTarget = if ($pairing.remote_urls -and $pairing.remote_urls.Count -gt 0) { $pairing.remote_urls[0] } else { "https://127.0.0.1:32146/remote" }
$remoteApiBase = $remoteTarget.TrimEnd('/')

$pairBody = @{ code = "$code"; device_name = 'Antigravity Mobile Automation' } | ConvertTo-Json
$pairRes = Invoke-RestMethod -Uri "$remoteApiBase/api/pair" -Method Post -Body $pairBody -ContentType 'application/json'
$token = $pairRes.token
$headers = @{ 'X-LocalCode-Remote-Token' = "$token" }

$projectName = "LocalCodeAndroidTest"
$projectDir = "C:\Users\frede\Projekte\$projectName"

Write-Host "1. Creating project $projectName..."
$createBody = @{ path = 'C:\Users\frede\Projekte'; action = 'create_project'; value = $projectName } | ConvertTo-Json
$proj = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $createBody -ContentType 'application/json'
Write-Host "  -> Project created at: $($proj.project.path)"

Write-Host "2. Starting chat thread..."
$taskBody = @{ project = "$($proj.project.path)" } | ConvertTo-Json
$chat = Invoke-RestMethod -Uri "$remoteApiBase/api/new-chat" -Method Post -Headers $headers -Body $taskBody -ContentType 'application/json'
$threadId = $chat.thread.id
Write-Host "  -> Thread: $threadId"

Write-Host "3. Verifying initial snapshot..."
$snap = Invoke-RestMethod -Uri "$remoteApiBase/api/snapshot?thread_id=$threadId" -Headers $headers
Write-Host "  -> Running: $($snap.running), Phase: $($snap.run_phase)"

Write-Host "4. Cleaning up test project..."
$trashBody = @{ path = "$($proj.project.path)"; action = 'delete_recursive'; value = $projectName } | ConvertTo-Json
$del = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $trashBody -ContentType 'application/json'

$q = Invoke-RestMethod -Uri "$remoteApiBase/api/project-quarantine" -Headers $headers
$item = $q.quarantine | Where-Object { $_.name -eq $projectName } | Select-Object -First 1
if ($item) {
    $purgeBody = @{ action = 'purge'; id = $item.id; confirmation = "PURGE $projectName" } | ConvertTo-Json
    $null = Invoke-RestMethod -Uri "$remoteApiBase/api/project-quarantine-action" -Method Post -Headers $headers -Body $purgeBody -ContentType 'application/json'
    Write-Host "  -> Cleaned and purged $projectName."
}

Write-Host "`nAll Mobile Remote agent dispatch points verified successfully!"
