package subagent

import (
	"context"
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/bus"
)

func TestTaskSet_AddRemove(t *testing.T) {
	ts := &taskSet{tasks: make(map[string]bool)}

	ts.mu.Lock()
	ts.tasks["task-1"] = true
	ts.tasks["task-2"] = true
	ts.mu.Unlock()

	ts.mu.Lock()
	if len(ts.tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(ts.tasks))
	}
	delete(ts.tasks, "task-1")
	ts.mu.Unlock()

	ts.mu.Lock()
	if len(ts.tasks) != 1 {
		t.Errorf("expected 1 task after remove, got %d", len(ts.tasks))
	}
	ts.mu.Unlock()
}

func TestSubagentManager_NotifyCompletion(t *testing.T) {
	mb := bus.NewMessageBus()
	mgr := &SubagentManager{bus: mb}

	mgr.notifyCompletion("feishu", "chat1", "session-key", "test-label", "do something", "Task done", "ok")

	select {
	case msg := <-mb.ConsumeInbound(context.Background()):
		if msg.Channel != "system" {
			t.Errorf("Channel = %q, want %q", msg.Channel, "system")
		}
		if msg.ChatID != "feishu:chat1" {
			t.Errorf("ChatID = %q, want %q", msg.ChatID, "feishu:chat1")
		}
		if msg.SessionKeyOverride != "session-key" {
			t.Errorf("SessionKeyOverride = %q, want %q", msg.SessionKeyOverride, "session-key")
		}
		if msg.Metadata["type"] != "subagent_result" {
			t.Errorf("metadata type = %v, want subagent_result", msg.Metadata["type"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestSubagentManager_AddRemoveSessionTask(t *testing.T) {
	mgr := &SubagentManager{}

	mgr.addSessionTask("session-1", "task-a")
	mgr.addSessionTask("session-1", "task-b")
	mgr.addSessionTask("session-2", "task-c")

	tsIface, ok := mgr.sessionTasks.Load("session-1")
	if !ok {
		t.Fatal("session-1 tasks not found")
	}
	ts := tsIface.(*taskSet)
	ts.mu.Lock()
	if len(ts.tasks) != 2 {
		t.Errorf("session-1 should have 2 tasks, got %d", len(ts.tasks))
	}
	ts.mu.Unlock()

	mgr.removeSessionTask("session-1", "task-a")

	ts.mu.Lock()
	if len(ts.tasks) != 1 {
		t.Errorf("session-1 should have 1 task after remove, got %d", len(ts.tasks))
	}
	ts.mu.Unlock()
}

func TestSubagentManager_RemoveSessionTask_NonexistentSession(t *testing.T) {
	mgr := &SubagentManager{}
	mgr.removeSessionTask("nonexistent", "task-1")
}

func TestSubagentManager_CancelBySession_NoTasks(t *testing.T) {
	mgr := &SubagentManager{}
	cancelled := mgr.CancelBySession("no-session")
	if cancelled != 0 {
		t.Errorf("expected 0 cancelled, got %d", cancelled)
	}
}

func TestSubagentManager_CancelBySession_WithTasks(t *testing.T) {
	mgr := &SubagentManager{}

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	_ = ctx1
	_ = ctx2

	mgr.runningTasks.Store("task-1", cancel1)
	mgr.runningTasks.Store("task-2", cancel2)
	mgr.addSessionTask("session-x", "task-1")
	mgr.addSessionTask("session-x", "task-2")

	cancelled := mgr.CancelBySession("session-x")
	if cancelled != 2 {
		t.Errorf("expected 2 cancelled, got %d", cancelled)
	}

	if ctx1.Err() != context.Canceled {
		t.Error("task-1 context should be cancelled")
	}
	if ctx2.Err() != context.Canceled {
		t.Error("task-2 context should be cancelled")
	}
}

func TestSubagentManager_DefaultMaxStep(t *testing.T) {
	if defaultMaxStep != 15 {
		t.Errorf("defaultMaxStep = %d, want 15", defaultMaxStep)
	}
}
