package rca

import (
	"fmt"
	"sort"
	"time"

	"nangman-platform/agent/collector"
)

// IncidentReport: 이상 징후 발생 시 RCA 엔진이 생성하는 정형화된 리포트
type IncidentReport struct {
	Timestamp       time.Time `json:"timestamp"`
	NodeID          string    `json:"node_id"`
	PSIMemoryAvg10  float64   `json:"psi_memory_avg10"`
	Severity        string    `json:"severity"` // "WARNING" | "CRITICAL"
	AlertType       string    `json:"alert_type"`
	MetricValue     string    `json:"metric_value"`
	SpikeWorkload   string    `json:"spike_workload"`
	BaselineLive    string    `json:"baseline_live"`
	Summary         string    `json:"summary"`
}

// 이전 주기 노드별 워크로드 메모리 캐시 (Delta 추적용)
var (
	lastWorkloadMem = make(map[string]map[string]float64) // nodeID -> containerName -> memoryMB
)

// AnalyzeComprehensive: 발열, OOM, PSI, 저전압을 검사하여 정형화된 Alert를 반환합니다.
func AnalyzeComprehensive(payload collector.TelemetryPayload) []IncidentReport {
	var reports []IncidentReport
	now := time.Now()

	// 1. 하드웨어 발열 스로틀링 감지
	if payload.Thermal.IsThrottled || payload.Thermal.CPUTempCelsius >= 75.0 {
		sev := "WARNING"
		if payload.Thermal.CPUTempCelsius >= 82.0 || payload.Thermal.IsThrottled {
			sev = "CRITICAL"
		}
		reports = append(reports, IncidentReport{
			Timestamp:     now,
			NodeID:        payload.NodeID,
			Severity:      sev,
			AlertType:     "THERMAL_HIGH",
			MetricValue:   fmt.Sprintf("%.1f°C", payload.Thermal.CPUTempCelsius),
			SpikeWorkload: "SoC Thermal Throttling",
			Summary:       fmt.Sprintf("[%s] THERMAL_HIGH (%.1f°C)", payload.NodeID, payload.Thermal.CPUTempCelsius),
		})
	}

	// 2. cgroups v2 컨테이너 OOM-Kill 사살 감지
	for _, c := range payload.CgroupsV2 {
		if c.OOMKillCount > 0 {
			reports = append(reports, IncidentReport{
				Timestamp:     now,
				NodeID:        payload.NodeID,
				Severity:      "CRITICAL",
				AlertType:     "OOM_KILLED",
				MetricValue:   fmt.Sprintf("%d kills", c.OOMKillCount),
				SpikeWorkload: fmt.Sprintf("%s (%.0fMB)", c.ContainerName, c.MemoryUsedMB),
				Summary:       fmt.Sprintf("[%s] OOM_KILLED %s", payload.NodeID, c.ContainerName),
			})
			break
		}
	}

	// 3. 커널 PSI 메모리 압박 감지
	if psiRep := AnalyzePSI(payload); psiRep != nil {
		reports = append(reports, *psiRep)
	}

	// 4. 라즈베리파이 5V 전원 저전압 감지
	if payload.RPiHealth.UnderVoltageDetected {
		reports = append(reports, IncidentReport{
			Timestamp:     now,
			NodeID:        payload.NodeID,
			Severity:      "CRITICAL",
			AlertType:     "UNDER_VOLTAGE",
			MetricValue:   "4.63V Drop",
			SpikeWorkload: "Power Adapter Issue",
			Summary:       fmt.Sprintf("[%s] UNDER_VOLTAGE (5V Drop)", payload.NodeID),
		})
	}

	return reports
}

// AnalyzePSI: cgroups v2를 분석하여 최근 급증한 프로세스와 기존 상위 프로세스를 구분하여 추출합니다.
func AnalyzePSI(payload collector.TelemetryPayload) *IncidentReport {
	psiVal := payload.PSI.MemoryAvg10
	if psiVal < 5.0 {
		return nil
	}

	severity := "WARNING"
	if psiVal >= 10.0 {
		severity = "CRITICAL"
	}

	if len(payload.CgroupsV2) == 0 {
		return &IncidentReport{
			Timestamp:      time.Now(),
			NodeID:         payload.NodeID,
			PSIMemoryAvg10: psiVal,
			Severity:       severity,
			AlertType:      "PSI_STALL",
			MetricValue:    fmt.Sprintf("%.2f%%", psiVal),
			Summary:        fmt.Sprintf("[%s] PSI_STALL %.2f%%", payload.NodeID, psiVal),
		}
	}

	// 메모리 사용량 기준 내림차순 정렬
	containers := make([]collector.CgroupContainer, len(payload.CgroupsV2))
	copy(containers, payload.CgroupsV2)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].MemoryUsedMB > containers[j].MemoryUsedMB
	})

	nodeID := payload.NodeID
	prevMem, exists := lastWorkloadMem[nodeID]
	if !exists {
		prevMem = make(map[string]float64)
	}

	// 급증(Spike) 프로세스 탐색 (직전 대비 +150MB 이상 또는 신규 대형 할당)
	spikeName := ""
	spikeDelta := 0.0
	for _, c := range containers {
		prev := prevMem[c.ContainerName]
		delta := c.MemoryUsedMB - prev
		if delta > spikeDelta && delta >= 100.0 {
			spikeDelta = delta
			spikeName = fmt.Sprintf("%s (+%.0fMB)", c.ContainerName, delta)
		}
	}

	// 캐시 갱신
	currMem := make(map[string]float64)
	for _, c := range containers {
		currMem[c.ContainerName] = c.MemoryUsedMB
	}
	lastWorkloadMem[nodeID] = currMem

	// 기본 상위 프로세스 (Baseline)
	baseName := fmt.Sprintf("%s (%.0fMB)", containers[0].ContainerName, containers[0].MemoryUsedMB)
	if len(containers) > 1 {
		baseName += fmt.Sprintf(", %s (%.0fMB)", containers[1].ContainerName, containers[1].MemoryUsedMB)
	}

	if spikeName == "" {
		spikeName = "Steady Load"
	}

	return &IncidentReport{
		Timestamp:      time.Now(),
		NodeID:         payload.NodeID,
		PSIMemoryAvg10: psiVal,
		Severity:       severity,
		AlertType:      "PSI_STALL",
		MetricValue:    fmt.Sprintf("%.2f%%", psiVal),
		SpikeWorkload:  spikeName,
		BaselineLive:   baseName,
		Summary:        fmt.Sprintf("[%s] PSI_STALL %.2f%% | Spike: %s | Base: %s", payload.NodeID, psiVal, spikeName, baseName),
	}
}
