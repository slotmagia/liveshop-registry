param([switch]$Volumes)

$ErrorActionPreference = 'Stop'
$compose = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\deploy\compose.local.yml'))
$args = @('-f', $compose, 'down', '--remove-orphans')
if ($Volumes) { $args += '-v' }
$previous = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try { docker compose @args } finally { $ErrorActionPreference = $previous }
if ($LASTEXITCODE -ne 0) { throw 'Failed to stop the local Registry containers.' }
if ($Volumes) {
  Write-Output 'Registry containers and named volumes were removed. Stop Identity and Platform first if liveshop-grpc-certs is still mounted.'
} else {
  Write-Output 'Registry containers stopped. Named volumes were preserved.'
}
