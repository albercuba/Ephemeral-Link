//go:build !windows

package web

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func diskInfo(path string) DiskInfo {
	stat, ok := diskStat(path)
	if !ok {
		return DiskInfo{Available: "unknown", Total: "unknown", Used: "unknown", UsedPercent: "unknown"}
	}

	percent := "0%"
	if stat.total > 0 {
		percent = fmt.Sprintf("%.0f%%", (float64(stat.used)/float64(stat.total))*100)
	}

	return DiskInfo{Total: humanSizeInt(stat.total), Available: humanSizeInt(stat.available), Used: humanSizeInt(stat.used), UsedPercent: percent}
}

type diskStatInfo struct{ total, available, used uint64 }

func diskStat(path string) (diskStatInfo, bool) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return diskStatInfo{}, false
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	available := stat.Bavail * blockSize
	used := total - (stat.Bfree * blockSize)
	return diskStatInfo{total: total, available: available, used: used}, true
}

func diskCanAccept(path string, incoming int64) bool {
	stat, ok := diskStat(path)
	if !ok || stat.total == 0 || incoming < 0 {
		return false
	}
	projectedUsed := stat.used + uint64(incoming)
	return (float64(projectedUsed) / float64(stat.total)) < 0.98
}

func humanSizeInt(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}
