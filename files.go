package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func atomicJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".save-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	// A reader in the other window may briefly hold the destination open.
	// Retry only the replacement so readers always see a complete snapshot.
	deadline := time.Now().Add(time.Second)
	for delay := 5 * time.Millisecond; ; delay = min(2*delay, 100*time.Millisecond) {
		err = os.Rename(f.Name(), path)
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		wait := min(delay, time.Until(deadline))
		if wait <= 0 {
			return err
		}
		time.Sleep(wait)
	}
}
