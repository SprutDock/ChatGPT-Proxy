param([switch]$Background, [int]$LauncherPid = 0)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms

$root = $PSScriptRoot
$agent = Join-Path $root 'cert-agent.exe'
$logDirectory = Join-Path $env:LOCALAPPDATA 'SprutDock\ChatGPT Proxy'
$logPath = Join-Path $logDirectory 'launcher.log'
$localProxy = 'http://127.0.0.1:3128'

function Write-Log($message) {
    try {
        New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
        Add-Content -Path $logPath -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $message"
    } catch {
    }
}

function Show-Message($message, $caption, $buttons, $icon) {
    return [Windows.Forms.MessageBox]::Show($message, $caption, $buttons, $icon)
}

function Invoke-Agent($arguments) {
    $stdout = [IO.Path]::GetTempFileName()
    $stderr = [IO.Path]::GetTempFileName()
    try {
        $process = Start-Process -FilePath $agent -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        $output = ((Get-Content -LiteralPath $stdout -Raw -ErrorAction SilentlyContinue) + "`n" + (Get-Content -LiteralPath $stderr -Raw -ErrorAction SilentlyContinue)).Trim()
        return [PSCustomObject]@{ ExitCode = $process.ExitCode; Output = $output }
    } finally {
        Remove-Item -LiteralPath $stdout, $stderr -Force -ErrorAction SilentlyContinue
    }
}

function Get-ReleaseArchitecture {
    try {
        $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
        if ($architecture -eq 'arm64') { return 'arm64' }
        return 'x64'
    } catch {
        if ($env:PROCESSOR_ARCHITEW6432 -match 'ARM64' -or $env:PROCESSOR_ARCHITECTURE -match 'ARM64') { return 'arm64' }
        return 'x64'
    }
}

function Start-WindowsUpdate($asset, $version) {
    $digest = [string]$asset.digest
    if ($asset.name -notmatch '^ChatGPT-Proxy-Windows-(x64|arm64)\.zip$' -or $asset.size -le 0 -or $asset.size -gt 104857600 -or $asset.browser_download_url -notmatch '^https://github\.com/' -or $digest -notmatch '^sha256:[0-9a-fA-F]{64}$') {
        throw 'GitHub вернул некорректные сведения об архиве обновления.'
    }

    $staging = Join-Path $env:TEMP ('sprutdock-proxy-update-' + [Guid]::NewGuid().ToString('N'))
    $extracted = Join-Path $staging 'package'
    New-Item -ItemType Directory -Path $extracted -Force | Out-Null
    $archive = Join-Path $staging $asset.name
    try {
        $progressPreference = $ProgressPreference
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive -TimeoutSec 60
        $ProgressPreference = $progressPreference
        $archiveSize = (Get-Item -LiteralPath $archive).Length
        if ($archiveSize -ne [long]$asset.size -or $archiveSize -le 0 -or $archiveSize -gt 104857600) {
            throw 'Размер загруженного архива обновления не совпал с ожидаемым.'
        }
        $expectedDigest = $digest.Substring(7).ToLowerInvariant()
        $actualDigest = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualDigest -ne $expectedDigest) {
            throw 'SHA-256 архива не совпал с данными GitHub.'
        }

        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            foreach ($entry in $zip.Entries) {
                $entryName = $entry.FullName.Replace('\', '/')
                if ([IO.Path]::IsPathRooted($entryName) -or $entryName -match '(^|/)\.\.(/|$)' -or $entryName -match '^[A-Za-z]:') {
                    throw 'Архив обновления содержит недопустимый путь.'
                }
            }
        } finally {
            $zip.Dispose()
        }
        Expand-Archive -LiteralPath $archive -DestinationPath $extracted -Force

        $newVersionPath = Join-Path $extracted 'version.txt'
        if (-not (Test-Path -LiteralPath (Join-Path $extracted 'ChatGPT Proxy.exe')) -or -not (Test-Path -LiteralPath (Join-Path $extracted 'cert-agent.exe')) -or -not (Test-Path -LiteralPath (Join-Path $extracted 'stunnel.exe')) -or -not (Test-Path -LiteralPath (Join-Path $extracted 'windows-launcher.ps1')) -or -not (Test-Path -LiteralPath $newVersionPath) -or (Get-Content -LiteralPath $newVersionPath -Raw).Trim() -ne $version) {
            throw 'В архиве обновления нет ожидаемой версии приложения.'
        }

        $helper = Join-Path $root 'windows-update.ps1'
        $parentProcess = if ($LauncherPid -gt 0) { $LauncherPid } else { $PID }
        $arguments = "-NoLogo -NoProfile -STA -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$helper`" -Stage `"$extracted`" -Destination `"$root`" -ParentPid $parentProcess -AgentPath `"$agent`""
        Start-Process -FilePath 'powershell.exe' -ArgumentList $arguments -WindowStyle Hidden
        Write-Log "update queued version=$version"
        return $true
    } catch {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
        throw
    }
}

