param(
  [switch]$Fresh
)

$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) {
  throw "This deployment requires PowerShell 7. Run: pwsh -File $PSCommandPath"
}

$tools = $PSScriptRoot
$compose = [IO.Path]::GetFullPath((Join-Path $tools '..\deploy\compose.local.yml'))

function Invoke-Native {
  param([Parameter(Mandatory)][scriptblock]$Command, [string]$FailureMessage)
  $previous = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & $Command } finally { $ErrorActionPreference = $previous }
  if ($LASTEXITCODE -ne 0 -and $FailureMessage) { throw $FailureMessage }
}

function Ensure-LocalNetwork {
  $network = Invoke-Native { docker network ls --filter name='^liveshop-local$' --format '{{.Name}}' }
  if ($network -ne 'liveshop-local') {
    Invoke-Native { docker network create liveshop-local | Out-Null } 'Failed to create the shared Docker network liveshop-local.'
  }
}

function Wait-Ready([string]$Url, [int]$TimeoutMinutes = 5) {
  $deadline = [DateTime]::UtcNow.AddMinutes($TimeoutMinutes)
  while ([DateTime]::UtcNow -lt $deadline) {
    try {
      $response = Invoke-WebRequest -Uri $Url -TimeoutSec 3 -UseBasicParsing -SkipHttpErrorCheck
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) { return }
    } catch {}
    Start-Sleep -Milliseconds 500
  }
  throw "Timed out waiting for ready service $Url"
}

Ensure-LocalNetwork
if ($Fresh) {
  Invoke-Native { docker compose -f $compose down -v --remove-orphans } 'Failed to reset the local Registry stack.'
}

Invoke-Native { docker compose -f $compose up --build --no-deps grpc-certs }
if ($LASTEXITCODE -ne 0) { throw 'Local gRPC certificate bootstrap failed.' }
$certState = Invoke-Native { docker compose -f $compose ps --all --format '{{.Service}}|{{.State}}|{{.ExitCode}}' grpc-certs }
if (@($certState).Count -ne 1 -or "$certState" -ne 'grpc-certs|exited|0') {
  Invoke-Native { docker compose -f $compose run --rm --no-deps --entrypoint /app/grpccerts grpc-certs -out /certs -owner 65532 -force } 'Failed to rotate local gRPC certificates for Registry.'
  Invoke-Native { docker compose -f $compose up --no-deps grpc-certs } 'Failed to record the rotated gRPC certificate job.'
  $certState = Invoke-Native { docker compose -f $compose ps --all --format '{{.Service}}|{{.State}}|{{.ExitCode}}' grpc-certs }
  if (@($certState).Count -ne 1 -or "$certState" -ne 'grpc-certs|exited|0') {
    throw "Local gRPC certificate bootstrap did not complete successfully: $certState"
  }
}

Invoke-Native { docker compose -f $compose up -d --build --remove-orphans } 'Local Registry container deployment failed.'
Wait-Ready 'http://127.0.0.1:18070/readyz'
Invoke-Native { docker compose -f $compose ps }
Write-Host 'Registry local containers are running: http://127.0.0.1:18070  gRPC 127.0.0.1:19070'
Write-Host 'Start Identity after Registry. Platform, Catalog, Trade and Live register here.'
