package accounts

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const slotLockPollInterval = 100 * time.Millisecond

type slotFileLock struct {
	lock *flock.Flock
	gate *flock.Flock
}

func acquireSlotLock(ctx context.Context, path string, background bool) (*slotFileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := ensureLockFile(path); err != nil {
		return nil, err
	}
	gatePath := gateLockPath(path)
	if err := ensureLockFile(gatePath); err != nil {
		return nil, err
	}

	lock := &slotFileLock{
		lock: flock.New(path),
		gate: flock.New(gatePath),
	}
	deadline := time.Now().Add(slotLockTimeoutSeconds * time.Second)
	var err error
	if background {
		err = waitSlotLock(ctx, deadline, func() (bool, error) {
			gotGate, gateErr := lock.gate.TryRLock()
			if gateErr != nil || !gotGate {
				return false, gateErr
			}
			gotSlot, slotErr := lock.lock.TryLock()
			_ = lock.gate.Unlock()
			return gotSlot, slotErr
		})
	} else {
		holdingGate := false
		err = waitSlotLock(ctx, deadline, func() (bool, error) {
			if !holdingGate {
				gotGate, gateErr := lock.gate.TryLock()
				if gateErr != nil || !gotGate {
					return false, gateErr
				}
				holdingGate = true
			}
			gotSlot, slotErr := lock.lock.TryLock()
			if slotErr != nil || gotSlot {
				_ = lock.gate.Unlock()
				holdingGate = false
			}
			return gotSlot, slotErr
		})
		if holdingGate {
			_ = lock.gate.Unlock()
		}
	}
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return lock, nil
}

func gateLockPath(path string) string {
	return path[:len(path)-len(filepath.Ext(path))] + ".gate"
}

func ensureLockFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func waitSlotLock(ctx context.Context, deadline time.Time, attempt func() (bool, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, err := attempt()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if !time.Now().Before(deadline) {
			return ErrSlotLockTimeout
		}
		timer := time.NewTimer(slotLockPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *slotFileLock) Close() error {
	if l == nil {
		return nil
	}
	if l.lock != nil {
		_ = l.lock.Close()
	}
	if l.gate != nil {
		_ = l.gate.Close()
	}
	return nil
}

func slotLockContended(path string) bool {
	gatePath := gateLockPath(path)
	if err := ensureLockFile(gatePath); err != nil {
		return true
	}
	gate := flock.New(gatePath)
	ok, err := gate.TryRLock()
	if err != nil || !ok {
		return true
	}
	_ = gate.Close()
	return false
}
