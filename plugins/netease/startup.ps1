$ErrorActionPreference = 'Stop'
$inputData = [Console]::ReadLine() | ConvertFrom-Json
$action = [string]$inputData.action
$port = [int]$inputData.port
if ($action -notin @('inspect', 'repair', 'restore') -or $port -lt 1024 -or $port -gt 65535) { throw 'Invalid request' }
$dataDir = [IO.Path]::GetFullPath([string]$inputData.dir)
$backupPath = Join-Path $dataDir 'netease-launch.protected'
Add-Type -AssemblyName System.Security
$wsh = New-Object -ComObject WScript.Shell
$roots = @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('CommonDesktopDirectory'), [Environment]::GetFolderPath('StartMenu'), [Environment]::GetFolderPath('CommonStartMenu'), (Join-Path $env:APPDATA 'Microsoft/Internet Explorer/Quick Launch/User Pinned')) | Where-Object { $_ } | Select-Object -Unique
$runPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'

function Allowed-Link([string]$path) {
    $full = [IO.Path]::GetFullPath($path)
    if ([IO.Path]::GetExtension($full) -ine '.lnk') { return $false }
    foreach ($root in $roots) {
        if ($full.StartsWith([IO.Path]::GetFullPath($root).TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)) {
            if (Test-Path -LiteralPath $full) {
                return -not ((Get-Item -LiteralPath $full).Attributes -band [IO.FileAttributes]::ReparsePoint)
            }
            return $true
        }
    }
    return $false
}
function Music-Target([string]$path) {
    return $path -and [IO.Path]::IsPathRooted($path) -and [IO.Path]::GetFileName($path) -ieq 'cloudmusic.exe' -and (Test-Path -LiteralPath $path -PathType Leaf)
}
function User-Link([string]$path) {
    foreach ($pair in @(
        @([Environment]::GetFolderPath('CommonStartMenu'), [Environment]::GetFolderPath('StartMenu')),
        @([Environment]::GetFolderPath('CommonDesktopDirectory'), [Environment]::GetFolderPath('Desktop'))
    )) {
        $prefix = [IO.Path]::GetFullPath($pair[0]).TrimEnd('\') + '\'
        if ($path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
            return Join-Path $pair[1] $path.Substring($prefix.Length)
        }
    }
    return $null
}
function Writable-Link([string]$path) {
    try {
        $stream = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::ReadWrite)
        $stream.Dispose()
        return $true
    } catch { return $false }
}
function With-Flags([string]$argsValue) {
    $clean = [regex]::Replace($argsValue, '(?i)(?<!\S)--remote-debugging-(?:port|address)(?:=(?:"[^"]*"|\S+)|\s+(?:"[^"]*"|\S+))(?=\s|$)', '').Trim()
    return ($clean + ' --remote-debugging-address=127.0.0.1 --remote-debugging-port=' + $port).Trim()
}
function Run-Target([string]$value) {
    # Only a quoted CloudMusic executable is eligible; never parse or execute arbitrary commands.
    if ($value -match '^"([^"\r\n]+)"(?:\s+(.*))?$' -and (Music-Target $Matches[1])) {
        return [PSCustomObject]@{ Target = $Matches[1]; Arguments = [string]$Matches[2] }
    }
    return $null
}
function Load-Backup {
    if (-not (Test-Path -LiteralPath $backupPath)) { return @() }
    $raw = [IO.File]::ReadAllBytes($backupPath)
    if ($raw.Length -gt 524288) { throw 'Backup too large' }
    $plain = [Security.Cryptography.ProtectedData]::Unprotect($raw, $null, [Security.Cryptography.DataProtectionScope]::CurrentUser)
    $decoded = [Text.Encoding]::UTF8.GetString($plain) | ConvertFrom-Json
    foreach ($record in $decoded) { $record }
}
function Save-Backup($records) {
    [IO.Directory]::CreateDirectory($dataDir) | Out-Null
    $plain = [Text.Encoding]::UTF8.GetBytes((ConvertTo-Json -InputObject @($records) -Depth 6 -Compress))
    if ($plain.Length -gt 262144) { throw 'Backup too large' }
    $protected = [Security.Cryptography.ProtectedData]::Protect($plain, $null, [Security.Cryptography.DataProtectionScope]::CurrentUser)
    $temporary = $backupPath + '.tmp'
    [IO.File]::WriteAllBytes($temporary, $protected)
    if (Test-Path -LiteralPath $backupPath) { [IO.File]::Replace($temporary, $backupPath, [NullString]::Value) }
    else { [IO.File]::Move($temporary, $backupPath) }
}
function Set-Entry($entry, [bool]$restore) {
    if ($entry.Kind -eq 'link') {
        if (-not (Allowed-Link $entry.Path) -or -not (Music-Target $entry.Target)) { return $false }
        if (-not (Test-Path -LiteralPath $entry.Path)) {
            if ($restore -or -not $entry.Created -or -not (Allowed-Link $entry.Source)) { return $false }
            if ((User-Link $entry.Source) -ine $entry.Path) { return $false }
            $source = $wsh.CreateShortcut($entry.Source)
            if ([IO.Path]::GetFullPath($source.TargetPath) -ine [IO.Path]::GetFullPath($entry.Target) -or $source.Arguments -cne $entry.Before) { return $false }
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($entry.Path)) | Out-Null
            Copy-Item -LiteralPath $entry.Source -Destination $entry.Path
            (Get-Item -LiteralPath $entry.Path).IsReadOnly = $false
        }
        $link = $wsh.CreateShortcut($entry.Path)
        if ([IO.Path]::GetFullPath($link.TargetPath) -ine [IO.Path]::GetFullPath($entry.Target)) { return $false }
        if ($restore) {
            if ($link.Arguments -cne $entry.After) { return $false }
            if ($entry.Created) {
                Remove-Item -LiteralPath $entry.Path
                return $true
            }
            $link.Arguments = $entry.Before
        } else {
            if ($link.Arguments -cne $entry.Before -and $link.Arguments -cne $entry.After) { return $false }
            $link.Arguments = $entry.After
        }
        $link.Save()
        return $true
    }
    if ($entry.Kind -eq 'run' -and $entry.Path -eq $runPath) {
        $current = Get-ItemPropertyValue -LiteralPath $runPath -Name $entry.Name -ErrorAction SilentlyContinue
        if (-not $current -or -not (Run-Target $current)) { return $false }
        if ($restore) {
            if ($current -cne $entry.After) { return $false }
            Set-ItemProperty -LiteralPath $runPath -Name $entry.Name -Value $entry.Before
        } else {
            if ($current -cne $entry.Before -and $current -cne $entry.After) { return $false }
            Set-ItemProperty -LiteralPath $runPath -Name $entry.Name -Value $entry.After
        }
        return $true
    }
    return $false
}

