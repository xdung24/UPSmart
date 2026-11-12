package events

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MemoryLimit = 200

type Record struct {
	SchemaVersion int       `json:"schema_version"`
	SessionID     string    `json:"session_id"`
	Sequence      uint64    `json:"sequence"`
	Time          time.Time `json:"time"`
	Type          string    `json:"type"`
	Severity      string    `json:"severity"`
	Message       string    `json:"message"`
}

type Options struct {
	Directory    string `json:"directory"`
	MaxFileBytes int64  `json:"max_file_bytes"`
	MaxFiles     int    `json:"max_files"`
}

func (o Options) Validate() error {
	if !filepath.IsAbs(o.Directory) || strings.HasPrefix(o.Directory, `\\`) {
		return errors.New("event_history.directory must be an absolute local path")
	}
	clean := filepath.Clean(o.Directory)
	if clean == filepath.VolumeName(clean)+string(os.PathSeparator) {
		return errors.New("event_history.directory must not be a filesystem root")
	}
	if o.MaxFileBytes < 1024 || o.MaxFileBytes > 50<<20 {
		return errors.New("event_history.max_file_bytes must be between 1024 and 52428800")
	}
	if o.MaxFiles < 1 || o.MaxFiles > 10 {
		return errors.New("event_history.max_files must be between 1 and 10")
	}
	return nil
}

