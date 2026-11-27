package projects

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxProjectsReturned  = 12
	dismissedStorageFile = "agym_dismissed_projects.json"
)

// ProjectView contains safe, read-only project activity, context window usage,
// and token totals read from local agy state.
type ProjectView struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	ConversationID  string `json:"conversationId"`
	Active          bool   `json:"active"`
	Model           string `json:"model,omitempty"`
	LastActiveAt    int64  `json:"lastActiveAt"`
	StepCount       int    `json:"stepCount"`
	ContextLimit    int64  `json:"contextLimit"`
	LatestInput     int64  `json:"latestInput"`
	LatestCacheRead int64  `json:"latestCacheRead"`
	LatestOutput    int64  `json:"latestOutput"`
	TotalInput      int64  `json:"totalInput"`
	TotalCacheRead  int64  `json:"totalCacheRead"`
	TotalOutput     int64  `json:"totalOutput"`
}

type transcriptSnapshot struct {
	modTime         int64
	offset          int64
	stepCount       int
	model           string
	contextLimit    int64
	latestInput     int64
	latestCacheRead int64
	latestOutput    int64
	totalInput      int64
	totalCacheRead  int64
	totalOutput     int64
	lastType        string
	lastStatus      string
}

type dbRunningCache struct {
	modTime    int64
	size       int64
	walModTime int64
	walSize    int64
	running    map[string]bool
}

var (
	cacheMu         sync.Mutex
	transcriptCache = make(map[string]*transcriptSnapshot)
	dbCacheMu       sync.Mutex
	dbCache         = make(map[string]dbRunningCache)
)

// DefaultAgyDir returns the default antigravity-cli state directory (~/.gemini/antigravity-cli).
func DefaultAgyDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "antigravity-cli"), nil
}

// ListProjects reads local history, presence locks, and conversation transcripts
// to project per-workspace context and token statistics.
func ListProjects(agyDir string) ([]ProjectView, error) {
	if agyDir == "" {
		var err error
		agyDir, err = DefaultAgyDir()
		if err != nil {
			return nil, err
		}
	}

	activeIDs := readActivePresence(filepath.Join(agyDir, "presence"))
	runningInDB := readRunningFromDB(agyDir)

	type candidate struct {
		workspace      string
		conversationID string
		lastActiveAt   int64
		hasLock        bool
		active         bool
	}
	byWorkspace := make(map[string]candidate)

	historyPath := filepath.Join(agyDir, "history.jsonl")
	if f, err := os.Open(historyPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var row struct {
				Workspace      string `json:"workspace"`
				ConversationID string `json:"conversationId"`
				Timestamp      int64  `json:"timestamp"`
			}
			if json.Unmarshal(scanner.Bytes(), &row) != nil {
				continue
			}
			ws := normalizeWorkspacePath(row.Workspace)
			cid := strings.TrimSpace(row.ConversationID)
			if ws == "" || cid == "" {
				continue
			}
			hasLock := activeIDs[cid]
			isActive := hasLock && runningInDB[cid]
			prev, exists := byWorkspace[ws]
			if !exists || (isActive && !prev.active) || (isActive == prev.active && row.Timestamp >= prev.lastActiveAt) {
				byWorkspace[ws] = candidate{
					workspace:      ws,
					conversationID: cid,
					lastActiveAt:   row.Timestamp,
					hasLock:        hasLock,
					active:         isActive,
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	dismissed := readDismissedProjects(agyDir)
	dismissedDirty := false
	items := make([]candidate, 0, len(byWorkspace))
	for _, c := range byWorkspace {
		if dismissedAt, hidden := dismissed[c.workspace]; hidden {
			if c.active || c.lastActiveAt > dismissedAt {
				delete(dismissed, c.workspace)
				dismissedDirty = true
			} else {
				continue
			}
		}
		items = append(items, c)
	}
	if dismissedDirty {
		_ = writeDismissedProjects(agyDir, dismissed)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].active != items[j].active {
			return items[i].active
		}
		if items[i].lastActiveAt != items[j].lastActiveAt {
			return items[i].lastActiveAt > items[j].lastActiveAt
		}
		return items[i].workspace < items[j].workspace
	})
	if len(items) > maxProjectsReturned {
		items = items[:maxProjectsReturned]
	}

	views := make([]ProjectView, 0, len(items))
	for _, item := range items {
		name := filepath.Base(item.workspace)
		if name == "." || name == "/" || name == "" {
			name = item.workspace
		}
		view := ProjectView{
			Name:           name,
			Path:           item.workspace,
			ConversationID: item.conversationID,
			LastActiveAt:   item.lastActiveAt,
			ContextLimit:   contextLimitForModel(""),
		}
		transcriptPath := filepath.Join(agyDir, "brain", item.conversationID, ".system_generated", "logs", "transcript.jsonl")
		enrichFromTranscript(&view, transcriptPath, item.hasLock)
		if item.active {
			view.Active = true
		}
		views = append(views, view)
	}
	return views, nil
}

func readActivePresence(presenceDir string) map[string]bool {
	active := make(map[string]bool)
	entries, err := os.ReadDir(presenceDir)
	if err != nil {
		return active
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".lock") {
			active[strings.TrimSuffix(name, ".lock")] = true
		}
	}
	return active
}

