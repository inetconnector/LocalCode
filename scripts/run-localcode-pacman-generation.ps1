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

$projectName = "PacmanNative"
$projectDir = "C:\Users\frede\Projekte\$projectName"

Write-Host "=========================================="
Write-Host "Starting Autonomous Pacman Generation in LocalCode"
Write-Host "=========================================="

if (-not (Test-Path $projectDir)) {
    Write-Host "1. Creating project $projectName via LocalCode..."
    $createBody = @{ path = 'C:\Users\frede\Projekte'; action = 'create_project'; value = $projectName } | ConvertTo-Json
    $proj = Invoke-RestMethod -Uri "$remoteApiBase/api/project-action" -Method Post -Headers $headers -Body $createBody -ContentType 'application/json'
    Write-Host "  -> Created project: $($proj.project.path)"
} else {
    Write-Host "1. Project $projectName already exists, using it."
}

Write-Host "2. Starting new chat thread for Pacman generation..."
$taskBody = @{ project = "$projectDir" } | ConvertTo-Json
$chat = Invoke-RestMethod -Uri "$remoteApiBase/api/new-chat" -Method Post -Headers $headers -Body $taskBody -ContentType 'application/json'
$threadId = $chat.thread.id
Write-Host "  -> Thread ID: $threadId"

$prompt = @"
Erstelle die native Android Pacman App, indem du mit write_file alle Quellcode-Dateien schreibst:
1. AndroidManifest.xml (package com.pacman.nativeapp, PacmanActivity)
2. res/values/strings.xml, res/values/styles.xml, res/values/colors.xml
3. java/com/pacman/nativeapp/SoundManager.java (Synthetisierte AudioTrack 8-Bit Töne für Waka-Waka, Energizer und Tod)
4. java/com/pacman/nativeapp/Maze.java (28x31 Classic Arcade Labyrinth mit Pellets und Energizern)
5. java/com/pacman/nativeapp/Pacman.java (Pacman Spielfigur, Mund-Animation, Touch/Swipe-Steuerung)
6. java/com/pacman/nativeapp/Ghost.java (Blinky, Pinky, Inky, Clyde mit Chase und Scatter KI)
7. java/com/pacman/nativeapp/GameView.java (60 FPS SurfaceView Arcade Loop)
8. java/com/pacman/nativeapp/PacmanActivity.java
Führe nach dem Schreiben aller Dateien die Aktion build_project aus.
"@

Write-Host "3. Dispatching prompt to LocalCode Agent..."
$sendBody = @{
    message = $prompt
    model = "qwen2.5-coder:14b"
    project = $projectDir
    thread_id = $threadId
    attachments = @()
} | ConvertTo-Json

$res = Invoke-RestMethod -Uri "$remoteApiBase/api/chat" -Method Post -Headers $headers -Body $sendBody -ContentType 'application/json'
Write-Host "  -> Agent run started: $($res.ok)"

Write-Host "`n4. Monitoring agent execution progress..."
$maxWaitSeconds = 600
$startTime = Get-Date

while (((Get-Date) - $startTime).TotalSeconds -lt $maxWaitSeconds) {
    Start-Sleep -Seconds 3
    $snap = Invoke-RestMethod -Uri "$remoteApiBase/api/snapshot?thread_id=$threadId" -Headers $headers
    
    $eventCount = if ($snap.events) { $snap.events.Count } else { 0 }
    $lastEvent = if ($eventCount -gt 0) { $snap.events[-1].message } else { "None" }
    
    Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Running: $($snap.running), Phase: $($snap.run_phase), Events: $eventCount - Last: $lastEvent"
    
    if ($snap.pending) {
        $pendingId = $snap.pending.id
        Write-Host "  -> Auto-approving pending action: $($snap.pending.action) ($($snap.pending.message))"
        $appBody = @{ id = $pendingId; approve = $true; decision = 'project' } | ConvertTo-Json
        $null = Invoke-RestMethod -Uri "$remoteApiBase/api/approve" -Method Post -Headers $headers -Body $appBody -ContentType 'application/json'
    }

    if (-not $snap.running -and $eventCount -gt 0) {
        Write-Host "`nAgent completed execution!"
        break
    }
}

Write-Host "`n5. Inspecting created project files in $projectDir..."
Get-ChildItem -Path $projectDir -Recurse | Select-Object FullName
