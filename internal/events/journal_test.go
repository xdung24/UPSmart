package events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func record(sequence uint64) Record {
	return Record{1, "SESSIONA", sequence, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), "power_changed", "info", "online"}
}

func closeJournal(t *testing.T, journal *Journal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := journal.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRotationExactBoundaryAndRestartRecovery(t *testing.T) {
	options := Options{t.TempDir(), 1024, 2}
	journal, history, err := Open(options)
	if err != nil || len(history) != 0 {
		t.Fatalf("open: %v, %d records", err, len(history))
	}
	event := record(1)
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	event.Message += strings.Repeat("x", 512-len(data)-1)
	for sequence := uint64(1); sequence <= 9; sequence++ {
		event.Sequence = sequence
		journal.Enqueue(event)
	}
	closeJournal(t, journal)
	files, err := os.ReadDir(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("retention file count = %d, want 2", len(files))
	}
	var total int64
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > options.MaxFileBytes {
			t.Fatalf("%s exceeds exact file cap: %d", file.Name(), info.Size())
		}
		total += info.Size()
	}
	if total != 1536 {
		t.Fatalf("rotation did not honor exact boundary: total %d, want 1536", total)
	}
	reopened, history, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	defer closeJournal(t, reopened)
	if len(history) != 3 || history[0].Sequence != 7 || history[2].Sequence != 9 {
		t.Fatalf("incorrect chronological retained history: %+v", history)
	}
}

func TestPartialTailAndCorruptLinePreserveValidRecords(t *testing.T) {
	options := Options{t.TempDir(), 4096, 2}
	path := filepath.Join(options.Directory, "events.jsonl")
	first, _ := json.Marshal(record(1))
	second, _ := json.Marshal(record(2))
	contents := append(append(append(first, '\n'), []byte("{bad json}\n")...), second...)
	contents = append(contents, '\n')
	completeSize := len(contents)
	contents = append(contents, []byte(`{"incomplete":`)...)
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	journal, history, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || journal.Status().Warning == "" {
		t.Fatalf("valid history lost or corruption hidden: %+v %+v", history, journal.Status())
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != int64(completeSize) {
		t.Fatalf("incomplete tail not repaired: %v %v", info, err)
	}
	journal.Enqueue(record(3))
	closeJournal(t, journal)
	reopened, history, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	defer closeJournal(t, reopened)
	if len(history) != 3 || history[2].Sequence != 3 {
		t.Fatal("repair caused the next append to be lost")
	}
}

func TestPersistenceFailureIsVisibleAndRecoveryKeepsLossCount(t *testing.T) {
	options := Options{t.TempDir(), 4096, 1}
	journal, _, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(options.Directory, "events.jsonl")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	journal.Enqueue(record(1))
	deadline := time.Now().Add(3 * time.Second)
	for journal.Status().Unpersisted == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if status := journal.Status(); status.Mode != "degraded" || status.Error == "" || status.Unpersisted != 1 {
		t.Fatalf("storage failure hidden: %+v", status)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	journal.Enqueue(record(2))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := journal.Close(ctx); err == nil {
		t.Fatal("flush reported success despite an unpersisted event")
	}
	if status := journal.Status(); status.Mode != "persistent" || status.Unpersisted != 1 || status.Error != "" {
		t.Fatalf("recovery lost failure evidence: %+v", status)
	}
	reopened, history, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	defer closeJournal(t, reopened)
	if len(history) != 1 || history[0].Sequence != 2 {
		t.Fatalf("unexpected recovered records: %+v", history)
	}
}

func TestOversizedExistingFileIsNotSilentlyDeleted(t *testing.T) {
	options := Options{t.TempDir(), 1024, 1}
	path := filepath.Join(options.Directory, "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 1025)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(options); err == nil {
		t.Fatal("oversized history was accepted")
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 1025 {
		t.Fatal("failed open destroyed evidence")
	}
}

func TestReducingRetentionPrunesOnlyOwnedArchives(t *testing.T) {
	options := Options{t.TempDir(), 1024, 4}
	journal, _, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= 6; sequence++ {
		event := record(sequence)
		event.Message = strings.Repeat("x", 600)
		journal.Enqueue(event)
	}
	closeJournal(t, journal)
	unrelated := filepath.Join(options.Directory, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	options.MaxFiles = 2
	reopened, history, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	closeJournal(t, reopened)
	if len(history) != 2 || history[0].Sequence != 5 || history[1].Sequence != 6 {
		t.Fatalf("reduced retention recovered wrong records: %+v", history)
	}
	files, err := os.ReadDir(options.Directory)
	if err != nil || len(files) != 3 {
		t.Fatalf("expected two journal files and unrelated file: %v %v", files, err)
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "preserve" {
		t.Fatal("retention cleanup modified an unrelated file")
	}
}

func TestRetentionNeverDeletesAnArchiveNamedDirectory(t *testing.T) {
	options := Options{t.TempDir(), 1024, 1}
	path := filepath.Join(options.Directory, "events.1.jsonl")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(options); err == nil {
		t.Fatal("unexpected archive directory was silently treated as a journal file")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatal("retention deleted an unexpected directory")
	}
}
