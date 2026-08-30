[CmdletBinding()]
param(
    [ValidateSet('Doctor', 'Setup', 'Publish')]
    [string]$Mode = 'Doctor',
    [string]$RepositoryDir = (Split-Path -Parent $PSScriptRoot),
    [string]$InstallDir = 'C:\BBERP\server',
    [string]$CacheDir = 'C:\BBERP\tool-cache',
    [string]$RemoteName = 'origin',
    [string]$Branch = 'main',
    [string]$WindowsServiceName = '',
    [ValidateRange(1, 65535)]
    [int]$HttpPort = 8080,
    [string]$HealthBaseUrl = 'http://127.0.0.1:8080',
    [string]$DatabasePath = '',
    [ValidateRange(2, 10)]
    [int]$RetainReleases = 2
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$script:RepositoryDir = [IO.Path]::GetFullPath($RepositoryDir)
$script:InstallDir = [IO.Path]::GetFullPath($InstallDir)
$script:CacheDir = [IO.Path]::GetFullPath($CacheDir)
$configuredDatabasePath = if (-not [string]::IsNullOrWhiteSpace($DatabasePath)) { $DatabasePath } elseif (-not [string]::IsNullOrWhiteSpace($env:BB_ERP_DATABASE_PATH)) { $env:BB_ERP_DATABASE_PATH } else { 'data\erp.db' }
$script:DatabasePath = if ([IO.Path]::IsPathRooted($configuredDatabasePath)) { [IO.Path]::GetFullPath($configuredDatabasePath) } else { [IO.Path]::GetFullPath((Join-Path $script:InstallDir $configuredDatabasePath)) }
$script:ToolchainFile = Join-Path $PSScriptRoot 'windows-toolchain.json'
$script:Toolchain = Get-Content -LiteralPath $script:ToolchainFile -Raw | ConvertFrom-Json
$script:LogFile = $null
$script:EnvironmentErrors = New-Object System.Collections.Generic.List[string]
$script:RebootRequired = $false
$script:SigningPrivateKey = [Environment]::GetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY', 'Process')
$script:SigningPrivateKeyPassword = [Environment]::GetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY_PASSWORD', 'Process')
$script:UpdaterPublicKey = [Environment]::GetEnvironmentVariable('TAURI_UPDATER_PUBLIC_KEY', 'Process')
$script:NormalizedPublicKey = $null
foreach ($name in @('TAURI_SIGNING_PRIVATE_KEY', 'TAURI_SIGNING_PRIVATE_KEY_PASSWORD', 'TAURI_UPDATER_PUBLIC_KEY', 'BB_ERP_UPDATE_PUBLIC_KEY')) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
}

function Write-ReleaseLog {
    param([string]$Message, [ValidateSet('INFO', 'WARN', 'ERROR')][string]$Level = 'INFO')
    $line = '{0} [{1}] {2}' -f (Get-Date).ToString('yyyy-MM-dd HH:mm:ss'), $Level, $Message
    Write-Host $line
    if ($script:LogFile) { Add-Content -LiteralPath $script:LogFile -Value $line -Encoding UTF8 }
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$Arguments = @(),
        [string]$WorkingDirectory = $script:RepositoryDir,
        [hashtable]$Environment = @{}
    )
    Write-ReleaseLog ("Run: {0} {1}" -f $FilePath, ($Arguments -join ' '))
    $saved = @{}
    foreach ($name in $Environment.Keys) {
        $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, [string]$Environment[$name], 'Process')
    }
    try {
        Push-Location $WorkingDirectory
        try {
            & $FilePath @Arguments
            if ($LASTEXITCODE -ne 0) { throw "$FilePath exited with code $LASTEXITCODE" }
        } finally { Pop-Location }
    } finally {
        foreach ($name in $Environment.Keys) {
            [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
        }
    }
}

function Get-CommandVersion {
    param([string]$Command, [string[]]$Arguments)
    $resolved = Get-Command $Command -ErrorAction SilentlyContinue
    if (-not $resolved) { return $null }
    $output = & $resolved.Source @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    return (($output | Select-Object -First 1) -as [string]).Trim()
}

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Setup must be run from an elevated Windows PowerShell session.'
    }
}

function Test-SupportedWindows {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        $script:EnvironmentErrors.Add('This release tool only supports Windows.')
        return
    }
    $os = Get-CimInstance Win32_OperatingSystem
    if ($os.OSArchitecture -ne '64-bit') {
        $script:EnvironmentErrors.Add("Unsupported architecture: $($os.OSArchitecture); x64 is required.")
    }
    $version = [Version]$os.Version
    $isServer = [int]$os.ProductType -ne 1
    if ($isServer) {
        if ($os.Caption -notmatch 'Windows Server 2016') {
            $script:EnvironmentErrors.Add("Only Windows Server 2016 Desktop Experience is supported for the build/distribution server; detected: $($os.Caption).")
        }
        $featureCommand = Get-Command Get-WindowsFeature -ErrorAction SilentlyContinue
        if ($featureCommand) {
            $guiFeature = Get-WindowsFeature Server-Gui-Shell -ErrorAction SilentlyContinue
            if ($guiFeature -and -not $guiFeature.Installed) {
                $script:EnvironmentErrors.Add('Windows Server 2016 Desktop Experience is required; Server Core is not supported.')
            }
        }
    } elseif ($version.Major -ne 10 -or $version.Build -lt [int]$script:Toolchain.supported_os.windows_10_minimum_build) {
        $script:EnvironmentErrors.Add("Windows 10 build $($script:Toolchain.supported_os.windows_10_minimum_build) or newer is required; current build is $($version.Build).")
    }
}

function Test-DiskAndLongPaths {
    $drives = @(
        [IO.Path]::GetPathRoot($script:InstallDir)
        [IO.Path]::GetPathRoot($script:CacheDir)
        [IO.Path]::GetPathRoot($script:RepositoryDir)
    ) | Select-Object -Unique
    foreach ($drive in $drives) {
        $disk = Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='{0}'" -f $drive.TrimEnd('\'))
        if (-not $disk -or [int64]$disk.FreeSpace -lt 30GB) {
            $script:EnvironmentErrors.Add("At least 30 GiB free space is required on $drive for toolchains, builds, staging and rollback snapshots.")
        }
    }
    $longPaths = (Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name LongPathsEnabled -ErrorAction SilentlyContinue).LongPathsEnabled
    if ($longPaths -ne 1) { $script:EnvironmentErrors.Add('NTFS long paths are disabled (LongPathsEnabled != 1).') }
}

function Test-GoToolchain {
    $line = Get-CommandVersion 'go' @('version')
    $expected = "go$($script:Toolchain.go.version)"
    if (-not $line -or $line -notmatch ("^go version {0} windows/amd64$" -f [regex]::Escape($expected))) {
        $script:EnvironmentErrors.Add("Go $($script:Toolchain.go.version) must be installed manually from $($script:Toolchain.go.download_url); detected: $line")
    }
}

function Test-NodeToolchain {
    $node = Get-CommandVersion 'node' @('--version')
    $npm = Get-CommandVersion 'npm.cmd' @('--version')
    if ($node -ne "v$($script:Toolchain.node.version)") { $script:EnvironmentErrors.Add("Node.js $($script:Toolchain.node.version) is required; detected: $node") }
    if ($npm -ne [string]$script:Toolchain.node.npm_version) { $script:EnvironmentErrors.Add("npm $($script:Toolchain.node.npm_version) is required; detected: $npm") }
}

