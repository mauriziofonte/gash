# Gash 3.4 — Windows fixture capture (read-only)
#
# Run this in a NORMAL Windows PowerShell window (not WSL), then run it once
# more in an ELEVATED window ("Run as administrator"). It only reads: it runs
# the same Get-* queries that `gash win` sends over WSL interop and saves
# their JSON so the Gash test suite can be pinned to your real machine. It
# changes nothing.
#
#   powershell -ExecutionPolicy Bypass -File capture.ps1
#
# It writes gash-win-capture-<normal|elevated>.json next to this script.
# Send that file back (both runs if you can). Nothing secret is collected:
# no file contents, no credentials, no registry hives — only metadata.

$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$idn = [Security.Principal.WindowsIdentity]::GetCurrent()
$elevated = @($idn.Groups | ForEach-Object { $_.Value }) -contains 'S-1-16-12288'
$suffix = if ($elevated) { 'elevated' } else { 'normal' }
$outPath = Join-Path $PSScriptRoot "gash-win-capture-$suffix.json"

$capture = [ordered]@{
  _meta = [ordered]@{
    captured_at = (Get-Date).ToString('o')
    elevated    = $elevated
    ps_version  = $PSVersionTable.PSVersion.ToString()
    ui_culture  = (Get-UICulture).Name   # so we know if output is localized
    culture     = (Get-Culture).Name
  }
}

function Grab([string]$name, [scriptblock]$block) {
  try {
    $capture[$name] = & $block
  } catch {
    $capture[$name] = [ordered]@{ _error = $_.Exception.Message }
  }
}

Grab 'info' {
  $os = Get-CimInstance Win32_OperatingSystem
  $cs = Get-CimInstance Win32_ComputerSystem
  [ordered]@{ caption=$os.Caption; version=$os.Version; build=$os.BuildNumber;
    ubr=(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -EA SilentlyContinue).UBR;
    hostname=$cs.Name; domain=$cs.Domain; manufacturer=$cs.Manufacturer; model=$cs.Model;
    user=$idn.Name; elevated=$elevated; boot=$os.LastBootUpTime.ToString('o'); arch=$env:PROCESSOR_ARCHITECTURE }
}

Grab 'events' {
  @{ items = @(Get-WinEvent -FilterHashtable @{ LogName='System'; Level=2 } -MaxEvents 10 -EA SilentlyContinue |
    Select-Object @{n='time';e={$_.TimeCreated.ToString('o')}}, @{n='level';e={$_.LevelDisplayName}},
      @{n='id';e={$_.Id}}, @{n='provider';e={$_.ProviderName}}, @{n='message';e={($_.Message -split '\r?\n')[0]}}) }
}

Grab 'services' {
  @{ items = @(Get-CimInstance Win32_Service | Select-Object -First 10 Name, DisplayName, State, StartMode,
    @{n='pid';e={$_.ProcessId}}, @{n='account';e={$_.StartName}}) }
}

Grab 'procs' {
  @{ items = @(Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 10 |
    Select-Object @{n='pid';e={$_.Id}}, @{n='name';e={$_.ProcessName}}, @{n='ws';e={$_.WorkingSet64}}, @{n='cpu';e={[math]::Round($_.CPU,1)}}) }
}

Grab 'ports' {
  @{ items = @(Get-NetTCPConnection -EA SilentlyContinue | Select-Object -First 15 @{n='local';e={$_.LocalAddress+':'+$_.LocalPort}},
    @{n='remote';e={$_.RemoteAddress+':'+$_.RemotePort}}, @{n='state';e={$_.State.ToString()}},
    @{n='pid';e={$_.OwningProcess}}, @{n='process';e={(Get-Process -Id $_.OwningProcess -EA SilentlyContinue).ProcessName}}) }
}

