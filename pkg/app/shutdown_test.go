package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"
)

type fakeCancelAller struct {
	called int
	ret    int
}

func (f *fakeCancelAller) CancelAll() int {
	f.called++
	return f.ret
}

type fakeFeishuStopper struct {
	called int
}

func (f *fakeFeishuStopper) Stop(ctx context.Context) error {
	f.called++
	return nil
}

type fakeHeartbeatStopper struct {
	called int
}

func (f *fakeHeartbeatStopper) Stop() {
	f.called++
}

type fakeCronStopper struct {
	called int
}

func (f *fakeCronStopper) Stop() {
	f.called++
}

func TestStopRuntimeComponents_CallsInOrder(t *testing.T) {
	feishu := &fakeFeishuStopper{}
	heartbeat := &fakeHeartbeatStopper{}
	cron := &fakeCronStopper{}
	mcpClosed := 0

	StopRuntimeComponents(RuntimeComponents{
		Feishu:               feishu,
		Heartbeat:            heartbeat,
		Cron:                 cron,
		ComponentStopTimeout: time.Second,
		CloseMCP: func() int {
			mcpClosed++
			return 1
		},
	})

	if feishu.called != 1 || heartbeat.called != 1 || cron.called != 1 || mcpClosed != 1 {
		t.Fatalf("unexpected stop calls: feishu=%d heartbeat=%d cron=%d mcp=%d", feishu.called, heartbeat.called, cron.called, mcpClosed)
	}
}

func TestStartGracefulShutdown_InvokesAllSteps(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	agent := &fakeCancelAller{ret: 2}
	sub := &fakeCancelAller{ret: 3}
	feishu := &fakeFeishuStopper{}
	heartbeat := &fakeHeartbeatStopper{}
	cron := &fakeCronStopper{}

	cancelCalled := 0
	closeCalled := 0
	var wg sync.WaitGroup

	done := StartGracefulShutdown(GracefulShutdownConfig{
		SigCh: sigCh,
		CancelRoot: func() {
			cancelCalled++
		},
		CancelAgentTasks:   agent,
		CancelSubagentTask: sub,
		CloseInbound: func() {
			closeCalled++
		},
		WaitGroup:       &wg,
		ShutdownTimeout: 100 * time.Millisecond,
		Components: RuntimeComponents{
			Feishu:               feishu,
			Heartbeat:            heartbeat,
			Cron:                 cron,
			ComponentStopTimeout: time.Second,
			CloseMCP: func() int {
				return 1
			},
		},
	})

	sigCh <- syscall.SIGTERM
	<-done

	if cancelCalled != 1 || closeCalled != 1 {
		t.Fatalf("unexpected cancel/close calls: cancel=%d close=%d", cancelCalled, closeCalled)
	}
	if agent.called != 1 || sub.called != 1 {
		t.Fatalf("unexpected cancelAll calls: agent=%d sub=%d", agent.called, sub.called)
	}
	if feishu.called != 1 || heartbeat.called != 1 || cron.called != 1 {
		t.Fatalf("unexpected component stop calls: feishu=%d heartbeat=%d cron=%d", feishu.called, heartbeat.called, cron.called)
	}
}

func TestStopRuntimeComponents_NilFeishuNoPanic(t *testing.T) {
	// Typed nil: *fakeFeishuStopper(nil) assigned to feishuStopper interface.
	var feishu *fakeFeishuStopper // nil pointer
	StopRuntimeComponents(RuntimeComponents{
		Feishu:               feishu, // typed nil — must not panic
		ComponentStopTimeout: time.Second,
	})
}

func TestStopRuntimeComponents_InterfaceNilNoPanic(t *testing.T) {
	// Pure nil interface.
	StopRuntimeComponents(RuntimeComponents{
		ComponentStopTimeout: time.Second,
	})
}

func TestNewSignalChannel_Buffered(t *testing.T) {
	sigCh := NewSignalChannel()
	defer signal.Stop(sigCh)

	if sigCh == nil {
		t.Fatal("expected non-nil signal channel")
	}
	if cap(sigCh) != 1 {
		t.Fatalf("expected signal channel cap=1, got %d", cap(sigCh))
	}
}
