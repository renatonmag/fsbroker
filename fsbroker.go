package fsbroker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FSBroker collects fsnotify events, groups them, dedupes them, and processes them as a single event.
type FSBroker struct {
	watcher        *fsnotify.Watcher
	watched        map[string]bool
	watchermu      sync.Mutex
	watchrecursive bool          // watch recursively on directories, set by AddRecursiveWatch
	events         chan *FSEvent // internal events channel, processes FSevent for every FSNotify Op
	emitch         chan *FSEvent // emitted events channel, sends FSevent to the user after deduplication, grouping, and processing
	errors         chan error
	quit           chan struct{}
	timeout        time.Duration
	filtersys      bool
	Filter         func(*FSEvent) bool
}

// NewFSBroker creates a new FSBroker instance.
// timeout is the duration to wait for events to be grouped and processed.
// ignoreSysFiles will ignore common system files and directories.
func NewFSBroker(timeout time.Duration, ignoreSysFiles bool) (*FSBroker, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FSBroker{
		watcher:        watcher,
		watched:        make(map[string]bool),
		watchrecursive: false,
		events:         make(chan *FSEvent, 100),
		emitch:         make(chan *FSEvent, 0),
		errors:         make(chan error),
		quit:           make(chan struct{}),
		timeout:        timeout,
		filtersys:      ignoreSysFiles,
	}, nil
}

// Start starts the broker, listening for events and processing them.
func (b *FSBroker) Start() {
	go b.eventloop()

	go func() {
		for {
			select {
			case event := <-b.watcher.Events:
				b.addEvent(event.Op, event.Name)
			case err := <-b.watcher.Errors:
				b.errors <- err
			}
		}
	}()
}

// Next returns the channel to receive events.
func (b *FSBroker) Next() <-chan *FSEvent {
	return b.emitch
}

// Error returns the channel to receive errors.
func (b *FSBroker) Error() <-chan error {
	return b.errors
}

