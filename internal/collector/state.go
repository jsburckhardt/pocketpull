package collector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type coordination struct {
	raw   *os.Root
	lock  *os.File
	state *os.Root
	log   *slog.Logger
}

func coordinate(options Options) (*coordination, error) {
	if err := os.MkdirAll(options.RawDir, 0o700); err != nil {
		return nil, errors.New("raw directory unavailable")
	}
	rawPath, err := filepath.EvalSymlinks(options.RawDir)
	if err != nil {
		return nil, errors.New("raw directory unavailable")
	}
	rawPath, err = filepath.Abs(rawPath)
	if err != nil {
		return nil, errors.New("raw directory unavailable")
	}
	lock, err := os.Open(rawPath)
	if err != nil {
		return nil, errors.New("raw directory unavailable")
	}
	// The directory inode, not a state path or marker, identifies the lock.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("collection already running")
		}
		return nil, errors.New("collection lock unavailable")
	}
	raw, err := os.OpenRoot(rawPath)
	if err != nil {
		lock.Close()
		return nil, errors.New("raw directory unavailable")
	}
	store := &coordination{raw: raw, lock: lock, log: options.Log}
	statePath, err := canonicalLocation(options.StateDir)
	if err == nil && within(rawPath, statePath) {
		store.close()
		return nil, errors.New("state directory must be outside raw")
	}
	if err == nil {
		statePath = filepath.Join(statePath, pathHash(rawPath))
		if within(rawPath, statePath) {
			store.close()
			return nil, errors.New("state directory must be outside raw")
		}
		err = os.MkdirAll(statePath, 0o700)
	}
	if err == nil {
		info, statErr := os.Lstat(statePath)
		if statErr != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			err = errors.New("private state directory unavailable")
		} else if owner, ok := info.Sys().(*syscall.Stat_t); !ok || owner.Uid != uint32(os.Geteuid()) {
			err = errors.New("private state directory unavailable")
		}
	}
	if err == nil {
		store.state, err = os.OpenRoot(statePath)
	}
	if err != nil {
		options.Log.Warn("freshness state unavailable; refetching", "event", "state_unavailable")
	}
	return store, nil
}

// Resolve even a not-yet-created location before creating any private state.
func canonicalLocation(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		// A dangling alias cannot safely identify a future state location.
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return "", errors.New("state location unavailable")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", errors.New("state location unavailable")
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func pathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func (store *coordination) close() {
	if store.state != nil {
		store.state.Close()
	}
	store.raw.Close()
	store.lock.Close()
}

func (store *coordination) fresh(id string, now time.Time, interval time.Duration) bool {
	if interval == 0 || store.state == nil {
		return false
	}
	data, err := regularFile(store.state, pathHash(id)+".json", 4096, true)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	var entry struct {
		LastSuccess time.Time `json:"last_success"`
	}
	if err != nil || json.Unmarshal(data, &entry) != nil || entry.LastSuccess.IsZero() || entry.LastSuccess.After(now) {
		store.log.Warn("freshness entry unusable; refetching", "event", "state_unusable", "recording_id", id)
		return false
	}
	if now.Sub(entry.LastSuccess) >= interval {
		return false
	}
	info, err := store.raw.Lstat(id)
	if err != nil || !info.IsDir() {
		return false
	}
	directory, err := store.raw.OpenRoot(id)
	if err != nil {
		return false
	}
	defer directory.Close()
	data, err = regularFile(directory, "recording.json", 32<<20, false)
	if err != nil {
		return false
	}
	_, valid := validRecording(data, id)
	return valid
}

func (store *coordination) remember(ctx context.Context, id string, completed time.Time) error {
	if store.state == nil {
		return errors.New("freshness state unavailable")
	}
	data, err := json.Marshal(struct {
		LastSuccess time.Time `json:"last_success"`
	}{completed})
	if err != nil {
		return errors.New("freshness timestamp invalid")
	}
	// Raw is committed and synced first. A crash here can only cause refetching.
	if err := atomicReplace(ctx, store.state, pathHash(id)+".json", data); err != nil {
		return errors.New("freshness commit failed")
	}
	return nil
}

func regularFile(root *os.Root, name string, limit int64, private bool) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("file unusable")
	}
	if private {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("file not private")
		}
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("file read failed")
	}
	return data, nil
}
