package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// ReadMemory: 리눅스 커널 /proc/meminfo를 파싱하여 정확한 메모리 가용량과 사용률을 계산합니다.
func ReadMemory() (MemoryMetrics, error) {
	metrics := MemoryMetrics{}

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return metrics, err
	}
	defer file.Close()

	var memTotalKB, memAvailableKB, swapTotalKB, swapFreeKB float64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal":
			memTotalKB = val
		case "MemAvailable":
			memAvailableKB = val
		case "SwapTotal":
			swapTotalKB = val
		case "SwapFree":
			swapFreeKB = val
		}
	}

	if memTotalKB > 0 {
		metrics.TotalMB = memTotalKB / 1024.0
		metrics.AvailableMB = memAvailableKB / 1024.0
		metrics.UsedMB = (memTotalKB - memAvailableKB) / 1024.0
		metrics.UsagePct = (metrics.UsedMB / metrics.TotalMB) * 100.0
		metrics.SwapUsedMB = (swapTotalKB - swapFreeKB) / 1024.0
	}

	return metrics, nil
}