func readRunningFromDB(agyDir string) map[string]bool {
	running := make(map[string]bool)
	dbPath := filepath.Join(agyDir, "conversation_summaries.db")
	info, err := os.Stat(dbPath)
	if err != nil {
		return running
	}
	var walMod, walSize int64
	if walInfo, walErr := os.Stat(dbPath + "-wal"); walErr == nil {
		walMod = walInfo.ModTime().UnixNano()
		walSize = walInfo.Size()
	}
	modTime := info.ModTime().UnixNano()
	size := info.Size()

	dbCacheMu.Lock()
	if cached, ok := dbCache[dbPath]; ok && cached.modTime == modTime && cached.size == size && cached.walModTime == walMod && cached.walSize == walSize {
		for k, v := range cached.running {
			running[k] = v
		}
		dbCacheMu.Unlock()
		return running
	}
	dbCacheMu.Unlock()

	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		return running
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(
		ctx,
		sqliteBin,
		"-readonly",
		dbPath,
		"SELECT conversation_id FROM conversation_summaries WHERE not_fully_idle = 1 OR status = 'CASCADE_RUN_STATUS_RUNNING';",
	).Output()
	if err != nil {
		return running
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		cid := strings.TrimSpace(scanner.Text())
		if cid != "" {
			running[cid] = true
		}
	}
	cachedCopy := make(map[string]bool, len(running))
	for k, v := range running {
		cachedCopy[k] = v
	}
	dbCacheMu.Lock()
	dbCache[dbPath] = dbRunningCache{
		modTime:    modTime,
		size:       size,
		walModTime: walMod,
		walSize:    walSize,
		running:    cachedCopy,
	}
	dbCacheMu.Unlock()
	return running
}

func enrichFromTranscript(view *ProjectView, transcriptPath string, hasPresenceLock bool) {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return
	}
	modTime := info.ModTime().UnixNano()
	size := info.Size()

	cacheMu.Lock()
	cached := transcriptCache[transcriptPath]
	if cached != nil && cached.modTime == modTime && cached.offset == size {
		applyTranscriptSnapshot(view, cached, hasPresenceLock)
		cacheMu.Unlock()
		return
	}
	var snap *transcriptSnapshot
	if cached != nil && size >= cached.offset {
		snap = cloneSnapshot(cached)
	} else {
		snap = &transcriptSnapshot{
			contextLimit: contextLimitForModel(""),
		}
	}
	cacheMu.Unlock()

	f, err := os.Open(transcriptPath)
	if err != nil {
		return
	}
	defer f.Close()

	if snap.offset > 0 {
		if _, err := f.Seek(snap.offset, io.SeekStart); err != nil {
			return
		}
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	consumed := snap.offset
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			consumed += int64(len(line))
			parseTranscriptStep(snap, line)
		}
		if readErr != nil {
			break
		}
	}
	snap.modTime = modTime
	snap.offset = consumed

	cacheMu.Lock()
	transcriptCache[transcriptPath] = cloneSnapshot(snap)
	applyTranscriptSnapshot(view, snap, hasPresenceLock)
	cacheMu.Unlock()
}

