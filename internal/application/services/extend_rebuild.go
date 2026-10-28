package services

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const (
	rebuildBusyWindow     = 30 * time.Minute
	rebuildUnsupportedAge = 90 * time.Second
	rebuildLogTailLines   = 200
	rebuildUnsupportedMsg = "installed concierge does not handle rebuild-frontend; replace /home/t8k/scripts/t8k-concierge.sh by hand"
)

var (
	ErrRebuildUnavailable = errors.New("rebuild unavailable")
	ErrRebuildNotFound    = errors.New("rebuild job not found")
)

// ErrRebuildBusy is returned when an in-progress job younger than 30 minutes exists.
type ErrRebuildBusy struct {
	JobID string
}

func (e *ErrRebuildBusy) Error() string {
	return fmt.Sprintf("rebuild busy: %s", e.JobID)
}

// RebuildJob is the on-disk job JSON shape (queued by Go, updated by concierge).
type RebuildJob struct {
	Status     string  `json:"status"`
	JobID      string  `json:"jobId,omitempty"`
	QueuedAt   string  `json:"queuedAt,omitempty"`
	StartedAt  string  `json:"startedAt,omitempty"`
	BuiltAt    string  `json:"builtAt,omitempty"`
	ReloadedAt string  `json:"reloadedAt,omitempty"`
	Type       string  `json:"type,omitempty"`
	Site       *string `json:"site,omitempty"`
	Log        string  `json:"log,omitempty"`
	Message    string  `json:"message,omitempty"`
	Whitelist  string  `json:"whitelist,omitempty"`
}

// RebuildJobResponse is GET job payload (optional log tail).
type RebuildJobResponse struct {
	RebuildJob
	LogTail string `json:"logTail,omitempty"`
}

// ExtendRebuildService serves /api/v1/extend/rebuild*.
type ExtendRebuildService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewExtendRebuildService creates the rebuild job service.
func NewExtendRebuildService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendRebuildService {
	return &ExtendRebuildService{logger: logger, perfTracker: perfTracker}
}

func (s *ExtendRebuildService) requirePublish() error {
	if !PublishEnabled(config.BackendPath) {
		return ErrRebuildUnavailable
	}
	return nil
}

// QueueRebuild mints a ULID, writes queued job + csv, returns jobId.
func (s *ExtendRebuildService) QueueRebuild() (string, error) {
	if err := s.requirePublish(); err != nil {
		return "", err
	}
	target, ok := PublishTargetFromBackend(config.BackendPath)
	if !ok {
		return "", ErrRebuildUnavailable
	}

	if busyID, busy := s.findBusyJob(); busy {
		return "", &ErrRebuildBusy{JobID: busyID}
	}

	jobID := security.GenerateULID()
	jobsDir := JobsDir()
	if err := os.MkdirAll(jobsDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir jobs: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	job := RebuildJob{
		Status:   "queued",
		JobID:    jobID,
		QueuedAt: now,
	}
	jobPath := filepath.Join(jobsDir, jobID+".json")
	if err := writeJobJSON(jobPath, &job); err != nil {
		return "", err
	}

	csvPath := filepath.Join(StateDir, "build-"+jobID+".csv")
	var csvLine string
	switch target.Type {
	case "main":
		csvLine = fmt.Sprintf("type=main,command=rebuild-frontend,job=%s\n", jobID)
	case "dedicated":
		csvLine = fmt.Sprintf("type=dedicated,site=%s,command=rebuild-frontend,job=%s\n", target.Site, jobID)
	default:
		_ = os.Remove(jobPath)
		return "", ErrRebuildUnavailable
	}
	if err := os.WriteFile(csvPath, []byte(csvLine), 0o644); err != nil {
		_ = os.Remove(jobPath)
		return "", fmt.Errorf("write csv: %w", err)
	}
	return jobID, nil
}

// GetJob returns job JSON plus log tail when building|failed; applies unsupported timeout.
func (s *ExtendRebuildService) GetJob(jobID string) (*RebuildJobResponse, error) {
	if err := s.requirePublish(); err != nil {
		return nil, err
	}
	if jobID == "" || strings.Contains(jobID, "/") || strings.Contains(jobID, "..") {
		return nil, ErrRebuildNotFound
	}

	jobPath := filepath.Join(JobsDir(), jobID+".json")
	job, err := readJobJSON(jobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRebuildNotFound
		}
		return nil, err
	}
	if job.JobID == "" {
		job.JobID = jobID
	}

	if job.Status == "queued" {
		if marked, markErr := s.maybeMarkUnsupported(job, jobPath, jobID); markErr != nil {
			return nil, markErr
		} else if marked != nil {
			job = marked
		}
	}

	resp := &RebuildJobResponse{RebuildJob: *job}
	if job.Status == "building" || job.Status == "failed" {
		logPath := filepath.Join(JobsDir(), jobID+".log")
		if job.Log != "" {
			logPath = job.Log
		}
		if tail, tErr := tailFileLines(logPath, rebuildLogTailLines); tErr == nil {
			resp.LogTail = tail
		}
	}
	return resp, nil
}

