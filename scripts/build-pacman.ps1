param(
    [string]$ProjectDir = "C:\Users\frede\Projekte\PacmanNative"
)

$ErrorActionPreference = 'Stop'

Write-Host "=========================================="
Write-Host "Building Pacman Android APK"
Write-Host "=========================================="

$sdkDir = "C:\Users\frede\AppData\Local\Android\Sdk"
$buildToolsDir = "$sdkDir\build-tools\37.0.0"
$platformJar = "$sdkDir\platforms\android-34\android.jar"
if (-not (Test-Path $platformJar)) {
    $platforms = Get-ChildItem "$sdkDir\platforms" -Directory
    if ($platforms.Count -gt 0) {
        $platformJar = "$($platforms[0].FullName)\android.jar"
    } else {
        throw "No android.jar found in $sdkDir\platforms"
    }
}
Write-Host "Using Android Platform: $platformJar"

$aapt2 = "$buildToolsDir\aapt2.exe"
$d8 = "$buildToolsDir\d8.bat"
$zipalign = "$buildToolsDir\zipalign.exe"
$apksigner = "$buildToolsDir\apksigner.bat"
$adb = "$sdkDir\platform-tools\adb.exe"

# JDK javac and keytool
$javac = "C:\Program Files\Microsoft\jdk-17.0.19.10-hotspot\bin\javac.exe"
if (-not (Test-Path $javac)) {
    $javac = "C:\Program Files\Android\openjdk\jdk-21.0.8\bin\javac.exe"
}
$keytool = "C:\Program Files\Android\openjdk\jdk-21.0.8\bin\keytool.exe"

$buildDir = "$ProjectDir\build"
$compiledResDir = "$buildDir\compiled-res"
$genDir = "$buildDir\gen"
$classesDir = "$buildDir\classes"
$outDir = "$buildDir\outputs\apk\debug"

New-Item -ItemType Directory -Force -Path $compiledResDir, $genDir, $classesDir, $outDir | Out-Null

Write-Host "1. Compiling Android Resources (aapt2 compile)..."
& $aapt2 compile --dir "$ProjectDir\res" -o "$compiledResDir\res.zip"
if ($LASTEXITCODE -ne 0) { throw "aapt2 compile failed with exit code $LASTEXITCODE" }

Write-Host "2. Linking Android Resources & generating R.java (aapt2 link)..."
& $aapt2 link -I $platformJar --manifest "$ProjectDir\AndroidManifest.xml" -o "$compiledResDir\resources.zip" --java $genDir "$compiledResDir\res.zip" --auto-add-overlay
if ($LASTEXITCODE -ne 0) { throw "aapt2 link failed with exit code $LASTEXITCODE" }

Write-Host "3. Compiling Java Source Files (javac)..."
$javaFiles = @(Get-ChildItem -Path "$ProjectDir\java", $genDir -Filter *.java -Recurse | Select-Object -ExpandProperty FullName)
Write-Host "  Found $($javaFiles.Count) Java files to compile."
& $javac -cp "$platformJar;$classesDir" -d $classesDir -sourcepath "$ProjectDir\java;$genDir" $javaFiles
if ($LASTEXITCODE -ne 0) { throw "javac compilation failed with exit code $LASTEXITCODE" }

Write-Host "4. Compiling Bytecode to Dalvik Executable (d8)..."
$classFiles = @(Get-ChildItem -Path $classesDir -Filter *.class -Recurse | Select-Object -ExpandProperty FullName)
& $d8 --output $outDir --lib $platformJar --min-api 26 $classFiles
if ($LASTEXITCODE -ne 0) { throw "d8 failed with exit code $LASTEXITCODE" }

$dexFile = "$outDir\classes.dex"
if (-not (Test-Path $dexFile)) { throw "classes.dex not found at $dexFile" }

Write-Host "5. Packaging DEX into APK container..."
$unalignedApk = "$outDir\app-unaligned.apk"
Copy-Item -Path "$compiledResDir\resources.zip" -Destination $unalignedApk -Force

# Pure PowerShell / .NET Zip injection
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$mode = [System.IO.Compression.ZipArchiveMode]::Update
$zipArchive = [System.IO.Compression.ZipFile]::Open($unalignedApk, $mode)
$existingEntry = $zipArchive.GetEntry("classes.dex")
if ($existingEntry) { $existingEntry.Delete() }
[System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zipArchive, $dexFile, "classes.dex", [System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
$zipArchive.Dispose()
Write-Host "  Injected classes.dex ($((Get-Item $dexFile).Length) bytes)."

Write-Host "6. Aligning APK (zipalign)..."
$alignedApk = "$outDir\app-aligned.apk"
if (Test-Path $alignedApk) { Remove-Item $alignedApk -Force }
& $zipalign -v -p 4 $unalignedApk $alignedApk
if ($LASTEXITCODE -ne 0) { throw "zipalign failed with exit code $LASTEXITCODE" }

Write-Host "7. Signing APK (debug keystore / apksigner)..."
$debugKeystore = "$env:USERPROFILE\.android\debug.keystore"
if (-not (Test-Path $debugKeystore)) {
    New-Item -ItemType Directory -Force -Path (Split-Path $debugKeystore) | Out-Null
    & $keytool -genkey -v -keystore $debugKeystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US"
}

$finalApk = "$outDir\app-debug.apk"
& $apksigner sign --ks $debugKeystore --ks-pass pass:android --ks-key-alias androiddebugkey --key-pass pass:android --out $finalApk $alignedApk
if ($LASTEXITCODE -ne 0) { throw "apksigner failed with exit code $LASTEXITCODE" }

Write-Host "`n=========================================="
Write-Host "BUILD SUCCESSFUL!"
Write-Host "Final APK: $finalApk"
Write-Host "Size: $((Get-Item $finalApk).Length) bytes"
Write-Host "=========================================="

Write-Host "`n8. Checking ADB Devices..."
$adbDevices = & $adb devices
$adbDevices

$deviceLines = $adbDevices | Where-Object { $_ -match '\tdevice$' }
if ($deviceLines.Count -gt 0) {
    Write-Host "`nFound $($deviceLines.Count) connected device(s). Installing APK..."
    & $adb install -r $finalApk
    Write-Host "Launching Pacman App..."
    & $adb shell am start -n "com.pacman.nativeapp/.PacmanActivity"
} else {
    Write-Host "`nKein aktives Android-Gerät über ADB verbunden."
    Write-Host "Sobald du dein Smartphone per USB (USB-Debugging aktiv) oder Wireless ADB verbindest, kann die APK mit:"
    Write-Host "  adb install -r $finalApk"
    Write-Host "installiert werden!"
}
