$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$projectRoot = Split-Path -Parent $PSScriptRoot

function Invoke-SetupCommand {
    param([string]$Command, [string[]]$CommandArguments)
    & $Command @CommandArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Setup failed: $Command $($CommandArguments -join ' ') (exit $LASTEXITCODE)"
    }
}

Push-Location -LiteralPath $projectRoot
try {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'CS Demo Review desktop development requires Windows.'
    }
    $nodeVersion = & node --version
    if ($LASTEXITCODE -ne 0 -or [version]($nodeVersion.TrimStart('v')) -lt [version]'22.12.0') {
        throw 'Install Node.js 22.12 or newer, then run npm run setup:worktree again.'
    }

    # Managed worktrees can receive this portable toolchain through .worktreeinclude.
    $localGo = Join-Path $projectRoot '.tools/go/bin/go.exe'
    $goCommand = if ($env:GO_BINARY) { $env:GO_BINARY }
        elseif (Test-Path -LiteralPath $localGo) { $localGo }
        elseif (Get-Command go -ErrorAction SilentlyContinue) { 'go' }
        else { $null }

    if (-not $goCommand) {
        Write-Host 'Installing the pinned portable Go toolchain for this checkout...'
        Invoke-SetupCommand 'node' @('scripts/setup-tools.mjs', '--go')
        $goCommand = $localGo
    }
    $goVersion = & $goCommand version
    if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch 'go(\d+)\.(\d+)') {
        throw 'Unable to run Go. Check GO_BINARY or the installed Go toolchain.'
    }
    if ([version]"$($Matches[1]).$($Matches[2])" -lt [version]'1.25') {
        throw 'Go 1.25 or newer is required. Update Go or point GO_BINARY to a supported executable.'
    }

    Write-Host 'Installing dependencies into this checkout...'
    Invoke-SetupCommand 'npm.cmd' @('ci', '--no-audit', '--no-fund')
    Invoke-SetupCommand 'npm.cmd' @('run', 'typecheck')
    Invoke-SetupCommand 'npm.cmd' @('run', 'build:worker')
    Invoke-SetupCommand 'npm.cmd' @('run', 'build:app')
    Write-Host 'Worktree ready. Run npm run dev or npm start.'
}
finally {
    Pop-Location
}
