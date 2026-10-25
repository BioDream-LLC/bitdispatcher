//go:build unix

package api

import "syscall"

// freeBytes is the space left for an unprivileged user on the disk holding dir.
func freeBytes(dir string) (uint64, bool) {
	var st syscall.Statfs_t
	if dir == "" || syscall.Statfs(dir, &st) != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