function Test-RustToolchain {
    $rustup = Get-Command 'rustup.exe' -ErrorAction SilentlyContinue
    if (-not $rustup) {
        $script:EnvironmentErrors.Add('rustup is not installed.')
        return
    }
    $rustc = & rustup.exe run $script:Toolchain.rust.toolchain rustc --version 2>$null
    if ($LASTEXITCODE -ne 0 -or $rustc -notmatch ("^rustc {0}(\s|$)" -f [regex]::Escape([string]$script:Toolchain.rust.version))) {
        $script:EnvironmentErrors.Add("Rust toolchain $($script:Toolchain.rust.toolchain) is required; detected: $rustc")
    }
    $targets = & rustup.exe target list --installed --toolchain $script:Toolchain.rust.toolchain 2>$null
    if ($targets -notcontains [string]$script:Toolchain.rust.target) { $script:EnvironmentErrors.Add("Rust target $($script:Toolchain.rust.target) is missing.") }
    $components = & rustup.exe component list --installed --toolchain $script:Toolchain.rust.toolchain 2>$null
    if (-not ($components -match '^rustfmt-')) { $script:EnvironmentErrors.Add('Rust component rustfmt is missing.') }
}

function Test-VisualStudioBuildTools {
    $programFilesX86 = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFilesX86)
    $vswhere = Join-Path $programFilesX86 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (-not (Test-Path -LiteralPath $vswhere -PathType Leaf)) {
        $script:EnvironmentErrors.Add('Visual Studio Build Tools 2022 is missing.')
        return
    }
    $installation = & $vswhere -products Microsoft.VisualStudio.Product.BuildTools -version '[17.0,18.0)' -requires $script:Toolchain.visual_studio.workload $script:Toolchain.visual_studio.windows_sdk_component -property installationPath
    if (-not $installation) { $script:EnvironmentErrors.Add('Visual Studio Build Tools 2022 C++ workload, MSVC x64 tools or Windows 10 SDK is incomplete.') }
}

function Test-WebView2Runtime {
    $clients = @(
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F1E7E3B6-50F1-4B0C-A1C0-9F8D3F6A9A7A}',
        'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F1E7E3B6-50F1-4B0C-A1C0-9F8D3F6A9A7A}'
    )
    $found = $false
    foreach ($path in $clients) { if (Test-Path $path) { $found = $true } }
    if (-not $found) {
        $programFilesX86 = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFilesX86)
        $runtimeRoot = Join-Path $programFilesX86 'Microsoft\EdgeWebView\Application'
        $runtime = Get-ChildItem $runtimeRoot -Directory -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $runtime) { $script:EnvironmentErrors.Add('Microsoft Edge WebView2 Runtime is missing.') }
    }
}

function Update-ProcessPath {
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $env:Path = "$machinePath;$userPath"
}

function Test-RequirementAndRepair {
    param([scriptblock]$Test, [scriptblock]$Repair)
    $script:EnvironmentErrors.Clear()
    & $Test
    if ($script:EnvironmentErrors.Count -gt 0) {
        foreach ($issue in $script:EnvironmentErrors) { Write-ReleaseLog $issue 'WARN' }
        $script:EnvironmentErrors.Clear()
        & $Repair
        Update-ProcessPath
        if ($script:RebootRequired) {
            Write-ReleaseLog 'An installer requires a reboot. The script will not reboot automatically; restart Windows and run Doctor.' 'WARN'
            exit 3010
        }
    }
}

function Test-Msys2Gcc {
    $gcc = Join-Path $script:Toolchain.msys2.install_dir 'mingw64\bin\gcc.exe'
    if (-not (Test-Path -LiteralPath $gcc -PathType Leaf)) { $script:EnvironmentErrors.Add("MSYS2 MinGW64 GCC is missing at $gcc") }
}

function Invoke-Doctor {
    $script:EnvironmentErrors.Clear()
    Test-SupportedWindows
    Test-DiskAndLongPaths
    Test-GoToolchain
    if (-not (Get-Command 'git.exe' -ErrorAction SilentlyContinue)) { $script:EnvironmentErrors.Add('Git for Windows is missing.') }
    Test-NodeToolchain
    Test-RustToolchain
    Test-VisualStudioBuildTools
    Test-WebView2Runtime
    Test-Msys2Gcc
    if ($script:EnvironmentErrors.Count -gt 0) {
        foreach ($issue in $script:EnvironmentErrors) { Write-ReleaseLog $issue 'ERROR' }
        return $false
    }
    Write-ReleaseLog 'Windows build environment matches the repository toolchain lock.'
    return $true
}

function Get-CachedInstaller {
    param([string]$Name, [string]$Url, [string]$ExpectedSha256 = '', [string[]]$AllowedPublishers = @())
    New-Item -ItemType Directory -Force -Path $script:CacheDir | Out-Null
    $path = Join-Path $script:CacheDir $Name
    $hashRecord = "$path.sha256"
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $temporary = "$path.download"
        Write-ReleaseLog "Download $Url"
        Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $temporary
        Move-Item -LiteralPath $temporary -Destination $path -Force
    }
    $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($ExpectedSha256 -and $hash -ne $ExpectedSha256.ToLowerInvariant()) { throw "SHA-256 mismatch for $path" }
    if (-not $ExpectedSha256 -and (Test-Path -LiteralPath $hashRecord -PathType Leaf)) {
        $cachedHash = (Get-Content -LiteralPath $hashRecord -Raw).Trim().ToLowerInvariant()
        if ($cachedHash -ne $hash) { throw "Cached installer SHA-256 mismatch for $path" }
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $path
    if ($AllowedPublishers.Count -gt 0) {
        if ($signature.Status -ne 'Valid') { throw "Installer signature is not valid: $path ($($signature.Status))" }
        $subject = $signature.SignerCertificate.Subject
        $accepted = $false
        foreach ($publisher in $AllowedPublishers) { if ($subject -like "*$publisher*") { $accepted = $true } }
        if (-not $accepted) { throw "Unexpected installer publisher for $path: $subject" }
    }
    if (-not (Test-Path -LiteralPath $hashRecord -PathType Leaf)) {
        Set-Content -LiteralPath $hashRecord -Value $hash -Encoding ASCII
    }
    Write-ReleaseLog "Verified installer SHA-256 $hash : $path"
    return $path
}

function Invoke-Installer {
    param([string]$Path, [string[]]$Arguments)
    $process = Start-Process -FilePath $Path -ArgumentList $Arguments -Wait -PassThru
    if ($process.ExitCode -in @(1641, 3010)) { $script:RebootRequired = $true; return }
    if ($process.ExitCode -ne 0) { throw "Installer failed with code $($process.ExitCode): $Path" }
}

function Install-Node {
    $checksums = Join-Path $script:CacheDir "node-$($script:Toolchain.node.version)-SHASUMS256.txt"
    $fileName = "node-v$($script:Toolchain.node.version)-x64.msi"
    $cachedInstaller = Join-Path $script:CacheDir $fileName
    if (-not (Test-Path $checksums) -and -not ((Test-Path $cachedInstaller) -and (Test-Path "$cachedInstaller.sha256"))) {
        Invoke-WebRequest -UseBasicParsing -Uri $script:Toolchain.node.checksums_url -OutFile $checksums
    }
    if (-not (Test-Path $checksums)) {
        $installer = Get-CachedInstaller $fileName $script:Toolchain.node.installer_url '' @('OpenJS Foundation', 'Node.js Foundation')
        Invoke-Installer $installer @('/qn', '/norestart')
        return
    }
    $line = Get-Content $checksums | Where-Object { $_ -match "\s+$([regex]::Escape($fileName))$" } | Select-Object -First 1
    if (-not $line) { throw "Official Node checksum is missing for $fileName" }
    $sha = ($line -split '\s+')[0]
    $installer = Get-CachedInstaller $fileName $script:Toolchain.node.installer_url $sha @('OpenJS Foundation', 'Node.js Foundation')
    Invoke-Installer $installer @('/qn', '/norestart')
}