function Check-ForUpdates {
    $job = $null
    try {
        $job = Start-Job -ScriptBlock {
            [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
            Invoke-RestMethod -Uri 'https://api.github.com/repos/SprutDock/ChatGPT-Proxy/releases/latest' -Headers @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'ChatGPT-Proxy' } -TimeoutSec 4
        }
        if (-not (Wait-Job -Job $job -Timeout 3)) {
            Stop-Job -Job $job -ErrorAction SilentlyContinue
            Write-Log 'update check skipped: request timed out'
            return $false
        }
        $request = Receive-Job -Job $job -ErrorAction Stop
        $version = ([string]$request.tag_name).TrimStart('v')
        $current = (Get-Content -LiteralPath (Join-Path $root 'version.txt') -Raw).Trim()
        if ($version -notmatch '^\d+(\.\d+){1,3}$' -or $current -notmatch '^\d+(\.\d+){1,3}$' -or [version]$version -le [version]$current) {
            return $false
        }
        $assetName = 'ChatGPT-Proxy-Windows-' + (Get-ReleaseArchitecture) + '.zip'
        $asset = @($request.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1)
        if ($asset.Count -eq 0) { return $false }
        $answer = Show-Message "Установлена версия $current. Доступна версия $version. Установить обновление?" 'Доступна новая версия ChatGPT Proxy' ([Windows.Forms.MessageBoxButtons]::YesNo) ([Windows.Forms.MessageBoxIcon]::Information)
        if ($answer -ne [Windows.Forms.DialogResult]::Yes) { return $false }
        $installing = Start-WindowsUpdate $asset[0] $version
        return $installing
    } catch {
        Write-Log "update check failed: $($_.Exception.Message)"
        if ($_.Exception.Message -match 'архива|SHA-256|GitHub|версии приложения') {
            [void](Show-Message $_.Exception.Message 'Не удалось установить обновление' ([Windows.Forms.MessageBoxButtons]::OK) ([Windows.Forms.MessageBoxIcon]::Error))
        }
        return $false
    } finally {
        if ($null -ne $job) {
            Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        }
    }
}

if ($Background) {
    $result = Invoke-Agent 'ensure --background'
    if (-not [string]::IsNullOrWhiteSpace($result.Output)) {
        Write-Log "background agent notice: $($result.Output)"
    }
    if ($result.ExitCode -ne 0) {
        Write-Log "background certificate check failed: $($result.Output)"
        exit $result.ExitCode
    }
    exit 0
}

if (-not (Test-Path -LiteralPath (Join-Path $root 'ChatGPT Proxy.exe')) -or -not (Test-Path -LiteralPath $agent) -or -not (Test-Path -LiteralPath (Join-Path $root 'stunnel.exe'))) {
    [void](Show-Message 'В папке приложения не найдены необходимые файлы. Переустановите ChatGPT Proxy из полного Windows-архива.' 'ChatGPT Proxy не запущен' ([Windows.Forms.MessageBoxButtons]::OK) ([Windows.Forms.MessageBoxIcon]::Error))
    exit 0
}

if (Check-ForUpdates) { exit 0 }

