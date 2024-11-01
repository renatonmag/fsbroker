package fsbroker

import (
	"testing"
	"time"
)

func TestNewFSEvent(t *testing.T) {
	path := "/test/path"
	event := NewFSEvent(Create, path, time.Now())

	if event.Type != Create {
		t.Errorf("Expected event type Create, got %v", event.Type)
	}
	if event.Path != path {
		t.Errorf("Expected path %s, got %s", path, event.Path)
	}
	if event.Properties == nil {
		t.Errorf("Expected Properties map to be initialized, got nil")
	}
}

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{Create, "Create"},
		{Modify, "Modify"},
		{Rename, "Rename"},
		{Remove, "Remove"},
	}

	for _, tt := range tests {
		if tt.eventType.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.eventType.String())
		}
	}
}

func TestFSEventSignature(t *testing.T) {
	event := NewFSEvent(Create, "/test/path", time.Now())
	expectedSignature := "0-/test/path"
	if event.Signature() != expectedSignature {
		t.Errorf("Expected signature %s, got %s", expectedSignature, event.Signature())
	}
}
