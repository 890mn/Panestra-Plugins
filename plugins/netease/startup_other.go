//go:build !windows

package netease

import (
	"context"
	"fmt"
)

type nativeStartup struct{}

func newStartup() startupManager { return nativeStartup{} }
func (nativeStartup) Call(context.Context, string, string, int) (StartupInfo, error) {
	return StartupInfo{}, fmt.Errorf("启动入口修复仅支持 Windows")
}
