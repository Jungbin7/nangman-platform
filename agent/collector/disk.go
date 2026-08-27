package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// ReadDisk: 디스크 총량, 가용량, Inode 사용률 및 /proc/diskstats I/O를 읽어옵니다.
func ReadDisk() (DiskMetrics, error) {
	metrics := DiskMetrics{
		MountPoint: "/",
	}

	// 1. Root Filesystem Statfs (용량 및 Inode 고갈 여부)
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalBytes := float64(stat.Blocks) * float64(stat.Bsize)
		freeBytes := float64(stat.Bfree) * float64(stat.Bsize)
		usedBytes := totalBytes - freeBytes

		metrics.TotalGB = totalBytes / (1024 * 1024 * 1024)
		metrics.FreeGB = freeBytes / (1024 * 1024 * 1024)
		metrics.UsedGB = usedBytes / (1024 * 1024 * 1024)
		if metrics.TotalGB > 0 {
			metrics.UsagePct = (metrics.UsedGB / metrics.TotalGB) * 100.0
		}

		// Inode 사용률 계산
		if stat.Files > 0 {
			totalInodes := float64(stat.Files)
			freeInodes := float64(stat.Ffree)
			metrics.InodeUsedPct = ((totalInodes - freeInodes) / totalInodes) * 100.0
		}
	}

	// 2. /proc/diskstats I/O 초당 처리량 (주요 디스크 파싱)
	if file, err := os.Open("/proc/diskstats"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 14 {
				devName := fields[2]
				// sda, nvme0n1, vda, mmcblk0 (라즈베리파이 SD카드)
				if strings.HasPrefix(devName, "sd") || strings.HasPrefix(devName, "nvme") || strings.HasPrefix(devName, "vd") || strings.HasPrefix(devName, "mmcblk") {
					readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
					writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)
					metrics.ReadBytesSec = readSectors * 512
					metrics.WriteBytesSec = writeSectors * 512
					break
				}
			}
		}
	}

	return metrics, nil
}
