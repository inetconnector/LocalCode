param(
    [string]$OutputDirectory = ""
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

if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
        $OutputDirectory = Join-Path ([IO.Path]::GetTempPath()) 'localcode-android'
    } else {
        $OutputDirectory = Join-Path $env:RUNNER_TEMP 'localcode-android'
    }
}
Remove-Item -LiteralPath $OutputDirectory -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$classes = Join-Path $OutputDirectory 'classes'
$dex = Join-Path $OutputDirectory 'dex'
$compiledRes = Join-Path $OutputDirectory 'compiled-res'
New-Item -ItemType Directory -Path $classes,$dex,$compiledRes -Force | Out-Null

$unsigned = Join-Path $OutputDirectory 'LocalCode-Remote-unsigned.apk'
$aligned = Join-Path $OutputDirectory 'LocalCode-Remote-aligned.apk'
$signed = Join-Path $OutputDirectory 'LocalCode-Remote-debug.apk'
$keystore = Join-Path $OutputDirectory 'debug.keystore'

$resourceArgs = @()
if (Test-Path -LiteralPath $resDir -PathType Container) {
    & $aapt2 compile --dir $resDir -o $compiledRes
    if ($LASTEXITCODE -ne 0) { throw "aapt2 compile resources failed with exit code $LASTEXITCODE" }
    $resourceArgs = @(Get-ChildItem -LiteralPath $compiledRes -Recurse -File -Filter *.flat | ForEach-Object FullName)
}
& $aapt2 link --manifest $manifest -I $androidJar --min-sdk-version 26 --target-sdk-version 36 --version-code 1 --version-name '1.0' -o $unsigned @resourceArgs
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

$releaseKeystore = '\\diskstation\dani\apk-sign\4711Google!'
if (Test-Path -LiteralPath $releaseKeystore -PathType Leaf) {
    & $apksigner sign --ks $releaseKeystore --ks-pass "pass:4711Google!" --ks-key-alias "key0" --key-pass "pass:4711Google!" --out $signed $aligned
    if ($LASTEXITCODE -ne 0) { throw "apksigner release sign failed with exit code $LASTEXITCODE" }
} else {
    & $keytool -genkeypair -keystore $keystore -storepass android -keypass android -alias androiddebugkey -dname 'CN=LocalCode Debug,O=inetconnector,C=DE' -keyalg RSA -keysize 2048 -validity 3650 -noprompt
    if ($LASTEXITCODE -ne 0) { throw "keytool failed with exit code $LASTEXITCODE" }
    & $apksigner sign --ks $keystore --ks-pass pass:android --key-pass pass:android --out $signed $aligned
    if ($LASTEXITCODE -ne 0) { throw "apksigner debug sign failed with exit code $LASTEXITCODE" }
}
& $apksigner verify --verbose $signed
if ($LASTEXITCODE -ne 0) { throw "apksigner verify failed with exit code $LASTEXITCODE" }

if (-not (Test-Path -LiteralPath $signed -PathType Leaf)) { throw "Signed APK missing: $signed" }
$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $signed).Hash.ToLowerInvariant()
Write-Host "Android APK: $signed"
Write-Host "Android APK SHA-256: $hash"
