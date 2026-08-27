package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadCgroupsV2: /sys/fs/cgroup 하위의 컨테이너별 메모리 사용량, OOM-Kill 카운터를 파싱합니다.
// Mattermost에 PSI 알림이 떴을 때 "어떤 컨테이너가 메모리를 독점하는지" 0.1초 만에 밝혀내는 RCA 핵심 로직입니다.
func ReadCgroupsV2() ([]CgroupContainer, error) {
	var containers []CgroupContainer

	// cgroup v2 기본 경로 탐색
	basePaths := []string{
		"/sys/fs/cgroup/system.slice",
		"/sys/fs/cgroup/docker",
		"/sys/fs/cgroup",
	}

	for _, basePath := range basePaths {
		entries, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() && (strings.Contains(entry.Name(), "docker") || strings.Contains(entry.Name(), "container") || strings.HasSuffix(entry.Name(), ".service")) {
				cPath := filepath.Join(basePath, entry.Name())
				c := parseSingleCgroup(entry.Name(), cPath)
				if c.MemoryUsedMB > 1.0 { // 1MB 이상 쓰는 컨테이너만 수집
					containers = append(containers, c)
				}
			}
		}
	}

	return containers, nil
}

func parseSingleCgroup(name, path string) CgroupContainer {
	c := CgroupContainer{
		ContainerName: name,
	}

	// 1. memory.current (현재 사용 메모리 바이트)
	if data, err := os.ReadFile(filepath.Join(path, "memory.current")); err == nil {
		bytesVal, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		c.MemoryUsedMB = bytesVal / (1024 * 1024)
	}

	// 2. memory.events (oom_kill 횟수)
	if data, err := os.ReadFile(filepath.Join(path, "memory.events")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "oom_kill") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					c.OOMKillCount, _ = strconv.ParseUint(fields[1], 10, 64)
				}
			}
		}
	}

	// 3. cpu.stat (throttled_usec)
	if data, err := os.ReadFile(filepath.Join(path, "cpu.stat")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "throttled_usec") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					c.CPUThrottledUsec, _ = strconv.ParseUint(fields[1], 10, 64)
				}
			}
		}
	}

	return c
}
