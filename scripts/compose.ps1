$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content -LiteralPath (Join-Path $repositoryRoot "VERSION") -Encoding utf8 -Raw).Trim()
$composeArguments = @($args)
$injectedMarker = "__nebula_doppler_injected__"
$injected = $composeArguments.Count -gt 0 -and $composeArguments[0] -eq $injectedMarker
if ($injected) {
    $composeArguments = @($composeArguments | Select-Object -Skip 1)
}

if ($version -notmatch '^\d+\.\d+\.\d+$') {
    throw "VERSION must contain a semantic version such as 0.1.0"
}

if (-not $injected) {
    if (-not (Get-Command doppler -ErrorAction SilentlyContinue)) {
        throw "Doppler CLI is required"
    }
    $childArguments = @(
        "run", "--project", "nebula-api", "--config", "dev_personal", "--",
        "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $PSCommandPath,
        $injectedMarker
    ) + $composeArguments
    & doppler @childArguments
    exit $LASTEXITCODE
}

$requiredSecrets = @(
    "JWT_SIGNING_KEY",
    "AUTH_STATE_HASH_PEPPER",
    "API_KEY_HASH_PEPPER",
    "PROVIDER_CREDENTIAL_ENCRYPTION_KEY",
    "BOOTSTRAP_TEACHER_NAME",
    "BOOTSTRAP_TEACHER_EMAIL",
    "BOOTSTRAP_TEACHER_PASSWORD",
    "SMTP_HOST",
    "SMTP_FROM"
)
$invalidSecrets = foreach ($name in $requiredSecrets) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ([string]::IsNullOrWhiteSpace($value) -or $value.StartsWith("replace_with", [StringComparison]::OrdinalIgnoreCase)) {
        $name
    }
}
if ($invalidSecrets.Count -gt 0) {
    throw "Doppler nebula-api/dev_personal has missing or placeholder values for: $($invalidSecrets -join ', ')"
}

$shortSecrets = foreach ($name in @("JWT_SIGNING_KEY", "AUTH_STATE_HASH_PEPPER", "API_KEY_HASH_PEPPER")) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ([Text.Encoding]::UTF8.GetByteCount($value) -lt 32) {
        $name
    }
}
if ($shortSecrets.Count -gt 0) {
    throw "Doppler nebula-api/dev_personal requires at least 32 UTF-8 bytes for: $($shortSecrets -join ', ')"
}

try {
    $providerKey = [Convert]::FromBase64String($env:PROVIDER_CREDENTIAL_ENCRYPTION_KEY)
}
catch {
    throw "PROVIDER_CREDENTIAL_ENCRYPTION_KEY in Doppler nebula-api/dev_personal must be valid Base64 for exactly 32 bytes"
}
if ($providerKey.Length -ne 32) {
    throw "PROVIDER_CREDENTIAL_ENCRYPTION_KEY in Doppler nebula-api/dev_personal must decode to exactly 32 bytes"
}

function Get-RequiredUri {
    param([string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "$Name is missing from Doppler nebula-api/dev_personal"
    }
    try {
        return [Uri]$value
    }
    catch {
        throw "$Name in Doppler nebula-api/dev_personal is not a valid URI"
    }
}

function Assert-LocalHost {
    param([string]$Name, [Uri]$Uri, [string]$ServiceName)

    if ($Uri.Host -notin @("localhost", "127.0.0.1", "::1", $ServiceName)) {
        throw "$Name must target localhost or the '$ServiceName' Compose service for local Docker startup"
    }
}

$databaseUri = Get-RequiredUri -Name "DATABASE_URL"
Assert-LocalHost -Name "DATABASE_URL" -Uri $databaseUri -ServiceName "postgres"
$databaseCredentials = $databaseUri.UserInfo.Split(':', 2)
if ($databaseCredentials.Count -ne 2) {
    throw "DATABASE_URL must include a username and password"
}
$databaseName = [Uri]::UnescapeDataString($databaseUri.AbsolutePath.Trim('/'))
if ([string]::IsNullOrWhiteSpace($databaseName)) {
    throw "DATABASE_URL must include a database name"
}

$databaseBuilder = [UriBuilder]$databaseUri
$databaseBuilder.Host = "postgres"
$databaseBuilder.Port = 5432
$env:DATABASE_URL = $databaseBuilder.Uri.AbsoluteUri
$env:POSTGRES_USER = [Uri]::UnescapeDataString($databaseCredentials[0])
$env:POSTGRES_PASSWORD = [Uri]::UnescapeDataString($databaseCredentials[1])
$env:POSTGRES_DB = $databaseName

$redisUri = Get-RequiredUri -Name "REDIS_URL"
Assert-LocalHost -Name "REDIS_URL" -Uri $redisUri -ServiceName "redis"
if (-not [string]::IsNullOrWhiteSpace($redisUri.UserInfo)) {
    throw "REDIS_URL contains credentials, but the local Redis service is intentionally configured without ACL credentials"
}
$redisBuilder = [UriBuilder]$redisUri
$redisBuilder.Host = "redis"
$redisBuilder.Port = 6379
$env:REDIS_URL = $redisBuilder.Uri.AbsoluteUri

$env:NEBULA_VERSION = $version

if ($env:HTTP_ADDRESS -ne ":8080") {
    throw "HTTP_ADDRESS in Doppler nebula-api/dev_personal must be :8080 for the current Compose port and health-check contract"
}

if (-not $composeArguments -or $composeArguments.Count -eq 0) {
    $composeArguments = @("up", "-d", "--build")
}

Push-Location $repositoryRoot
try {
    & docker compose --project-name nebula-api --file compose.yaml @composeArguments
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
