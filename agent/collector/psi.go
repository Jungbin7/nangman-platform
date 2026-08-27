package collector

import (
	"os"
	"strconv"
	"strings"
)

// ReadPSI: 리눅스 커널 4.20+ 표준 /proc/pressure/ (memory, cpu, io) 데이터를 정밀 파싱합니다.
// Mattermost 알림에 찍히는 PSI full avg 지수를 추출하는 핵심 함수입니다.
func ReadPSI() (PSIMetrics, error) {
	metrics := PSIMetrics{
		MemoryPressure: "N/A",
		MemoryAvg10:    0.0,
		CPUPressure:    "N/A",
		IOPressure:     "N/A",
	}

	// 1. 메모리 압박 파싱 (/proc/pressure/memory)
	if memData, err := os.ReadFile("/proc/pressure/memory"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(memData)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "full") {
				metrics.MemoryPressure = line
				// "full avg10=1.25 avg60=0.50 avg300=0.10 total=12400" 에서 avg10 파싱
				metrics.MemoryAvg10 = parseAvg10(line)
			}
		}
	}

	// 2. CPU 압박 파싱 (/proc/pressure/cpu)
	if cpuData, err := os.ReadFile("/proc/pressure/cpu"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(cpuData)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "some") {
				metrics.CPUPressure = line
			}
		}
	}

	// 3. 디스크 I/O 압박 파싱 (/proc/pressure/io)
	if ioData, err := os.ReadFile("/proc/pressure/io"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(ioData)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "full") {
				metrics.IOPressure = line
			}
		}
	}

	return metrics, nil
}

// parseAvg10: "some/full avg10=2.35 avg60=..." 문자열에서 2.35 float64 추출
func parseAvg10(line string) float64 {
	parts := strings.Fields(line)
	for _, p := range parts {
		if strings.HasPrefix(p, "avg10=") {
			valStr := strings.TrimPrefix(p, "avg10=")
			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				return val
			}
		}
	}
	return 0.0
}
