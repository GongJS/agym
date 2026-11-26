package proc

import (
	"errors"
	"io"
	"os/exec"
	"time"
)

// ErrPTYUnsupported is returned on platforms without a PTY implementation.
var ErrPTYUnsupported = errors.New("pseudo-terminal is not supported on this platform")

// PTYSession represents an interactive pseudo-terminal attached to a subprocess.
type PTYSession interface {
	io.ReadWriteCloser
}

// TerminateProcessGroup sends a graceful termination signal to cmd's process
// group and escalates to a forced kill if it does not exit within grace.
func TerminateProcessGroup(cmd *exec.Cmd, grace time.Duration, wait <-chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	terminateGroup(cmd, false)
	if wait != nil {
		select {
		case <-wait:
			return
		case <-time.After(grace):
		}
	}
	terminateGroup(cmd, true)
	if wait != nil {
		select {
		case <-wait:
		case <-time.After(2 * time.Second):
		}
	}
}
