package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"craftstack/internal/master/watcher"
)

const (
	DefaultChunkSize = 256 * 1024
)

// FileInfo contains metadata about a file to be synced.
type FileInfo struct {
	RelPath     string // source directory basis relative path
	AbsPath     string
	Hash        string
	Size        int64
	Dest        string   // agent target path
	MappingName string   // mapping name
	Targets     []string // target agent list
}

// TargetAgent represents an agent that should receive sync updates.
type TargetAgent struct {
	ID      string
	Name    string
	Address string
}

// SyncCallback is called when a file needs to be pushed to agents.
type SyncCallback func(file FileInfo, agents []TargetAgent) error

// Engine manages file synchronization from source directories to agents.
type Engine struct {
	log      *slog.Logger
	w        *watcher.Watcher
	callback SyncCallback

	mu     sync.RWMutex
	agents map[string]*TargetAgent
}

// NewEngine creates a new sync engine.
func NewEngine(w *watcher.Watcher, log *slog.Logger) *Engine {
	return &Engine{
		log:    log,
		w:      w,
		agents: make(map[string]*TargetAgent),
	}
}

// SetCallback sets the function called when a sync operation is needed.
func (e *Engine) SetCallback(cb SyncCallback) {
	e.callback = cb
}

// RegisterAgent adds an agent as a sync target.
func (e *Engine) RegisterAgent(agent *TargetAgent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[agent.ID] = agent
	e.log.Info("sync target register agent", "agent_id", agent.ID, "name", agent.Name)
}

// UnregisterAgent removes an agent from sync targets.
func (e *Engine) UnregisterAgent(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.agents, agentID)
	e.log.Info("sync target agent release", "agent_id", agentID)
}

// Start begins processing file watch events.
func (e *Engine) Start() {
	go e.processEvents()
	e.log.Info("sync engine start")
}

func (e *Engine) processEvents() {
	for event := range e.w.Events() {
		e.handleEvent(event)
	}
}

func (e *Engine) handleEvent(event watcher.FileEvent) {
	// target agent filterring
	e.mu.RLock()
	var filtered []TargetAgent
	for _, a := range e.agents {
		if matchTarget(event.Targets, a.ID, a.Name) {
			filtered = append(filtered, *a)
		}
	}
	e.mu.RUnlock()

	if len(filtered) == 0 {
		e.log.Debug("target agent none, sync skipped", "file", event.RelPath, "mapping", event.MappingName)
		return
	}

	// combine agent target path with file relative path
	destPath := event.RelPath
	if event.Dest != "" && event.Dest != "." {
		destPath = filepath.ToSlash(filepath.Join(event.Dest, event.RelPath))
	}

	if event.EventType == watcher.EventDelete {
		e.log.Info("delete file notification", "mapping", event.MappingName, "file", destPath)
		if e.callback != nil {
			e.callback(FileInfo{RelPath: destPath, MappingName: event.MappingName, Targets: event.Targets}, filtered)
		}
		return
	}

	info, err := computeFileInfo(event.Path, destPath)
	if err != nil {
		e.log.Error("file info calculate failed", "file", event.Path, "error", err)
		return
	}
	info.MappingName = event.MappingName
	info.Dest = event.Dest
	info.Targets = event.Targets

	e.log.Info("file sync before",
		"mapping", event.MappingName,
		"file", info.RelPath,
		"size", info.Size,
		"agents", len(filtered),
	)

	if e.callback != nil {
		if err := e.callback(*info, filtered); err != nil {
			e.log.Error("sync callback failed", "file", info.RelPath, "error", err)
		}
	}
}

// matchTarget checks if an agent matches the target list.
func matchTarget(targets []string, agentID, agentName string) bool {
	for _, t := range targets {
		if t == "*" || t == agentID || t == agentName {
			return true
		}
	}
	return false
}

func computeFileInfo(absPath, relPath string) (*FileInfo, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("hash file: %w", err)
	}

	return &FileInfo{
		RelPath: relPath,
		AbsPath: absPath,
		Hash:    hex.EncodeToString(h.Sum(nil)),
		Size:    stat.Size(),
	}, nil
}

// ReadFileChunks reads a file and returns it as chunks.
func ReadFileChunks(path string, chunkSize int) ([][]byte, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var chunks [][]byte
	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			chunks = append(chunks, chunk)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
	}

	return chunks, nil
}
