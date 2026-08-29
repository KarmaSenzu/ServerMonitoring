# Helper PowerShell untuk operasi compose dari Windows.
# Pakai: .\start.ps1 <command>
# Contoh: .\start.ps1 up

param(
    [Parameter(Position=0)]
    [ValidateSet("env","build","up","down","restart","logs","ps","status","update","help")]
    [string]$Command = "help"
)

$ErrorActionPreference = "Stop"

switch ($Command) {
    "env" {
        if (-not (Test-Path ".env")) {
            Copy-Item ".env.docker.example" ".env"
            Write-Host "✓ .env created. Edit it sebelum lanjut." -ForegroundColor Green
        } else {
            Write-Host ".env sudah ada." -ForegroundColor Yellow
        }
    }
    "build"   { docker compose build --pull }
    "up"      { docker compose up -d; docker compose ps }
    "down"    { docker compose down }
    "restart" { docker compose restart }
    "logs"    { docker compose logs -f --tail=100 }
    "ps"      { docker compose ps }
    "status"  {
        Write-Host "=== Stack status ===" -ForegroundColor Cyan
        docker compose ps
        Write-Host "`n=== Tunnel logs (last 20) ===" -ForegroundColor Cyan
        docker compose logs --tail=20 cloudflared
    }
    "update"  {
        docker compose pull
        docker compose build --pull
        docker compose up -d
    }
    default {
        Write-Host "Available commands:" -ForegroundColor Cyan
        Write-Host "  env      - Buat .env dari template"
        Write-Host "  build    - Build semua image"
        Write-Host "  up       - Start stack"
        Write-Host "  down     - Stop stack"
        Write-Host "  restart  - Restart semua service"
        Write-Host "  logs     - Tail log"
        Write-Host "  ps       - Status container"
        Write-Host "  status   - Status + tunnel logs"
        Write-Host "  update   - Pull + rebuild + restart"
    }
}
