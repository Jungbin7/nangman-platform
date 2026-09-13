package rca

import (
	"fmt"
	"sort"
	"time"

	"nangman-platform/agent/collector"
)

// IncidentReport: PSI 스파이크 발생 시 RCA 엔진이 생성하는 진단 리포트
type IncidentReport struct {
	Timestamp       time.Time `json:"timestamp"`
	NodeID          string    `json:"node_id"`
	PSIMemoryAvg10  float64   `json:"psi_memory_avg10"`
	Severity        string    `json:"severity"` // "WARNING" | "CRITICAL"
	CulpritName     string    `json:"culprit_container"`
	CulpritMemoryMB float64   `json:"culprit_memory_mb"`
	CulpritSharePct float64   `json:"culprit_share_pct"`
	Summary         string    `json:"summary"`
}

// AnalyzeComprehensive: 발열 스로틀링, OOM-Kill, 커널 PSI 압박, 라즈베리파이 저전압을 0.001초 만에 전수 검사합니다.
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
			Timestamp: now,
			NodeID:    payload.NodeID,
			Severity:  sev,
			Summary:   fmt.Sprintf("🔥 [%s] 하드웨어 고온(%.1f°C) 감지! 발열 스로틀링 위험", payload.NodeID, payload.Thermal.CPUTempCelsius),
		})
	}

	// 2. cgroups v2 컨테이너 OOM-Kill 사살 감지
	for _, c := range payload.CgroupsV2 {
		if c.OOMKillCount > 0 {
			reports = append(reports, IncidentReport{
				Timestamp:       now,
				NodeID:          payload.NodeID,
				Severity:        "CRITICAL",
				CulpritName:     c.ContainerName,
				CulpritMemoryMB: c.MemoryUsedMB,
				Summary:         fmt.Sprintf("💀 [%s] 컨테이너 OOM 사살 감지! [%s] 프로세스 강제 종료됨 (누적 %d건)", payload.NodeID, c.ContainerName, c.OOMKillCount),
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
			Timestamp: now,
			NodeID:    payload.NodeID,
			Severity:  "CRITICAL",
			Summary:   fmt.Sprintf("⚡ [%s] 5V 전원 저전압(Under-voltage) 발생! 전원 어댑터 불량 위험", payload.NodeID),
		})
	}

	return reports
}

// AnalyzePSI: LLM 없이 0.001초 만에 cgroups v2를 분석하여 범인 컨테이너(Noisy Neighbor)를 특정합니다.
func AnalyzePSI(payload collector.TelemetryPayload) *IncidentReport {
	psiVal := payload.PSI.MemoryAvg10
	if psiVal < 5.0 {
		return nil // 정상 상태
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
			Summary:        fmt.Sprintf("🚨 [%s] PSI 메모리 압박 %.2f%% 감지 (상세 cgroups 정보 없음)", payload.NodeID, psiVal),
		}
	}

	containers := make([]collector.CgroupContainer, len(payload.CgroupsV2))
	copy(containers, payload.CgroupsV2)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].MemoryUsedMB > containers[j].MemoryUsedMB
	})

	topCulprit := containers[0]
	sharePct := 0.0
	if payload.Memory.UsedMB > 0 {
		sharePct = (topCulprit.MemoryUsedMB / payload.Memory.UsedMB) * 100.0
	}

	summary := fmt.Sprintf("🚨 [%s] PSI 메모리 압박 %.2f%% 초과! 원인: [%s] 컨테이너가 %.1fMB(전체 사용량의 %.1f%%) 점유 중",
		payload.NodeID, psiVal, topCulprit.ContainerName, topCulprit.MemoryUsedMB, sharePct)

	return &IncidentReport{
		Timestamp:       time.Now(),
		NodeID:          payload.NodeID,
		PSIMemoryAvg10:  psiVal,
		Severity:        severity,
		CulpritName:     topCulprit.ContainerName,
		CulpritMemoryMB: topCulprit.MemoryUsedMB,
		CulpritSharePct: sharePct,
		Summary:         summary,
	}
}
