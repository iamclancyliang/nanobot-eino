package cron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCronService_AtJobRunsOnceAndAutoDeleted(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "cron-store.json")
	done := make(chan struct{}, 1)

	svc := NewCronService(storePath, func(ctx context.Context, job *CronJob) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer svc.Stop()

	_, err := svc.AddJob(
		"one-shot",
		CronSchedule{
			Kind: JobKindAt,
			AtMs: time.Now().Add(150 * time.Millisecond).UnixMilli(),
		},
		CronPayload{Kind: "test", Message: "hello"},
		true,
	)
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("at job did not execute in time")
	}

	// Give saveStore() a brief moment to flush.
	time.Sleep(100 * time.Millisecond)

	if got := svc.ListJobs(); len(got) != 0 {
		t.Fatalf("expected no jobs after deleteAfterRun execution, got %d", len(got))
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("failed to read store file: %v", err)
	}
	var store CronStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("failed to unmarshal store: %v", err)
	}
	if len(store.Jobs) != 0 {
		t.Fatalf("expected persisted store to have 0 jobs, got %d", len(store.Jobs))
	}
}

func TestCronService_RemoveJobCancelsEverySchedule(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "cron-store.json")
	var runs int32

	svc := NewCronService(storePath, func(ctx context.Context, job *CronJob) error {
		atomic.AddInt32(&runs, 1)
		return nil
	})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer svc.Stop()

	job, err := svc.AddJob(
		"every-job",
		CronSchedule{
			Kind:    JobKindEvery,
			EveryMs: 300,
		},
		CronPayload{Kind: "test", Message: "loop"},
		false,
	)
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	if ok := svc.RemoveJob(job.ID); !ok {
		t.Fatalf("RemoveJob returned false for existing job %s", job.ID)
	}

	time.Sleep(650 * time.Millisecond)
	if got := atomic.LoadInt32(&runs); got != 0 {
		t.Fatalf("expected no runs after immediate remove, got %d", got)
	}
}

func TestCronService_RemoveJobCancelsAtTimer(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "cron-store.json")
	var runs int32

	svc := NewCronService(storePath, func(ctx context.Context, job *CronJob) error {
		atomic.AddInt32(&runs, 1)
		return nil
	})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer svc.Stop()

	job, err := svc.AddJob(
		"at-job",
		CronSchedule{
			Kind: JobKindAt,
			AtMs: time.Now().Add(400 * time.Millisecond).UnixMilli(),
		},
		CronPayload{Kind: "test", Message: "one-shot"},
		true,
	)
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	if ok := svc.RemoveJob(job.ID); !ok {
		t.Fatalf("RemoveJob returned false for existing job %s", job.ID)
	}

	time.Sleep(700 * time.Millisecond)
	if got := atomic.LoadInt32(&runs); got != 0 {
		t.Fatalf("expected at timer to be cancelled, got %d runs", got)
	}
}
