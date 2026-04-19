package watcher

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"craftstack/internal/common"
)

// EventType represents the type of file system change.
type EventType string

const (
	EventCreate EventType = "create"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)

// FileEvent represents a debounced file system change event.
type FileEvent struct {
	Path        string // absolute path
	RelPath     string // source directory basis relative path
	EventType   EventType
	Timestamp   time.Time
	MappingName string   // generated from some mapping
	Dest        string   // agent target path
	Targets     []string // target agent list
}

// Watcher monitors multiple directories for file changes and emits debounced events.
type Watcher struct {
	debounce  time.Duration
	log       *slog.Logger
	fsWatcher *fsnotify.Watcher

	eventCh chan FileEvent
	stopCh  chan struct{}

	mu       sync.Mutex
	pending  map[string]*pendingEvent
	mappings []common.SyncMapping
	// srcMap: absolute path -> mapping index (some mapping source in reverse)
	srcMap map[string]int
}

type pendingEvent struct {
	event FileEvent
	timer *time.Timer
}

// New creates a new file system watcher.
func New(debounceMs int, log *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		debounce:  time.Duration(debounceMs) * time.Millisecond,
		log:       log,
		fsWatcher: fsw,
		eventCh:   make(chan FileEvent, 100),
		stopCh:    make(chan struct{}),
		pending:   make(map[string]*pendingEvent),
		srcMap:    make(map[string]int),
	}

	return w, nil
}

// Events returns the channel of debounced file events.
func (w *Watcher) Events() <-chan FileEvent {
	return w.eventCh
}

// Start begins the event processing loop (does not add any watches yet).
func (w *Watcher) Start() {
	go w.loop()
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.fsWatcher.Close()
}

// Mappings returns the current sync mappings.
func (w *Watcher) Mappings() []common.SyncMapping {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]common.SyncMapping, len(w.mappings))
	copy(cp, w.mappings)
	return cp
}

// LoadMappings sets the mappings and starts watching their source directories.
// Call this on startup and whenever mappings change in the DB.
func (w *Watcher) LoadMappings(mappings []common.SyncMapping) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// existing watch all release
	for absDir := range w.srcMap {
		// fsnotify.Remove without path ignore, so notbefore
		w.fsWatcher.Remove(absDir)
	}
	// existing watch in progress all path release (sub directory include)
	for _, p := range w.fsWatcher.WatchList() {
		w.fsWatcher.Remove(p)
	}

	w.mappings = mappings
	w.srcMap = make(map[string]int)

	for i, m := range w.mappings {
		absDir, err := filepath.Abs(m.Src)
		if err != nil {
			w.log.Error("mapping source path parse failed", "src", m.Src, "error", err)
			continue
		}
		w.srcMap[absDir] = i

		// source directory create
		if err := os.MkdirAll(absDir, 0755); err != nil {
			w.log.Error("mapping source directory create failed", "src", absDir, "error", err)
			continue
		}

		// recursively register directory
		err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				w.log.Debug("directory watch register", "path", path, "mapping", m.Name)
				return w.fsWatcher.Add(path)
			}
			return nil
		})
		if err != nil {
			w.log.Error("directory watch register failed", "src", absDir, "error", err)
			continue
		}

		w.log.Info("sync mapping watch start", "name", m.Name, "src", absDir, "dest", m.Dest, "targets", m.Targets)
	}

	return nil
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.log.Error("watch error", "error", err)
		}
	}
}

// resolveMapping finds which mapping a file path belongs to.
func (w *Watcher) resolveMapping(path string) (int, string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return -1, "", err
	}

	for absDir, idx := range w.srcMap {
		rel, err := filepath.Rel(absDir, absPath)
		if err != nil {
			continue
		}
		if len(rel) >= 2 && rel[:2] == ".." {
			continue
		}
		return idx, rel, nil
	}
	return -1, "", fmt.Errorf("mapping not found: %s", path)
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	w.mu.Lock()
	idx, relPath, err := w.resolveMapping(event.Name)
	if err != nil {
		w.mu.Unlock()
		return
	}
	mapping := w.mappings[idx]
	w.mu.Unlock()

	// Exclude pattern check
	for _, pattern := range mapping.Exclude {
		matched, _ := filepath.Match(pattern, relPath)
		if matched {
			return
		}
		matched, _ = filepath.Match(pattern, filepath.Base(relPath))
		if matched {
			return
		}
	}

	var eventType EventType
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		eventType = EventCreate
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			w.fsWatcher.Add(event.Name)
			return
		}
	case event.Op&fsnotify.Write == fsnotify.Write:
		eventType = EventUpdate
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		eventType = EventDelete
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		eventType = EventDelete
	default:
		return
	}

	fe := FileEvent{
		Path:        event.Name,
		RelPath:     filepath.ToSlash(relPath),
		EventType:   eventType,
		Timestamp:   time.Now(),
		MappingName: mapping.Name,
		Dest:        mapping.Dest,
		Targets:     mapping.Targets,
	}

	w.mu.Lock()
	if pe, exists := w.pending[event.Name]; exists {
		pe.timer.Stop()
		pe.event = fe
		pe.timer = time.AfterFunc(w.debounce, func() { w.emit(event.Name) })
	} else {
		w.pending[event.Name] = &pendingEvent{
			event: fe,
			timer: time.AfterFunc(w.debounce, func() { w.emit(event.Name) }),
		}
	}
	w.mu.Unlock()
}

func (w *Watcher) emit(path string) {
	w.mu.Lock()
	pe, exists := w.pending[path]
	if !exists {
		w.mu.Unlock()
		return
	}
	delete(w.pending, path)
	w.mu.Unlock()

	w.log.Info("file event",
		"type", pe.event.EventType,
		"mapping", pe.event.MappingName,
		"path", pe.event.RelPath,
		"dest", pe.event.Dest,
		"targets", pe.event.Targets,
	)
	w.eventCh <- pe.event
}