function Install-Rust {
    # rustup-init.exe is not Authenticode-signed upstream. Pin it to the
    # SHA-256 published next to the official static.rust-lang.org binary.
    $installer = Get-CachedInstaller 'rustup-init.exe' $script:Toolchain.rust.rustup_installer_url $script:Toolchain.rust.rustup_installer_sha256
    Invoke-Installer $installer @('-y', '--default-toolchain', 'none', '--profile', 'minimal')
    $cargoBin = Join-Path $env:USERPROFILE '.cargo\bin'
    if ($env:Path -notlike "*$cargoBin*") { $env:Path = "$cargoBin;$env:Path" }
    Invoke-Checked 'rustup.exe' @('toolchain', 'install', $script:Toolchain.rust.toolchain, '--profile', 'minimal', '--component', 'rustfmt', '--target', $script:Toolchain.rust.target)
}

function Install-BuildTools {
    $bootstrapper = Get-CachedInstaller 'vs_BuildTools.exe' $script:Toolchain.visual_studio.bootstrapper_url '' @('Microsoft Corporation')
    $layout = Join-Path $script:CacheDir 'vs2022-layout'
    $layoutSetup = Join-Path $layout 'vs_setup.exe'
    if (-not (Test-Path -LiteralPath $layoutSetup -PathType Leaf)) {
        New-Item -ItemType Directory -Force -Path $layout | Out-Null
        Invoke-Installer $bootstrapper @('--layout', $layout, '--lang', 'en-US', '--add', $script:Toolchain.visual_studio.workload, '--includeRecommended', '--add', $script:Toolchain.visual_studio.windows_sdk_component)
    }
    $verifiedLayoutSetup = Get-CachedInstaller 'vs2022-layout\vs_setup.exe' 'offline-cache' '' @('Microsoft Corporation')
    Invoke-Installer $verifiedLayoutSetup @('--quiet', '--wait', '--norestart', '--noweb', '--add', $script:Toolchain.visual_studio.workload, '--includeRecommended', '--add', $script:Toolchain.visual_studio.windows_sdk_component)
}

function Install-WebView2 {
    $installer = Get-CachedInstaller 'MicrosoftEdgeWebView2RuntimeInstallerX64.exe' $script:Toolchain.webview2.installer_url '' @('Microsoft Corporation')
    Invoke-Installer $installer @('/silent', '/install')
}

