//go:build windows

package proc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// ConfigureDetached places cmd in a new Windows process group without flashing a console window.
func ConfigureDetached(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | createNoWindow,
	}
}

// KillProcessGroup terminates cmd and its descendant process tree on Windows.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := SignalPID(cmd.Process.Pid, true); err != nil {
		_ = cmd.Process.Kill()
	}
}

func terminateGroup(cmd *exec.Cmd, force bool) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = SignalPID(cmd.Process.Pid, force)
}

// SignalPID terminates pid (and its child tree when force is true) via taskkill/TerminateProcess.
func SignalPID(pid int, force bool) error {
	args := []string{"/PID", strconv.Itoa(pid), "/T"}
	if force {
		args = append(args, "/F")
	}
	killCmd := exec.Command("taskkill", args...)
	killCmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: createNoWindow}
	if err := killCmd.Run(); err == nil {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// IsPIDAlive reports whether pid is still running on Windows.
func IsPIDAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}

// NotifyShutdown registers os.Interrupt on Windows.
func NotifyShutdown(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// SignalExitCode maps an interrupt signal to exit code 130.
func SignalExitCode(_ os.Signal) int {
	return 130
}

type windowsPTY struct {
	cpty *conpty.ConPty
}

func (p *windowsPTY) Read(b []byte) (int, error)  { return p.cpty.Read(b) }
func (p *windowsPTY) Write(b []byte) (int, error) { return p.cpty.Write(b) }
func (p *windowsPTY) Close() error                { return p.cpty.Close() }

func (p *windowsPTY) Wait() error {
	code, err := p.cpty.Wait(context.Background())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("process exited with status %d", code)
	}
	return nil
}

func (p *windowsPTY) Terminate(grace time.Duration, wait <-chan error) {
	if p.cpty == nil {
		return
	}
	pid := p.cpty.Pid()
	if pid > 0 {
		_ = SignalPID(pid, false)
	}
	if wait != nil {
		select {
		case <-wait:
			return
		case <-time.After(grace):
		}
	}
	if pid > 0 {
		_ = SignalPID(pid, true)
	}
	_ = p.cpty.Close()
	if wait != nil {
		select {
		case <-wait:
		case <-time.After(2 * time.Second):
		}
	}
}

// StartPTY launches cmd inside a Windows ConPTY pseudo-terminal.
func StartPTY(cmd *exec.Cmd) (PTYSession, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, ErrPTYUnsupported
	}
	commandLine := buildWindowsCommandLine(cmd.Path, cmd.Args)
	var opts []conpty.ConPtyOption
	if cmd.Dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(cmd.Dir))
	}
	if len(cmd.Env) > 0 {
		opts = append(opts, conpty.ConPtyEnv(cmd.Env))
	}
	cpty, err := conpty.Start(commandLine, opts...)
	if err != nil {
		return nil, err
	}
	return &windowsPTY{cpty: cpty}, nil
}

func buildWindowsCommandLine(path string, args []string) string {
	parts := make([]string, 0, max(1, len(args)))
	parts = append(parts, windows.EscapeArg(path))
	if len(args) > 1 {
		for _, arg := range args[1:] {
			parts = append(parts, windows.EscapeArg(arg))
		}
	}
	return strings.Join(parts, " ")
}
