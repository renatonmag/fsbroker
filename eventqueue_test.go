package fsbroker

import (
	"sync"
	"testing"
	"time"
)

func TestEventQueue(t *testing.T) {
	t.Run("Push and Pop single event", func(t *testing.T) {
		eq := NewEventQueue()
		event := NewFSEvent(Create, "/test/path", time.Now())
		eq.Push(event)

		popped := eq.Pop()
		if popped != event {
			t.Errorf("Expected event %v, got %v", event, popped)
		}
		if len(eq.List()) != 0 {
			t.Errorf("Expected queue to be empty, got %d events", len(eq.List()))
		}
	})

	t.Run("Push and Pop multiple events", func(t *testing.T) {
		eq := NewEventQueue()
		event1 := NewFSEvent(Create, "/test/path", time.Now())
		event2 := NewFSEvent(Remove, "/test/path", time.Now())

		eq.Push(event1)
		eq.Push(event2)

		if len(eq.List()) != 2 {
			t.Errorf("Expected queue length 2, got %d", len(eq.List()))
		}

		popped1 := eq.Pop()
		popped2 := eq.Pop()

		if popped1 != event1 || popped2 != event2 {
			t.Errorf("Events popped in wrong order: %v, %v", popped1, popped2)
		}
		if len(eq.List()) != 0 {
			t.Errorf("Expected queue to be empty after popping all events")
		}
	})

	t.Run("Pop from empty queue", func(t *testing.T) {
		eq := NewEventQueue()
		popped := eq.Pop()
		if popped != nil {
			t.Errorf("Expected nil when popping from empty queue, got %v", popped)
		}
	})

	t.Run("Concurrent Push and List", func(t *testing.T) {
		eq := NewEventQueue()
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				eq.Push(NewFSEvent(Create, string(rune(i)), time.Now()))
			}(i)
		}

		wg.Wait()
		if len(eq.List()) != 100 {
			t.Errorf("Expected 100 events in queue, got %d", len(eq.List()))
		}
	})
}
