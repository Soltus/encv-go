//go:build !windows

package simverse

import "golang.org/x/sys/unix"

func fillFilesystemStats(path string, stats *filesystemStats) {
	var statfs unix.Statfs_t
	err := unix.Statfs(path, &statfs)
	if err != nil {
		return
	}

	bsize := statfs.Bsize
	stats.TotalBytes = int64(statfs.Blocks) * int64(bsize)
	stats.AvailableBytes = int64(statfs.Bavail) * int64(bsize)
	stats.FreeBytes = int64(statfs.Bfree) * int64(bsize)
}
