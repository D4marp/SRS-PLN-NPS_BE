#Requires -RunAsAdministrator
$TaskName = "PLN-SRS-API"
$task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if (-not $task) {
    Write-Host "Task '$TaskName' tidak ditemukan."
    exit 0
}
Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
Write-Host "Task '$TaskName' dihapus."

# Hentikan proses API yang masih jalan
Get-Process -Name "pln-api" -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host "Selesai."
