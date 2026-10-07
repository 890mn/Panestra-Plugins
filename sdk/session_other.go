//go:build !windows

package sdk

import "errors"

func LockSession() error { return errors.New("session locking requires Windows") }
