package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/envutil"
	"github.com/gongjs/agym/internal/usage"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	sessionEventMaxBytes = 1 << 20
	sessionPollInterval  = 250 * time.Millisecond
	sessionProbeTimeout  = 25 * time.Second
	sessionCooldown      = 10 * time.Minute
	sessionFastFailSpan  = 2 * time.Minute
)

type handoffChoice struct {
	Email       string
	ModelID     string
	OriginalRaw string
}

type handoffAction func(*accounts.LockedSlot, handoffChoice) (bool, error)

// HandleCommand implements the session-related CLI subcommands.
func HandleCommand(command string, args []string, stdout, stderr io.Writer) (int, bool) {
	switch command {
	case "hook":
		return runSessionHook(args, stdout, stderr), true
	case "install-hook":
		return runInstallSessionHook(args, stdout, stderr), true
	case "remove-hook":
		return runRemoveSessionHook(args, stdout, stderr), true
	case "supervise":
		return runSessionSupervisor(args, stdout, stderr), true
	default:
		return 0, false
	}
}

func runSessionHook(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: agym hook")
		return 2
	}
	path := strings.TrimSpace(os.Getenv("AGY_TOOLS_SESSION_EVENTS"))
	if path == "" {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, sessionEventMaxBytes+1))
	if err != nil || len(data) > sessionEventMaxBytes || !json.Valid(data) {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	event, ok := ParseEvent(string(data))
	if !ok {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	verdict := Classify(event)
	if verdict == Ignore {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	errorSummary := "quota-like error (unconfirmed)"
	if verdict == Quota {
		errorSummary = "RESOURCE_EXHAUSTED (code 429): Individual quota reached"
	}
	data, err = json.Marshal(struct {
		HookEventName     string `json:"hookEventName"`
		ConversationID    string `json:"conversationId"`
		ModelName         string `json:"modelName"`
		TerminationReason string `json:"terminationReason"`
		Error             string `json:"error"`
		FullyIdle         bool   `json:"fullyIdle"`
	}{"Stop", safeConversationID(event.ConversationID), safeModelName(event.ModelName), event.TerminationReason, errorSummary, event.FullyIdle})
	if err != nil {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_, _ = io.WriteString(stdout, "{}\n")
		return 0
	}
	_, _ = io.WriteString(stdout, "{}\n")
	return 0
}

func runInstallSessionHook(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: agym install-hook")
		return 2
	}
	binary, err := os.Executable()
	if err == nil {
		binary, err = filepath.Abs(binary)
	}
	hooksPath, pathErr := DefaultHooksPath()
	if err == nil && pathErr != nil {
		err = pathErr
	}
	if err == nil {
		_, err = InstallHook(hooksPath, ShellQuote(binary)+" hook")
	}
	if err != nil {
		fmt.Fprintf(stderr, "install hook: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Installed session hook in %s\n", hooksPath)
	return 0
}

func runRemoveSessionHook(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: agym remove-hook")
		return 2
	}
	hooksPath, err := DefaultHooksPath()
	if err != nil {
		fmt.Fprintf(stderr, "remove hook: %v\n", err)
		return 1
	}
	if _, err := UninstallHook(hooksPath); err != nil {
		fmt.Fprintf(stderr, "remove hook: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Removed session hook from %s\n", hooksPath)
	return 0
}

func runOfficialAgy(args []string, stdout, stderr io.Writer) int {
	binary, err := officialAgyBinary()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 127
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		return processExitCode(err)
	}
	return 0
}

func officialAgyBinary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("AGY_BIN")); configured != "" {
		if path, err := exec.LookPath(configured); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("configured AGY_BIN %q is not executable", configured)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".local", "bin", "agy")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("agy"); err == nil {
		return path, nil
	}
	return "", errors.New("official agy CLI not found; install it or set AGY_BIN")
}

