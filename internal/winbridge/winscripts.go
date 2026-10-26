// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package winbridge

// The read-only PowerShell scripts behind `gash win`. Each reads its
// arguments from $a (the JSON object Gash puts on stdin) and assigns the
// value to serialize to $result. They use only Get-*, queries and
// transforms; TestScriptsReadOnly proves it. Property names are forced
// (Select-Object) so a localized Windows cannot change the JSON keys, and
// @(...) forces a list even for a single row.
//
// These scripts are Gash's best reading of the Windows surface; a capture
// run on a real Windows 11 (docs/windows/capture.ps1) records the JSON they
// produce, which the golden tests then pin.

// ScriptInfo: the Windows build, edition, host, current user and whether
// the shell's token is elevated (High Mandatory Level, S-1-16-12288).
var ScriptInfo = register(Script{Name: "info", Depth: 3, Body: `
$os=Get-CimInstance Win32_OperatingSystem;
$cs=Get-CimInstance Win32_ComputerSystem;
$idn=[Security.Principal.WindowsIdentity]::GetCurrent();
$sids=@($idn.Groups | ForEach-Object { $_.Value });
$result=[ordered]@{
  caption=$os.Caption; version=$os.Version; build=$os.BuildNumber; ubr=(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -EA SilentlyContinue).UBR;
  hostname=$cs.Name; domain=$cs.Domain; manufacturer=$cs.Manufacturer; model=$cs.Model;
  user=$idn.Name; elevated=($sids -contains 'S-1-16-12288');
  boot=$os.LastBootUpTime.ToString('o'); arch=$env:PROCESSOR_ARCHITECTURE
}`})

// ScriptEvents: the Windows event log. Args: log, level (numeric 1..5, 0 =
// any), since (ISO time, optional), max.
var ScriptEvents = register(Script{Name: "events", Depth: 4, Body: `
$f=@{ LogName=$a.log };
if([int]$a.level -gt 0){ $f.Level=[int]$a.level };
if($a.since){ $f.StartTime=[datetime]::Parse($a.since) };
$max=[int]$a.max; if($max -le 0){ $max=50 };
$result=[ordered]@{items=@(Get-WinEvent -FilterHashtable $f -MaxEvents $max -EA SilentlyContinue |
  Select-Object @{n='time';e={$_.TimeCreated.ToString('o')}}, @{n='level';e={$_.LevelDisplayName}},
    @{n='id';e={$_.Id}}, @{n='provider';e={$_.ProviderName}}, @{n='message';e={($_.Message -split '\r?\n')[0]}})}`})

// ScriptServices: Windows services, optionally filtered by a name
// substring (case-insensitive).
var ScriptServices = register(Script{Name: "services", Depth: 3, Body: `
$q=Get-CimInstance Win32_Service;
if($a.filter){ $q=$q | Where-Object { $_.Name -like ('*'+$a.filter+'*') -or $_.DisplayName -like ('*'+$a.filter+'*') } };
$result=[ordered]@{items=@($q | Select-Object Name, DisplayName, State, StartMode,
  @{n='pid';e={$_.ProcessId}}, @{n='account';e={$_.StartName}} | Sort-Object Name)}`})

// ScriptProcs: top processes by working set, optionally filtered by name.
var ScriptProcs = register(Script{Name: "procs", Depth: 3, Body: `
$q=Get-Process;
if($a.filter){ $q=$q | Where-Object { $_.ProcessName -like ('*'+$a.filter+'*') } };
$max=[int]$a.max; if($max -le 0){ $max=40 };
$result=[ordered]@{items=@($q | Sort-Object WorkingSet64 -Descending | Select-Object -First $max |
  Select-Object @{n='pid';e={$_.Id}}, @{n='name';e={$_.ProcessName}},
    @{n='ws';e={$_.WorkingSet64}}, @{n='cpu';e={[math]::Round($_.CPU,1)}})}`})

// ScriptPorts: listening and established TCP connections with the owning
// process. Args: port (optional, 0 = any).
var ScriptPorts = register(Script{Name: "ports", Depth: 3, Body: `
$c=Get-NetTCPConnection -EA SilentlyContinue;
if([int]$a.port -gt 0){ $c=$c | Where-Object { $_.LocalPort -eq [int]$a.port } };
$result=[ordered]@{items=@($c | Select-Object @{n='local';e={$_.LocalAddress+':'+$_.LocalPort}},
  @{n='remote';e={$_.RemoteAddress+':'+$_.RemotePort}}, @{n='state';e={$_.State.ToString()}},
  @{n='pid';e={$_.OwningProcess}}, @{n='process';e={(Get-Process -Id $_.OwningProcess -EA SilentlyContinue).ProcessName}} |
  Sort-Object local)}`})

