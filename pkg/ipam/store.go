package ipam

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type state struct {
	ContainerToIP map[string]string `json:"containerToIP"`
	IPToContainer map[string]string `json:"ipToContainer"`
	LastReserved  string            `json:"lastReserved,omitempty"`
}

func newState() *state {
	return &state{
		ContainerToIP: map[string]string{},
		IPToContainer: map[string]string{},
	}
}

func lockNetwork(dataDir, network string) (*os.File, string, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create data dir: %w", err)
	}

	lockPath := filepath.Join(dataDir, network+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, "", fmt.Errorf("lock state: %w", err)
	}
	return f, filepath.Join(dataDir, network+".json"), nil
}

func unlockNetwork(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

func loadState(path string) (*state, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newState(), nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	st := newState()
	if len(content) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(content, st); err != nil {
		return nil, fmt.Errorf("ipam state file %s is corrupted: %w", path, err)
	}
	if st.ContainerToIP == nil {
		st.ContainerToIP = map[string]string{}
	}
	if st.IPToContainer == nil {
		st.IPToContainer = map[string]string{}
	}
	return st, nil
}

func saveState(path string, st *state) error {
	content, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