// AddRecursiveWatch adds a watch on a directory and all its subdirectories.
// It will also add watches on all new directories created within the directory.
// Note: If this is called at least once, all newly created directories will be watched automatically, even if they were added using AddWatch and not using AddRecursiveWatch.
func (b *FSBroker) AddRecursiveWatch(path string) error {
	b.watchrecursive = true // enable recursive watch
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := b.AddWatch(p); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// AddWatch adds a watch on a file or directory.
func (b *FSBroker) AddWatch(path string) error {
	b.watchermu.Lock()
	defer b.watchermu.Unlock()

	if b.watched[path] {
		return nil
	}

	if err := b.watcher.Add(path); err != nil {
		return err
	}
	b.watched[path] = true
	return nil
}

// RemoveWatch removes a watch on a file or directory.
func (b *FSBroker) RemoveWatch(path string) {
	b.watchermu.Lock()
	defer b.watchermu.Unlock()

	if !b.watched[path] {
		return
	}

	b.watcher.Remove(path)
	delete(b.watched, path)
}

// eventloop starts the broker, grouping and interpreting events as a single action.
func (b *FSBroker) eventloop() {
	eventQueue := make([]*FSEvent, 0) // queue of events to be processed, gets cleared every tick

	ticker := time.NewTicker(b.timeout)
	defer ticker.Stop()

	for {
		select {
		case event := <-b.events:
			// Add the event to the queue for grouping
			// fmt.Println("Adding event to queue", event)
			eventQueue = append(eventQueue, event)

		case <-ticker.C:
			// Process grouped events, detecting related Create and Rename events
			processedPaths := make(map[string]bool)

			for _, action := range eventQueue {
				// Ignore already processed paths
				if processedPaths[action.Signature()] {
					continue
				}

				switch action.Type {
				case Remove:
					// Check if there's any preceeding event for the same path within the queue, ignore it and only raise the remove event
					for _, relatedAction := range eventQueue {
						if relatedAction.Path == action.Path && relatedAction.Timestamp.Before(action.Timestamp) {
							processedPaths[relatedAction.Signature()] = true
						}
					}
					// If a directory is removed, remove the watch
					b.RemoveWatch(action.Path)
					// Process the Remove event normally
					b.handleEvent(action)
					processedPaths[action.Signature()] = true
				case Create:
					// If a directory is created, add a watch
					if b.watchrecursive {
						info, err := os.Stat(action.Path)
						if err == nil && info.IsDir() {
							b.AddWatch(action.Path)
						}
					}
					// Check if there's a Rename event for the same path within the queue
					isRename := false
					for _, relatedrename := range eventQueue {
						if filepath.Dir(relatedrename.Path) == filepath.Dir(action.Path) && relatedrename.Type == Rename {
							result := NewFSEvent(Rename, action.Path, action.Timestamp)
							result.Properties["OldPath"] = relatedrename.Path
							b.handleEvent(result)
							processedPaths[action.Signature()] = true
							processedPaths[relatedrename.Signature()] = true
							isRename = true
							break
						}
					}
					if !isRename {
						// Process the Create event normally
						b.handleEvent(action)
						processedPaths[action.Signature()] = true
					}
				case Modify:
					// Check if there are multiple save events for the same path within the queue, treat them as a single Modify event
					latestModify := action
					for _, relatedModify := range eventQueue {
						if relatedModify.Path == action.Path && relatedModify.Type == Modify && relatedModify.Timestamp.After(latestModify.Timestamp) {
							latestModify = relatedModify
						}
						processedPaths[relatedModify.Signature()] = true
					}
					// Process the latest Modify event
					b.handleEvent(latestModify)
					processedPaths[latestModify.Signature()] = true
				default:
					// Ignore other event types
				}
			}
			eventQueue = []*FSEvent{} // Clear the event queue after processing

		case <-b.quit:
			return
		}
	}
}

// Stop stops the broker.
func (b *FSBroker) Stop() {
	close(b.quit)
	b.watcher.Close()
}

// AddEvent queues a new file system event into the broker.
func (b *FSBroker) addEvent(op fsnotify.Op, name string) {
	if b.filtersys {
		switch runtime.GOOS {
		case "linux":
			if isLinuxSystemFile(name) {
				return
			}
		case "windows":
			if isWindowsSystemFile(name) {
				return
			}
		case "darwin":
			if isMacSystemFile(name) {
				return
			}
		}
	}

	eventType := mapOpToEventType(op)
	event := NewFSEvent(eventType, name, time.Now())

	if b.Filter != nil && b.Filter(event) {
		return
	}

	b.events <- event
}

// isLinuxSystemFile checks if the file is a common Linux system or temporary file.
func isLinuxSystemFile(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	switch base {
	case ".bash_history", ".bash_logout", ".bash_profile", ".bashrc", ".profile",
		".login", ".sudo_as_admin_successful", ".xauthority", ".xsession-errors",
		".viminfo", ".cache", ".config", ".local", ".dbus", ".gvfs",
		".recently-used", ".fontconfig", ".iceauthority":
		return true
	}

	// Patterns for temporary GNOME/GTK files and trash directories
	return strings.HasPrefix(base, ".goutputstream-") ||
		strings.HasPrefix(base, ".trash-") ||
		base == "snap" || base == ".flatpak"
}

// isWindowsSystemFile checks if the file is a common Windows system or metadata file.
func isWindowsSystemFile(name string) bool {
	// Base filenames commonly generated by Windows
	base := strings.ToLower(filepath.Base(name))
	switch base {
	case "desktop.ini", "thumbs.db", "$recycle.bin", "system volume information":
		return true
	}

	return false
}

// isMacSystemFile checks if the file is a common macOS system or metadata file.
func isMacSystemFile(name string) bool {
	// Base filenames commonly generated by macOS
	base := strings.ToLower(filepath.Base(name))
	switch base {
	case ".ds_store", ".appledouble", ".spotlight-v100", ".temporaryitems",
		".trashes", ".fseventsd", ".volumeicon.icns", "icon\r",
		".documentrevisions-v100", ".pkinstallsandboxmanager", ".apdisk":
		return true
	}

	// Match patterns for resource fork or metadata files
	return strings.HasPrefix(base, "._") || base == ".com.apple.timemachine.donotpresent"
}

// mapOpToEventType maps fsnotify.Op to EventType.
func mapOpToEventType(op fsnotify.Op) EventType {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return Create
	case op&fsnotify.Write == fsnotify.Write:
		return Modify
	case op&fsnotify.Rename == fsnotify.Rename:
		return Rename
	case op&fsnotify.Remove == fsnotify.Remove:
		return Remove
	default:
		return -1 // Unknown event
	}
}

// handleEvent sends the event to the user after deduplication, grouping, and processing.
func (b *FSBroker) handleEvent(event *FSEvent) {
	b.emitch <- event
}
