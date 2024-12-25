package fsbroker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewFSBroker ensures the broker initializes correctly.
func TestNewFSBroker(t *testing.T) {
  config := DefaultFSConfig()
	broker, err := NewFSBroker(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer broker.Stop()
}

// TestAddRecursiveWatch verifies recursive watch on directories.
func TestAddRecursiveWatch(t *testing.T) {
	// Create a temp directory and subdirectory to watch
	rootDir := t.TempDir()
	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
  config := DefaultFSConfig()
	broker, err := NewFSBroker(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer broker.Stop()

	// Add recursive watch
	if err := broker.AddRecursiveWatch(rootDir); err != nil {
		t.Fatalf("Failed to add recursive watch: %v", err)
	}

	if _, ok := broker.watched[rootDir]; !ok {
		t.Errorf("Expected root directory to be watched, but it's not")
	}
	if _, ok := broker.watched[subDir]; !ok {
		t.Errorf("Expected sub directory to be watched, but it's not")
	}
}

// TestFilterSysFiles verifies filtering system files based on OS type.
func TestFilterSysFiles(t *testing.T) {
  config := DefaultFSConfig()
	broker, err := NewFSBroker(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer broker.Stop()

	// Define a filter function that only allows Create events.
	broker.Filter = func(event *FSEvent) bool {
		return event.Type != Create
	}

	// Test filtering non-Create events
	event := NewFSEvent(Remove, "/some/path", time.Now())
	broker.events <- event

	select {
	case <-broker.Next():
		t.Error("Expected event to be filtered, but it was not")
	case <-time.After(100 * time.Millisecond):
		// Success if we get here without any event being passed on
	}
}

// TestEventLoop verifies event deduplication and handling
func TestEventLoop(t *testing.T) {
  config := DefaultFSConfig()
	broker, err := NewFSBroker(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer broker.Stop()

	// Inject events directly into the broker for testing
	go broker.eventloop()

	event1 := NewFSEvent(Create, "/test/path", time.Now())
	event2 := NewFSEvent(Modify, "/test/path", time.Now().Add(100*time.Millisecond))
	event3 := NewFSEvent(Remove, "/test/path", time.Now().Add(200*time.Millisecond))

	broker.events <- event1
	broker.events <- event2
	broker.events <- event3

	// Receive deduplicated events from emitted channel
	select {
	case result := <-broker.Next():
		if result.Type != Create {
			t.Errorf("Expected Create event, got %v", result.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timed out waiting for event deduplication")
	}
}
