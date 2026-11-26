//go:build !darwin && !linux && !windows

package proc

import (
	"os"
	"os/exec"
	"os/signal"
)

// ConfigureDetached is a no-op stub on unsupported platforms.
func ConfigureDetached(_ *exec.Cmd) {}

// KillProcessGroup terminates cmd's process on unsupported platforms.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func terminateGroup(cmd *exec.Cmd, _ bool) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

// SignalPID terminates the process identified by pid.
func SignalPID(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// IsPIDAlive reports whether a process handle can be acquired for pid.
func IsPIDAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

// NotifyShutdown registers os.Interrupt on unsupported platforms.
func NotifyShutdown(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// SignalExitCode maps an interrupt signal to exit code 130.
func SignalExitCode(_ os.Signal) int {
	return 130
}

// StartPTY is unsupported on this platform.
func StartPTY(_ *exec.Cmd) (PTYSession, error) {
	return nil, ErrPTYUnsupported
}
