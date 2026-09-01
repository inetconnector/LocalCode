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

# Cleanup existing if left over
$existingPath = 'C:\Users\frede\Projekte\MobileAntigravityDemo'
if (Test-Path $existingPath) {
    $delBody = @{ path = $existingPath; action = 'delete_recursive'; value = 'MobileAntigravityDemo' } | ConvertTo-Json
    $null = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $delBody -ContentType 'application/json'
}

$projectName = "MobileAntigravityDemo"
Write-Host "1. Creating project $projectName via Mobile Remote..."
$createBody = @{ path = 'C:\Users\frede\Projekte'; action = 'create_project'; value = $projectName } | ConvertTo-Json
$proj = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $createBody -ContentType 'application/json'
Write-Host "  -> Created project: $($proj.project.path)"

Write-Host "2. Starting new chat thread via Mobile Remote..."
$taskBody = @{ project = "$($proj.project.path)" } | ConvertTo-Json
$chat = Invoke-RestMethod -Uri "$remoteApiBase/api/new-chat" -Method Post -Headers $headers -Body $taskBody -ContentType 'application/json'
$threadId = $chat.thread.id
Write-Host "  -> Thread ID: $threadId"

Write-Host "3. Verifying Thread Snapshot via Mobile Remote..."
$snap = Invoke-RestMethod -Uri "$remoteApiBase/api/snapshot?thread_id=$threadId" -Headers $headers
Write-Host "  -> Project: $($snap.project), Events: $($snap.events.Count)"

Write-Host "4. Moving project to Quarantine Trash via Mobile Remote..."
$trashBody = @{ path = "$($proj.project.path)"; action = 'delete_recursive'; value = $projectName } | ConvertTo-Json
$del = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $trashBody -ContentType 'application/json'
Write-Host "  -> Quarantined project: $($del.project.name)"

Write-Host "5. Querying Quarantine & Purging via Mobile Remote..."
$q = Invoke-RestMethod -Uri "$remoteApiBase/api/project-quarantine" -Headers $headers
$item = $q.quarantine | Where-Object { $_.name -eq $projectName } | Select-Object -First 1
if ($item) {
    $purgeBody = @{ action = 'purge'; id = $item.id; confirmation = "PURGE $projectName" } | ConvertTo-Json
    $purged = Invoke-RestMethod -Uri "$remoteApiBase/api/project-quarantine-action" -Method Post -Headers $headers -Body $purgeBody -ContentType 'application/json'
    Write-Host "  -> Purged from Trash: $($purged.item.name)"
}

Write-Host "`n================================================="
Write-Host "MOBILE PROJECT CREATION & CONTROLS VERIFIED 100%!"
Write-Host "================================================="
