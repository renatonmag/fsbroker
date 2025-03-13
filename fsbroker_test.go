package fsbroker

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestIsSystemFile tests the isSystemFile function for the current platform.
func TestIsSystemFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Common test cases for all platforms
		{"Regular file", "/path/to/regular.txt", false},
		{"Regular directory", "/path/to/regular", false},
	}

	// Platform-specific test cases
	switch runtime.GOOS {
	case "windows":
		windowsTests := []struct {
			name     string
			path     string
			expected bool
		}{
			{"Desktop.ini", "C:\\Users\\test\\Desktop.ini", true},
			{"Thumbs.db", "C:\\Users\\test\\Thumbs.db", true},
			{"Recycle Bin", "C:\\$Recycle.bin", true},
			{"System Volume Information", "D:\\System Volume Information", true},
		}
		for _, tt := range windowsTests {
			tests = append(tests, struct {
				name     string
				path     string
				expected bool
			}{tt.name, tt.path, tt.expected})
		}
	case "darwin":
		macTests := []struct {
			name     string
			path     string
			expected bool
		}{
			{".DS_Store", "/Users/test/.DS_Store", true},
			{".Trashes", "/Users/test/.Trashes", true},
			{"._ResourceFork", "/Users/test/._Document", true},
			{".AppleDouble", "/Users/test/.AppleDouble", true},
		}
		for _, tt := range macTests {
			tests = append(tests, struct {
				name     string
				path     string
				expected bool
			}{tt.name, tt.path, tt.expected})
		}
	case "linux":
		linuxTests := []struct {
			name     string
			path     string
			expected bool
		}{
			{".bash_history", "/home/user/.bash_history", true},
			{".config", "/home/user/.config", true},
			{".goutputstream", "/tmp/.goutputstream-XYZ", true},
			{".trash", "/home/user/.trash-1000", true},
		}
		for _, tt := range linuxTests {
			tests = append(tests, struct {
				name     string
				path     string
				expected bool
			}{tt.name, tt.path, tt.expected})
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemFile(tt.path)
			if result != tt.expected {
				t.Errorf("isSystemFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

// TestIsHiddenFile tests the isHiddenFile function for the current platform.
func TestIsHiddenFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a hidden file (platform-specific)
	var hiddenFilePath string
	var regularFilePath string

	switch runtime.GOOS {
	case "windows":
		// On Windows, we can't easily create a hidden file in tests
		// So we'll just test the function with known paths
		hiddenFilePath = filepath.Join(tempDir, "hidden.txt")
		regularFilePath = filepath.Join(tempDir, "regular.txt")

		// Create the files
		if err := os.WriteFile(hiddenFilePath, []byte("hidden"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := os.WriteFile(regularFilePath, []byte("regular"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Note: On Windows, we can't set the hidden attribute in a cross-platform way
		// So we'll just check that the function doesn't error
		_, err := isHiddenFile(hiddenFilePath)
		if err != nil {
			t.Errorf("isHiddenFile(%q) returned error: %v", hiddenFilePath, err)
		}

		_, err = isHiddenFile(regularFilePath)
		if err != nil {
			t.Errorf("isHiddenFile(%q) returned error: %v", regularFilePath, err)
		}
	default:
		// On Unix-like systems, hidden files start with a dot
		hiddenFilePath = filepath.Join(tempDir, ".hidden")
		regularFilePath = filepath.Join(tempDir, "regular")

		// Create the files
		if err := os.WriteFile(hiddenFilePath, []byte("hidden"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := os.WriteFile(regularFilePath, []byte("regular"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Test hidden file
		hidden, err := isHiddenFile(hiddenFilePath)
		if err != nil {
			t.Errorf("isHiddenFile(%q) returned error: %v", hiddenFilePath, err)
		}
		if !hidden {
			t.Errorf("isHiddenFile(%q) = %v, want true", hiddenFilePath, hidden)
		}

		// Test regular file
		hidden, err = isHiddenFile(regularFilePath)
		if err != nil {
			t.Errorf("isHiddenFile(%q) returned error: %v", regularFilePath, err)
		}
		if hidden {
			t.Errorf("isHiddenFile(%q) = %v, want false", regularFilePath, hidden)
		}
	}
}