function Install-Msys2 {
    if (-not [string]::Equals([IO.Path]::GetFullPath($script:Toolchain.msys2.install_dir).TrimEnd('\'), 'C:\msys64', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'The locked MSYS2 self-extracting archive requires install_dir C:\msys64.'
    }
    $fileName = [IO.Path]::GetFileName([Uri]$script:Toolchain.msys2.installer_url)
    $installer = Get-CachedInstaller $fileName $script:Toolchain.msys2.installer_url $script:Toolchain.msys2.installer_sha256 @('MSYS2', 'Christoph Reiter')
    Invoke-Installer $installer @('-y', '-oC:\')
    $bash = Join-Path $script:Toolchain.msys2.install_dir 'usr\bin\bash.exe'
    if (-not (Test-Path $bash)) { throw 'MSYS2 installation did not provide bash.exe.' }
    Invoke-Checked $bash @('-lc', ' ') $script:Toolchain.msys2.install_dir
    Invoke-Checked $bash @('-lc', 'pacman --noconfirm -Syuu') $script:Toolchain.msys2.install_dir
    Invoke-Checked $bash @('-lc', 'pacman --noconfirm -Syuu') $script:Toolchain.msys2.install_dir
    Invoke-Checked $bash @('-lc', "pacman -S --needed --noconfirm $($script:Toolchain.msys2.gcc_package)") $script:Toolchain.msys2.install_dir
}

function Install-Git {
    $cached = Get-ChildItem -LiteralPath $script:CacheDir -Filter 'Git-*-64-bit.exe' -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
    if ($cached) {
        $installer = Get-CachedInstaller $cached.Name 'offline-cache' '' @('Open Source Developer', 'Git for Windows')
    } else {
        $metadata = Invoke-RestMethod -UseBasicParsing -Uri 'https://api.github.com/repos/git-for-windows/git/releases/latest'
        $asset = $metadata.assets | Where-Object { $_.name -match '^Git-[0-9].*-64-bit\.exe$' } | Select-Object -First 1
        if (-not $asset) { throw 'Unable to resolve the official Git for Windows x64 installer.' }
        $expectedSha = if ($asset.digest -match '^sha256:(.+)$') { $Matches[1] } else { '' }
        $installer = Get-CachedInstaller $asset.name $asset.browser_download_url $expectedSha @('Open Source Developer', 'Git for Windows')
    }
    Invoke-Installer $installer @('/VERYSILENT', '/NORESTART')
}

function Invoke-Setup {
    Assert-Administrator
    $script:EnvironmentErrors.Clear()
    Test-SupportedWindows
    Test-GoToolchain
    Test-DiskAndLongPaths
    $blockingSetupErrors = @($script:EnvironmentErrors | Where-Object { $_ -notmatch '^NTFS long paths are disabled' })
    if ($blockingSetupErrors.Count -gt 0) {
        foreach ($issue in $blockingSetupErrors) { Write-ReleaseLog $issue 'ERROR' }
        throw 'Setup cannot continue until Windows, free disk space and the manually installed Go version meet repository requirements.'
    }
    New-Item -ItemType Directory -Force -Path $script:CacheDir | Out-Null
    $longPathKey = 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem'
    Set-ItemProperty -LiteralPath $longPathKey -Name LongPathsEnabled -Type DWord -Value 1
    if (-not (Get-Command git.exe -ErrorAction SilentlyContinue)) { Install-Git; Update-ProcessPath }
    Test-RequirementAndRepair ${function:Test-VisualStudioBuildTools} ${function:Install-BuildTools}
    Test-RequirementAndRepair ${function:Test-WebView2Runtime} ${function:Install-WebView2}
    Test-RequirementAndRepair ${function:Test-NodeToolchain} ${function:Install-Node}
    Test-RequirementAndRepair ${function:Test-RustToolchain} ${function:Install-Rust}
    Test-RequirementAndRepair ${function:Test-Msys2Gcc} ${function:Install-Msys2}
    if ($script:RebootRequired) {
        Write-ReleaseLog 'One or more installers require a reboot. The script will not reboot automatically.' 'WARN'
        exit 3010
    }
    if (-not (Invoke-Doctor)) { throw 'Setup completed, but Doctor still reports missing requirements. Restart Windows PowerShell or the server and run Doctor again.' }
}

function Get-StableHeadTag {
    param([string]$Remote, [string]$Head)
    $remoteTargets = @{}
    $lines = @(& git.exe -C $script:RepositoryDir ls-remote --tags $Remote 'refs/tags/v*')
    if ($LASTEXITCODE -ne 0) { throw "Unable to read authoritative tags from Gitee remote $Remote." }
    foreach ($line in $lines) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -ne 2) { continue }
        $ref = $parts[1]
        $peeled = $ref.EndsWith('^{}')
        if ($peeled) { $ref = $ref.Substring(0, $ref.Length - 3) }
        if (-not $ref.StartsWith('refs/tags/')) { continue }
        $tag = $ref.Substring('refs/tags/'.Length)
        if ($peeled -or -not $remoteTargets.ContainsKey($tag)) { $remoteTargets[$tag] = $parts[0].ToLowerInvariant() }
    }
    $valid = @()
    foreach ($tag in $remoteTargets.Keys) {
        if ($tag -match '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
            if ($remoteTargets[$tag] -ne $Head.ToLowerInvariant()) { continue }
            $localTarget = (& git.exe -C $script:RepositoryDir rev-parse "$tag^{}" 2>$null | Select-Object -First 1)
            if ($LASTEXITCODE -ne 0 -or -not $localTarget -or $localTarget.Trim().ToLowerInvariant() -ne $Head.ToLowerInvariant()) {
                throw "Gitee tag $tag does not resolve to the fetched HEAD locally; a moved or inconsistent tag was rejected."
            }
            $valid += [pscustomobject]@{ Tag = $tag; Version = [Version]$tag.Substring(1) }
        }
    }
    if ($valid.Count -eq 0) { return $null }
    return ($valid | Sort-Object Version -Descending | Select-Object -First 1)
}

function Get-InstalledVersion {
    $path = Join-Path $script:InstallDir 'version.json'
    if (-not (Test-Path $path)) { return [Version]'0.0.0' }
    $metadata = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    $value = if ($metadata.server_version) { $metadata.server_version } else { $metadata.version }
    try { return [Version]$value } catch { throw "Installed version.json contains an invalid version: $value" }
}

function Get-FileMetadata {
    param([string]$Path)
    $item = Get-Item -LiteralPath $Path
    return [ordered]@{
        sha256 = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
        size = [int64]$item.Length
    }
}

function Write-JsonUtf8 {
    param([object]$Value, [string]$Path, [int]$Depth = 10, [switch]$Compress)
    $json = if ($Compress) { $Value | ConvertTo-Json -Depth $Depth -Compress } else { $Value | ConvertTo-Json -Depth $Depth }
    [IO.File]::WriteAllText($Path, $json, (New-Object Text.UTF8Encoding($false)))
}

function Sign-ReleaseFile {
    param([string]$Path)
    Invoke-Checked 'npx.cmd' @('--no-install', 'tauri', 'signer', 'sign', $Path) (Join-Path $script:RepositoryDir 'client') @{
        TAURI_SIGNING_PRIVATE_KEY = $script:SigningPrivateKey
        TAURI_SIGNING_PRIVATE_KEY_PASSWORD = $script:SigningPrivateKeyPassword
    }
    $signaturePath = "$Path.sig"
    if (-not (Test-Path $signaturePath)) { throw "Tauri signer did not create $signaturePath" }
    $signature = (Get-Content -LiteralPath $signaturePath -Raw).Trim()
    Remove-Item -LiteralPath $signaturePath -Force
    if (-not $signature) { throw "Empty signature for $Path" }
    return $signature
}

function Clear-SigningEnvironment {
    foreach ($secret in @('TAURI_SIGNING_PRIVATE_KEY', 'TAURI_SIGNING_PRIVATE_KEY_PASSWORD', 'TAURI_UPDATER_PUBLIC_KEY', 'BB_ERP_UPDATE_PUBLIC_KEY')) {
        [Environment]::SetEnvironmentVariable($secret, $null, 'Process')
    }
    $script:SigningPrivateKey = $null
    $script:SigningPrivateKeyPassword = $null
}

function Add-ContentAddressedArtifact {
    param([string]$Source, [System.Collections.IDictionary]$Metadata)
    $artifactRoot = Join-Path $script:InstallDir 'updates\artifacts'
    New-Item -ItemType Directory -Force -Path $artifactRoot | Out-Null
    $destination = Join-Path $artifactRoot ([string]$Metadata.sha256)
    if (Test-Path -LiteralPath $destination -PathType Leaf) {
        $existing = Get-FileMetadata $destination
        if ($existing.sha256 -ne $Metadata.sha256 -or $existing.size -ne $Metadata.size) {
            throw "Content-addressed artifact collision at $destination"
        }
        return
    }
    $staged = "$destination.new-$PID"
    Copy-Item -LiteralPath $Source -Destination $staged
    try {
        $copied = Get-FileMetadata $staged
        if ($copied.sha256 -ne $Metadata.sha256 -or $copied.size -ne $Metadata.size) {
            throw "Copied artifact verification failed: $Source"
        }
        Move-Item -LiteralPath $staged -Destination $destination
    } finally {
        if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged -Force }
    }
}

function Remove-OldReleaseHistory {
    param([string]$CurrentVersion, [string]$PreviousVersion)
    $releaseRoot = Join-Path $script:InstallDir 'updates\releases'
    $artifactRoot = Join-Path $script:InstallDir 'updates\artifacts'
    $versioned = @()
    foreach ($directory in @(Get-ChildItem -LiteralPath $releaseRoot -Directory -ErrorAction SilentlyContinue)) {
        try { $versioned += [pscustomobject]@{ Directory = $directory; Version = [Version]$directory.Name } } catch {
            Write-ReleaseLog "Skip non-version release directory during cleanup: $($directory.FullName)" 'WARN'
        }
    }
    $ordered = @($versioned | Sort-Object Version -Descending)
    $retained = @($ordered | Where-Object {
        [string]$_.Version -eq $CurrentVersion -or ($PreviousVersion -and [string]$_.Version -eq $PreviousVersion)
    })
    $obsolete = @($ordered | Where-Object {
        $candidate = $_
        -not ($retained | Where-Object { $_.Directory.FullName -eq $candidate.Directory.FullName })
    })
    foreach ($entry in $obsolete) {
        Remove-Item -LiteralPath $entry.Directory.FullName -Recurse -Force
        Write-ReleaseLog "Removed obsolete release archive $($entry.Version)."
    }

    $referenced = @{}
    $canCleanArtifacts = $true
    foreach ($entry in $retained) {
        try {
            $manifestPath = Join-Path $entry.Directory.FullName 'update-manifest.json'
            $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
            $payloadBytes = [Convert]::FromBase64String([string]$manifest.client_update_v3.payload)
            $payload = [Text.Encoding]::UTF8.GetString($payloadBytes) | ConvertFrom-Json
            $referenced[[string]$payload.full.nsis.sha256] = $true
            $referenced[[string]$payload.full.portable.sha256] = $true
        } catch {
            $canCleanArtifacts = $false
            Write-ReleaseLog "Artifact cleanup skipped because retained release $($entry.Version) could not be parsed: $($_.Exception.Message)" 'WARN'
            break
        }
    }
    if ($canCleanArtifacts) {
        foreach ($artifact in @(Get-ChildItem -LiteralPath $artifactRoot -File -ErrorAction SilentlyContinue)) {
            if (-not $referenced.ContainsKey($artifact.Name)) {
                Remove-Item -LiteralPath $artifact.FullName -Force
                Write-ReleaseLog "Removed unreferenced client artifact $($artifact.Name)."
            }
        }
    }
}

