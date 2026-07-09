$TaskName = "PLN-SRS-API"
$task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if (-not $task) {
    Write-Host "Task '$TaskName' belum terpasang. Jalankan install-autostart.ps1 sebagai Administrator."
    exit 1
}

$info = Get-ScheduledTaskInfo -TaskName $TaskName
Write-Host "Task     : $TaskName"
Write-Host "State    : $($task.State)"
Write-Host "Last run : $($info.LastRunTime)"
Write-Host "Result   : $($info.LastTaskResult)"
Write-Host "Next run : $($info.NextRunTime)"

try {
    $health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 3
    Write-Host "API      : OK — $($health.baseUrl)"
} catch {
    Write-Host "API      : tidak merespons di :8080 — $_"
}

$proc = Get-Process -Name "pln-api" -ErrorAction SilentlyContinue
if ($proc) { Write-Host "Process  : pln-api.exe (PID $($proc.Id))" }
