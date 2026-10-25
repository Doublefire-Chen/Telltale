# Telltale Agent installer for Windows (Linux and macOS: install.sh). Run it in
# PowerShell as Administrator:
#
#   install.ps1 [install] -DashboardUrl URL (-Binary PATH | -Version vX.Y.Z [-Sha256 HASH,...]) [-Interval 2s] [-TokenFile FILE]
#   install.ps1 [install] (-Binary PATH | -Version vX.Y.Z [-Sha256 HASH,...])   # upgrade: keeps the existing config
#   install.ps1 uninstall
#
# or straight from a release tag, as the Dashboard shows it:
#
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/Doublefire-Chen/Telltale/vX.Y.Z/deploy/install.ps1))) -DashboardUrl URL -Version vX.Y.Z -Sha256 ...
#
# On a first install it asks for the Host token (or reads it from -TokenFile,
# "-" for stdin), so the token never appears in the process list or your
# history. -Version checks the download against -Sha256 (HASH, or
# os-arch=HASH once per platform; the Dashboard's command does this), or
# without it against the release's SHA256SUMS after checking that file's
# cosign signature, which needs cosign.
#
# Read it before you run it. It touches only these paths, and uninstall removes
# all of them:
#
#   C:\Program Files\Telltale\telltale-agent.exe    the Agent
#   C:\ProgramData\Telltale\agent.toml              its config (readable only by the Agent, SYSTEM and Administrators)
#   the telltale-agent service and its event log source, which the Agent sets
#   up itself (`telltale-agent service install`); it runs as NT SERVICE\telltale-agent
#
# It never touches the registry beyond that, scheduled tasks, the firewall,
# DNS or Defender (ADR 0001).
param(
	[ValidateSet('install', 'uninstall')]
	[string]$Action = 'install',
	[string]$DashboardUrl,
	[string]$Binary,
	[string]$Version,
	[string[]]$Sha256 = @(),
	[string]$Interval,
	[string]$TokenFile
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is many times slower with its progress bar

$Repo = 'Doublefire-Chen/Telltale'
$ServiceName = 'telltale-agent'
$BinDir = Join-Path $env:ProgramFiles 'Telltale'
$Bin = Join-Path $BinDir 'telltale-agent.exe'
$ConfigDir = Join-Path $env:ProgramData 'Telltale'
$Config = Join-Path $ConfigDir 'agent.toml'

# Errors are thrown, not exited on: run from a scriptblock, exit would close
# the PowerShell window.
function Die([string]$Message) { throw "telltale install: $Message" }

function Assert-Administrator {
	$me = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
	if (-not $me.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
		Die 'run this in PowerShell as Administrator'
	}
}

function Get-Asset {
	$arch = $env:PROCESSOR_ARCHITEW6432 # set when 32-bit PowerShell runs on 64-bit Windows
	if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
	switch ($arch) {
		'AMD64' { return 'telltale-agent-windows-amd64.exe' }
		'ARM64' { return 'telltale-agent-windows-arm64.exe' }
		default { Die "unsupported architecture $arch; use -Binary" }
	}
}

# Get-Sha256 hashes with .NET rather than Get-FileHash, which Windows
# PowerShell cannot load when started from PowerShell 7 (its module path is
# inherited).
function Get-Sha256([string]$Path) {
	$sha = [Security.Cryptography.SHA256]::Create()
	$file = [IO.File]::OpenRead($Path)
	try {
		return (-join ($sha.ComputeHash($file) | ForEach-Object { $_.ToString('x2') }))
	} finally {
		$file.Dispose()
		$sha.Dispose()
	}
}

# Select-Sha256 returns the hash for Asset among the -Sha256 values: the one
# given as os-arch=HASH for its platform, or a bare HASH.
function Select-Sha256([string]$Asset) {
	foreach ($h in $Sha256) {
		if ($h -notlike '*=*') { return $h }
		$platform, $hash = $h -split '=', 2
		if ("telltale-agent-$platform.exe" -eq $Asset) { return $hash }
	}
	return ''
}

# Get-Release downloads Version's Agent to Dest and checks it: against its
# -Sha256 if given, otherwise against the release's SHA256SUMS after checking
# that file's cosign signature, which proves this repository's release
# workflow made it at that tag.
function Get-Release([string]$Dest) {
	$asset = Get-Asset
	$base = "https://github.com/$Repo/releases/download/$Version"
	$tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
	New-Item -ItemType Directory -Path $tmp | Out-Null
	try {
		Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
		$want = Select-Sha256 $asset
		if (-not $want) {
			Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS')
			Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS.sigstore.json" -OutFile (Join-Path $tmp 'SHA256SUMS.sigstore.json')
			& cosign verify-blob (Join-Path $tmp 'SHA256SUMS') --bundle (Join-Path $tmp 'SHA256SUMS.sigstore.json') `
				--certificate-oidc-issuer https://token.actions.githubusercontent.com `
				--certificate-identity "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$Version" 2>&1 | Out-Null
			if ($LASTEXITCODE -ne 0) { Die "SHA256SUMS of $Version does not carry a valid signature from the $Repo release workflow" }
			Write-Host "checked the signature of ${Version}'s SHA256SUMS"
			$lines = @(Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { ($_ -split '\s+', 2)[1] -ceq $asset })
			if ($lines.Count -ne 1) { Die "SHA256SUMS has no single checksum for $asset" }
			$want = ($lines[0] -split '\s+', 2)[0]
		}
		if ((Get-Sha256 (Join-Path $tmp $asset)) -ne $want) { Die "checksum mismatch for $asset" }
		Copy-Item -LiteralPath (Join-Path $tmp $asset) -Destination $Dest -Force
	} finally {
		Remove-Item -LiteralPath $tmp -Recurse -Force
	}
}

# Read-Token returns the Host token from TokenFile ("-" for stdin), or asks for
# it without echoing it.
function Read-Token {
	if ($TokenFile -eq '-') { return ([Console]::In.ReadLine()).Trim() }
	if ($TokenFile) { return (Get-Content -LiteralPath $TokenFile -TotalCount 1).Trim() }
	$secure = Read-Host -AsSecureString 'Host token (input hidden)'
	$ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
	try { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr) }
}