function Remove-OldBackupHistory {
    $backupRoot = Join-Path $script:InstallDir 'backups'
    $backups = @(Get-ChildItem -LiteralPath $backupRoot -Directory -ErrorAction SilentlyContinue | Sort-Object Name -Descending)
    foreach ($backup in @($backups | Select-Object -Skip $RetainReleases)) {
        Remove-Item -LiteralPath $backup.FullName -Recurse -Force
        Write-ReleaseLog "Removed obsolete rollback snapshot $($backup.Name)."
    }
}

function New-ServerPackage {
    param([string]$Stage, [string]$Version, [string]$ManifestRelativePath, [int]$ServerHttpPort, [string]$ServerDatabasePath)
    if ($ServerDatabasePath -match '[^\x20-\x7e]' -or $ServerDatabasePath -match '[\r\n%!\^&|<>]') {
        throw 'DatabasePath contains characters that cannot be safely represented in the Windows startup batch file. Use an ASCII path without CMD metacharacters.'
    }
    $root = Join-Path $Stage 'server-package'
    New-Item -ItemType Directory -Force -Path (Join-Path $root 'web') | Out-Null
    Copy-Item (Join-Path $Stage 'bb-erp-server.exe') (Join-Path $root 'bb-erp-server.exe')
    Copy-Item (Join-Path $Stage 'bb-erp-updater.exe') (Join-Path $root 'bb-erp-updater.exe')
    Copy-Item (Join-Path $script:RepositoryDir 'web\dist') (Join-Path $root 'web\dist') -Recurse
    [IO.File]::WriteAllText((Join-Path $root 'update-public.key'), $script:NormalizedPublicKey, (New-Object Text.UTF8Encoding($false)))
    Write-JsonUtf8 ([ordered]@{ version = $Version; server_version = $Version; client_version = $Version; manifest_file = $ManifestRelativePath }) (Join-Path $root 'version.json')
    @"
@echo off
setlocal
cd /d "%~dp0"
set BB_ERP_APP_ENVIRONMENT=production
set BB_ERP_HTTP_HOST=0.0.0.0
set BB_ERP_HTTP_PORT=$ServerHttpPort
set "BB_ERP_DATABASE_PATH=$ServerDatabasePath"
set BB_ERP_LOG_DIR=logs
set BB_ERP_FILES_ROOT_DIR=static\uploads
set BB_ERP_WEB_ENABLED=true
set BB_ERP_WEB_DIST_DIR=web\dist
set BB_ERP_UPDATE_ENABLED=true
set BB_ERP_UPDATE_MANIFEST_FILE=updates\stable\update-manifest.json
set BB_ERP_UPDATE_SIGNING_PUBLIC_KEY=
set BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key
bb-erp-server.exe
"@ | Set-Content -LiteralPath (Join-Path $root '启动服务端.bat') -Encoding ASCII
    @'
@echo off
setlocal
cd /d "%~dp0"
if exist bb-erp-updater.pending.exe move /y bb-erp-updater.pending.exe bb-erp-updater.exe >nul
if exist bb-erp-upgrade-runner.pending.bat move /y bb-erp-upgrade-runner.pending.bat bb-erp-upgrade-runner.bat >nul
bb-erp-updater.exe -install-dir "%~dp0"
exit /b %ERRORLEVEL%
'@ | Set-Content -LiteralPath (Join-Path $root 'bb-erp-upgrade-runner.bat') -Encoding ASCII
    $zip = Join-Path $Stage 'bb-erp-server-windows.zip'
    Compress-Archive -Path (Join-Path $root '*') -DestinationPath $zip -Force
    return $zip
}

function Assert-ServiceReleaseConfiguration {
    $health = $null
    try { $health = [Uri]$HealthBaseUrl } catch { throw "HealthBaseUrl is invalid: $HealthBaseUrl" }
    if (-not $health.IsLoopback -or $health.Port -ne $HttpPort) {
        throw "HealthBaseUrl must use the loopback address and the configured HttpPort $HttpPort: $HealthBaseUrl"
    }
    if (-not $WindowsServiceName) { return }

    $expectedPaths = @{
        BB_ERP_DATABASE_PATH = $script:DatabasePath
        BB_ERP_WEB_DIST_DIR = (Join-Path $script:InstallDir 'web\dist')
        BB_ERP_UPDATE_MANIFEST_FILE = (Join-Path $script:InstallDir 'updates\stable\update-manifest.json')
        BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE = (Join-Path $script:InstallDir 'update-public.key')
    }
    foreach ($name in $expectedPaths.Keys) {
        $configured = [Environment]::GetEnvironmentVariable($name, 'Machine')
        if ([string]::IsNullOrWhiteSpace($configured)) { throw "Windows Service mode requires machine environment variable $name." }
        $resolved = if ([IO.Path]::IsPathRooted($configured)) { [IO.Path]::GetFullPath($configured) } else { [IO.Path]::GetFullPath((Join-Path $script:InstallDir $configured)) }
        if (-not [string]::Equals($resolved, [IO.Path]::GetFullPath($expectedPaths[$name]), [StringComparison]::OrdinalIgnoreCase)) {
            throw "Windows Service machine environment $name points to $resolved; expected $($expectedPaths[$name])."
        }
    }
    $expectedValues = @{
        BB_ERP_APP_ENVIRONMENT = 'production'
        BB_ERP_HTTP_HOST = '0.0.0.0'
        BB_ERP_HTTP_PORT = [string]$HttpPort
        BB_ERP_WEB_ENABLED = 'true'
        BB_ERP_UPDATE_ENABLED = 'true'
    }
    foreach ($name in $expectedValues.Keys) {
        $configured = [Environment]::GetEnvironmentVariable($name, 'Machine')
        if (-not [string]::Equals([string]$configured, [string]$expectedValues[$name], [StringComparison]::OrdinalIgnoreCase)) {
            throw "Windows Service machine environment $name must be '$($expectedValues[$name])'; detected '$configured'."
        }
    }
}

function Copy-FileAtomically {
    param([string]$Source, [string]$Destination)
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    $temporary = "$Destination.new-$PID"
    Copy-Item -LiteralPath $Source -Destination $temporary -Force
    try {
        if (Test-Path -LiteralPath $Destination -PathType Leaf) {
            [IO.File]::Replace($temporary, $Destination, $null, $true)
        } else {
            Move-Item -LiteralPath $temporary -Destination $Destination
        }
    } finally {
        if (Test-Path -LiteralPath $temporary -PathType Leaf) { Remove-Item -LiteralPath $temporary -Force }
    }
}