func parseTranscriptStep(snap *transcriptSnapshot, line []byte) {
	var step struct {
		Type            string `json:"type"`
		Status          string `json:"status"`
		Model           string `json:"model"`
		InputTokens     int64  `json:"input_tokens"`
		CacheReadTokens int64  `json:"cache_read_tokens"`
		OutputTokens    int64  `json:"output_tokens"`
	}
	if json.Unmarshal(line, &step) != nil {
		return
	}
	snap.lastType = step.Type
	snap.lastStatus = step.Status
	snap.stepCount++
	if step.Model != "" {
		snap.model = step.Model
		snap.contextLimit = contextLimitForModel(step.Model)
	}
	if step.InputTokens > 0 || step.CacheReadTokens > 0 || step.OutputTokens > 0 {
		snap.latestInput = step.InputTokens
		snap.latestCacheRead = step.CacheReadTokens
		snap.latestOutput = step.OutputTokens
		snap.totalInput += step.InputTokens
		snap.totalCacheRead += step.CacheReadTokens
		snap.totalOutput += step.OutputTokens
	}
}

func applyTranscriptSnapshot(view *ProjectView, snap *transcriptSnapshot, hasPresenceLock bool) {
	view.StepCount = snap.stepCount
	if snap.model != "" {
		view.Model = snap.model
	}
	view.ContextLimit = snap.contextLimit
	view.LatestInput = snap.latestInput
	view.LatestCacheRead = snap.latestCacheRead
	view.LatestOutput = snap.latestOutput
	view.TotalInput = snap.totalInput
	view.TotalCacheRead = snap.totalCacheRead
	view.TotalOutput = snap.totalOutput
	view.Active = isModelRunning(hasPresenceLock, snap.lastType, snap.lastStatus)
}

func cloneSnapshot(src *transcriptSnapshot) *transcriptSnapshot {
	return &transcriptSnapshot{
		modTime:         src.modTime,
		offset:          src.offset,
		stepCount:       src.stepCount,
		model:           src.model,
		contextLimit:    src.contextLimit,
		latestInput:     src.latestInput,
		latestCacheRead: src.latestCacheRead,
		latestOutput:    src.latestOutput,
		totalInput:      src.totalInput,
		totalCacheRead:  src.totalCacheRead,
		totalOutput:     src.totalOutput,
		lastType:        src.lastType,
		lastStatus:      src.lastStatus,
	}
}

func isModelRunning(hasPresenceLock bool, lastType, lastStatus string) bool {
	if !hasPresenceLock {
		return false
	}
	status := strings.ToUpper(strings.TrimSpace(lastStatus))
	switch status {
	case "", "DONE", "COMPLETED", "ERROR", "FAILED", "CANCELLED", "CANCELED":
		return strings.EqualFold(strings.TrimSpace(lastType), "USER_INPUT")
	default:
		return true
	}
}

func contextLimitForModel(model string) int64 {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
		return 200_000
	}
	return 1_000_000
}

// DismissProject hides a workspace from ListProjects until new agy activity occurs for it.
func DismissProject(agyDir, workspace string) error {
	if agyDir == "" {
		var err error
		agyDir, err = DefaultAgyDir()
		if err != nil {
			return err
		}
	}
	ws := normalizeWorkspacePath(workspace)
	if ws == "" {
		return nil
	}
	dismissedAt := time.Now().UnixMilli()
	if latest := latestWorkspaceTimestamp(agyDir, ws); latest > dismissedAt {
		dismissedAt = latest
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	dismissed := readDismissedProjects(agyDir)
	dismissed[ws] = dismissedAt
	return writeDismissedProjects(agyDir, dismissed)
}

func normalizeWorkspacePath(workspace string) string {
	trimmed := strings.TrimSpace(workspace)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func latestWorkspaceTimestamp(agyDir, normWorkspace string) int64 {
	f, err := os.Open(filepath.Join(agyDir, "history.jsonl"))
	if err != nil {
		return 0
	}
	defer f.Close()
	var latest int64
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var row struct {
			Workspace string `json:"workspace"`
			Timestamp int64  `json:"timestamp"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		if normalizeWorkspacePath(row.Workspace) == normWorkspace && row.Timestamp > latest {
			latest = row.Timestamp
		}
	}
	return latest
}

func readDismissedProjects(agyDir string) map[string]int64 {
	raw, err := os.ReadFile(filepath.Join(agyDir, dismissedStorageFile))
	if err != nil {
		return make(map[string]int64)
	}
	out := make(map[string]int64)
	if json.Unmarshal(raw, &out) != nil {
		return make(map[string]int64)
	}
	return out
}

func writeDismissedProjects(agyDir string, dismissed map[string]int64) error {
	data, err := json.Marshal(dismissed)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(agyDir, dismissedStorageFile), data, 0o600)
}
