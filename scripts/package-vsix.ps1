# SPDX-License-Identifier: Apache-2.0
param(
    [string]$OutFile = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$ExtDir = Join-Path $Root 'extensions\localcode'
$DistDir = Join-Path $Root 'dist'

if (-not (Test-Path -LiteralPath $DistDir)) {
    New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
}

$PackageJson = Get-Content -LiteralPath (Join-Path $ExtDir 'package.json') -Raw | ConvertFrom-Json
$Version = $PackageJson.version
if ([string]::IsNullOrWhiteSpace($OutFile)) {
    $OutFile = Join-Path $DistDir "localcode-$Version.vsix"
}

$Staging = Join-Path $env:TEMP ("localcode-vsix-" + [Guid]::NewGuid().ToString('N'))
if (Test-Path -LiteralPath $Staging) {
    Remove-Item -LiteralPath $Staging -Recurse -Force
}

$ExtStaging = Join-Path $Staging 'extension'
New-Item -ItemType Directory -Path $ExtStaging -Force | Out-Null

# Copy essential runtime files to extension staging
Copy-Item -LiteralPath (Join-Path $ExtDir 'package.json') -Destination $ExtStaging -Force
Copy-Item -LiteralPath (Join-Path $ExtDir 'package.nls.json') -Destination $ExtStaging -Force
Copy-Item -LiteralPath (Join-Path $ExtDir 'package.nls.de.json') -Destination $ExtStaging -Force
Copy-Item -LiteralPath (Join-Path $ExtDir 'README.md') -Destination (Join-Path $ExtStaging 'readme.md') -Force
Copy-Item -LiteralPath (Join-Path $ExtDir 'LICENSE') -Destination (Join-Path $ExtStaging 'LICENSE.txt') -Force

Copy-Item -LiteralPath (Join-Path $ExtDir 'src') -Destination $ExtStaging -Recurse -Force
Copy-Item -LiteralPath (Join-Path $ExtDir 'media') -Destination $ExtStaging -Recurse -Force

# Generate manifest and content types
$ManifestXml = @"
<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Language="en-US" Id="localcode" Version="$Version" Publisher="inetconnector"/>
    <DisplayName>LocalCode</DisplayName>
    <Description xml:space="preserve">AI coding agent with local models, full privacy, and controlled tool execution.</Description>
    <Tags>localcode,ollama,local,coding agent,antigravity,ai</Tags>
    <Categories>AI,Other</Categories>
    <GalleryFlags>Public</GalleryFlags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.85.0" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionDependencies" Value="" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionPack" Value="" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" />
      <Property Id="Microsoft.VisualStudio.Code.LocalizedLanguages" Value="de,en" />
    </Properties>
    <License>extension/LICENSE.txt</License>
    <Icon>extension/media/localcode.svg</Icon>
  </Metadata>
  <Installation>
    <InstallationTarget Id="Microsoft.VisualStudio.Code"/>
  </Installation>
  <Dependencies/>
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" />
    <Asset Type="Microsoft.VisualStudio.Services.Content.Details" Path="extension/readme.md" Addressable="true" />
    <Asset Type="Microsoft.VisualStudio.Services.Content.License" Path="extension/LICENSE.txt" Addressable="true" />
    <Asset Type="Microsoft.VisualStudio.Services.Icons.Default" Path="extension/media/localcode.svg" Addressable="true" />
  </Assets>
</PackageManifest>
"@

$ContentTypesXml = @"
<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="json" ContentType="application/json"/>
  <Default Extension="js" ContentType="application/javascript"/>
  <Default Extension="css" ContentType="text/css"/>
  <Default Extension="html" ContentType="text/html"/>
  <Default Extension="svg" ContentType="image/svg+xml"/>
  <Default Extension="md" ContentType="text/markdown"/>
  <Default Extension="txt" ContentType="text/plain"/>
  <Default Extension="vsixmanifest" ContentType="text/xml"/>
  <Override PartName="/extension.vsixmanifest" ContentType="text/xml"/>
  <Override PartName="/[Content_Types].xml" ContentType="text/xml"/>
</Types>
"@

Set-Content -LiteralPath (Join-Path $Staging 'extension.vsixmanifest') -Value $ManifestXml -Encoding Utf8
Set-Content -LiteralPath (Join-Path $Staging '[Content_Types].xml') -Value $ContentTypesXml -Encoding Utf8

if (Test-Path -LiteralPath $OutFile) {
    Remove-Item -LiteralPath $OutFile -Force
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::CreateFromDirectory($Staging, $OutFile, [System.IO.Compression.CompressionLevel]::Optimal, $false)

Remove-Item -LiteralPath $Staging -Recurse -Force

Write-Host "VSIX package built successfully: $OutFile"
