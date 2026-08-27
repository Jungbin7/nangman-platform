package collector

import "time"

// TelemetryPayload: 에이전트가 0.5초마다 커널에서 수집하여 뱉어내는 엔터프라이즈 텔레메트리
type TelemetryPayload struct {
	NodeID     string             `json:"node_id"`
	Hostname   string             `json:"hostname"`
	Timestamp  time.Time          `json:"timestamp"`
	Thermal    ThermalMetrics     `json:"thermal"`
	PSI        PSIMetrics         `json:"psi"`
	Memory     MemoryMetrics      `json:"memory"`
	Disk       DiskMetrics        `json:"disk"`
	Network    NetworkMetrics     `json:"network"`
	CgroupsV2  []CgroupContainer  `json:"cgroups_v2_containers"`
	RPiHealth  RPiHardwareHealth  `json:"rpi_hardware_health"`
}

// ThermalMetrics: CPU 및 SoC 온도 메트릭
type ThermalMetrics struct {
	CPUTempCelsius float64 `json:"cpu_temp_c"`
	IsThrottled    bool    `json:"is_throttled"`
}

// PSIMetrics: 리눅스 커널 3대 압박 지수 (/proc/pressure)
type PSIMetrics struct {
	MemoryPressure string  `json:"memory_pressure_full"`
	MemoryAvg10    float64 `json:"memory_avg10"`
	CPUPressure    string  `json:"cpu_pressure_some"`
	IOPressure     string  `json:"io_pressure_full"`
}

// MemoryMetrics: /proc/meminfo 기반 정밀 메모리 메트릭
type MemoryMetrics struct {
	TotalMB     float64 `json:"total_mb"`
	AvailableMB float64 `json:"available_mb"`
	UsedMB      float64 `json:"used_mb"`
	UsagePct    float64 `json:"usage_pct"`
	SwapUsedMB  float64 `json:"swap_used_mb"`
}

// DiskMetrics: 디스크 용량, Inode 고갈 여부 및 I/O 큐 통계
type DiskMetrics struct {
	MountPoint   string  `json:"mount_point"`
	TotalGB      float64 `json:"total_gb"`
	UsedGB       float64 `json:"used_gb"`
	FreeGB       float64 `json:"free_gb"`
	UsagePct     float64 `json:"usage_pct"`
	InodeUsedPct float64 `json:"inode_used_pct"` // Inode 100% 도달 시 디스크 있어도 파일 생성 불가 장애 방지
	ReadBytesSec uint64  `json:"read_bytes_sec"`
	WriteBytesSec uint64 `json:"write_bytes_sec"`
}

// NetworkMetrics: /proc/net/snmp & /proc/net/netstat (TCP 재전송 및 대역폭)
type NetworkMetrics struct {
	RxBytesSec      uint64 `json:"rx_bytes_sec"`
	TxBytesSec      uint64 `json:"tx_bytes_sec"`
	TCPRetransTotal uint64 `json:"tcp_retrans_total"` // TCP 패킷 재전송 (VPN 터널 불안정성 감지)
	TCPErrors       uint64 `json:"tcp_in_errors"`
}

// CgroupContainer: /sys/fs/cgroup 컨테이너별 자원 점유 (PSI 범인 색출용)
type CgroupContainer struct {
	ContainerName string  `json:"container_name"`
	MemoryUsedMB  float64 `json:"memory_used_mb"`
	OOMKillCount  uint64  `json:"oom_kill_count"`
	CPUThrottledUsec uint64 `json:"cpu_throttled_usec"`
}

// RPiHardwareHealth: 라즈베리파이 저전압 및 하드웨어 스로틀링 플래그
type RPiHardwareHealth struct {
	UnderVoltageDetected bool `json:"under_voltage_detected"` // 전원 어댑터 불량 감지
	ArmFreqCapped        bool `json:"arm_freq_capped"`        // 발열로 인한 클럭 제한
	CurrentlyThrottled   bool `json:"currently_throttled"`
}

