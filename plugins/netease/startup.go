package netease

import (
	"context"
	"fmt"
	"path/filepath"
)

// StartupInfo never exposes shortcut arguments or registry values to devices.
type StartupInfo struct {
	Message    string `json:"message"`
	Configured int    `json:"configured"`
	Skipped    int    `json:"skipped"`
	Restorable int    `json:"restorable"`
}

type startupManager interface {
	Call(context.Context, string, string, int) (StartupInfo, error)
}

func (s *Service) Startup(ctx context.Context, action string) (StartupInfo, error) {
	if action != "inspect" && action != "repair" && action != "restore" {
		return StartupInfo{}, fmt.Errorf("不支持的启动设置操作")
	}
	if !s.startupOperation.TryLock() {
		return StartupInfo{}, fail("busy", "启动设置正在处理，请稍后重试")
	}
	defer s.startupOperation.Unlock()
	s.mu.Lock()
	port := s.config.TimelinePort
	s.mu.Unlock()
	if action == "repair" && port == 0 {
		return StartupInfo{}, fmt.Errorf("请先保存本机进度通道端口，再修复启动方式")
	}
	if port == 0 {
		port = 19228
	}
	return s.startup.Call(ctx, action, filepath.Dir(s.filename), port)
}
