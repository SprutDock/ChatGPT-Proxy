$ErrorActionPreference = 'Stop'

function Show-LauncherError([string]$Message) {
    try {
        Add-Type -AssemblyName System.Windows.Forms
        [System.Windows.Forms.MessageBox]::Show(
            $Message,
            'ChatGPT Proxy',
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Error
        ) | Out-Null
    } catch {
        [Console]::Error.WriteLine($Message)
    }
}

try {
    $codexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE '.codex' }
    $envFile = Join-Path $codexHome '.env'
    $proxyUrlFile = Join-Path $PSScriptRoot 'proxy-url.txt'

    if (-not (Test-Path -LiteralPath $proxyUrlFile -PathType Leaf)) {
        throw "Не найден файл $proxyUrlFile"
    }
    $defaultProxy = [System.IO.File]::ReadAllText($proxyUrlFile).Trim()
    if (-not $defaultProxy.StartsWith('http://')) { throw "Некорректный proxy URL в $proxyUrlFile" }

    $package = Get-AppxPackage -Name 'OpenAI.Codex' | Select-Object -First 1
    if (-not $package) {
        throw 'Не найден установленный пакет ChatGPT Desktop OpenAI.Codex для текущего пользователя.'
    }

    New-Item -ItemType Directory -Force -Path $codexHome | Out-Null
    if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) {
        [System.IO.File]::WriteAllText($envFile, '', [System.Text.UTF8Encoding]::new($false))
    }

    $proxyValues = @{}
    foreach ($line in Get-Content -LiteralPath $envFile) {
        if ($line -match '^\s*(?:export\s+)?(?<name>HTTP_PROXY|HTTPS_PROXY|NO_PROXY|http_proxy|https_proxy|no_proxy)\s*=\s*(?<value>.*)\s*$') {
            $name = $Matches.name.ToUpperInvariant()
            $value = $Matches.value.Trim()
            if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            $proxyValues[$name] = $value
        }
    }

    $httpProxy = $proxyValues.HTTP_PROXY
    $httpsProxy = $proxyValues.HTTPS_PROXY
    $missingProxyLines = @()
    if (-not $httpProxy) {
        $httpProxy = if ($httpsProxy) { $httpsProxy } else { $defaultProxy }
        $missingProxyLines += "HTTP_PROXY=$httpProxy"
    }
    if (-not $httpsProxy) {
        $httpsProxy = $httpProxy
        $missingProxyLines += "HTTPS_PROXY=$httpsProxy"
    }
    if ($missingProxyLines.Count -gt 0) {
        [System.IO.File]::AppendAllText($envFile, "`r`n$($missingProxyLines -join "`r`n")`r`n", [System.Text.Encoding]::ASCII)
    }

    $env:HTTP_PROXY = $httpProxy
    $env:HTTPS_PROXY = $httpsProxy
    $env:http_proxy = $httpProxy
    $env:https_proxy = $httpsProxy
    $noProxy = @($proxyValues.NO_PROXY, $env:NO_PROXY, 'localhost,127.0.0.1,::1') | Where-Object { $_ }
    $env:NO_PROXY = $noProxy -join ','
    $env:no_proxy = $env:NO_PROXY

    $manifest = [xml](Get-AppxPackageManifest -Package $package).OuterXml
    $appEntry = $manifest.SelectSingleNode("/*[local-name()='Package']/*[local-name()='Applications']/*[local-name()='Application']")
    if (-not $appEntry) { throw 'Не удалось найти приложение ChatGPT в манифесте Windows-пакета.' }
    $appId = $appEntry.GetAttribute('Id')
    if (-not $appId) { throw 'Не удалось определить App ID ChatGPT в манифесте Windows-пакета.' }

    $appExecutable = Join-Path $package.InstallLocation $appEntry.GetAttribute('Executable')
    if (-not (Test-Path -LiteralPath $appExecutable -PathType Leaf)) {
        throw "Не найден исполняемый файл ChatGPT: $appExecutable"
    }

    $processName = [System.IO.Path]::GetFileNameWithoutExtension($appExecutable)
    $runningApps = Get-Process -Name $processName -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 }
    foreach ($process in $runningApps) { $null = $process.CloseMainWindow() }
    foreach ($process in $runningApps) {
        if (-not $process.WaitForExit(5000)) {
            throw 'ChatGPT не закрылся. Закройте приложение вручную и запустите этот ярлык ещё раз.'
        }
    }

    Invoke-CommandInDesktopPackage `
        -PackageFamilyName $package.PackageFamilyName `
        -AppId $appId `
        -Command $appExecutable `
        -Args "--proxy-server=$httpProxy" | Out-Null
} catch {
    Show-LauncherError $_.Exception.Message
    exit 1
}