func runSessionSupervisor(args []string, stdout, stderr io.Writer) int {
	pinned := false
	if len(args) > 0 && args[0] == "--pinned" {
		pinned = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: agym supervise [--pinned] [--] <agy arguments...>")
		return 2
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) || !ShouldSupervise(args, true, true) {
		return runOfficialAgy(args, stdout, stderr)
	}
	hooksPath, err := DefaultHooksPath()
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: %v\n", err)
		return 1
	}
	program, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: %v\n", err)
		return 1
	}
	if absolute, absErr := filepath.Abs(program); absErr == nil {
		program = absolute
	}
	if !HookConfigured(hooksPath, ShellQuote(program)+" hook") {
		fmt.Fprintln(stderr, "session supervisor requires the hook; run `agym install-hook` first")
		return 2
	}
	agyBinary, err := officialAgyBinary()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 127
	}
	roster, err := accounts.NewDefaultRoster()
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: %v\n", err)
		return 1
	}
	currentEmail, err := roster.CurrentEmail(context.Background())
	if err != nil || currentEmail == "" {
		fmt.Fprintln(stderr, "session supervisor: no active agy account found; running official agy without handoff")
		return runOfficialAgy(args, stdout, stderr)
	}
	members, err := roster.List(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: cannot read account roster: %v\n", err)
		return 1
	}
	activeEligible := false
	for _, member := range members {
		if strings.EqualFold(member.Email, currentEmail) {
			activeEligible = member.Rotation && member.HasSavedLogin
			break
		}
	}
	if !activeEligible {
		fmt.Fprintln(stderr, "session supervisor: active account is not enabled for rotation; running official agy without handoff")
		return runOfficialAgy(args, stdout, stderr)
	}

	return superviseInteractive(agyBinary, args, pinned, currentEmail, roster, stdout, stderr)
}

