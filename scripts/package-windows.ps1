param(
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9.+-]*$')]
    [string] $Version
)

$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $arch = go env GOARCH
    if ($LASTEXITCODE -ne 0) { throw 'Unable to determine the target architecture.' }
    if ($arch -eq 'amd64') { $arch = 'x64' }
    $name = (@('calender', $Version, 'windows', $arch) | Where-Object { $_ }) -join '-'
    $dist = New-Item -ItemType Directory -Path 'dist' -Force
    $target = Join-Path $dist.FullName "$name.zip"
    $temp = Join-Path $dist.FullName ("package-" + [guid]::NewGuid() + ".zip")
    try {
        Compress-Archive -LiteralPath 'calender.exe', 'README.md' -DestinationPath $temp
        if ([IO.File]::Exists($target)) {
            [IO.File]::Replace($temp, $target, [NullString]::Value)
        } else {
            [IO.File]::Move($temp, $target)
        }
    } finally {
        if ([IO.File]::Exists($temp)) { Remove-Item -LiteralPath $temp }
    }
    $sha256 = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($target)
    try {
        $digest = [BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '')
        Write-Output "$target`nSHA-256 $digest"
    } finally {
        $stream.Dispose()
        $sha256.Dispose()
    }
} finally {
    Pop-Location
}