function Complete-RecordedActivation {
    param([Version]$InstalledVersion)
    if ($InstalledVersion -eq [Version]'0.0.0') { return }
    $releaseDir = Join-Path $script:InstallDir ("updates\releases\{0}" -f $InstalledVersion)
    $statePath = Join-Path $releaseDir 'release-state.json'
    $manifestPath = Join-Path $releaseDir 'update-manifest.json'
    $stablePath = Join-Path $script:InstallDir 'updates\stable\update-manifest.json'
    if (-not (Test-Path -LiteralPath $releaseDir -PathType Container)) {
        Write-ReleaseLog "Installed version $InstalledVersion predates the local release archive scheme; activation bookkeeping will begin with its next tagged upgrade." 'WARN'
        return
    }
    foreach ($required in @($statePath, $manifestPath, $stablePath, (Join-Path $releaseDir 'bb-erp-server-windows.zip'), (Join-Path $releaseDir 'bb-erp-client-windows.zip'))) {
        if (-not (Test-Path -LiteralPath $required -PathType Leaf)) { throw "Installed tagged release is missing its immutable activation record: $required" }
    }
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $stable = Get-Content -LiteralPath $stablePath -Raw | ConvertFrom-Json
    if ([string]$state.version -ne [string]$InstalledVersion -or [string]$state.tag -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$' -or [string]$state.gitee_commit -notmatch '^[0-9a-fA-F]{40}$') {
        throw "Installed release metadata is incomplete or does not match version $InstalledVersion."
    }
    if ([string]$manifest.version -ne [string]$InstalledVersion -or [string]$stable.version -ne [string]$InstalledVersion) {
        throw "Installed stable manifest does not match version $InstalledVersion."
    }
    $deploymentPath = Join-Path $script:InstallDir 'deployment-state.json'
    $previousState = $null
    if (Test-Path -LiteralPath $deploymentPath -PathType Leaf) {
        try { $previousState = Get-Content -LiteralPath $deploymentPath -Raw | ConvertFrom-Json } catch {
            Write-ReleaseLog "Existing deployment-state.json is invalid and will be repaired: $($_.Exception.Message)" 'WARN'
        }
    }
    if ($previousState -and [string]$previousState.version -eq [string]$state.version -and
        [string]$previousState.tag -eq [string]$state.tag -and [string]$previousState.gitee_commit -eq [string]$state.gitee_commit) {
        return
    }
    $server = Get-FileMetadata (Join-Path $releaseDir 'bb-erp-server-windows.zip')
    $client = Get-FileMetadata (Join-Path $releaseDir 'bb-erp-client-windows.zip')
    if ($server.sha256 -ne [string]$state.artifacts.server.sha256 -or $server.size -ne [int64]$state.artifacts.server.size -or
        $client.sha256 -ne [string]$state.artifacts.client_zip.sha256 -or $client.size -ne [int64]$state.artifacts.client_zip.size -or
        $server.sha256 -ne [string]$manifest.server.sha256 -or $client.sha256 -ne [string]$manifest.client.sha256) {
        throw "Installed release archive hashes do not match the signed release records for $InstalledVersion."
    }
    $previousVersion = [string]$state.previous_successful_version
    if (-not $previousVersion -and $previousState -and [string]$previousState.version -ne [string]$InstalledVersion) {
        $previousVersion = [string]$previousState.version
    }
    Copy-FileAtomically $statePath $deploymentPath
    try {
        Remove-OldReleaseHistory ([string]$InstalledVersion) $previousVersion
        Remove-OldBackupHistory
    } catch {
        Write-ReleaseLog "Activation metadata was repaired, but cleanup failed: $($_.Exception.Message)" 'WARN'
    }
    Write-ReleaseLog "Activation bookkeeping is complete for $($state.tag) at $($state.gitee_commit)."
}

function Invoke-InterruptedUpgradeRecovery {
    $journalPath = Join-Path $script:InstallDir 'updates\pending\server-upgrade-transaction.json'
    if (-not (Test-Path -LiteralPath $journalPath -PathType Leaf)) { return }

    Write-ReleaseLog 'An interrupted server upgrade journal was found; recovery must finish before tag checks.' 'WARN'
    $journal = $null
    foreach ($line in @(Get-Content -LiteralPath $journalPath -ErrorAction Stop)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        try { $journal = $line | ConvertFrom-Json } catch {
            Write-ReleaseLog 'Ignored an incomplete final transaction-journal record left by an interrupted append.' 'WARN'
            break
        }
    }
    if (-not $journal -or -not $journal.updater_path -or -not $journal.updater_sha256) {
        Remove-Item -LiteralPath $journalPath -Force
        Write-ReleaseLog 'Removed an incomplete initial transaction record; the updater never reached the server-stop step.' 'WARN'
        return
    }
    $updaterPath = [IO.Path]::GetFullPath([string]$journal.updater_path)
    $expectedUpdater = [IO.Path]::GetFullPath((Join-Path $script:InstallDir 'updates\recovery\bb-erp-updater.exe'))
    if (-not [string]::Equals($updaterPath, $expectedUpdater, [StringComparison]::OrdinalIgnoreCase) -or -not (Test-Path -LiteralPath $updaterPath -PathType Leaf)) {
        throw "Interrupted upgrade recovery updater is missing or is not the fixed managed executable: $updaterPath"
    }
    $expectedHash = ([string]$journal.updater_sha256).Trim().ToLowerInvariant()
    $actualHash = (Get-FileHash -LiteralPath $updaterPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedHash -notmatch '^[0-9a-f]{64}$' -or $actualHash -ne $expectedHash) {
        throw "Interrupted upgrade recovery updater SHA-256 mismatch: $updaterPath"
    }
    $arguments = @('-recover-interrupted', '-install-dir', $script:InstallDir, '-database-path', $script:DatabasePath)
    if ($WindowsServiceName) { $arguments += @('-service', $WindowsServiceName) }
    Invoke-Checked $updaterPath $arguments (Split-Path -Parent $updaterPath)
    if (Test-Path -LiteralPath $journalPath -PathType Leaf) {
        throw "Interrupted upgrade recovery did not clear its transaction journal: $journalPath"
    }
    Write-ReleaseLog 'Interrupted server upgrade recovery completed.'
}