// ScriptNet: network adapters, DNS servers and the configured HTTP proxy.
var ScriptNet = register(Script{Name: "net", Depth: 4, Body: `
$ad=@(Get-NetIPConfiguration -EA SilentlyContinue | Select-Object @{n='alias';e={$_.InterfaceAlias}},
  @{n='status';e={$_.NetAdapter.Status}}, @{n='ipv4';e={$_.IPv4Address.IPAddress}},
  @{n='gateway';e={$_.IPv4DefaultGateway.NextHop}}, @{n='dns';e={@($_.DNSServer.ServerAddresses)}});
$proxy=Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -EA SilentlyContinue;
$result=[ordered]@{ adapters=$ad;
  proxy=[ordered]@{ enabled=[bool]$proxy.ProxyEnable; server=$proxy.ProxyServer; bypass=$proxy.ProxyOverride } }`})

// ScriptUpdates: installed hotfixes (newest first) and whether a reboot is
// pending (the component-based servicing and Windows Update flags).
var ScriptUpdates = register(Script{Name: "updates", Depth: 3, Body: `
$hf=@(Get-HotFix -EA SilentlyContinue | Sort-Object InstalledOn -Descending | Select-Object -First 20 |
  Select-Object @{n='id';e={$_.HotFixID}}, @{n='kind';e={$_.Description}}, @{n='installed';e={if($_.InstalledOn){$_.InstalledOn.ToString('yyyy-MM-dd')}else{''}}});
$cbs=Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending';
$wu=Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired';
$result=[ordered]@{ hotfixes=$hf; reboot_pending=($cbs -or $wu) }`})

// ScriptDefender: Microsoft Defender status (engine, signatures, real-time).
var ScriptDefender = register(Script{Name: "defender", Depth: 3, Body: `
$m=Get-MpComputerStatus -EA SilentlyContinue;
if($null -eq $m){ $result=[ordered]@{ available=$false } }
else { $result=[ordered]@{ available=$true; antivirus_enabled=$m.AntivirusEnabled;
  realtime=$m.RealTimeProtectionEnabled; tamper_protection=$m.IsTamperProtected;
  engine=$m.AMEngineVersion; signature_version=$m.AntivirusSignatureVersion;
  signature_age_days=$m.AntivirusSignatureAge } }`})

// ScriptApps: installed programs from the Uninstall registry keys (both
// 64- and 32-bit), optionally filtered by a display-name substring.
var ScriptApps = register(Script{Name: "apps", Depth: 3, Body: `
$keys='HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
  'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
  'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*';
$q=Get-ItemProperty $keys -EA SilentlyContinue | Where-Object { $_.DisplayName };
if($a.filter){ $q=$q | Where-Object { $_.DisplayName -like ('*'+$a.filter+'*') } };
$result=[ordered]@{items=@($q | Select-Object @{n='name';e={$_.DisplayName}}, @{n='version';e={$_.DisplayVersion}},
  @{n='publisher';e={$_.Publisher}} | Sort-Object name -Unique)}`})

// ScriptDisk: fixed volumes with size and free space.
var ScriptDisk = register(Script{Name: "disk", Depth: 3, Body: `
$result=[ordered]@{items=@(Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' |
  Select-Object @{n='drive';e={$_.DeviceID}}, @{n='label';e={$_.VolumeName}},
    @{n='size';e={$_.Size}}, @{n='free';e={$_.FreeSpace}}, @{n='fs';e={$_.FileSystem}} | Sort-Object drive)}`})

// ScriptFile: a file's metadata, owner, ACL summary and Authenticode
// signature. Args: path (a Windows path).
var ScriptFile = register(Script{Name: "file", Depth: 4, Body: `
$p=$a.path;
$it=Get-Item -LiteralPath $p -EA Stop;
$acl=Get-Acl -LiteralPath $p -EA SilentlyContinue;
$sig=Get-AuthenticodeSignature -LiteralPath $p -EA SilentlyContinue;
$result=[ordered]@{ path=$it.FullName; length=$it.Length; attributes=$it.Attributes.ToString();
  created=$it.CreationTime.ToString('o'); modified=$it.LastWriteTime.ToString('o');
  owner=$acl.Owner; access=@($acl.Access | Select-Object @{n='identity';e={$_.IdentityReference.Value}},
    @{n='rights';e={$_.FileSystemRights.ToString()}}, @{n='type';e={$_.AccessControlType.ToString()}});
  signature=[ordered]@{ status=$sig.Status.ToString(); signer=$sig.SignerCertificate.Subject };
  version=$it.VersionInfo.FileVersion }`})

// ScriptPolicy: the identity/management posture — Entra/Intune join from
// dsregcmd is native; here the applied policy registry roots and their
// value counts, so a human sees where policy is set.
var ScriptPolicy = register(Script{Name: "policy", Depth: 4, Body: `
$roots='HKLM:\SOFTWARE\Policies','HKCU:\SOFTWARE\Policies',
  'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies','HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WSL';
$out=@();
foreach($r in $roots){
  if(Test-Path $r){
    foreach($k in Get-ChildItem $r -Recurse -EA SilentlyContinue){
      $vals=@($k.Property); if($vals.Count -gt 0){ $out+=[ordered]@{ key=$k.Name; values=$vals.Count } }
    }
  }
}
$result=[ordered]@{items=@($out)}`})
