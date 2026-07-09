# Wrapper: jalankan API dari root project (load .env, log ke file)
$ErrorActionPreference = "Stop"
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $ProjectRoot

$exe = Join-Path $ProjectRoot "bin\pln-api.exe"
if (-not (Test-Path $exe)) {
    Write-Error "Binary tidak ada: $exe — jalankan scripts\install-autostart.ps1 atau: go build -o bin/pln-api.exe ./cmd/server"
}

$logDir = Join-Path $ProjectRoot "logs"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$logFile = Join-Path $logDir ("api-{0:yyyy-MM-dd}.log" -f (Get-Date))

Add-Content -Path $logFile -Value ("[{0:u}] Starting PLN API..." -f (Get-Date))
& $exe 2>&1 | ForEach-Object {
    $line = $_.ToString()
    Add-Content -Path $logFile -Value ("[{0:u}] {1}" -f (Get-Date), $line)
    Write-Output $line
}
$code = $LASTEXITCODE
Add-Content -Path $logFile -Value ("[{0:u}] Exited with code {1}" -f (Get-Date), $code)
exit $code