func superviseInteractive(agyBinary string, args []string, pinned bool, currentEmail string, roster *accounts.Roster, stdout, stderr io.Writer) int {
	eventsDir, err := os.MkdirTemp("", "agy-tools-session-*")
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: create event directory: %v\n", err)
		return 1
	}
	_ = os.Chmod(eventsDir, 0o700)
	defer os.RemoveAll(eventsDir)

	var serial uint64
	start := func(resumeArgs []string) (*exec.Cmd, <-chan error, *EventTail, string, error) {
		serial++
		eventPath := filepath.Join(eventsDir, "events-"+strconv.FormatUint(serial, 10)+".jsonl")
		f, err := os.OpenFile(eventPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, nil, "", err
		}
		if err := f.Close(); err != nil {
			return nil, nil, nil, "", err
		}
		cmd := exec.Command(agyBinary, resumeArgs...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
		cmd.Env = envutil.Replace(os.Environ(), map[string]string{"AGY_TOOLS_SESSION_EVENTS": eventPath})
		if err := cmd.Start(); err != nil {
			_ = os.Remove(eventPath)
			return nil, nil, nil, "", err
		}
		wait := make(chan error, 1)
		go func() { wait <- cmd.Wait() }()
		return cmd, wait, NewEventTail(eventPath), eventPath, nil
	}
	cmd, wait, tail, eventPath, err := start(args)
	if err != nil {
		fmt.Fprintf(stderr, "session supervisor: start agy: %v\n", err)
		return 1
	}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	ticker := time.NewTicker(sessionPollInterval)
	defer ticker.Stop()
	var cooldownUntil time.Time
	var lastSwitch time.Time
	fastFailures := 0
	rotationSize := countRotationMembers(roster)
	if rotationSize < 2 {
		rotationSize = 2
	}

	for {
		select {
		case err := <-wait:
			_ = os.Remove(eventPath)
			return processExitCode(err)
		case sig := <-signals:
			if cmd.ProcessState == nil {
				_ = stopProcessTree(cmd, wait, stderr)
			}
			_ = os.Remove(eventPath)
			return signalExitCode(sig)
		case <-ticker.C:
			for _, event := range tail.Poll() {
				if Classify(event) != Quota {
					if Classify(event) == Suspect {
						fmt.Fprintln(stderr, "session supervisor: quota-like error did not match the strict handoff rule; leaving this session unchanged")
					}
					continue
				}
				if cmd.ProcessState != nil {
					continue
				}
				if safeConversationID(event.ConversationID) == "" || !event.FullyIdle {
					fmt.Fprintln(stderr, "session supervisor: quota reached but no trusted conversation id was present; not restarting")
					continue
				}
				if pinned {
					fmt.Fprintln(stderr, "session supervisor: this run is pinned to the active account; automatic handoff is disabled")
					continue
				}
				now := time.Now()
				if now.Before(cooldownUntil) {
					fmt.Fprintf(stderr, "session supervisor: handoff cooling down until %s; continuing on the current account\n", cooldownUntil.Format(time.RFC3339))
					continue
				}
				if !lastSwitch.IsZero() && now.Sub(lastSwitch) < sessionFastFailSpan {
					fastFailures++
					if fastFailures >= rotationSize-1 {
						cooldownUntil = now.Add(sessionCooldown)
						fmt.Fprintln(stderr, "session supervisor: successive accounts exhausted quota too quickly; stopping rotation for 10 minutes")
						continue
					}
				} else {
					fastFailures = 0
				}
				probeCtx, cancelProbe := context.WithCancel(context.Background())
				var signalMu sync.Mutex
				var requestedSignal os.Signal
				signalWatchDone := make(chan struct{})
				go func() {
					select {
					case sig := <-signals:
						signalMu.Lock()
						requestedSignal = sig
						signalMu.Unlock()
						cancelProbe()
					case <-signalWatchDone:
					}
				}()
				stopRequested := func() bool {
					signalMu.Lock()
					defer signalMu.Unlock()
					return requestedSignal != nil
				}
				replacementCmd, replacementWait, replacementTail, replacementPath := (*exec.Cmd)(nil), (<-chan error)(nil), (*EventTail)(nil), ""
				choice, moved, chooseErr := chooseSessionAccount(probeCtx, roster, agyBinary, currentEmail, event, func() bool {
					return !stopRequested() && stillRunning(cmd)()
				}, func(_ *accounts.LockedSlot, selected handoffChoice) (bool, error) {
					if stopRequested() {
						return false, errors.New("session stop requested during handoff")
					}
					if err := stopProcessTree(cmd, wait, stderr); err != nil {
						return false, err
					}
					if stopRequested() {
						return false, errors.New("session stop requested during handoff")
					}
					resumeArgs := ResumeArgs(args, event.ConversationID, "继续", selected.ModelID)
					var startErr error
					replacementCmd, replacementWait, replacementTail, replacementPath, startErr = start(resumeArgs)
					if startErr != nil {
						return true, startErr
					}
					grace := time.NewTimer(2 * time.Second)
					defer grace.Stop()
					select {
					case <-probeCtx.Done():
					case <-grace.C:
					}
					return true, nil
				})
				close(signalWatchDone)
				cancelProbe()
				signalMu.Lock()
				sig := requestedSignal
				signalMu.Unlock()
				if sig != nil {
					if replacementCmd != nil && replacementCmd.ProcessState == nil {
						_ = stopProcessTree(replacementCmd, replacementWait, stderr)
					} else if cmd.ProcessState == nil {
						_ = stopProcessTree(cmd, wait, stderr)
					}
					_ = os.Remove(eventPath)
					return signalExitCode(sig)
				}
				if chooseErr != nil {
					if moved {
						fmt.Fprintf(stderr, "session supervisor: replacement account selected but agy could not resume: %v\nmanual resume: agy --conversation=%s\n", chooseErr, event.ConversationID)
						return 1
					}
					fmt.Fprintf(stderr, "session supervisor: no safe replacement account: %v\n", chooseErr)
					continue
				}
				if !moved {
					continue
				}
				_ = os.Remove(eventPath)
				currentEmail = choice.Email
				lastSwitch = time.Now()
				fmt.Fprintf(stderr, "session supervisor: resumed conversation on alternate account %s\n", currentEmail)
				cmd, wait, tail, eventPath = replacementCmd, replacementWait, replacementTail, replacementPath
			}
		}
	}
}

