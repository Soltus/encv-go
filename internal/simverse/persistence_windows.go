//go:build windows

package simverse

import "golang.org/x/sys/windows"

func fillFilesystemStats(path string, stats *filesystemStats) {
	freeBytesAvailable := uint64(0)
	totalNumberOfBytes := uint64(0)
	totalNumberOfFreeBytes := uint64(0)

	err := windows.GetDiskFreeSpaceEx(
		windows.StringToUTF16Ptr(path),
		&freeBytesAvailable,
		&totalNumberOfBytes,
		&totalNumberOfFreeBytes,
	)
	if err != nil {
		return
	}

	stats.TotalBytes = int64(totalNumberOfBytes)
	stats.AvailableBytes = int64(freeBytesAvailable)
	stats.FreeBytes = int64(totalNumberOfFreeBytes)
}
