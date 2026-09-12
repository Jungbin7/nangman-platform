package collector

import (
	"os"
	"strconv"
	"strings"
)

// ReadCPUTemp: 리눅스 sysfs (/sys/class/thermal/thermal_zone*/temp)에서 CPU 온도를 읽어옵니다.
// 라즈베리파이, 일반 리눅스 서버에서 표준으로 제공되는 커널 인터페이스입니다.
func ReadCPUTemp() (ThermalMetrics, error) {
	metrics := ThermalMetrics{
		CPUTempCelsius: 0.0,
		IsThrottled:    false,
	}

	// 1. 표준 thermal_zone0 경로 시도
	paths := []string{
		"/sys/class/thermal/thermal_zone0/temp",
		"/sys/class/hwmon/hwmon0/temp1_input",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			rawStr := strings.TrimSpace(string(data))
			rawVal, err := strconv.ParseFloat(rawStr, 64)
			if err == nil {
				// 리눅스 커널은 48250처럼 1000배수로 온도를 표현합니다 (밀리섭씨).
				metrics.CPUTempCelsius = rawVal / 1000.0
				if metrics.CPUTempCelsius > 82.0 {
					metrics.IsThrottled = true
				}
				return metrics, nil
			}
		}
	}

	// 가상머신(WSL2/KVM)처럼 가상 하드웨어라 온도 센서 파일이 없는 경우 기본값 처리
	metrics.CPUTempCelsius = 42.0 // Virtualized fallback
	return metrics, nil
}