$chatGPT = Get-StartApps | Where-Object { $_.Name -eq 'ChatGPT' } | Select-Object -First 1
if ($null -eq $chatGPT -or [string]::IsNullOrWhiteSpace($chatGPT.AppID)) {
    [void](Show-Message 'Не найдено приложение ChatGPT из Microsoft Store. Установите ChatGPT и повторите запуск.' 'ChatGPT Proxy не запущен' ([Windows.Forms.MessageBoxButtons]::OK) ([Windows.Forms.MessageBoxIcon]::Error))
    exit 0
}

Write-Log 'proxy ensure started'
$result = Invoke-Agent 'ensure'
if ($result.ExitCode -ne 0) {
    Write-Log "proxy ensure failed: $($result.Output)"
    $message = if ([string]::IsNullOrWhiteSpace($result.Output)) { 'Регистрация, сертификат или защищённое соединение не прошли проверку. ChatGPT не запущен.' } else { $result.Output }
    [void](Show-Message $message 'Прокси SprutDock не готов' ([Windows.Forms.MessageBoxButtons]::OK) ([Windows.Forms.MessageBoxIcon]::Error))
    exit 0
}
Write-Log 'proxy ensure ready'
if (-not [string]::IsNullOrWhiteSpace($result.Output)) {
    Write-Log "certificate agent notice: $($result.Output)"
}

$existingProxies = @($env:HTTP_PROXY, $env:HTTPS_PROXY, $env:http_proxy, $env:https_proxy) | Where-Object { $_ -and $_ -ne $localProxy }
if ($existingProxies.Count -gt 0) {
    $answer = Show-Message "ChatGPT Proxy переключит этот запуск на локальный сертификатный туннель $localProxy. Переменные окружения и файлы настроек останутся без изменений." 'Прокси ChatGPT Proxy' ([Windows.Forms.MessageBoxButtons]::YesNo) ([Windows.Forms.MessageBoxIcon]::Warning)
    if ($answer -ne [Windows.Forms.DialogResult]::Yes) { exit 0 }
}

$env:HTTP_PROXY = $localProxy
$env:HTTPS_PROXY = $localProxy
$env:http_proxy = $localProxy
$env:https_proxy = $localProxy
$env:NO_PROXY = (@($env:NO_PROXY, $env:no_proxy, 'localhost,127.0.0.1,::1') | Where-Object { $_ }) -join ','
$env:no_proxy = $env:NO_PROXY

$existing = Get-Process -Name 'ChatGPT' -ErrorAction SilentlyContinue
foreach ($process in $existing) { [void]$process.CloseMainWindow() }
if ($existing) {
    Start-Sleep -Milliseconds 500
    Get-Process -Name 'ChatGPT' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}

try {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace SprutDock {
    [ComImport, Guid("2E941141-7F97-4756-BA1D-9DECDE894A3D"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    public interface IApplicationActivationManager {
        [PreserveSig] int ActivateApplication([MarshalAs(UnmanagedType.LPWStr)] string appUserModelId, [MarshalAs(UnmanagedType.LPWStr)] string arguments, uint options, out uint processId);
    }
    [ComImport, Guid("45BA127D-10A8-46EA-8AB7-56EA9078943C"), ClassInterface(ClassInterfaceType.None)]
    public class ApplicationActivationManager { }
    public static class ApplicationActivation {
        public static int Activate(string appUserModelId, string arguments, out uint processId) {
            IApplicationActivationManager manager = (IApplicationActivationManager)Activator.CreateInstance(Type.GetTypeFromCLSID(new Guid("45BA127D-10A8-46EA-8AB7-56EA9078943C")));
            return manager.ActivateApplication(appUserModelId, arguments, 0, out processId);
        }
    }
}
'@
    [uint32]$processId = 0
    $status = [SprutDock.ApplicationActivation]::Activate($chatGPT.AppID, "--proxy-server=$localProxy", [ref]$processId)
    if ($status -lt 0) { [System.Runtime.InteropServices.Marshal]::ThrowExceptionForHR($status) }
    Write-Log 'ChatGPT process launched'
} catch {
    Write-Log "ChatGPT launch failed: $($_.Exception.Message)"
    [void](Show-Message 'Не удалось запустить приложение ChatGPT через сертификатный туннель.' 'ChatGPT Proxy не запущен' ([Windows.Forms.MessageBoxButtons]::OK) ([Windows.Forms.MessageBoxIcon]::Error))
    exit 0
}
