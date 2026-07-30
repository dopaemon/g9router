package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gofrs/flock"
)

func Acquire(ports ...int) (func(), error) {
	ports = uniquePorts(ports)
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("find config directory: %w", err)
	}
	lockDir := filepath.Join(configDir, "g9router")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	locks := make([]*flock.Flock, 0, len(ports))
	for _, port := range ports {
		lock := flock.New(filepath.Join(lockDir, fmt.Sprintf("g9router-%d.lock", port)))
		locked, err := lock.TryLock()
		if err != nil {
			releaseLocks(locks)
			return nil, fmt.Errorf("lock port %d: %w", port, err)
		}
		if !locked {
			releaseLocks(locks)
			return nil, fmt.Errorf("G9Router is already running on port %d", port)
		}
		locks = append(locks, lock)
	}
	return func() { releaseLocks(locks) }, nil
}

func uniquePorts(ports []int) []int {
	seen := make(map[int]struct{}, len(ports))
	unique := make([]int, 0, len(ports))
	for _, port := range ports {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		unique = append(unique, port)
	}
	sort.Ints(unique)
	return unique
}

func releaseLocks(locks []*flock.Flock) {
	for _, lock := range locks {
		_ = lock.Unlock()
	}
}