function Assert-Arguments {
	if (-not $Binary -and -not $Version) { Die 'give -Binary PATH or -Version vX.Y.Z' }
	if ($Version -and $Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$') { Die '-Version must look like v1.2.3' }
	foreach ($h in $Sha256) {
		if ($h -cnotmatch '^([a-z]+-[a-z0-9]+=)?[0-9a-f]{64}$') { Die '-Sha256 must be 64 lowercase hex digits, or os-arch= and those' }
	}
	if ($Interval) {
		if ($Interval -cnotmatch '^([0-9]+(\.[0-9]+)?)(ms|s|m|h)$') { Die '-Interval must be a duration such as 2s' }
		# The Agent refuses to start below one second.
		$ms = [double]$Matches[1] * @{ ms = 1; s = 1000; m = 60000; h = 3600000 }[$Matches[3]]
		if ($ms -lt 1000) { Die '-Interval must be at least 1s' }
	}
	if ($DashboardUrl) {
		# Quotes, backslashes and whitespace would break out of the TOML string.
		if ($DashboardUrl -cnotmatch '^https?://[^\s"\\@/?#]+(/[^\s"\\?#]*)?$') {
			Die '-DashboardUrl must be a plain http(s) URL such as https://status.example.com'
		}
		# As the Agent does: plain http only to this machine, since the token
		# would otherwise cross the network in the clear.
		if ($DashboardUrl -cnotmatch '^(https://.*|http://(localhost|127\.[0-9]+\.[0-9]+\.[0-9]+|\[::1\])(:[0-9]+)?(/.*)?)$') {
			Die '-DashboardUrl must use https (plain http is allowed only for a loopback address)'
		}
	} elseif ($TokenFile -or $Interval) {
		Die 'a token or interval needs -DashboardUrl too'
	}
	if (-not (Test-Path -LiteralPath $Config) -and -not $DashboardUrl) { Die 'first install needs -DashboardUrl' }
	if (-not $Binary) {
		if ($Sha256.Count -gt 0) {
			if (-not (Select-Sha256 (Get-Asset))) { Die "no -Sha256 given for $(Get-Asset)" }
		} elseif (-not (Get-Command cosign -ErrorAction SilentlyContinue)) {
			Die "cosign is needed to check who published $Version (https://docs.sigstore.dev/cosign/system_config/installation/), or pass -Sha256 with the checksum of $(Get-Asset) from a SHA256SUMS you verified with cosign elsewhere"
		}
	}
}

# Remove-AgentService stops and removes an installed service, keeping its config,
# so its exe can be replaced.
function Remove-AgentService {
	if (-not (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue)) { return }
	if (Test-Path -LiteralPath $Bin) {
		& $Bin -config $Config service uninstall
		if ($LASTEXITCODE -ne 0) { Die "telltale-agent service uninstall failed" }
	} else {
		& sc.exe stop $ServiceName | Out-Null
		& sc.exe delete $ServiceName | Out-Null
	}
}

function Install-Agent {
	Assert-Arguments
	if ($DashboardUrl) {
		$token = Read-Token
		if ($token -cnotmatch '^tt_[A-Za-z0-9_-]+$') { Die 'the Host token must be the one the Dashboard showed (it starts with "tt_")' }
	}

	$tmpBin = [IO.Path]::GetTempFileName()
	try {
		if ($Binary) { Copy-Item -LiteralPath $Binary -Destination $tmpBin -Force } else { Get-Release $tmpBin }
		Remove-AgentService
		New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
		Copy-Item -LiteralPath $tmpBin -Destination $Bin -Force
	} finally {
		Remove-Item -LiteralPath $tmpBin -Force -ErrorAction SilentlyContinue
	}
	$from = if ($Version) { $Version } else { "from $Binary" }
	Write-Host "installed $Bin ($from, sha256 $(Get-Sha256 $Bin))"

	if ($DashboardUrl) {
		New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
		# Only SYSTEM and Administrators until the Agent adds its own account,
		# so the token is never readable by other users, even briefly.
		& icacls.exe $ConfigDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
		if ($LASTEXITCODE -ne 0) { Die "could not restrict $ConfigDir" }
		$text = "dashboard_url = `"$DashboardUrl`"`ntoken = `"$token`"`n"
		if ($Interval) { $text += "interval = `"$Interval`"`n" }
		[IO.File]::WriteAllText($Config, $text) # UTF-8 without a byte order mark
		Write-Host "wrote $Config"
	} else {
		Write-Host "kept existing $Config"
	}
	# The Agent registers its service, account and event log source and locks
	# down the config itself.
	& $Bin -config $Config service install
	if ($LASTEXITCODE -ne 0) { Die 'telltale-agent service install failed' }
}

function Uninstall-Agent {
	if (Test-Path -LiteralPath $Bin) {
		& $Bin -config $Config service uninstall --purge
		if ($LASTEXITCODE -ne 0) { Die 'telltale-agent service uninstall failed' }
	} else {
		Remove-AgentService
	}
	foreach ($path in $Bin, $Config) {
		if (Test-Path -LiteralPath $path) {
			Remove-Item -LiteralPath $path -Force
			Write-Host "removed $path"
		}
	}
	foreach ($dir in $BinDir, $ConfigDir) {
		if ((Test-Path -LiteralPath $dir) -and -not (Get-ChildItem -LiteralPath $dir -Force)) {
			Remove-Item -LiteralPath $dir -Force
			Write-Host "removed $dir"
		}
	}
}

# PowerShell 7 runs on Linux and macOS too, where install.sh is the installer.
if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
	Die 'this installer is for Windows; on Linux and macOS copy the install.sh command from the Dashboard'
}
Assert-Administrator
if ($Action -eq 'install') { Install-Agent } else { Uninstall-Agent }
