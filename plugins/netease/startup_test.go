package netease

import (
	"context"
	"panestra.local/plugins/sdk"
	"path/filepath"
	"testing"
)

type fakeStartup struct {
	calls       int
	action, dir string
	port        int
}

func TestStartupOperationsRequireOwner(t *testing.T) {
	fake := &fakeStartup{}
	s := &Service{filename: filepath.Join(t.TempDir(), "netease.protected"), startup: fake, config: Config{TimelinePort: 19228}}
	handler := newHandler(s)
	for _, operation := range []string{"setup", "repair-startup", "restore-startup"} {
		for _, role := range []string{"viewer", "operator", "", "OWNER"} {
			if response := handler(context.Background(), sdk.Request{Role: role, Operation: operation}); response.Status != 403 || fake.calls != 0 {
				t.Fatal("non-owner startup access", operation, role, response.Status)
			}
		}
	}
	for _, operation := range []string{"setup", "repair-startup", "restore-startup"} {
		if response := handler(context.Background(), sdk.Request{Role: "owner", Operation: operation}); response.Status != 200 {
			t.Fatal("owner startup rejected", operation, response.Status)
		}
	}
	if fake.calls != 3 {
		t.Fatal("owner operations not forwarded")
	}
}

func (f *fakeStartup) Call(_ context.Context, action, dir string, port int) (StartupInfo, error) {
	f.calls++
	f.action, f.dir, f.port = action, dir, port
	return StartupInfo{Configured: 1}, nil
}
func TestStartupSerializesChangesWithoutBlockingPlaybackPolling(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeStartup{}
	s := &Service{filename: filepath.Join(dir, "netease.protected"), startup: fake}
	ctx := context.Background()
	if _, err := s.Startup(ctx, "repair"); err == nil || fake.calls != 0 {
		t.Fatal("repair without configured port")
	}
	if _, err := s.Startup(ctx, "launch"); err == nil || fake.calls != 0 {
		t.Fatal("unexpected startup action accepted")
	}
	s.config.TimelinePort = 19228
	s.operation.Lock()
	if _, err := s.Startup(ctx, "repair"); err != nil || fake.calls != 1 {
		t.Fatal("playback polling blocked startup repair")
	}
	s.operation.Unlock()
	s.startupOperation.Lock()
	if _, err := s.Startup(ctx, "repair"); err == nil || fake.calls != 1 {
		t.Fatal("concurrent startup changes accepted")
	}
	s.startupOperation.Unlock()
	if _, err := s.Startup(ctx, "repair"); err != nil || fake.action != "repair" || fake.port != 19228 || fake.dir != dir {
		t.Fatal("incorrect startup configuration", err, fake)
	}
	s.config.TimelinePort = 0
	if _, err := s.Startup(ctx, "restore"); err != nil || fake.action != "restore" {
		t.Fatal("restore blocked after port disabled", err)
	}
}
