#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Pasang API sebagai Windows Scheduled Task: auto-start saat boot + restart jika crash.

.USAGE
  PowerShell (Run as Administrator):
    cd C:\Users\npspl\PLN\SRS-PLN-NPS_BE
    .\scripts\install-autostart.ps1
#>
$ErrorActionPreference = "Stop"

$TaskName = "PLN-SRS-API"
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$ExePath = Join-Path $ProjectRoot "bin\pln-api.exe"
$RunnerPath = Join-Path $ProjectRoot "scripts\run-server.ps1"

Write-Host "==> Build binary..."
Push-Location $ProjectRoot
& go build -o bin/pln-api.exe ./cmd/server
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "go build gagal" }
Pop-Location

Write-Host "==> Pastikan MySQL service auto-start..."
Get-Service -ErrorAction SilentlyContinue | Where-Object {
    $_.Name -match 'mysql|mariadb' -or $_.DisplayName -match 'mysql|mariadb'
} | ForEach-Object {
    if ($_.StartType -ne 'Automatic') {
        Set-Service -Name $_.Name -StartupType Automatic
        Write-Host "    $($_.DisplayName) -> Automatic"
    }
}

# Pakai powershell wrapper agar log tersimpan; task restart jika proses mati
$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$RunnerPath`"" `
    -WorkingDirectory $ProjectRoot

$trigger = New-ScheduledTaskTrigger -AtStartup
$trigger.Delay = "PT90S"  # tunggu MySQL / Windows selesai boot

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 999 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew

$principal = New-ScheduledTaskPrincipal `
    -UserId "SYSTEM" `
    -LogonType ServiceAccount `
    -RunLevel Highest

$existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "==> Hapus task lama..."
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Principal $principal `
    -Description "PLN SRS Bookify API — auto start at boot, restart on failure" | Out-Null

Write-Host "==> Jalankan task sekarang..."
Start-ScheduledTask -TaskName $TaskName

Start-Sleep -Seconds 3
$info = Get-ScheduledTaskInfo -TaskName $TaskName
Write-Host ""
Write-Host "Berhasil terpasang: $TaskName"
Write-Host "  Status   : $($info.LastTaskResult) (0 = OK)"
Write-Host "  Log file : $ProjectRoot\logs\"
Write-Host "  API LAN  : http://10.7.41.116:8080  (sesuaikan IP di .env BASE_URL)"
Write-Host ""
Write-Host "Perintah berguna:"
Write-Host "  Get-ScheduledTask -TaskName $TaskName | fl"
Write-Host "  Start-ScheduledTask -TaskName $TaskName"
Write-Host "  Stop-ScheduledTask  -TaskName $TaskName"
Write-Host "  .\scripts\uninstall-autostart.ps1"