type Status struct {
	Mode        string `json:"mode"`
	Error       string `json:"error,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Unpersisted uint64 `json:"unpersisted_events"`
}

type Journal struct {
	options Options
	mu      sync.Mutex
	status  Status
	queue   chan Record
	done    chan struct{}
	closed  bool
}

func Open(options Options) (*Journal, []Record, error) {
	if err := options.Validate(); err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(options.Directory, 0700); err != nil {
		return nil, nil, fmt.Errorf("create event directory: %w", err)
	}
	j := &Journal{options: options, status: Status{Mode: "persistent"}, queue: make(chan Record, 256), done: make(chan struct{})}
	var history []Record
	var damaged int
	for index := options.MaxFiles - 1; index >= 0; index-- {
		records, bad, err := j.readFile(index)
		if err != nil {
			return nil, nil, err
		}
		damaged += bad
		history = append(history, records...)
		if len(history) > MemoryLimit {
			history = history[len(history)-MemoryLimit:]
		}
	}
	if err := j.pruneArchives(); err != nil {
		return nil, nil, err
	}
	if damaged > 0 {
		j.status.Warning = fmt.Sprintf("Recovered history with %d invalid or incomplete record(s); valid records were preserved", damaged)
		log.Print(j.status.Warning)
	}
	go j.run()
	return j, history, nil
}

func (j *Journal) path(index int) string {
	name := "events.jsonl"
	if index > 0 {
		name = fmt.Sprintf("events.%d.jsonl", index)
	}
	return filepath.Join(j.options.Directory, name)
}

func (j *Journal) pruneArchives() error {
	files, err := os.ReadDir(j.options.Directory)
	if err != nil {
		return err
	}
	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, "events.") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		indexText := strings.TrimSuffix(strings.TrimPrefix(name, "events."), ".jsonl")
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 1 || strconv.Itoa(index) != indexText {
			continue
		}
		if index >= j.options.MaxFiles {
			path := filepath.Join(j.options.Directory, name)
			if _, err := regularFile(path); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("enforce event file count: %w", err)
			}
			log.Printf("Removed rotated event file %s to enforce max_files=%d", name, j.options.MaxFiles)
		}
	}
	return nil
}

func (j *Journal) readFile(index int) ([]Record, int, error) {
	if info, err := regularFile(j.path(index)); err != nil {
		return nil, 0, err
	} else if info == nil {
		return nil, 0, nil
	}
	f, err := os.OpenFile(j.path(index), os.O_RDWR, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("open event history: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if info.Size() > j.options.MaxFileBytes {
		return nil, 0, errors.New("event file exceeds configured size; preserve it and resolve the retention setting")
	}
	reader := bufio.NewReader(io.LimitReader(f, j.options.MaxFileBytes+1))
	records := make([]Record, 0, MemoryLimit)
	var offset int64
	damaged := 0
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, damaged, fmt.Errorf("read event history: %w", readErr)
		}
		if len(line) > 0 {
			if !strings.HasSuffix(line, "\n") {
				damaged++
				if index == 0 {
					if err := f.Truncate(offset); err != nil {
						return nil, damaged, fmt.Errorf("remove incomplete event tail: %w", err)
					}
					if err := f.Sync(); err != nil {
						return nil, damaged, err
					}
				}
			} else {
				var record Record
				if len(line) > 65536 || json.Unmarshal([]byte(line), &record) != nil || !valid(record) {
					damaged++
				} else {
					records = append(records, record)
					if len(records) > MemoryLimit {
						copy(records, records[1:])
						records = records[:MemoryLimit]
					}
				}
				offset += int64(len(line))
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return records, damaged, nil
}

func valid(r Record) bool {
	return r.SchemaVersion == 1 && len(r.SessionID) > 0 && len(r.SessionID) <= 64 &&
		!strings.Contains(r.SessionID, ":") && r.Sequence > 0 && !r.Time.IsZero() &&
		len(r.Type) > 0 && len(r.Type) <= 64 && len(r.Message) <= 32768 &&
		(r.Severity == "info" || r.Severity == "warning" || r.Severity == "error")
}

func (j *Journal) Enqueue(record Record) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		j.failedLocked(errors.New("event journal is closed"))
		return
	}
	select {
	case j.queue <- record:
	default:
		j.failedLocked(errors.New("event journal queue is full; event remains memory-only"))
	}
}

func (j *Journal) failedLocked(err error) {
	j.status.Mode = "degraded"
	j.status.Error = err.Error()
	j.status.Unpersisted++
}

func (j *Journal) run() {
	defer close(j.done)
	repair := false
	for record := range j.queue {
		var err error
		damaged := 0
		if repair {
			_, damaged, err = j.readFile(0)
		}
		if err == nil {
			err = j.append(record)
		}
		repair = err != nil
		j.mu.Lock()
		previousError := j.status.Error
		if damaged > 0 {
			j.status.Warning = fmt.Sprintf("Recovered %d incomplete or invalid event record(s) after a write failure", damaged)
		}
		if err != nil {
			j.failedLocked(err)
		} else if j.status.Error != "" {
			j.status.Mode = "persistent"
			j.status.Error = ""
		}
		j.mu.Unlock()
		if err != nil && previousError != err.Error() {
			log.Printf("Event persistence failed: %v", err)
		} else if err == nil && previousError != "" {
			log.Print("Event persistence recovered; previously unpersisted events remain reported")
		}
		if damaged > 0 {
			log.Printf("Repaired event history after write failure: %d damaged record(s)", damaged)
		}
	}
}

func (j *Journal) append(record Record) error {
	if !valid(record) {
		return errors.New("invalid event record")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > 65536 || int64(len(data)) > j.options.MaxFileBytes {
		return errors.New("event record exceeds configured size")
	}
	info, err := regularFile(j.path(0))
	if err != nil {
		return err
	}
	if info != nil && info.Size()+int64(len(data)) > j.options.MaxFileBytes {
		if err := j.rotate(); err != nil {
			return fmt.Errorf("rotate events: %w", err)
		}
	}
	f, err := os.OpenFile(j.path(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	before, err := f.Stat()
	if err != nil {
		return errors.Join(err, f.Close())
	}
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = f.Sync()
	} else {
		// A partial record must not corrupt the next successfully appended record.
		writeErr = errors.Join(writeErr, f.Truncate(before.Size()), f.Sync())
	}
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func regularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("event history target must be a regular file, not a directory or link: %s", path)
	}
	return info, nil
}

func (j *Journal) rotate() error {
	oldest := j.path(j.options.MaxFiles - 1)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := j.options.MaxFiles - 2; i >= 0; i-- {
		if err := os.Rename(j.path(i), j.path(i+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (j *Journal) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *Journal) Close(ctx context.Context) error {
	j.mu.Lock()
	if !j.closed {
		j.closed = true
		close(j.queue)
	}
	j.mu.Unlock()
	select {
	case <-j.done:
		status := j.Status()
		if status.Unpersisted > 0 {
			return fmt.Errorf("%d event(s) were not persisted; %s", status.Unpersisted, status.Error)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("flush event journal: %w", ctx.Err())
	}
}