Grab 'net' {
  $ad = @(Get-NetIPConfiguration -EA SilentlyContinue | Select-Object @{n='alias';e={$_.InterfaceAlias}},
    @{n='status';e={$_.NetAdapter.Status}}, @{n='ipv4';e={$_.IPv4Address.IPAddress}},
    @{n='gateway';e={$_.IPv4DefaultGateway.NextHop}}, @{n='dns';e={@($_.DNSServer.ServerAddresses)}})
  $proxy = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -EA SilentlyContinue
  [ordered]@{ adapters=$ad; proxy=[ordered]@{ enabled=[bool]$proxy.ProxyEnable; server=$proxy.ProxyServer; bypass=$proxy.ProxyOverride } }
}

Grab 'updates' {
  $hf = @(Get-HotFix -EA SilentlyContinue | Sort-Object InstalledOn -Descending | Select-Object -First 10 |
    Select-Object @{n='id';e={$_.HotFixID}}, @{n='kind';e={$_.Description}}, @{n='installed';e={if($_.InstalledOn){$_.InstalledOn.ToString('yyyy-MM-dd')}else{''}}})
  [ordered]@{ hotfixes=$hf;
    reboot_pending=((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or
      (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) }
}

Grab 'defender' {
  $m = Get-MpComputerStatus -EA SilentlyContinue
  if ($null -eq $m) { [ordered]@{ available=$false } }
  else { [ordered]@{ available=$true; antivirus_enabled=$m.AntivirusEnabled; realtime=$m.RealTimeProtectionEnabled;
    tamper_protection=$m.IsTamperProtected; engine=$m.AMEngineVersion; signature_version=$m.AntivirusSignatureVersion;
    signature_age_days=$m.AntivirusSignatureAge } }
}

Grab 'apps' {
  $keys='HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
  @{ items = @(Get-ItemProperty $keys -EA SilentlyContinue | Where-Object { $_.DisplayName } | Select-Object -First 15 |
    Select-Object @{n='name';e={$_.DisplayName}}, @{n='version';e={$_.DisplayVersion}}, @{n='publisher';e={$_.Publisher}}) }
}

Grab 'disk' {
  @{ items = @(Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' |
    Select-Object @{n='drive';e={$_.DeviceID}}, @{n='label';e={$_.VolumeName}}, @{n='size';e={$_.Size}},
      @{n='free';e={$_.FreeSpace}}, @{n='fs';e={$_.FileSystem}}) }
}

Grab 'file_notepad' {
  $p = Join-Path $env:WINDIR 'System32\notepad.exe'
  $it = Get-Item -LiteralPath $p
  $acl = Get-Acl -LiteralPath $p -EA SilentlyContinue
  $sig = Get-AuthenticodeSignature -LiteralPath $p -EA SilentlyContinue
  [ordered]@{ path=$it.FullName; length=$it.Length; attributes=$it.Attributes.ToString();
    created=$it.CreationTime.ToString('o'); modified=$it.LastWriteTime.ToString('o'); owner=$acl.Owner;
    access=@($acl.Access | Select-Object @{n='identity';e={$_.IdentityReference.Value}},
      @{n='rights';e={$_.FileSystemRights.ToString()}}, @{n='type';e={$_.AccessControlType.ToString()}});
    signature=[ordered]@{ status=$sig.Status.ToString(); signer=$sig.SignerCertificate.Subject }; version=$it.VersionInfo.FileVersion }
}

# Native tools whose text output Gash parses: capture it verbatim.
Grab 'reg_currentversion_text' {
  (reg query "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion" 2>&1 | Out-String)
}
Grab 'dsregcmd_status_text' {
  (dsregcmd /status 2>&1 | Out-String)
}
Grab 'hosts_text' {
  Get-Content -Raw (Join-Path $env:WINDIR 'System32\drivers\etc\hosts') -EA SilentlyContinue
}

$capture | ConvertTo-Json -Depth 8 | Set-Content -Encoding UTF8 -Path $outPath
Write-Host "Saved $outPath"
Write-Host "Send this file back to Gash. It holds metadata only; review it first if you like."
