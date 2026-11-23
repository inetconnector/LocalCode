param(
    [string]$OutputDirectory = "",
    [string]$VersionName = "",
    [int]$VersionCode = 0
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = Split-Path -Parent $PSScriptRoot
$androidRoot = Join-Path $repo 'android\app\src\main'
$manifest = Join-Path $androidRoot 'AndroidManifest.xml'
$javaSource = Join-Path $androidRoot 'java\com\inetconnector\localcode\MainActivity.java'
if (-not (Test-Path -LiteralPath $javaSource -PathType Leaf)) {
    $javaSource = Join-Path $androidRoot 'java\com\inetconnector\localcode\remote\MainActivity.java'
}
$resDir = Join-Path $androidRoot 'res'
if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) { throw "Android manifest missing: $manifest" }
if (-not (Test-Path -LiteralPath $javaSource -PathType Leaf)) { throw "Android source missing: $javaSource" }

$sdk = $env:ANDROID_SDK_ROOT
if ([string]::IsNullOrWhiteSpace($sdk)) { $sdk = $env:ANDROID_HOME }
if ([string]::IsNullOrWhiteSpace($sdk) -or -not (Test-Path -LiteralPath $sdk -PathType Container)) {
    throw 'ANDROID_SDK_ROOT/ANDROID_HOME is not available. The Quality runner must provide the Android SDK.'
}

$platform = Join-Path $sdk 'platforms\android-36'
$androidJar = Join-Path $platform 'android.jar'
if (-not (Test-Path -LiteralPath $androidJar -PathType Leaf)) {
    throw "Android API 36 platform missing: $androidJar"
}

$buildToolsRoot = Join-Path $sdk 'build-tools'
$buildTools = Get-ChildItem -LiteralPath $buildToolsRoot -Directory |
    Sort-Object { try { [version]$_.Name } catch { [version]'0.0' } } -Descending |
    Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'aapt2.exe') } |
    Select-Object -First 1
if (-not $buildTools) { throw "No Android build-tools with aapt2.exe found under $buildToolsRoot" }

$aapt2 = Join-Path $buildTools.FullName 'aapt2.exe'
$aapt = Join-Path $buildTools.FullName 'aapt.exe'
$d8 = Join-Path $buildTools.FullName 'd8.bat'
$zipalign = Join-Path $buildTools.FullName 'zipalign.exe'
$apksigner = Join-Path $buildTools.FullName 'apksigner.bat'
foreach ($tool in @($aapt2, $aapt, $d8, $zipalign, $apksigner)) {
    if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) { throw "Android build tool missing: $tool" }
}

$javacCommand = Get-Command javac.exe -ErrorAction SilentlyContinue
$keytoolCommand = Get-Command keytool.exe -ErrorAction SilentlyContinue
if (-not $javacCommand) { throw 'javac.exe not found in PATH' }
if (-not $keytoolCommand) { throw 'keytool.exe not found in PATH' }
$javac = $javacCommand.Source
$keytool = $keytoolCommand.Source

$stagingDir = Join-Path ([IO.Path]::GetTempPath()) ('localcode-android-' + [Guid]::NewGuid().ToString('N'))
Remove-Item -LiteralPath $stagingDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $stagingDir -Force | Out-Null
$classes = Join-Path $stagingDir 'classes'
$dex = Join-Path $stagingDir 'dex'
$compiledRes = Join-Path $stagingDir 'compiled-res'
New-Item -ItemType Directory -Path $classes,$dex,$compiledRes -Force | Out-Null

$versionFile = Join-Path $repo 'VERSION.txt'
if ([string]::IsNullOrWhiteSpace($VersionName)) {
    if (Test-Path -LiteralPath $versionFile -PathType Leaf) {
        $VersionName = (Get-Content -LiteralPath $versionFile -Raw).Trim()
    } else {
        $VersionName = '1.0.0'
    }
}
if ($VersionCode -le 0) {
    if ($VersionName -match '^(?<major>\d+)\.(?<minor>\d+)\.(?<patch>\d+)(?:[-+].*)?$') {
        $VersionCode = ([int]$Matches['major'] * 10000) + ([int]$Matches['minor'] * 100) + [int]$Matches['patch']
    } else {
        $VersionCode = 1
    }
}

$unsigned = Join-Path $stagingDir 'LocalCode-Remote-unsigned.apk'
$aligned = Join-Path $stagingDir 'LocalCode-Remote-aligned.apk'
$signed = Join-Path $stagingDir 'LocalCode-Remote-debug.apk'

$resourceArgs = @()
if (Test-Path -LiteralPath $resDir -PathType Container) {
    & $aapt2 compile --dir $resDir -o $compiledRes
    if ($LASTEXITCODE -ne 0) { throw "aapt2 compile resources failed with exit code $LASTEXITCODE" }
    $resourceArgs = @(Get-ChildItem -LiteralPath $compiledRes -Recurse -File -Filter *.flat | ForEach-Object FullName)
}
& $aapt2 link --manifest $manifest -I $androidJar --min-sdk-version 26 --target-sdk-version 36 --version-code $VersionCode --version-name $VersionName -o $unsigned @resourceArgs
if ($LASTEXITCODE -ne 0) { throw "aapt2 link failed with exit code $LASTEXITCODE" }

