//go:build windows

package netease

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func startupTestCommand(t *testing.T, script string) *exec.Cmd {
	t.Helper()
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "startup-test.ps1")
	if err := os.WriteFile(file, append([]byte{0xef, 0xbb, 0xbf}, []byte(script)...), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(directory, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", file)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func TestStartupArgumentRewritePreservesUnrelatedFlags(t *testing.T) {
	// Evaluate only the pure argument transformer, without invoking any OS mutations.
	ps := `$port = 19228
$scriptSource = @'
` + startupScript + `
'@
$tokens = $null; $errors = $null
$tree = [Management.Automation.Language.Parser]::ParseInput($scriptSource, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw 'Invalid startup script' }
$function = $tree.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'With-Flags'}, $true)
. ([scriptblock]::Create($function.Extent.Text))
$cases = @(
 @{ Before = ''; After = '--remote-debugging-address=127.0.0.1 --remote-debugging-port=19228' },
 @{ Before = '--minimized'; After = '--minimized --remote-debugging-address=127.0.0.1 --remote-debugging-port=19228' },
 @{ Before = '--minimized --remote-debugging-port=9222 --remote-debugging-address=0.0.0.0'; After = '--minimized --remote-debugging-address=127.0.0.1 --remote-debugging-port=19228' },
 @{ Before = '--remote-debugging-port 9222 --remote-debugging-address "0.0.0.0" --minimized'; After = '--minimized --remote-debugging-address=127.0.0.1 --remote-debugging-port=19228' }
)
foreach ($case in $cases) {
 $result = With-Flags $case.Before
 if ($result -cne $case.After) { throw ('Incorrect rewrite: ' + $result) }
 if ((With-Flags $result) -cne $result) { throw 'Rewrite is not idempotent' }
}
`
	cmd := startupTestCommand(t, ps)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("argument rewrite: %v %s", err, output)
	}
}
func TestStartupInspectionDoesNotCreateRecoveryFile(t *testing.T) {
	if len(encodedScript(startupScript)) > 31000 {
		t.Fatal("startup helper exceeds Windows command line limit")
	}
	dir := t.TempDir()
	result, err := newStartup().Call(context.Background(), "inspect", dir, 19228)
	if err != nil || result.Message == "" {
		t.Fatal("startup inspection failed", result, err)
	}
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil || len(entries) != 0 {
		t.Fatal("inspection mutated data directory", entries, err)
	}
}

func TestStartupRecoveryDoesNotOverwriteUserChanges(t *testing.T) {
	dir := t.TempDir()
	ps := `$ErrorActionPreference = 'Stop'
$testDir = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString([]byte(dir)) + `'))
$scriptSource = @'
` + startupScript + `
'@
$tokens = $null; $errors = $null
$tree = [Management.Automation.Language.Parser]::ParseInput($scriptSource, [ref]$tokens, [ref]$errors)
foreach ($name in @('Allowed-Link','Music-Target','With-Flags','Load-Backup','Save-Backup','Set-Entry','User-Link')) {
 $function = $tree.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name}, $true)
 . ([scriptblock]::Create($function.Extent.Text))
}
Add-Type -AssemblyName System.Security
$roots = @($testDir); $port = 19228; $dataDir = $testDir
$backupPath = Join-Path $testDir 'netease-launch.protected'
$wsh = New-Object -ComObject WScript.Shell
$exe = Join-Path $testDir 'cloudmusic.exe'
[IO.File]::WriteAllBytes($exe, [byte[]]@())
$path = Join-Path $testDir 'music.lnk'
$link = $wsh.CreateShortcut($path); $link.TargetPath = $exe; $link.Arguments = '--minimized'; $link.Save()
$entry = [PSCustomObject]@{ Kind='link'; Path=$path; Target=$exe; Before='--minimized'; After=(With-Flags '--minimized') }
Save-Backup @($entry)
Save-Backup @($entry)
Save-Backup @($entry, $entry)
if (@(Load-Backup).Count -ne 2) { throw 'Multiple recovery records lost' }
Save-Backup @($entry)
if ((Load-Backup)[0].Before -cne '--minimized') { throw 'Recovery data lost' }
if ([Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($backupPath)).Contains('--minimized')) { throw 'Unprotected recovery file' }
if (-not (Set-Entry $entry $false)) { throw ('Repair rejected: allowed=' + (Allowed-Link $entry.Path) + '; target=' + (Music-Target $entry.Target) + '; current=' + $wsh.CreateShortcut($path).TargetPath + '; before=' + $wsh.CreateShortcut($path).Arguments + '; planned=' + $entry.Before) }
if (-not (Set-Entry $entry $false)) { throw 'Repeated repair rejected' }
if (-not (Set-Entry $entry $true)) { throw 'Restore rejected' }
if ($wsh.CreateShortcut($path).Arguments -cne '--minimized') { throw 'Original arguments not restored' }
if (-not (Set-Entry $entry $false)) { throw 'Second repair rejected' }
$link = $wsh.CreateShortcut($path); $link.Arguments = '--user-changed'; $link.Save()
if (Set-Entry $entry $true) { throw 'Restore overwrote user changes' }
if ($wsh.CreateShortcut($path).Arguments -cne '--user-changed') { throw 'User arguments lost' }
$entry.Path = Join-Path (Split-Path $testDir) 'outside.lnk'
if (Set-Entry $entry $false) { throw 'Outside shortcut accepted' }
$shared = [Environment]::GetFolderPath('CommonStartMenu')
$user = [Environment]::GetFolderPath('StartMenu')
if ((User-Link (Join-Path $shared 'Programs\music.lnk')) -ine (Join-Path $user 'Programs\music.lnk')) { throw 'User shortcut mapping failed' }
# Synthetic shared/user folders exercise copy and restore without touching a real Windows shortcut.
$source = Join-Path $testDir 'shared.lnk'
$destination = Join-Path $testDir 'user.lnk'
$link = $wsh.CreateShortcut($source); $link.TargetPath = $exe; $link.Arguments = '--original'; $link.Save()
function User-Link([string]$path) { if ($path -eq $source) { return $destination }; return $null }
$copy = [PSCustomObject]@{Kind='link'; Path=$destination; Source=$source; Created=$true; Target=$exe; Before='--original'; After=(With-Flags '--original')}
if (-not (Set-Entry $copy $false)) { throw 'User fallback copy failed' }
if ($wsh.CreateShortcut($source).Arguments -cne '--original') { throw 'Shared shortcut changed' }
if (-not (Set-Entry $copy $true) -or (Test-Path -LiteralPath $destination)) { throw 'Created shortcut restore failed' }
if (-not (Test-Path -LiteralPath $source)) { throw 'Shared shortcut removed' }
`
	cmd := startupTestCommand(t, ps)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("startup recovery: %v %s", err, output)
	}
}
