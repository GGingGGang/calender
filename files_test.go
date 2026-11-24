package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceSnapshotDuringRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := atomicJSON(path, "old"); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- atomicJSON(path, "new") }()
	var value string
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(60 * time.Millisecond):
		if err := readJSON(path, &value); err != nil || value != "old" {
			t.Fatal(value, err)
		}
		reader.Close()
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	if err := readJSON(path, &value); err != nil || value != "new" {
		t.Fatal(value, err)
	}
}

func TestLocalSnapshotReadableDuringSync(t *testing.T) {
	s := Store{t.TempDir()}
	if err := atomicJSON(s.path("batch.json"), batchState{Snapshot: Snapshot{Last: "saved"}}); err != nil {
		t.Fatal(err)
	}
	locked, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- s.locked(func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case err := <-finished:
		t.Fatal(err)
	case <-locked:
	}
	defer func() { close(release); <-finished }()
	read := make(chan bool, 1)
	go func() {
		v, err := s.snapshot()
		read <- err == nil && v.Last == "saved"
	}()
	select {
	case ok := <-read:
		if !ok {
			t.Fatal("cached snapshot unavailable")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read waited for the Git sync lock")
	}
}
