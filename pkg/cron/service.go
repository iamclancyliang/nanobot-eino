package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

var logCron = slog.With("module", "cron")

type JobKind string

const (
	JobKindAt    JobKind = "at"
	JobKindEvery JobKind = "every"
	JobKindCron  JobKind = "cron"
)

type CronSchedule struct {
	Kind    JobKind `json:"kind"`
	AtMs    int64   `json:"atMs,omitempty"`
	EveryMs int64   `json:"everyMs,omitempty"`
	Expr    string  `json:"expr,omitempty"`
	TZ      string  `json:"tz,omitempty"`
}

type CronPayload struct {
	Kind    string `json:"kind"` // e.g., "agent_turn"
	Message string `json:"message"`
	Deliver bool   `json:"deliver"`
	Channel string `json:"channel,omitempty"`
	To      string `json:"to,omitempty"`
}

type CronJobState struct {
	NextRunAtMs int64  `json:"nextRunAtMs,omitempty"`
	LastRunAtMs int64  `json:"lastRunAtMs,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

type CronJob struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Enabled        bool         `json:"enabled"`
	Schedule       CronSchedule `json:"schedule"`
	Payload        CronPayload  `json:"payload"`
	State          CronJobState `json:"state"`
	CreatedAtMs    int64        `json:"createdAtMs"`
	UpdatedAtMs    int64        `json:"updatedAtMs"`
	DeleteAfterRun bool         `json:"deleteAfterRun"`
}

type CronStore struct {
	Version int        `json:"version"`
	Jobs    []*CronJob `json:"jobs"`
}

type CronService struct {
	storePath string
	onJob     func(ctx context.Context, job *CronJob) error
	cron      *cron.Cron
	jobs      map[string]*CronJob
	entryIDs  map[string]cron.EntryID
	timers    map[string]*time.Timer
	mu        sync.RWMutex
}

func NewCronService(storePath string, onJob func(ctx context.Context, job *CronJob) error) *CronService {
	return &CronService{
		storePath: storePath,
		onJob:     onJob,
		cron:      cron.New(cron.WithSeconds()),
		jobs:      make(map[string]*CronJob),
		entryIDs:  make(map[string]cron.EntryID),
		timers:    make(map[string]*time.Timer),
	}
}

func (s *CronService) Start(ctx context.Context) error {
	if err := s.loadStore(); err != nil {
		return err
	}

	s.cron.Start()
	logCron.Info("Cron service started", "jobs", len(s.jobs))
	return nil
}

func (s *CronService) Stop() {
	s.cron.Stop()
	s.mu.Lock()
	for id, timer := range s.timers {
		timer.Stop()
		delete(s.timers, id)
	}
	s.mu.Unlock()
}

func (s *CronService) loadStore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return err
	}

	var store CronStore
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}

	skipped := 0
	for _, job := range store.Jobs {
		if job.Schedule.Kind == JobKindAt && time.Now().UnixMilli() >= job.Schedule.AtMs {
			logCron.Info("Dropping expired at-job", "name", job.Name, "id", job.ID)
			skipped++
			continue
		}
		s.jobs[job.ID] = job
		if job.Enabled {
			if err := s.scheduleJob(job); err != nil {
				job.Enabled = false
				job.State.LastStatus = "error"
				job.State.LastError = err.Error()
			}
		}
	}
	if skipped > 0 {
		go s.saveStore()
	}
	return nil
}

func (s *CronService) saveStore() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	store := CronStore{
		Version: 1,
		Jobs:    make([]*CronJob, 0, len(s.jobs)),
	}
	for _, job := range s.jobs {
		store.Jobs = append(store.Jobs, job)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.storePath), 0755); err != nil {
		return err
	}

	return os.WriteFile(s.storePath, data, 0644)
}

func normalizeCronExprForSeconds(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		return "0 " + expr
	}
	return expr
}

// scheduleJob must be called with s.mu held.
func (s *CronService) scheduleJob(job *CronJob) error {
	switch job.Schedule.Kind {
	case JobKindCron:
		expr := normalizeCronExprForSeconds(job.Schedule.Expr)
		if job.Schedule.TZ != "" {
			expr = fmt.Sprintf("CRON_TZ=%s %s", job.Schedule.TZ, expr)
		}
		entryID, err := s.cron.AddFunc(expr, func() {
			s.executeJob(job)
		})
		if err != nil {
			logCron.Warn("Failed to schedule cron job", "name", job.Name, "error", err)
			return err
		}
		s.entryIDs[job.ID] = entryID

	case JobKindEvery:
		duration := time.Duration(job.Schedule.EveryMs) * time.Millisecond
		entryID, err := s.cron.AddFunc(fmt.Sprintf("@every %s", duration.String()), func() {
			s.executeJob(job)
		})
		if err != nil {
			logCron.Warn("Failed to schedule every job", "name", job.Name, "error", err)
			return err
		}
		s.entryIDs[job.ID] = entryID

	case JobKindAt:
		delay := time.Until(time.UnixMilli(job.Schedule.AtMs))
		if delay <= 0 {
			overdue := -delay
			if overdue <= 5*time.Minute {
				logCron.Info("At-job overdue, executing immediately", "name", job.Name, "overdue", overdue)
				go s.executeJob(job)
			} else {
				logCron.Info("At-job scheduled time already passed, skipping", "name", job.Name, "overdue", overdue)
			}
			return nil
		}
		timer := time.AfterFunc(delay, func() {
			s.executeJob(job)
		})
		s.timers[job.ID] = timer
	}
	return nil
}

// unscheduleJob cancels the scheduling for a job. Must be called with s.mu held.
func (s *CronService) unscheduleJob(id string) {
	if entryID, ok := s.entryIDs[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entryIDs, id)
	}
	if timer, ok := s.timers[id]; ok {
		timer.Stop()
		delete(s.timers, id)
	}
}

func (s *CronService) executeJob(job *CronJob) {
	logCron.Info("Executing job", "name", job.Name, "id", job.ID)
	ctx := context.Background()
	err := s.onJob(ctx, job)

	s.mu.Lock()
	job.State.LastRunAtMs = time.Now().UnixMilli()
	if err != nil {
		job.State.LastStatus = "error"
		job.State.LastError = err.Error()
		logCron.Warn("Job failed", "name", job.Name, "error", err)
	} else {
		job.State.LastStatus = "ok"
		job.State.LastError = ""
		logCron.Info("Job completed", "name", job.Name)
	}

	if job.DeleteAfterRun {
		delete(s.jobs, job.ID)
		s.unscheduleJob(job.ID)
		logCron.Info("Job auto-deleted", "name", job.Name)
	}
	s.mu.Unlock()

	s.saveStore()
}

func (s *CronService) AddJob(name string, schedule CronSchedule, payload CronPayload, deleteAfterRun bool) (*CronJob, error) {
	job := &CronJob{
		ID:             uuid.New().String()[:8],
		Name:           name,
		Enabled:        true,
		Schedule:       schedule,
		Payload:        payload,
		CreatedAtMs:    time.Now().UnixMilli(),
		UpdatedAtMs:    time.Now().UnixMilli(),
		DeleteAfterRun: deleteAfterRun,
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	if err := s.scheduleJob(job); err != nil {
		delete(s.jobs, job.ID)
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	s.saveStore()
	return job, nil
}

func (s *CronService) ListJobs() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*CronJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		cp := *job
		jobs = append(jobs, &cp)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAtMs == jobs[j].CreatedAtMs {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAtMs < jobs[j].CreatedAtMs
	})
	return jobs
}

func (s *CronService) RemoveJob(id string) bool {
	s.mu.Lock()
	if _, ok := s.jobs[id]; !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.jobs, id)
	s.unscheduleJob(id)
	s.mu.Unlock()

	s.saveStore()
	return true
}
