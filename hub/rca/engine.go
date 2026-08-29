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

// AnalyzePSI: LLM 없이 0.001초 만에 cgroups v2를 분석하여 범인 컨테이너(Noisy Neighbor)를 특정합니다.
func AnalyzePSI(payload collector.TelemetryPayload) *IncidentReport {
	// 임계치: 메모리 PSI full avg10이 5.0% 이상이면 위험(Critical), 2.0% 이상이면 경고(Warning)
	psiVal := payload.PSI.MemoryAvg10
	if psiVal < 2.0 {
		return nil // 정상 상태
	}

	severity := "WARNING"
	if psiVal >= 5.0 {
		severity = "CRITICAL"
	}

	// cgroups 컨테이너 목록이 없는 경우
	if len(payload.CgroupsV2) == 0 {
		return &IncidentReport{
			Timestamp:      time.Now(),
			NodeID:         payload.NodeID,
			PSIMemoryAvg10: psiVal,
			Severity:       severity,
			Summary:        fmt.Sprintf("🚨 [%s] PSI 메모리 압박 %.2f%% 감지 (상세 cgroups 정보 없음)", payload.NodeID, psiVal),
		}
	}

	// 1. 컨테이너들을 메모리 사용량(MemoryUsedMB) 내림차순으로 정렬
	containers := make([]collector.CgroupContainer, len(payload.CgroupsV2))
	copy(containers, payload.CgroupsV2)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].MemoryUsedMB > containers[j].MemoryUsedMB
	})

	// 2. 1위 최다 점유 컨테이너 색출
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
