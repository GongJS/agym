//go:build windows

package gui

import "golang.org/x/sys/windows"

var (
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procFreeConsole = kernel32.NewProc("FreeConsole")
)

// hideConsoleWindow detaches the console window when launching the menu bar UI
// so the single binary does not leave a black cmd window open on Windows.
func hideConsoleWindow() {
	_, _, _ = procFreeConsole.Call()
}