func chooseSessionAccount(ctx context.Context, roster *accounts.Roster, agyBinary, currentEmail string, event StopEvent, stillRunning func() bool, action handoffAction) (choice handoffChoice, moved bool, err error) {
	cachePath, err := usage.DefaultCachePath()
	if err != nil {
		return handoffChoice{}, false, err
	}
	cache := usage.LoadCache(cachePath)
	catalogPath, err := usage.DefaultModelCatalogPath()
	if err != nil {
		return handoffChoice{}, false, err
	}
	models := catalog.Load(catalogPath)
	usageRunner := usage.AgyRunner{Binary: agyBinary}
	err = roster.WithSlotLock(ctx, true, func(slot *accounts.LockedSlot) (callbackErr error) {
		originalRaw, readErr := slot.CurrentRaw(ctx)
		if readErr != nil {
			return readErr
		}
		originalEmail, readErr := accounts.LoginEmail(originalRaw)
		if readErr != nil {
			return readErr
		}
		if !strings.EqualFold(originalEmail, currentEmail) {
			return fmt.Errorf("active slot changed from %s to %s; refusing to switch a running session", currentEmail, originalEmail)
		}
		choice.OriginalRaw = originalRaw
		committed := false
		defer func() {
			if !committed {
				if restoreErr := slot.WriteCurrent(context.Background(), originalRaw); restoreErr != nil {
					callbackErr = errors.Join(callbackErr, fmt.Errorf("restore original account: %w", restoreErr))
				}
			}
		}()

		members, listErr := roster.List(ctx)
		if listErr != nil {
			return listErr
		}
		pool := make([]accounts.Account, 0, len(members))
		backups := make(map[string]string, len(members))
		for _, member := range members {
			if !member.Rotation || !member.HasSavedLogin || strings.EqualFold(member.Email, currentEmail) {
				continue
			}
			backup, backupErr := slot.BackupRaw(ctx, member.Email)
			if backupErr != nil {
				continue
			}
			backupEmail, parseErr := accounts.LoginEmail(backup)
			if parseErr != nil || !strings.EqualFold(backupEmail, member.Email) {
				continue
			}
			backups[member.Email] = backup
			pool = append(pool, member)
		}
		modelID, ordered := CandidateOrder(pool, currentEmail, event.ModelName, cache, models)
		if len(ordered) == 0 {
			return errors.New("no eligible rotation accounts remain")
		}
		bestScore := 0
		bestEmail, bestModel := "", ""
		for _, email := range ordered {
			if !stillRunning() {
				return errors.New("agy session ended before account probing completed")
			}
			backup := backups[email]
			if backup == "" {
				continue
			}
			if slot.Contended() {
				return errors.New("an interactive account operation is waiting for the Keychain slot")
			}
			if writeErr := slot.WriteCurrent(ctx, backup); writeErr != nil {
				return fmt.Errorf("activate candidate %s: %w", email, writeErr)
			}
			probeCtx, cancel := context.WithTimeout(ctx, sessionProbeTimeout)
			probe := usageRunner.QueryUsage(probeCtx, slot.Contended)
			cancel()
			quota := usage.QuotaFromProbe(probe)
			cache.Record(email, quota, time.Now())
			_ = cache.Save()
			if probe.Outcome == "yielded" {
				return errors.New("an interactive account operation took priority during quota probing")
			}
			if score := QuotaScore(quota, event.ModelName); score > bestScore {
				bestScore, bestEmail = score, email
				bestModel = modelID
			}
			if QuotaScore(quota, event.ModelName) >= MinGoodPercent {
				bestEmail = email
				bestModel = modelID
				break
			}
		}
		if bestEmail == "" {
			return errors.New("all eligible accounts are exhausted, unavailable, or lack usable quota data")
		}
		if !stillRunning() {
			return errors.New("agy session ended during account probing")
		}
		if writeErr := slot.WriteCurrent(ctx, backups[bestEmail]); writeErr != nil {
			return fmt.Errorf("activate selected account: %w", writeErr)
		}
		choice.Email, choice.ModelID = bestEmail, bestModel
		keepSlot, actionErr := action(slot, choice)
		if keepSlot {
			committed = true
			moved = true
		}
		return actionErr
	})
	if err != nil {
		return handoffChoice{}, moved, err
	}
	return choice, moved, nil
}

