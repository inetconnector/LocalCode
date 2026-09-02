# SPDX-License-Identifier: Apache-2.0
# Automated cross-engine benchmark execution and reporting script

param(
    [string]$Manifest = "benchmarks/tasks/calculator-manifest.json",
    [string]$OutDir = "benchmarks/results",
    [string]$Model = "qwen2.5-coder:14b",
    [string]$OllamaHost = "http://127.0.0.1:11434"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$RootDir = Split-Path -Parent $ScriptDir
$BinDir = Join-Path $RootDir "benchmarks/bin"
$ResultsDir = Join-Path $RootDir $OutDir

New-Item -ItemType Directory -Force -Path $BinDir, $ResultsDir | Out-Null

Write-Host "=== Building LocalCode Benchmark Tools ===" -ForegroundColor Cyan

$env:CGO_ENABLED = "0"
$SrcDir = Join-Path $RootDir "src"

Push-Location $SrcDir
try {
    go build -o (Join-Path $BinDir "localcode-bench.exe") ./cmd/localcode-bench
    go build -o (Join-Path $BinDir "localcode-bench-native.exe") ./cmd/localcode-bench-native
    go build -o (Join-Path $BinDir "localcode-bench-aider.exe") ./cmd/localcode-bench-aider
    go build -o (Join-Path $BinDir "localcode-bench-opencode.exe") ./cmd/localcode-bench-opencode
    go build -o (Join-Path $BinDir "localcode-bench-claw.exe") ./cmd/localcode-bench-claw
} finally {
    Pop-Location
}

Write-Host "Benchmark binaries built successfully in $BinDir" -ForegroundColor Green

Write-Host "`n=== Ready to execute engine benchmarks with Model: $Model, Ollama: $OllamaHost ===" -ForegroundColor Cyan
Write-Host "Manifest: $Manifest"
Write-Host "Results output folder: $ResultsDir"
