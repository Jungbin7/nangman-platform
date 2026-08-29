package metrics

import (
	"fmt"
	"strings"

	"nangman-platform/hub/store"
)

// GeneratePrometheusMetrics: Prometheus 표준 텍스트 규격으로 전체 클러스터 메트릭을 렌더링합니다.
func GeneratePrometheusMetrics(s *store.ClusterStore) string {
	nodes := s.GetAllNodes()
	var sb strings.Builder

	sb.WriteString("# HELP nangman_node_cpu_temp_celsius CPU temperature in Celsius\n")
	sb.WriteString("# TYPE nangman_node_cpu_temp_celsius gauge\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("nangman_node_cpu_temp_celsius{node=\"%s\"} %.2f\n", n.NodeID, n.Thermal.CPUTempCelsius))
	}

	sb.WriteString("\n# HELP nangman_node_memory_used_mb Memory used in Megabytes\n")
	sb.WriteString("# TYPE nangman_node_memory_used_mb gauge\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("nangman_node_memory_used_mb{node=\"%s\"} %.2f\n", n.NodeID, n.Memory.UsedMB))
	}

	sb.WriteString("\n# HELP nangman_node_psi_memory_avg10 Linux kernel PSI memory pressure avg10\n")
	sb.WriteString("# TYPE nangman_node_psi_memory_avg10 gauge\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("nangman_node_psi_memory_avg10{node=\"%s\"} %.2f\n", n.NodeID, n.PSI.MemoryAvg10))
	}

	sb.WriteString("\n# HELP nangman_node_tcp_retrans_total Total TCP retransmitted segments\n")
	sb.WriteString("# TYPE nangman_node_tcp_retrans_total counter\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("nangman_node_tcp_retrans_total{node=\"%s\"} %d\n", n.NodeID, n.Network.TCPRetransTotal))
	}

	sb.WriteString("\n# HELP nangman_cluster_online_nodes Total online nodes in mesh\n")
	sb.WriteString("# TYPE nangman_cluster_online_nodes gauge\n")
	sb.WriteString(fmt.Sprintf("nangman_cluster_online_nodes %d\n", len(nodes)))

	return sb.String()
}
