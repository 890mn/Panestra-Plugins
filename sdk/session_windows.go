//go:build windows

package sdk

import "golang.org/x/sys/windows"

func LockSession() error {
	result, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("LockWorkStation").Call()
	if result == 0 {
		return err
	}
	return nil
}
