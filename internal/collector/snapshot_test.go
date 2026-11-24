package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCanceledReplacementAndRestart(t *testing.T) {
	raw := t.TempDir()
	root, err := os.OpenRoot(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	old := []byte(`{"id":"recording-1","future":9007199254740993}`)
	next := []byte(`{"id":"recording-1","future":9007199254740994}`)
	if changed, err := snapshot(context.Background(), root, "recording-1", old); err != nil || !changed {
		t.Fatalf("initial snapshot: changed=%t, err=%v", changed, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshot(ctx, root, "recording-1", next); !errors.Is(err, errInterrupted) {
		t.Fatalf("canceled replacement succeeded: %v", err)
	}
	path := filepath.Join("recording-1", "recording.json")
	got, err := root.ReadFile(path)
	if err != nil || !sameJSON(got, old) {
		t.Fatalf("canceled write corrupted prior snapshot: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(raw, "recording-1"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "recording.json" {
		t.Fatalf("canceled write left temporary files: %v, %v", entries, err)
	}
	if changed, err := snapshot(context.Background(), root, "recording-1", next); err != nil || !changed {
		t.Fatalf("restart failed: changed=%t, err=%v", changed, err)
	}
	got, err = root.ReadFile(path)
	if err != nil || !sameJSON(got, next) {
		t.Fatal("restart did not commit new snapshot")
	}
}

func TestSnapshotRejectsSymlinkDirectory(t *testing.T) {
	raw, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(raw, "recording-1")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := snapshot(context.Background(), root, "recording-1", []byte(`{"id":"recording-1"}`)); err == nil {
		t.Fatal("symlink recording folder accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("snapshot escaped raw directory")
	}
}
