param([switch]$Background)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$agent = Join-Path $root 'cert-agent.exe'

if ($Background) {
    if (-not (Test-Path -LiteralPath $agent -PathType Leaf)) {
        exit 1
    }
    & $agent ensure --background
    exit $LASTEXITCODE
}

$launcher = Join-Path $root 'ChatGPT Proxy.exe'
if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    $launcher = Join-Path (Split-Path -Parent $root) 'ChatGPT Proxy.exe'
}
if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    Add-Type -AssemblyName System.Windows.Forms
    [void][Windows.Forms.MessageBox]::Show('ChatGPT Proxy.exe was not found. Extract the complete application archive and try again.', 'ChatGPT Proxy could not start', [Windows.Forms.MessageBoxButtons]::OK, [Windows.Forms.MessageBoxIcon]::Error)
    exit 1
}

Start-Process -FilePath $launcher -WorkingDirectory (Split-Path -Parent $launcher)
