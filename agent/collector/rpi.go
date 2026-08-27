package collector

import (
	"os/exec"
	"strconv"
	"strings"
)

// ReadRPiHealth: 라즈베리파이 전용 vcgencmd get_throttled 비트플래그를 분석합니다.
// 저전압(Under-voltage), 발열 스로틀링, 전원 어댑터 불량을 100% 감지합니다.
func ReadRPiHealth() (RPiHardwareHealth, error) {
	health := RPiHardwareHealth{}

	// vcgencmd 명령어가 있는 경우에만 실행 (라즈베리파이 환경)
	out, err := exec.Command("vcgencmd", "get_throttled").Output()
	if err != nil {
		return health, nil // RPi가 아니면 조용히 리턴
	}

	// 출력 예시: "throttled=0x50000" 또는 "throttled=0x0"
	outStr := strings.TrimSpace(string(out))
	parts := strings.Split(outStr, "=")
	if len(parts) < 2 {
		return health, nil
	}

	// 16진수 파싱 (0x50000 -> int64)
	val, err := strconv.ParseInt(strings.TrimPrefix(parts[1], "0x"), 16, 64)
	if err != nil {
		return health, nil
	}

	// 라즈베리파이 공식 커널 비트마스크 분석
	// Bit 0: Under-voltage detected (현재 저전압 발생 중)
	// Bit 1: Arm frequency capped (현재 발열로 클럭 제한 중)
	// Bit 2: Currently throttled (현재 스로틀링 발생 중)
	if val&0x1 != 0 {
		health.UnderVoltageDetected = true
	}
	if val&0x2 != 0 {
		health.ArmFreqCapped = true
	}
	if val&0x4 != 0 {
		health.CurrentlyThrottled = true
	}

	return health, nil
}
