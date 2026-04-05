//go:build unix

package bridge

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func createSocketPairFDs() ([2]int, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return fds, fmt.Errorf("socketpair: %w", err)
	}
	return fds, nil
}
