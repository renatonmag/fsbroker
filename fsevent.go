package fsbroker

import (
	"fmt"
	"time"
)

type EventType int

const (
	Create EventType = iota
	Modify
	Rename
	Remove
)

func (t *EventType) String() string {
	switch *t {
	case Create:
		return "Create"
	case Modify:
		return "Modify"
	case Rename:
		return "Rename"
	case Remove:
		return "Remove"
	default:
		return "Unknown"
	}
}

type FSEvent struct {
	Type       EventType
	Path       string
	Timestamp  time.Time
	Properties map[string]string
}

func NewFSEvent(op EventType, path string, timestamp time.Time) *FSEvent {
	return &FSEvent{
		Type:       op,
		Path:       path,
		Timestamp:  timestamp,
		Properties: make(map[string]string),
	}
}

func (action *FSEvent) Signature() string {
	return fmt.Sprintf("%d-%s", action.Type, action.Path)
}