$records = @(Load-Backup)
$skipped = 0
$configured = 0
if ($action -eq 'restore') {
    $remaining = @()
    foreach ($entry in $records) {
        try {
            if (Set-Entry $entry $true) { $configured++ }
            else { $remaining += $entry; $skipped++ }
        } catch { $remaining += $entry; $skipped++ }
    }
    Save-Backup $remaining
    $records = $remaining
    $message = "已恢复 $configured 个启动入口"
    if ($skipped) { $message += "，$skipped 个入口已更改或不可写，保留备份" }
} else {
    $entries = @()
    $links = @($roots | ForEach-Object { if (Test-Path -LiteralPath $_) { Get-ChildItem -LiteralPath $_ -Filter '*.lnk' -Recurse -ErrorAction SilentlyContinue } } | Select-Object -Unique -Property FullName | Select-Object -First 3000)
    foreach ($file in $links) {
        if (-not (Allowed-Link $file.FullName)) { continue }
        try {
            $link = $wsh.CreateShortcut($file.FullName)
            if (-not (Music-Target $link.TargetPath)) { continue }
            $userLink = User-Link $file.FullName
            # Windows merges user and shared shortcuts by relative path; an existing user entry takes precedence.
            if ($userLink -and (Test-Path -LiteralPath $userLink)) { continue }
            $before = [string]$link.Arguments
            $after = With-Flags $before
            if ($before -ceq $after) { $configured++; continue }
            if ($action -eq 'repair' -and $userLink -and -not (Writable-Link $file.FullName)) {
                # Discard recovery records for shared links that were never modified by an earlier repair.
                $records = @($records | Where-Object { -not ($_.Kind -eq 'link' -and $_.Path -ieq $file.FullName -and $_.Before -ceq $before) })
                $entries += [PSCustomObject]@{ Kind = 'link'; Path = $userLink; Source = $file.FullName; Created = $true; Target = $link.TargetPath; Before = $before; After = $after }
            } else {
                $entries += [PSCustomObject]@{ Kind = 'link'; Path = $file.FullName; Target = $link.TargetPath; Before = $before; After = $after }
            }
        } catch { $skipped++ }
    }
    if (Test-Path -LiteralPath $runPath) {
        $run = Get-ItemProperty -LiteralPath $runPath
        foreach ($property in $run.PSObject.Properties) {
            if ($property.Name.StartsWith('PS') -or $property.Value -isnot [string]) { continue }
            $target = Run-Target $property.Value
            if (-not $target) { continue }
            $after = '"' + $target.Target + '" ' + (With-Flags $target.Arguments)
            if ($property.Value -ceq $after) { $configured++; continue }
            $entries += [PSCustomObject]@{ Kind = 'run'; Path = $runPath; Name = $property.Name; Before = $property.Value; After = $after }
        }
    }
    if ($action -eq 'repair') {
        $plan = @()
        foreach ($entry in $entries) {
            $old = @($records | Where-Object { $_.Kind -eq $entry.Kind -and $_.Path -ieq $entry.Path -and $_.Name -eq $entry.Name })
            if ($old.Count) {
                # Preserve original values across repeated repairs and port changes, but honor user edits.
                if ($old.Count -ne 1 -or $entry.Before -cne $old[0].After) { $skipped++; continue }
                $old[0].After = $entry.After
            } else { $records += $entry }
            $plan += $entry
        }
        # Persist recovery information before changing any Windows entry.
        Save-Backup $records
        foreach ($entry in $plan) {
            try { if (Set-Entry $entry $false) { $configured++ } else { $skipped++ } }
            catch { $skipped++ }
        }
    }
    $message = if ($configured) { "$configured 个启动入口已配置进度参数，退出网易云后从原入口重新打开" } elseif ($entries.Count) { "已找到 $($entries.Count) 个启动入口，点击修复启动方式后退出网易云再打开" } else { '未找到可修复的启动入口，请先创建网易云快捷方式' }
    if ($action -eq 'inspect' -and $entries.Count -and $configured) { $message += "，$($entries.Count) 个入口待修复" }
    if ($skipped) { $message += "，$skipped 个入口已更改或不可写" }
}
@{ message = $message; configured = $configured; skipped = $skipped; restorable = $records.Count } | ConvertTo-Json -Compress
