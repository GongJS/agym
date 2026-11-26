//go:build darwin || linux

package proc

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ConfigureDetached places cmd in its own process group so timeouts can clean up
// the entire subprocess tree.
func ConfigureDetached(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup immediately kills cmd's process group.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

func terminateGroup(cmd *exec.Cmd, force bool) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

// SignalPID sends a graceful (SIGTERM) or forced (SIGKILL) signal to pid.
func SignalPID(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(pid, sig)
}

// IsPIDAlive reports whether a process with pid still exists.
func IsPIDAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// NotifyShutdown registers OS termination signals on ch.
func NotifyShutdown(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

// SignalExitCode maps a received shutdown signal to its standard Unix exit code.
func SignalExitCode(sig os.Signal) int {
	switch sig {
	case syscall.SIGTERM:
		return 143
	case syscall.SIGHUP:
		return 129
	default:
		return 130
	}
}

type unixPTY struct {
	file *os.File
	cmd  *exec.Cmd
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.file.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.file.Write(b) }
func (p *unixPTY) Close() error                { return p.file.Close() }
func (p *unixPTY) Wait() error                 { return p.cmd.Wait() }
func (p *unixPTY) Terminate(grace time.Duration, wait <-chan error) {
	TerminateProcessGroup(p.cmd, grace, wait)
}

// StartPTY launches cmd inside a new pseudo-terminal.
func StartPTY(cmd *exec.Cmd) (PTYSession, error) {
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &unixPTY{file: f, cmd: cmd}, nil
}