func restoreRawSlot(roster *accounts.Roster, raw string) error {
	if raw == "" {
		return errors.New("original account slot was empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return roster.WithSlotLock(ctx, false, func(slot *accounts.LockedSlot) error {
		return slot.WriteCurrent(ctx, raw)
	})
}

func countRotationMembers(roster *accounts.Roster) int {
	members, err := roster.List(context.Background())
	if err != nil {
		return 0
	}
	count := 0
	for _, member := range members {
		if member.Rotation && member.HasSavedLogin {
			count++
		}
	}
	return count
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func stillRunning(cmd *exec.Cmd) func() bool {
	return func() bool { return cmd != nil && cmd.Process != nil && cmd.ProcessState == nil }
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func signalExitCode(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 143
	}
	if sig == syscall.SIGHUP {
		return 129
	}
	return 130
}

func safeConversationID(value string) string {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value {
		return ""
	}
	for _, char := range value {
		if unicode.IsControl(char) || unicode.IsSpace(char) {
			return ""
		}
	}
	return value
}

func safeModelName(value string) string {
	if len(value) > 256 {
		return ""
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return ""
		}
	}
	return value
}

func stopProcessTree(cmd *exec.Cmd, wait <-chan error, stderr io.Writer) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	select {
	case <-wait:
		return nil
	default:
	}
	pid := cmd.Process.Pid
	descendants := processDescendants(pid)
	for i := len(descendants) - 1; i >= 0; i-- {
		_ = syscall.Kill(descendants[i], syscall.SIGTERM)
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-wait:
		terminatePIDs(descendants, 2*time.Second)
		return nil
	case <-time.After(20 * time.Second):
	}
	fmt.Fprintln(stderr, "session supervisor: agy did not stop after SIGTERM; terminating its process tree")
	for _, child := range processDescendants(pid) {
		if !containsPID(descendants, child) {
			descendants = append(descendants, child)
		}
	}
	for i := len(descendants) - 1; i >= 0; i-- {
		_ = syscall.Kill(descendants[i], syscall.SIGKILL)
	}
	_ = cmd.Process.Kill()
	select {
	case <-wait:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("timed out waiting for agy process to exit")
	}
}

func containsPID(pids []int, pid int) bool {
	for _, existing := range pids {
		if existing == pid {
			return true
		}
	}
	return false
}

func terminatePIDs(pids []int, grace time.Duration) {
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range pids {
			if err := syscall.Kill(pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
				alive = true
				break
			}
		}
		if !alive {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func processDescendants(root int) []int {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	children := make(map[int][]int, len(procs))
	for _, proc := range procs {
		ppid, err := proc.Ppid()
		if err != nil {
			continue
		}
		children[int(ppid)] = append(children[int(ppid)], int(proc.Pid))
	}
	var result []int
	var visit func(int)
	visit = func(parent int) {
		for _, child := range children[parent] {
			visit(child)
			result = append(result, child)
		}
	}
	visit(root)
	return result
}
