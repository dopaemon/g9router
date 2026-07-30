package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	processnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

func ReleaseOwnedPorts(ports ...int) error {
	connections, err := processnet.Connections("tcp")
	if err != nil {
		return fmt.Errorf("inspect TCP listeners: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	executable, _ = filepath.EvalSymlinks(executable)
	wanted := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		wanted[port] = struct{}{}
	}
	for _, connection := range connections {
		if connection.Status != "LISTEN" || connection.Pid <= 0 {
			continue
		}
		if _, ok := wanted[int(connection.Laddr.Port)]; !ok || int(connection.Pid) == os.Getpid() {
			continue
		}
		owner, err := process.NewProcess(connection.Pid)
		if err != nil {
			continue
		}
		ownerPath, err := owner.Exe()
		if err == nil && ownerPath != "" {
			ownerPath, _ = filepath.EvalSymlinks(ownerPath)
		}
		if (err == nil && ownerPath != "" && !sameExecutable(ownerPath, executable)) || (err != nil || ownerPath == "") && !sameExecutableName(owner, executable) {
			continue
		}
		if err := owner.Kill(); err != nil {
			return fmt.Errorf("stop G9Router process %d: %w", connection.Pid, err)
		}
		for running, _ := owner.IsRunning(); running; running, _ = owner.IsRunning() {
			time.Sleep(25 * time.Millisecond)
		}
	}
	return nil
}

func sameExecutable(left, right string) bool {
	left = strings.TrimSuffix(left, " (deleted)")
	right = strings.TrimSuffix(right, " (deleted)")
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func sameExecutableName(owner *process.Process, executable string) bool {
	name, err := owner.Name()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(name, ".exe"), strings.TrimSuffix(filepath.Base(executable), ".exe"))
}

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
