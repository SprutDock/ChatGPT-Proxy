param(
    [Parameter(Mandatory = $true)][string]$Stage,
    [Parameter(Mandatory = $true)][string]$Destination,
    [Parameter(Mandatory = $true)][int]$ParentPid,
    [Parameter(Mandatory = $true)][string]$AgentPath
)

$ErrorActionPreference = 'Stop'
try {
    Wait-Process -Id $ParentPid -Timeout 30 -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $AgentPath) {
        $stop = Start-Process -FilePath $AgentPath -ArgumentList 'stop' -WindowStyle Hidden -Wait -PassThru
        if ($stop.ExitCode -ne 0) { throw 'Не удалось остановить сертификатный туннель для обновления.' }
    }

    $backup = Join-Path $env:TEMP ('sprutdock-proxy-backup-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $backup -Force | Out-Null
    Copy-Item -Path (Join-Path $Destination '*') -Destination $backup -Recurse -Force
    try {
        Copy-Item -Path (Join-Path $Stage '*') -Destination $Destination -Recurse -Force
    } catch {
        Remove-Item -Path (Join-Path $Destination '*') -Recurse -Force -ErrorAction SilentlyContinue
        Copy-Item -Path (Join-Path $backup '*') -Destination $Destination -Recurse -Force
        throw
    }
    Remove-Item -LiteralPath $backup, (Split-Path -Parent $Stage) -Recurse -Force -ErrorAction SilentlyContinue
    Start-Process -FilePath (Join-Path $Destination 'ChatGPT Proxy.exe')
} catch {
    Add-Type -AssemblyName System.Windows.Forms
    [void][Windows.Forms.MessageBox]::Show($_.Exception.Message, 'Не удалось установить обновление', [Windows.Forms.MessageBoxButtons]::OK, [Windows.Forms.MessageBoxIcon]::Error)
}
