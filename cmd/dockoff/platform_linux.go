//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

func installLock() (func(), error) {
	fd, err := syscall.Open("/run/dockoff-install.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "/run/dockoff-install.lock")
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another installation is running: %w", err)
	}
	return func() { file.Close() }, nil
}

func checkSpace(path string, minimum uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}
	if stat.Bavail*uint64(stat.Bsize) < minimum {
		return fmt.Errorf("at least %d MiB free required on %s", minimum>>20, path)
	}
	return nil
}