function Invoke-Publish {
    $mutex = New-Object Threading.Mutex($false, 'Global\BBERPWindowsRelease')
    $mutexAcquired = $false
    try {
        $mutexAcquired = $mutex.WaitOne(0)
    } catch [Threading.AbandonedMutexException] {
        $mutexAcquired = $true
        Write-ReleaseLog 'The previous release process ended unexpectedly; continuing while holding its abandoned global lock.' 'WARN'
    }
    if (-not $mutexAcquired) { throw 'Another BB ERP release is already running.' }
    try {
        Invoke-InterruptedUpgradeRecovery
        Complete-RecordedActivation (Get-InstalledVersion)
        if (-not (Invoke-Doctor)) { throw 'Publish never installs or upgrades system tools. Run Setup explicitly, then retry.' }
        Assert-ServiceReleaseConfiguration
        if (-not (Test-Path $script:RepositoryDir -PathType Container)) { throw "Repository directory does not exist: $script:RepositoryDir" }
        $dirty = @(& git.exe -C $script:RepositoryDir status --porcelain --untracked-files=all)
        if ($LASTEXITCODE -ne 0 -or $dirty.Count -gt 0) { throw "The release checkout must be clean. Found: $($dirty -join '; ')" }
        $remoteUrl = (& git.exe -C $script:RepositoryDir remote get-url $RemoteName).Trim()
        if ($remoteUrl -notmatch '(^|[@/:])gitee\.com[/:]') { throw "Remote $RemoteName is not a Gitee repository: $remoteUrl" }
        Invoke-Checked 'git.exe' @('fetch', $RemoteName, $Branch, '--tags', '--prune') $script:RepositoryDir
        Invoke-Checked 'git.exe' @('switch', $Branch) $script:RepositoryDir
        Invoke-Checked 'git.exe' @('pull', '--ff-only', $RemoteName, $Branch) $script:RepositoryDir
        $head = (& git.exe -C $script:RepositoryDir rev-parse HEAD).Trim()
        $remoteHead = (& git.exe -C $script:RepositoryDir rev-parse "$RemoteName/$Branch").Trim()
        if ($head -ne $remoteHead) { throw "HEAD $head does not match $RemoteName/$Branch $remoteHead" }
        $release = Get-StableHeadTag $RemoteName $head
        if (-not $release) { Write-ReleaseLog 'HEAD has no stable vMAJOR.MINOR.PATCH tag; nothing to publish.'; return }
        $installed = Get-InstalledVersion
        if ($release.Version -eq $installed) { Write-ReleaseLog "Release $($release.Tag) is already installed; nothing to publish."; return }
        if ($release.Version -lt $installed) { Write-ReleaseLog "Release $($release.Tag) is older than installed $installed; nothing to publish."; return }

        New-Item -ItemType Directory -Force -Path (Join-Path $script:InstallDir 'logs') | Out-Null
        $script:LogFile = Join-Path $script:InstallDir ("logs\release-{0}-{1}.log" -f $release.Version, (Get-Date -Format 'yyyyMMdd-HHmmss'))
        $stage = Join-Path (Join-Path $script:InstallDir 'updates\pending') ("{0}-{1}" -f $release.Version, $head.Substring(0, 12))
        if (Test-Path $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
        New-Item -ItemType Directory -Force -Path $stage | Out-Null

        Invoke-Checked 'go.exe' @('mod', 'download') $script:RepositoryDir @{ GOTOOLCHAIN = 'local' }
        Invoke-Checked 'go.exe' @('mod', 'tidy', '-diff') $script:RepositoryDir @{ GOTOOLCHAIN = 'local' }
        Invoke-Checked 'go.exe' @('vet', './...') $script:RepositoryDir @{ GOTOOLCHAIN = 'local' }
        Invoke-Checked 'go.exe' @('test', '-count=1', './...') $script:RepositoryDir @{ GOTOOLCHAIN = 'local' }
        Invoke-Checked 'npm.cmd' @('ci') (Join-Path $script:RepositoryDir 'web')
        Invoke-Checked 'npm.cmd' @('run', 'build') (Join-Path $script:RepositoryDir 'web')
        Invoke-Checked 'npm.cmd' @('ci') (Join-Path $script:RepositoryDir 'client')
        Push-Location (Join-Path $script:RepositoryDir 'client')
        try { $tauriVersion = (& npx.cmd --no-install tauri --version 2>$null) } finally { Pop-Location }
        $tauriVersionPattern = '(^|[^0-9]){0}([^0-9]|$)' -f [regex]::Escape([string]$script:Toolchain.tauri.cli_version)
        if ($tauriVersion -notmatch $tauriVersionPattern) { throw "Locked Tauri CLI $($script:Toolchain.tauri.cli_version) was not restored: $tauriVersion" }
        $cargoManifest = Join-Path $script:RepositoryDir 'client\src-tauri\Cargo.toml'
        Invoke-Checked 'rustup.exe' @('run', $script:Toolchain.rust.toolchain, 'cargo', 'fetch', '--locked', '--manifest-path', $cargoManifest)
        Invoke-Checked 'rustup.exe' @('run', $script:Toolchain.rust.toolchain, 'cargo', 'fmt', '--all', '--manifest-path', $cargoManifest, '--', '--check')
        Invoke-Checked 'rustup.exe' @('run', $script:Toolchain.rust.toolchain, 'cargo', 'check', '--locked', '--manifest-path', $cargoManifest)
        Invoke-Checked 'rustup.exe' @('run', $script:Toolchain.rust.toolchain, 'cargo', 'test', '--locked', '--manifest-path', $cargoManifest)

        $gcc = Join-Path $script:Toolchain.msys2.install_dir 'mingw64\bin\gcc.exe'
        $ldflags = "-s -w -X bb_erp_echo/internal/buildinfo.Version=$($release.Version)"
        Invoke-Checked 'go.exe' @('build', '-trimpath', '-ldflags', $ldflags, '-o', (Join-Path $stage 'bb-erp-server.exe'), './cmd/server') $script:RepositoryDir @{ GOTOOLCHAIN = 'local'; CGO_ENABLED = '1'; CC = $gcc }
        Invoke-Checked 'go.exe' @('build', '-trimpath', '-ldflags', '-s -w', '-o', (Join-Path $stage 'bb-erp-updater.exe'), './cmd/updater') $script:RepositoryDir @{ GOTOOLCHAIN = 'local'; CGO_ENABLED = '0' }

        if (-not $script:SigningPrivateKey) { throw 'Required signing environment variable is missing: TAURI_SIGNING_PRIVATE_KEY' }
        if ($null -eq $script:SigningPrivateKeyPassword) { throw 'Required signing environment variable is missing: TAURI_SIGNING_PRIVATE_KEY_PASSWORD' }
        if (-not $script:UpdaterPublicKey) { throw 'Required signing environment variable is missing: TAURI_UPDATER_PUBLIC_KEY' }
        $normalizeKeyScript = Join-Path $PSScriptRoot 'normalize-tauri-public-key.ps1'
        $normalizedPublicKey = (& $normalizeKeyScript -Value $script:UpdaterPublicKey | Select-Object -Last 1).Trim()
        if (-not $normalizedPublicKey) { throw 'TAURI_UPDATER_PUBLIC_KEY normalization failed.' }
        $script:NormalizedPublicKey = $normalizedPublicKey
        $trustedPublicKeyPath = Join-Path $stage 'trusted-update-public.key'
        [IO.File]::WriteAllText($trustedPublicKeyPath, $normalizedPublicKey, (New-Object Text.UTF8Encoding($false)))
        $tauriConfig = Join-Path $stage 'tauri-release.json'
        Write-JsonUtf8 ([ordered]@{ version = [string]$release.Version; bundle = @{ createUpdaterArtifacts = $false }; plugins = @{ updater = @{ pubkey = $normalizedPublicKey } } }) $tauriConfig 6
        Invoke-Checked 'npm.cmd' @('run', 'desktop:build', '--', '--bundles', 'nsis', '--config', $tauriConfig) (Join-Path $script:RepositoryDir 'client') @{
            BB_ERP_UPDATE_PUBLIC_KEY = $normalizedPublicKey
        }
        $portable = Join-Path $stage 'bb-erp-client-windows-x86_64.exe'
        Copy-Item (Join-Path $script:RepositoryDir 'client\src-tauri\target\release\bb_erp_client.exe') $portable
        $nsisSource = Get-ChildItem (Join-Path $script:RepositoryDir 'client\src-tauri\target\release\bundle\nsis') -Filter '*.exe' -Recurse | Sort-Object Length -Descending | Select-Object -First 1
        if (-not $nsisSource) { throw 'Tauri NSIS installer was not generated.' }
        $nsis = Join-Path $stage 'bb-erp-client-windows-x86_64-setup.exe'
        Copy-Item $nsisSource.FullName $nsis
        $portableSignature = Sign-ReleaseFile $portable
        $nsisSignature = Sign-ReleaseFile $nsis

        $clientZipRoot = Join-Path $stage 'client-recovery'
        New-Item -ItemType Directory -Force -Path (Join-Path $clientZipRoot 'client'), (Join-Path $clientZipRoot 'installer') | Out-Null
        Copy-Item $portable (Join-Path $clientZipRoot 'client\bb_erp_client.exe')
        Copy-Item $nsis (Join-Path $clientZipRoot 'installer\bb-erp-client-setup.exe')
        Write-JsonUtf8 ([ordered]@{ layout_version = 1; version = [string]$release.Version; install_mode = 'portable' }) (Join-Path $clientZipRoot 'client\bb-erp-portable.json')
        $clientZip = Join-Path $stage 'bb-erp-client-windows.zip'
        Compress-Archive -Path (Join-Path $clientZipRoot '*') -DestinationPath $clientZip -Force
        $clientZipSignature = Sign-ReleaseFile $clientZip

        $manifestRelative = 'updates\stable\update-manifest.json'
        $serverZip = New-ServerPackage $stage ([string]$release.Version) $manifestRelative $HttpPort $script:DatabasePath
        $serverSignature = Sign-ReleaseFile $serverZip
        $portableMeta = Get-FileMetadata $portable
        $nsisMeta = Get-FileMetadata $nsis
        $serverMeta = Get-FileMetadata $serverZip
        $clientZipMeta = Get-FileMetadata $clientZip
        $payload = [ordered]@{
            protocol_version = 3; version = [string]$release.Version; target = 'windows-x86_64'; layout_version = 1
            full = [ordered]@{
                nsis = [ordered]@{ kind = 'nsis'; size = $nsisMeta.size; sha256 = $nsisMeta.sha256; signature = $nsisSignature }
                portable = [ordered]@{ kind = 'portable'; size = $portableMeta.size; sha256 = $portableMeta.sha256; signature = $portableSignature }
            }
            deltas = @()
        }
        $payloadPath = Join-Path $stage 'client-update-v3-payload.json'
        Write-JsonUtf8 $payload $payloadPath 8 -Compress
        $payloadSignature = Sign-ReleaseFile $payloadPath
        $script:SigningPrivateKey = $null
        $script:SigningPrivateKeyPassword = $null
        $payloadBase64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($payloadPath))
        $manifest = [ordered]@{
            version = [string]$release.Version; published_at = (Get-Date).ToUniversalTime().ToString('o'); notes = 'BB ERP Windows LAN full release'
            server = [ordered]@{ version = [string]$release.Version; sha256 = $serverMeta.sha256; size = $serverMeta.size; signature = $serverSignature }
            client = [ordered]@{ version = [string]$release.Version; sha256 = $clientZipMeta.sha256; size = $clientZipMeta.size; signature = $clientZipSignature }
            client_update_v3 = [ordered]@{ payload = $payloadBase64; signature = $payloadSignature }
        }
        $candidateManifest = Join-Path $stage 'update-manifest.json'
        Write-JsonUtf8 $manifest $candidateManifest 10

        $rustcActual = (& rustup.exe run $script:Toolchain.rust.toolchain rustc --version | Select-Object -First 1).Trim()
        $state = [ordered]@{
            version = [string]$release.Version; tag = $release.Tag; gitee_commit = $head; published_at = (Get-Date).ToUniversalTime().ToString('o')
            previous_successful_version = if ($installed -gt [Version]'0.0.0') { [string]$installed } else { '' }
            toolchain_lock = $script:Toolchain
            toolchain_actual = [ordered]@{
                go = (Get-CommandVersion 'go.exe' @('version'))
                git = (Get-CommandVersion 'git.exe' @('--version'))
                node = (Get-CommandVersion 'node.exe' @('--version'))
                npm = (Get-CommandVersion 'npm.cmd' @('--version'))
                rustc = $rustcActual
                tauri_cli = ($tauriVersion | Select-Object -First 1).Trim()
                gcc = (Get-CommandVersion $gcc @('--version'))
            }
            artifacts = @{ server = $serverMeta; client_zip = $clientZipMeta; portable = $portableMeta; nsis = $nsisMeta }
        }
        $releaseDir = Join-Path $script:InstallDir ("updates\releases\{0}" -f $release.Version)
        if (Test-Path -LiteralPath $releaseDir -PathType Container) {
            $existingState = Get-Content -LiteralPath (Join-Path $releaseDir 'release-state.json') -Raw | ConvertFrom-Json
            $existingManifest = Get-Content -LiteralPath (Join-Path $releaseDir 'update-manifest.json') -Raw | ConvertFrom-Json
            $existingServer = Get-FileMetadata (Join-Path $releaseDir 'bb-erp-server-windows.zip')
            $existingClient = Get-FileMetadata (Join-Path $releaseDir 'bb-erp-client-windows.zip')
            $matches = [string]$existingState.version -eq [string]$release.Version
            $matches = $matches -and [string]$existingState.tag -eq [string]$release.Tag
            $matches = $matches -and [string]$existingState.gitee_commit -eq $head
            $matches = $matches -and $existingServer.sha256 -eq [string]$existingState.artifacts.server.sha256
            $matches = $matches -and $existingServer.size -eq [int64]$existingState.artifacts.server.size
            $matches = $matches -and $existingClient.sha256 -eq [string]$existingState.artifacts.client_zip.sha256
            $matches = $matches -and $existingClient.size -eq [int64]$existingState.artifacts.client_zip.size
            $matches = $matches -and $existingServer.sha256 -eq [string]$existingManifest.server.sha256
            $matches = $matches -and $existingServer.size -eq [int64]$existingManifest.server.size
            $matches = $matches -and $existingClient.sha256 -eq [string]$existingManifest.client.sha256
            $matches = $matches -and $existingClient.size -eq [int64]$existingManifest.client.size
            if (-not $matches) { throw "Existing immutable release does not match the Gitee tag and artifacts: $releaseDir" }
            $serverZip = Join-Path $releaseDir 'bb-erp-server-windows.zip'
            $clientZip = Join-Path $releaseDir 'bb-erp-client-windows.zip'
            $candidateManifest = Join-Path $releaseDir 'update-manifest.json'
            $serverSignature = [string]$existingManifest.server.signature
            Write-ReleaseLog "Reusing verified release archive after an earlier interrupted activation: $releaseDir"
        } else {
            Add-ContentAddressedArtifact $portable $portableMeta
            Add-ContentAddressedArtifact $nsis $nsisMeta
            $releasePending = Join-Path $stage 'release-archive'
            New-Item -ItemType Directory -Force -Path $releasePending | Out-Null
            Copy-Item $serverZip, $clientZip, $candidateManifest $releasePending
            Write-JsonUtf8 $state (Join-Path $releasePending 'release-state.json') 10
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $releaseDir) | Out-Null
            Move-Item -LiteralPath $releasePending -Destination $releaseDir
        }

        Clear-SigningEnvironment
        $updaterArgs = @('-package', $serverZip, '-package-signature', $serverSignature, '-candidate-manifest', $candidateManifest, '-trusted-public-key', $trustedPublicKeyPath, '-install-dir', $script:InstallDir, '-current-version', ([string]$installed), '-health-base-url', $HealthBaseUrl, '-database-path', $script:DatabasePath)
        if ($WindowsServiceName) { $updaterArgs += @('-service', $WindowsServiceName) }
        Invoke-Checked (Join-Path $stage 'bb-erp-updater.exe') $updaterArgs $stage
        Copy-FileAtomically (Join-Path $releaseDir 'release-state.json') (Join-Path $script:InstallDir 'deployment-state.json')
        try {
            Remove-OldReleaseHistory ([string]$release.Version) ([string]$installed)
            Remove-OldBackupHistory
        } catch {
            Write-ReleaseLog "Release succeeded, but post-transaction cleanup failed: $($_.Exception.Message)" 'WARN'
        }
        if (Test-Path -LiteralPath $stage -PathType Container) { Remove-Item -LiteralPath $stage -Recurse -Force }
        Write-ReleaseLog "Published and activated $($release.Tag) from Gitee commit $head."
    } finally {
        Clear-SigningEnvironment
        if ($mutex) {
            if ($mutexAcquired) { try { $mutex.ReleaseMutex() } catch {} }
            $mutex.Dispose()
        }
    }
}

switch ($Mode) {
    'Doctor' { if (-not (Invoke-Doctor)) { exit 1 } }
    'Setup' { Invoke-Setup }
    'Publish' { Invoke-Publish }
}
