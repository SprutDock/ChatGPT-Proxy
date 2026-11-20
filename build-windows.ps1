$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
$agentSource = Join-Path $root 'cert-agent'
$clientChain = if ($env:SPRU_CLIENT_CA_CHAIN_FILE) { $env:SPRU_CLIENT_CA_CHAIN_FILE } else { Join-Path $root 'assets\client-ca-chain.pem' }
$serverCA = $env:SPRU_SERVER_CA_FILE
$stunnelDirectory = $env:SPRU_WINDOWS_STUNNEL_DIR
$architecture = if ($env:SPRU_WINDOWS_ARCH) { $env:SPRU_WINDOWS_ARCH.ToLowerInvariant() } else { 'x64' }

if ($architecture -notin @('x64', 'arm64')) { throw 'SPRU_WINDOWS_ARCH must be x64 or arm64.' }
if (-not $serverCA -or -not (Test-Path -LiteralPath $serverCA -PathType Leaf)) { throw 'Set SPRU_SERVER_CA_FILE to the public server CA file.' }
if (-not $stunnelDirectory -or -not (Test-Path -LiteralPath (Join-Path $stunnelDirectory 'stunnel.exe') -PathType Leaf)) { throw 'Set SPRU_WINDOWS_STUNNEL_DIR to a directory containing stunnel.exe and its matching runtime DLLs.' }
if (-not (Test-Path -LiteralPath $clientChain -PathType Leaf)) { throw 'The public client CA chain is unavailable.' }
if (-not (Test-Path -LiteralPath (Join-Path $agentSource 'go.mod') -PathType Leaf)) { throw 'The cert-agent source is unavailable.' }

$stunnelRoot = $stunnelDirectory
if (-not (Test-Path -LiteralPath (Join-Path $stunnelRoot 'config\openssl.cnf') -PathType Leaf)) {
    $stunnelRoot = Split-Path -Parent $stunnelDirectory
}
$stunnelConfig = Join-Path $stunnelRoot 'config'
$stunnelModules = Join-Path $stunnelRoot 'ossl-modules'
if (-not (Test-Path -LiteralPath (Join-Path $stunnelConfig 'openssl.cnf') -PathType Leaf)) { throw 'The stunnel OpenSSL configuration is unavailable.' }
if (-not (Test-Path -LiteralPath (Join-Path $stunnelConfig 'fipsmodule.cnf') -PathType Leaf)) { throw 'The stunnel FIPS provider configuration is unavailable.' }
if (-not (Test-Path -LiteralPath (Join-Path $stunnelModules 'fips.dll') -PathType Leaf)) { throw 'The stunnel OpenSSL provider modules are unavailable.' }

$clientChainText = Get-Content -LiteralPath $clientChain -Raw
if ([regex]::Matches($clientChainText, '-----BEGIN CERTIFICATE-----').Count -lt 2) { throw 'The client CA chain must contain PEM issuing and root certificates.' }
$versionMatch = [regex]::Match((Get-Content -LiteralPath (Join-Path $root 'Info.plist') -Raw), '<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>')
if (-not $versionMatch.Success) { throw 'Could not read the app version from Info.plist.' }
$version = $versionMatch.Groups[1].Value
$goArchitecture = if ($architecture -eq 'x64') { 'amd64' } else { 'arm64' }
$dist = Join-Path $root 'dist'
$stageRoot = Join-Path ([IO.Path]::GetTempPath()) ('sprutdock-proxy-build-' + [Guid]::NewGuid().ToString('N'))
$stage = Join-Path $stageRoot 'package'
$resources = Join-Path $stage 'ChatGPT Proxy Files'
$archive = Join-Path $dist "ChatGPT-Proxy-Windows-$architecture.zip"

New-Item -ItemType Directory -Path $stage, $resources, $dist -Force | Out-Null
try {
    Copy-Item -LiteralPath (Join-Path $root 'windows-launcher.ps1'), (Join-Path $root 'windows-update.ps1') -Destination $resources
    Copy-Item -LiteralPath $clientChain -Destination (Join-Path $resources 'client-ca-chain.pem')
    Copy-Item -LiteralPath $serverCA -Destination (Join-Path $resources 'server-ca.pem')
    Copy-Item -LiteralPath (Join-Path $stunnelDirectory 'stunnel.exe') -Destination (Join-Path $resources 'stunnel.exe')
    Get-ChildItem -LiteralPath $stunnelDirectory -Filter '*.dll' -File | Copy-Item -Destination $resources
    $stageConfig = Join-Path $resources 'config'
    New-Item -ItemType Directory -Path $stageConfig -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $stunnelConfig 'openssl.cnf'), (Join-Path $stunnelConfig 'fipsmodule.cnf') -Destination $stageConfig
    Copy-Item -LiteralPath $stunnelModules -Destination $resources -Recurse
    Set-Content -LiteralPath (Join-Path $resources 'version.txt') -Value $version -NoNewline -Encoding ascii

    $oldGoOS = $env:GOOS
    $oldGoARCH = $env:GOARCH
    $oldCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = $goArchitecture
        $env:CGO_ENABLED = '0'
		& go -C $agentSource build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $resources 'cert-agent.exe') ./cmd/cert-agent
        if ($LASTEXITCODE -ne 0) { throw 'The Windows certificate agent build failed.' }
		& go -C $agentSource build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $stage 'ChatGPT Proxy.exe') ./cmd/windows-launcher
        if ($LASTEXITCODE -ne 0) { throw 'The Windows launcher build failed.' }
    } finally {
        $env:GOOS = $oldGoOS
        $env:GOARCH = $oldGoARCH
        $env:CGO_ENABLED = $oldCgo
    }

    Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -CompressionLevel Optimal
    Write-Output "Created $archive"
} finally {
    Remove-Item -LiteralPath $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
}
