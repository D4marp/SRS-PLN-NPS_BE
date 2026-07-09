# Jalankan installer dengan hak Administrator (minta UAC sekali)
$install = Join-Path $PSScriptRoot "install-autostart.ps1"
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Meminta izin Administrator (UAC)..."
    Start-Process powershell.exe -Verb RunAs -ArgumentList @(
        "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$install`""
    ) -Wait
    exit $LASTEXITCODE
}
& $install
