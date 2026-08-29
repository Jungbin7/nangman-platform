package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nangman-platform/agent/collector"
	"nangman-platform/hub/metrics"
	"nangman-platform/hub/rca"
	"nangman-platform/hub/store"
)

func main() {
	clusterStore := store.NewClusterStore()

	// 1. 에이전트 텔레메트리 수신 엔드포인트
	http.HandleFunc("/api/v1/telemetry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload collector.TelemetryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 인메모리 스토어에 갱신
		clusterStore.UpdateNode(payload)

		// 0.001초 RCA 엔진 실행 (PSI 이상 감지 시 즉시 콘솔 출력)
		if report := rca.AnalyzePSI(payload); report != nil {
			fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05.000"), report.Summary)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	})

	// 2. 36개 노드 전체 상태 JSON 조회 API (CLI & 프론트엔드용)
	http.HandleFunc("/api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		nodes := clusterStore.GetAllNodes()
		json.NewEncoder(w).Encode(nodes)
	})

	// 3. Prometheus 호환 /metrics 엔드포인트
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metricsText := metrics.GeneratePrometheusMetrics(clusterStore)
		w.Write([]byte(metricsText))
	})

	// 4. 루트 상태 헬스체크 (실시간 0.5초 자동 갱신 웹 대시보드)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
  <title>Nangman-Hub Realtime Telemetry Core</title>
  <style>
    body { background: #03050a; color: #e2e8f0; font-family: 'JetBrains Mono', monospace; padding: 25px; }
    .card { background: #070d19; border: 1px solid rgba(0, 240, 255, 0.2); border-radius: 12px; padding: 20px; box-shadow: 0 10px 30px rgba(0,0,0,0.8); }
    .badge { background: rgba(16, 185, 129, 0.2); color: #10b981; padding: 4px 10px; border-radius: 6px; font-weight: bold; border: 1px solid #10b981; }
    pre { background: #020408; padding: 15px; border-radius: 8px; border: 1px solid #1e293b; color: #38bdf8; overflow-x: auto; max-height: 500px; }
    h1 { color: #f8fafc; display: flex; align-items: center; gap: 10px; }
  </style>
</head>
<body>
  <div class="card">
    <h1>🧠 Nangman-Hub Telemetry Core <span class="badge" id="live-badge">● LIVE STREAMING</span></h1>
    <p>● Active Mesh Nodes: <strong id="node-count" style="color:#00f0ff;">1</strong> | 
       Prometheus: <a href="/metrics" target="_blank" style="color:#38bdf8;">/metrics</a> | 
       Raw API: <a href="/api/v1/nodes" target="_blank" style="color:#38bdf8;">/api/v1/nodes</a> |
       Last Updated: <span id="last-updated" style="color:#94a3b8;">-</span>
    </p>
    <hr style="border-color:#1e293b; margin: 15px 0;">
    <h3>Connected Nodes Telemetry Matrix (Live Auto-Update)</h3>
    <pre id="json-view">Loading realtime cluster state...</pre>
  </div>

  <script>
    // 0.5초마다 Hub API(/api/v1/nodes)에서 데이터를 긁어와 화면을 자동으로 갱신합니다!
    async function updateDashboard() {
      try {
        const res = await fetch('/api/v1/nodes');
        const data = await res.json();
        document.getElementById('node-count').innerText = data.length;
        document.getElementById('last-updated').innerText = new Date().toLocaleTimeString();
        document.getElementById('json-view').innerText = JSON.stringify(data, null, 2);
      } catch (err) {
        document.getElementById('live-badge').innerText = '○ OFFLINE';
        document.getElementById('live-badge').style.color = '#ef4444';
      }
    }
    setInterval(updateDashboard, 500);
    updateDashboard();
  </script>
</body>
</html>`
		w.Write([]byte(html))
	})

	port := ":8080"
	fmt.Println("==================================================================")
	fmt.Printf("🧠 Nangman-Hub v1.0 Server listening on %s\n", port)
	fmt.Println("📡 Ingest endpoint : POST /api/v1/telemetry")
	fmt.Println("🌐 Query endpoint  : GET  /api/v1/nodes")
	fmt.Println("📊 Prometheus API  : GET  /metrics")
	fmt.Println("==================================================================")

	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("❌ Hub server error: %v\n", err)
	}
}

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}