// GetLatest returns latest.json object, or nil when file is missing (caller returns 200 null).
func (s *ExtendRebuildService) GetLatest() (*RebuildJob, error) {
	if err := s.requirePublish(); err != nil {
		return nil, err
	}
	path := filepath.Join(JobsDir(), "latest.json")
	job, err := readJobJSON(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

// ReadLatestFrontendBuild returns jobId/status/builtAt when all present on latest.json.
func ReadLatestFrontendBuild() (jobID, status, builtAt string, ok bool) {
	if !PublishEnabled(config.BackendPath) {
		return "", "", "", false
	}
	job, err := readJobJSON(filepath.Join(JobsDir(), "latest.json"))
	if err != nil || job == nil {
		return "", "", "", false
	}
	if job.JobID == "" || job.Status == "" || job.BuiltAt == "" {
		return "", "", "", false
	}
	return job.JobID, job.Status, job.BuiltAt, true
}

func (s *ExtendRebuildService) maybeMarkUnsupported(job *RebuildJob, jobPath, jobID string) (*RebuildJob, error) {
	queuedAt, err := time.Parse(time.RFC3339, job.QueuedAt)
	if err != nil {
		// try without monotonic / Z variants
		queuedAt, err = time.Parse(time.RFC3339Nano, job.QueuedAt)
		if err != nil {
			return nil, nil
		}
	}
	if time.Since(queuedAt) < rebuildUnsupportedAge {
		return nil, nil
	}
	csvPath := filepath.Join(StateDir, "build-"+jobID+".csv")
	if _, err := os.Stat(csvPath); err != nil {
		return nil, nil
	}
	_ = os.Remove(csvPath)
	job.Status = "unsupported"
	job.Message = rebuildUnsupportedMsg
	if err := writeJobJSON(jobPath, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ExtendRebuildService) findBusyJob() (string, bool) {
	jobsDir := JobsDir()
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		return "", false
	}
	cutoff := time.Now().Add(-rebuildBusyWindow)
	check := func(name string) (string, bool) {
		path := filepath.Join(jobsDir, name)
		job, err := readJobJSON(path)
		if err != nil {
			return "", false
		}
		switch job.Status {
		case "started", "building", "swapping", "reloading":
		default:
			return "", false
		}
		started, err := parseJobTime(job.StartedAt)
		if err != nil || started.Before(cutoff) {
			return "", false
		}
		id := job.JobID
		if id == "" {
			id = strings.TrimSuffix(name, ".json")
			if id == "latest" {
				return "", false
			}
		}
		return id, true
	}
	// Prefer named job files; also check latest.json
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if id, ok := check(e.Name()); ok {
			return id, true
		}
	}
	return "", false
}

func parseJobTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func readJobJSON(path string) (*RebuildJob, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var job RebuildJob
	if err := json.Unmarshal(b, &job); err != nil {
		return nil, fmt.Errorf("parse job: %w", err)
	}
	return &job, nil
}

func writeJobJSON(path string, job *RebuildJob) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func tailFileLines(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	// Allow long log lines
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}
