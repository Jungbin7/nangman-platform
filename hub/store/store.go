package store

import (
	"sync"
	"time"

	"nangman-platform/agent/collector"
)

// NodeState: 허브가 인메모리에 보관하는 각 노드의 최신 상태
type NodeState struct {
	LastSeen  time.Time                  `json:"last_seen"`
	Telemetry collector.TelemetryPayload `json:"telemetry"`
}

// ClusterStore: 36개 노드의 상태를 동시성 안전(Concurrent-safe)하게 관리하는 인메모리 저장소
type ClusterStore struct {
	mu    sync.RWMutex
	nodes map[string]*NodeState
}

// NewClusterStore: 클러스터 스토어 초기화
func NewClusterStore() *ClusterStore {
	return &ClusterStore{
		nodes: make(map[string]*NodeState),
	}
}

// UpdateNode: 에이전트로부터 날아온 텔레메트리로 노드 상태를 갱신합니다.
func (s *ClusterStore) UpdateNode(payload collector.TelemetryPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[payload.NodeID] = &NodeState{
		LastSeen:  time.Now(),
		Telemetry: payload,
	}
}

// GetAllNodes: 현재 연결된 모든 노드의 최신 상태 목록을 반환합니다.
func (s *ClusterStore) GetAllNodes() []collector.TelemetryPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []collector.TelemetryPayload
	for _, state := range s.nodes {
		// 10초 이상 응답이 없으면 오프라인으로 간주 (선택적)
		result = append(result, state.Telemetry)
	}
	return result
}

// GetNodeCount: 활성 노드 수 반환
func (s *ClusterStore) GetNodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}
