# SPDX-License-Identifier: Apache-2.0
$ErrorActionPreference = "Stop"

$pairing = Invoke-RestMethod -Uri "http://127.0.0.1:32145/api/remote/pairing" -Method POST
Write-Host "Pairing Code:" $pairing.code
Write-Host "Deep Link:" $pairing.deep_link
Write-Host "Fingerprint:" $pairing.fingerprint

$adb = "C:\Users\frede\AppData\Local\Android\Sdk\platform-tools\adb.exe"
$device = "adb-RFCY21GXM9E-sRkdbH._adb-tls-connect._tcp"

# Wake and unlock device
& $adb -s $device shell input keyevent KEYCODE_WAKEUP
& $adb -s $device shell wm dismiss-keyguard

# Launch deep link (simulating QR scan)
& $adb -s $device shell am start -a android.intent.action.VIEW -d "`'$($pairing.deep_link)`'" com.inetconnector.localcode.remote

Start-Sleep -Seconds 2

# Capture screenshot
& $adb -s $device shell screencap -p /sdcard/qr_autopair_verified.png
& $adb -s $device pull /sdcard/qr_autopair_verified.png "C:\Users\frede\.gemini\antigravity-ide\brain\70adceb1-3631-469a-a6d0-39a7277a0e7d\qr_autopair_verified.png"
Write-Host "Screenshot captured successfully!" -ForegroundColor Green