& $javac -encoding UTF-8 -source 17 -target 17 -classpath $androidJar -d $classes $javaSource
if ($LASTEXITCODE -ne 0) { throw "javac failed with exit code $LASTEXITCODE" }
$classFiles = @(Get-ChildItem -LiteralPath $classes -Recurse -Filter *.class -File | ForEach-Object FullName)
if ($classFiles.Count -eq 0) { throw 'javac did not produce any .class files' }

& $d8 --lib $androidJar --min-api 26 --output $dex @classFiles
if ($LASTEXITCODE -ne 0) { throw "d8 failed with exit code $LASTEXITCODE" }
$classesDex = Join-Path $dex 'classes.dex'
if (-not (Test-Path -LiteralPath $classesDex -PathType Leaf)) { throw 'd8 did not produce classes.dex' }
Push-Location $dex
try {
    & $aapt add $unsigned 'classes.dex'
    if ($LASTEXITCODE -ne 0) { throw "aapt add failed with exit code $LASTEXITCODE" }
} finally {
    Pop-Location
}

& $zipalign -f 4 $unsigned $aligned
if ($LASTEXITCODE -ne 0) { throw "zipalign failed with exit code $LASTEXITCODE" }

$envKeystore = if ($null -ne $env:LOCALCODE_ANDROID_KEYSTORE) { $env:LOCALCODE_ANDROID_KEYSTORE.Trim() } else { '' }
if (-not [string]::IsNullOrWhiteSpace($envKeystore) -and (Test-Path -LiteralPath $envKeystore -PathType Leaf)) {
    $ksAlias = if ($null -ne $env:LOCALCODE_ANDROID_KEY_ALIAS -and $env:LOCALCODE_ANDROID_KEY_ALIAS.Trim() -ne '') { $env:LOCALCODE_ANDROID_KEY_ALIAS.Trim() } else { 'key0' }
    $keyPassArg = if ($null -ne $env:LOCALCODE_ANDROID_KEY_PASS -and $env:LOCALCODE_ANDROID_KEY_PASS -ne '') { 'env:LOCALCODE_ANDROID_KEY_PASS' } else { 'env:LOCALCODE_ANDROID_KEYSTORE_PASS' }
    & $apksigner sign --ks $envKeystore --ks-pass 'env:LOCALCODE_ANDROID_KEYSTORE_PASS' --ks-key-alias $ksAlias --key-pass $keyPassArg --out $signed $aligned
    if ($LASTEXITCODE -ne 0) { throw "apksigner release sign failed with exit code $LASTEXITCODE" }
} else {
    $localAppDataRoot = if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) { $env:LOCALAPPDATA } else { Join-Path $HOME '.localcode' }
    $debugKeystoreDir = Join-Path $localAppDataRoot 'LocalCode'
    if (-not (Test-Path -LiteralPath $debugKeystoreDir -PathType Container)) {
        New-Item -ItemType Directory -Path $debugKeystoreDir -Force | Out-Null
    }
    $debugKeystore = Join-Path $debugKeystoreDir 'android-debug.keystore'
    if (-not (Test-Path -LiteralPath $debugKeystore -PathType Leaf)) {
        & $keytool -genkeypair -keystore $debugKeystore -storepass android -keypass android -alias androiddebugkey -dname 'CN=LocalCode Debug,O=inetconnector,C=DE' -keyalg RSA -keysize 2048 -validity 3650 -noprompt
        if ($LASTEXITCODE -ne 0) { throw "keytool failed with exit code $LASTEXITCODE" }
    }
    & $apksigner sign --ks $debugKeystore --ks-pass pass:android --ks-key-alias androiddebugkey --key-pass pass:android --out $signed $aligned
    if ($LASTEXITCODE -ne 0) { throw "apksigner debug sign failed with exit code $LASTEXITCODE" }
}
& $apksigner verify --verbose $signed
if ($LASTEXITCODE -ne 0) { throw "apksigner verify failed with exit code $LASTEXITCODE" }

if (-not (Test-Path -LiteralPath $signed -PathType Leaf)) { throw "Signed APK missing: $signed" }

if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repo 'dist'
}
if (-not (Test-Path -LiteralPath $OutputDirectory -PathType Container)) {
    New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
}
$targetApk = Join-Path $OutputDirectory 'LocalCode-Remote-debug.apk'
Copy-Item -LiteralPath $signed -Destination $targetApk -Force

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $targetApk).Hash.ToLowerInvariant()
$sizeBytes = (Get-Item -LiteralPath $targetApk).Length
$metaObj = [ordered]@{
    version_name = $VersionName
    version_code = $VersionCode
    sha256       = $hash
    size_bytes   = $sizeBytes
    built_at     = (Get-Date).ToUniversalTime().ToString('o')
}
$metaJson = $metaObj | ConvertTo-Json -Depth 3
$targetMeta = $targetApk + '.json'
Set-Content -LiteralPath $targetMeta -Value $metaJson -Encoding UTF8

$distDir = Join-Path $repo 'dist'
if ((Test-Path -LiteralPath $distDir -PathType Container) -and ([IO.Path]::GetFullPath($OutputDirectory) -ne [IO.Path]::GetFullPath($distDir))) {
    $distApk = Join-Path $distDir 'LocalCode-Remote-debug.apk'
    Copy-Item -LiteralPath $signed -Destination $distApk -Force
    Set-Content -LiteralPath ($distApk + '.json') -Value $metaJson -Encoding UTF8
}

Remove-Item -LiteralPath $stagingDir -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "Android APK: $targetApk"
Write-Host "Android APK Version: $VersionName ($VersionCode)"
Write-Host "Android APK SHA-256: $hash"
