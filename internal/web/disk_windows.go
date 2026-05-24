//go:build windows

package web

func diskInfo(path string) DiskInfo {
	return DiskInfo{Available: "unknown", Total: "unknown", Used: "unknown", UsedPercent: "unknown"}
}

func diskCanAccept(path string, incoming int64) bool { return false }
