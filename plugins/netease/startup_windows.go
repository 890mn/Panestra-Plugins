//go:build windows

package netease

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

//go:embed startup.ps1
var startupScript string

type nativeStartup struct{}

func newStartup() startupManager { return nativeStartup{} }
func (nativeStartup) Call(parent context.Context, action, dir string, port int) (StartupInfo, error) {
	var result StartupInfo
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return result, err
	}
	input, _ := json.Marshal(map[string]any{"action": action, "dir": dir, "port": port})
	cmd := exec.CommandContext(ctx, filepath.Join(directory, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedScript(startupScript))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdin = bytes.NewReader(append(input, '\n'))
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err != nil || len(output) > 8192 || json.Unmarshal(output, &result) != nil {
		return StartupInfo{}, fmt.Errorf("无法处理网易云启动入口，请检查快捷方式是否可写")
	}
	return result, nil
}
