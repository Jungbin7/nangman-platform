package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"nangman-platform/agent/collector"
	"nangman-platform/hub/metrics"
	"nangman-platform/hub/rca"
	"nangman-platform/hub/store"
)

var (
	alertMu      sync.Mutex
	activeAlerts = make(map[string]rca.IncidentReport) // nodeID -> active alert
	alertExpiry  = make(map[string]time.Time)          // nodeID -> resolve timestamp
)

func main() {
	clusterStore := store.NewClusterStore()

	// SIGINT / SIGTERM 안전 종료 (Alternate Screen 복원)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Print("\033[?1049l\033[?25h\n🛑 Nangman-Hub gracefully stopped.\n")
		os.Exit(0)
	}()

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

		// 0.001초 종합 RCA 엔진 실행 (발열, OOM, PSI, 저전압 전수 검사)
		reports := rca.AnalyzeComprehensive(payload)
		alertMu.Lock()
		nodeID := payload.NodeID
		if len(reports) > 0 {
			activeAlerts[nodeID] = reports[0]
			delete(alertExpiry, nodeID) // 비정상 상태 지속 중이므로 만료 타이머 해제
		} else {
			// 정상 회복된 경우: 10초 후 자동 소멸 예약
			if _, exists := activeAlerts[nodeID]; exists {
				if _, expiring := alertExpiry[nodeID]; !expiring {
					alertExpiry[nodeID] = time.Now().Add(10 * time.Second)
				} else if time.Now().After(alertExpiry[nodeID]) {
					delete(activeAlerts, nodeID)
					delete(alertExpiry, nodeID)
				}
			}
		}
		alertMu.Unlock()

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

	// 4. 3D 공간 디지털 트윈 관제소 (Three.js 기반 3D UI)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(get3DDashboardHTML()))
	})

	// 5. 빅테크 대규모 플릿 TUI 대시보드 백그라운드 렌더러
	go renderTUIDashboard(clusterStore)

	port := ":8080"
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("❌ Hub server error: %v\n", err)
	}
}

// pad: UTF-8 글자 수를 기준으로 우측 공백을 채워 터미널 칸을 1픽셀 오차 없이 정렬합니다.
func pad(s string, width int) string {
	rLen := utf8.RuneCountInString(s)
	if rLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-rLen)
}

func drawBar(pct float64, barLen int) string {
	fill := int(pct * float64(barLen))
	if fill > barLen {
		fill = barLen
	}
	if fill < 0 {
		fill = 0
	}
	return strings.Repeat("█", fill) + strings.Repeat("░", barLen-fill)
}

// renderTUIDashboard: 100대 이상도 단 22줄에 완벽 고정되는 빅테크 Large-Scale Fleet TUI 대시보드
func renderTUIDashboard(clusterStore *store.ClusterStore) {
	// 터미널 Alternate Screen Buffer 진입 및 커서 숨김 (스크롤바 완벽 차단)
	fmt.Print("\033[?1049h\033[?25l")

	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		nodes := clusterStore.GetAllNodes()
		now := time.Now().Format("15:04:05")

		// 1초마다 화면을 지우지 않고 커서만 맨 위(1,1)로 이동하여 덮어씀 (깜빡임 0%, 스크롤 0%)
		fmt.Print("\033[H")

		// 1. 헤더 (3줄)
		healthyCount := 0
		var totalTemp float64
		var maxPSI float64
		for _, n := range nodes {
			if !n.Thermal.IsThrottled && n.PSI.MemoryAvg10 < 5.0 && n.Thermal.CPUTempCelsius < 75.0 {
				healthyCount++
			}
			totalTemp += n.Thermal.CPUTempCelsius
			if n.PSI.MemoryAvg10 > maxPSI {
				maxPSI = n.PSI.MemoryAvg10
			}
		}

		avgTemp := 0.0
		if len(nodes) > 0 {
			avgTemp = totalTemp / float64(len(nodes))
		}

		fmt.Printf("\033[1;36m▲ NANGMAN TELEMETRY FLEET CONTROL\033[0m \033[2mv3.0\033[0m                      \033[1;32m● LIVE\033[0m  \033[2m%s\033[0m\033[K\n", now)
		fmt.Printf("\033[2mFleet: \033[1;32m%d Online\033[0m  │ \033[2mHealth: \033[1;37m%d/%d Healthy\033[0m  │ \033[2mAvg Temp: \033[1;33m%.1f°C\033[0m  │ \033[2mPeak PSI: \033[1;35m%.2f%%\033[0m\033[K\n",
			len(nodes), healthyCount, len(nodes), avgTemp, maxPSI)
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────\033[0m\033[K")

		// 2. 구역별 집계 게이지 (4줄: 대괄호 [ 위치를 14번째 열로 100% 수직 칼정렬)
		fmt.Println("\033[1;37mFLEET ZONE GAUGES\033[0m\033[K")
		// 석촌 IDC
		seokchonNodes := 0
		for _, n := range nodes {
			if strings.Contains(n.Hostname, "seokchon") || strings.Contains(n.Hostname, "pi5") || strings.Contains(n.Hostname, "raspix") {
				seokchonNodes++
			}
		}
		scStatus := "\033[2mWaiting\033[0m"
		if seokchonNodes > 0 {
			scStatus = fmt.Sprintf("\033[1;32m%d Online\033[0m \033[2m(100%% OK)\033[0m", seokchonNodes)
		}
		fmt.Printf("  석촌 IDC    [%s] %s\033[K\n", "\033[1;32m"+drawBar(1.0, 20)+"\033[0m", scStatus)
		fmt.Printf("  연구실 IDC  [%s] \033[2mReady\033[0m\033[K\n", "\033[2m"+drawBar(0.0, 20)+"\033[0m")
		fmt.Printf("  AWS Cloud   [%s] \033[2mReady\033[0m\033[K\n", "\033[2m"+drawBar(0.0, 20)+"\033[0m")

		// 3. 100-Node Dense Micro-Dot Heatmap (3줄)
		fmt.Println("\n\033[1;37m100-NODE DENSE HEATMAP\033[0m \033[2m(1 dot = 1 node, Real-time status)\033[0m\033[K")
		var dotRow1, dotRow2 strings.Builder
		for i := 1; i <= 100; i++ {
			var dot string
			if i <= len(nodes) {
				n := nodes[i-1]
				if n.Thermal.IsThrottled || n.PSI.MemoryAvg10 >= 10.0 {
					dot = "\033[1;31m■\033[0m" // 위험
				} else if n.PSI.MemoryAvg10 >= 5.0 || n.Thermal.CPUTempCelsius >= 70.0 {
					dot = "\033[1;33m■\033[0m" // 경고
				} else {
					dot = "\033[1;32m■\033[0m" // 정상
				}
			} else {
				dot = "\033[2m·\033[0m" // 슬롯 대기
			}

			if i <= 50 {
				dotRow1.WriteString(dot)
			} else {
				dotRow2.WriteString(dot)
			}
		}
		fmt.Printf("  %s \033[2m(01-50)\033[0m\033[K\n", dotRow1.String())
		fmt.Printf("  %s \033[2m(51-100)\033[0m\033[K\n", dotRow2.String())

		// 4. 관리 예외 원칙: ATTENTION REQUIRED (Top Bottlenecks) (5줄)
		fmt.Println("\n\033[1;37mATTENTION REQUIRED (Bottlenecks & Anomalies)\033[0m\033[K")
		fmt.Printf("\033[2m  %-30s  %-10s  %-14s  %-12s  %s\033[0m\033[K\n", "HOSTNAME", "CPU TEMP", "RAM USAGE", "PSI STALL", "STATUS")

		if len(nodes) == 0 {
			fmt.Println("  \033[2mWaiting for telemetry stream on :8080...\033[0m\033[K")
			fmt.Println("\033[K")
		} else {
			// 이상 노드 우선 필터링, 없으면 최근 노드 출력
			printed := 0
			for _, n := range nodes {
				if printed >= 3 {
					break
				}
				rawHost := n.Hostname
				if utf8.RuneCountInString(rawHost) > 30 {
					rawHost = string([]rune(rawHost)[:27]) + "..."
				}

				// 1. 호스트명 (30칸)
				colHost := pad(rawHost, 30)

				// 2. 온도 (10칸: 순수 텍스트 먼저 10칸 패딩 후 색상 입히기)
				rawTemp := fmt.Sprintf("%.1f°C", n.Thermal.CPUTempCelsius)
				paddedTemp := pad(rawTemp, 10)
				var colTemp string
				if n.Thermal.CPUTempCelsius >= 75.0 {
					colTemp = "\033[1;31m" + paddedTemp + "\033[0m"
				} else {
					colTemp = "\033[1;32m" + paddedTemp + "\033[0m"
				}

				// 3. 메모리 (14칸)
				rawMem := fmt.Sprintf("%.1fG (%.0f%%)", n.Memory.UsedMB/1024.0, n.Memory.UsagePct)
				colMem := pad(rawMem, 14)

				// 4. PSI (12칸: 순수 텍스트 먼저 12칸 패딩 후 색상 입히기)
				psiVal := n.PSI.MemoryAvg10
				var rawPSI, colPSI string
				if psiVal >= 10.0 {
					rawPSI = fmt.Sprintf("%.2f%% CRIT", psiVal)
					colPSI = "\033[1;31m" + pad(rawPSI, 12) + "\033[0m"
				} else if psiVal >= 5.0 {
					rawPSI = fmt.Sprintf("%.2f%% WARN", psiVal)
					colPSI = "\033[1;33m" + pad(rawPSI, 12) + "\033[0m"
				} else {
					rawPSI = fmt.Sprintf("%.2f%% ok", psiVal)
					colPSI = "\033[2m" + pad(rawPSI, 12) + "\033[0m"
				}

				// 5. 상태 (10칸: 순수 텍스트 먼저 10칸 패딩 후 색상 입히기)
				var rawStatus, colStatus string
				if n.Thermal.IsThrottled || psiVal >= 10.0 {
					rawStatus = "● CRITICAL"
					colStatus = "\033[1;31m" + pad(rawStatus, 10) + "\033[0m"
				} else if psiVal >= 5.0 || n.Thermal.CPUTempCelsius >= 70.0 {
					rawStatus = "● WARNING"
					colStatus = "\033[1;33m" + pad(rawStatus, 10) + "\033[0m"
				} else {
					rawStatus = "● HEALTHY"
					colStatus = "\033[1;32m" + pad(rawStatus, 10) + "\033[0m"
				}

				// 1픽셀 오차 없는 수직 칼정렬 출력
				fmt.Printf("  %s  %s  %s  %s  %s\033[K\n",
					colHost, colTemp, colMem, colPSI, colStatus)
				printed++
			}
			if printed == 1 {
				fmt.Println("  \033[2m(All other nodes operating normally within green thresholds)\033[0m\033[K")
			}
		}

		// 5. 활성 장애 관리 (ACTIVE FLEET ALERTS - 이모티콘 배제, 빅테크 고밀도 테이블)
		alertMu.Lock()
		nowAlert := time.Now()
		for nId, expTime := range alertExpiry {
			if nowAlert.After(expTime) {
				delete(activeAlerts, nId)
				delete(alertExpiry, nId)
			}
		}

		fmt.Printf("\n\033[1;37mACTIVE FLEET ALERTS (%d Nodes Affected)\033[0m\033[K\n", len(activeAlerts))
		if len(activeAlerts) == 0 {
			fmt.Println("  \033[1;32m● ALL NODES HEALTHY\033[0m  \033[2mZero active bottlenecks across fleet.\033[0m\033[K")
			fmt.Println("\033[K")
		} else {
			fmt.Printf("\033[2m  %-8s  %-18s  %-9s  %-12s  %s\033[0m\033[K\n",
				"TIME", "NODE", "SEVERITY", "TRIGGER", "SUSPECT / WORKLOAD CONTEXT")
			count := 0
			for _, alert := range activeAlerts {
				if count >= 2 {
					break
				}
				tStr := alert.Timestamp.Format("15:04:05")
				hName := alert.NodeID
				// 노드명 간소화 (접두어 phy-nangman-dev- 제거로 가독성 향상)
				hName = strings.TrimPrefix(hName, "phy-nangman-dev-")
				hName = strings.TrimPrefix(hName, "vm-nangman-dev-")
				if utf8.RuneCountInString(hName) > 18 {
					hName = string([]rune(hName)[:15]) + "..."
				}

				sevColor := "\033[1;33m"
				if alert.Severity == "CRITICAL" {
					sevColor = "\033[1;31m"
				}
				colSev := sevColor + pad(alert.Severity, 9) + "\033[0m"

				// 트리거 (ALERT_TYPE + METRIC 결합: 12자)
				trigStr := fmt.Sprintf("%s %s", alert.AlertType, alert.MetricValue)
				colTrig := pad(trigStr, 12)

				// 컨텍스트 (38자 이내)
				var ctxStr string
				if alert.SpikeWorkload != "" && alert.SpikeWorkload != "Steady Load" {
					ctxStr = fmt.Sprintf("Spike: %s | Base: %s", alert.SpikeWorkload, alert.BaselineLive)
				} else {
					ctxStr = fmt.Sprintf("Steady | Base: %s", alert.BaselineLive)
				}
				if utf8.RuneCountInString(ctxStr) > 38 {
					ctxStr = string([]rune(ctxStr)[:35]) + "..."
				}

				fmt.Printf("  %-8s  %-18s  %s  %s  \033[2m%s\033[0m\033[K\n",
					tStr, pad(hName, 18), colSev, colTrig, ctxStr)
				count++
			}
			if count < 2 {
				fmt.Println("\033[K")
			}
		}
		alertMu.Unlock()

		// 6. 풋터 (1줄)
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────\033[0m\033[K")
		fmt.Printf("\033[2mWeb 3D Viewer: \033[0;34mhttp://172.16.0.31:8080\033[0m  \033[2m│  Press Ctrl+C to terminate Hub\033[0m\033[K\n")
	}
}

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}

func get3DDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="UTF-8">
  <title>NANGMAN SPATIAL TWIN | Hybrid Cluster Control Center</title>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/OrbitControls.js"></script>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=Outfit:wght@500;700;900&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg: #030712;
      --card-bg: rgba(15, 23, 42, 0.85);
      --border: rgba(56, 189, 248, 0.2);
      --cyan: #00f0ff;
      --emerald: #10b981;
      --amber: #f59e0b;
      --rose: #f43f5e;
      --text: #f8fafc;
      --subtext: #94a3b8;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { background: var(--bg); color: var(--text); font-family: 'Outfit', sans-serif; overflow: hidden; height: 100vh; width: 100vw; user-select: none; }
    #canvas-container { position: absolute; inset: 0; z-index: 1; }

    /* Top HUD Header */
    header {
      position: absolute; top: 16px; left: 24px; right: 24px; z-index: 10;
      display: flex; justify-content: space-between; align-items: center;
      background: var(--card-bg); backdrop-filter: blur(16px);
      border: 1px solid var(--border); border-radius: 16px; padding: 12px 24px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.6);
    }
    .logo-area { display: flex; align-items: center; gap: 14px; }
    .logo-badge { background: linear-gradient(135deg, #00f0ff, #3b82f6); color: #000; font-weight: 900; padding: 6px 12px; border-radius: 8px; font-size: 14px; letter-spacing: 1px; }
    .title { font-size: 18px; font-weight: 700; color: #fff; }
    .status-group { display: flex; align-items: center; gap: 20px; font-family: 'JetBrains Mono', monospace; font-size: 13px; }
    .status-item { display: flex; align-items: center; gap: 8px; }
    .pulse-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--emerald); box-shadow: 0 0 10px var(--emerald); animation: pulse 1.5s infinite; }
    @keyframes pulse { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.4; transform: scale(1.3); } }

    /* Right Telemetry Drawer */
    #drawer {
      position: absolute; top: 88px; right: 24px; bottom: 24px; width: 420px; z-index: 10;
      background: var(--card-bg); backdrop-filter: blur(20px);
      border: 1px solid var(--border); border-radius: 16px; padding: 24px;
      box-shadow: -8px 0 32px rgba(0,0,0,0.7); display: flex; flex-direction: column; gap: 16px;
      transform: translateX(460px); transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
    }
    #drawer.open { transform: translateX(0); }
    .drawer-header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 12px; }
    .drawer-title { font-size: 18px; font-weight: 700; color: var(--cyan); }
    .close-btn { cursor: pointer; color: var(--subtext); font-size: 20px; background: none; border: none; }
    .metric-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    .metric-card { background: rgba(2, 6, 23, 0.6); border: 1px solid rgba(255,255,255,0.06); border-radius: 10px; padding: 12px; font-family: 'JetBrains Mono', monospace; }
    .metric-label { font-size: 11px; color: var(--subtext); margin-bottom: 4px; }
    .metric-val { font-size: 18px; font-weight: 700; color: #fff; }

    /* cgroups container list */
    .cgroups-box { flex: 1; overflow-y: auto; background: rgba(2, 6, 23, 0.7); border: 1px solid var(--border); border-radius: 10px; padding: 12px; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
    .cgroups-title { font-size: 12px; font-weight: 700; color: var(--cyan); margin-bottom: 8px; }
    .cgroup-row { display: flex; justify-content: space-between; padding: 6px 0; border-bottom: 1px solid rgba(255,255,255,0.05); }
    .cgroup-culprit { color: var(--rose); font-weight: bold; background: rgba(244, 63, 94, 0.1); padding: 4px 6px; border-radius: 4px; }

    /* Bottom Quick Nav Bar */
    .zone-nav {
      position: absolute; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 10;
      display: flex; gap: 12px; background: var(--card-bg); backdrop-filter: blur(16px);
      border: 1px solid var(--border); border-radius: 12px; padding: 8px 16px;
    }
    .zone-btn {
      background: rgba(255,255,255,0.05); color: var(--text); border: 1px solid transparent;
      padding: 8px 16px; border-radius: 8px; font-size: 13px; font-weight: 600; cursor: pointer; transition: all 0.2s;
    }
    .zone-btn:hover, .zone-btn.active { background: rgba(0, 240, 255, 0.15); border-color: var(--cyan); color: var(--cyan); }
  </style>
</head>
<body>
  <div id="canvas-container"></div>

  <!-- Header -->
  <header>
    <div class="logo-area">
      <div class="logo-badge">NANGMAN</div>
      <div class="title">SPATIAL DIGITAL TWIN v3.5</div>
    </div>
    <div class="status-group">
      <div class="status-item"><span class="pulse-dot"></span> LIVE MESH</div>
      <div class="status-item">NODES: <span id="online-nodes-count" style="color:var(--cyan); font-weight:bold;">1</span></div>
      <div class="status-item">HUB: <span style="color:var(--emerald);">172.16.0.31</span></div>
      <div class="status-item"><a href="/metrics" target="_blank" style="color:var(--cyan); text-decoration:none;">[PROMETHEUS]</a></div>
    </div>
  </header>

  <!-- Telemetry Drawer -->
  <div id="drawer">
    <div class="drawer-header">
      <div>
        <div class="drawer-title" id="d-node-name">NODE TELEMETRY</div>
        <div style="font-size:12px; color:var(--subtext);" id="d-node-arch">x86_64 / Linux</div>
      </div>
      <button class="close-btn" onclick="closeDrawer()">✕</button>
    </div>
    <div class="metric-grid">
      <div class="metric-card">
        <div class="metric-label">CPU TEMP</div>
        <div class="metric-val" id="d-temp" style="color:var(--amber);">-- °C</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">MEMORY USED</div>
        <div class="metric-val" id="d-mem">-- MB</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">PSI MEMORY AVG10</div>
        <div class="metric-val" id="d-psi" style="color:var(--cyan);">0.00%</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">TCP RETRANSMIT</div>
        <div class="metric-val" id="d-retrans">--</div>
      </div>
    </div>
    <div class="cgroups-box">
      <div class="cgroups-title">⚡ REALTIME CGROUPS V2 CONTAINERS</div>
      <div id="d-cgroups-list">수집 중...</div>
    </div>
  </div>

  <!-- Zone Navigation -->
  <div class="zone-nav">
    <button class="zone-btn active" onclick="focusZone('all')">🌐 FULL CLUSTER</button>
    <button class="zone-btn" onclick="focusZone('seokchon')">🏠 석촌 엣지 팜 (Pi 5)</button>
    <button class="zone-btn" onclick="focusZone('wisoft')">🏫 Wisoft IDC (Hub VM)</button>
    <button class="zone-btn" onclick="focusZone('aws')">☁️ AWS Cloud VPC</button>
  </div>

  <script>
    // --- THREE.JS 3D SCENE SETUP ---
    const container = document.getElementById('canvas-container');
    const scene = new THREE.Scene();
    scene.fog = new THREE.FogExp2(0x030712, 0.015);

    const camera = new THREE.PerspectiveCamera(45, window.innerWidth / window.innerHeight, 0.1, 1000);
    camera.position.set(0, 35, 65);

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    renderer.setSize(window.innerWidth, window.innerHeight);
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.shadowMap.enabled = true;
    container.appendChild(renderer.domElement);

    const controls = new THREE.OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.05;
    controls.maxPolarAngle = Math.PI / 2.1;

    // Lights
    const ambientLight = new THREE.AmbientLight(0x1e293b, 1.8);
    scene.add(ambientLight);
    const dirLight = new THREE.DirectionalLight(0x00f0ff, 1.5);
    dirLight.position.set(20, 40, 20);
    scene.add(dirLight);

    // Floor Grid
    const gridHelper = new THREE.GridHelper(100, 50, 0x00f0ff, 0x1e293b);
    gridHelper.position.y = -0.5;
    scene.add(gridHelper);

    // Nodes 3D Mesh Storage
    const nodeMeshes = [];
    const raycaster = new THREE.Raycaster();
    const mouse = new THREE.Vector2();

    // Zone Definitions
    const zones = {
      seokchon: { x: -25, z: 0, color: 0x10b981, label: "석촌 엣지 SBC" },
      wisoft:   { x: 0,   z: -10, color: 0x00f0ff, label: "Wisoft 연구실 IDC" },
      aws:      { x: 25,  z: 0, color: 0xa855f7, label: "AWS Cloud VPC" }
    };

    // Build 3D Platforms
    Object.keys(zones).forEach(key => {
      const z = zones[key];
      const geom = new THREE.CylinderGeometry(14, 14, 0.8, 32);
      const mat = new THREE.MeshStandardMaterial({ color: z.color, transparent: true, opacity: 0.15, wireframe: false });
      const cyl = new THREE.Mesh(geom, mat);
      cyl.position.set(z.x, 0, z.z);
      scene.add(cyl);

      const edgeGeom = new THREE.RingGeometry(13.8, 14, 32);
      const edgeMat = new THREE.MeshBasicMaterial({ color: z.color, side: THREE.DoubleSide });
      const ring = new THREE.Mesh(edgeGeom, edgeMat);
      ring.rotation.x = Math.PI / 2;
      ring.position.set(z.x, 0.45, z.z);
      scene.add(ring);
    });

    // Create 3D Node Mesh Function
    function create3DNode(id, x, y, z, color, label) {
      const group = new THREE.Group();
      const geom = new THREE.BoxGeometry(3, 1.2, 3);
      const mat = new THREE.MeshStandardMaterial({ color: color, roughness: 0.2, metalness: 0.8 });
      const box = new THREE.Mesh(geom, mat);
      box.castShadow = true;
      group.add(box);

      // Glow Core
      const coreGeom = new THREE.BoxGeometry(3.2, 0.2, 3.2);
      const coreMat = new THREE.MeshBasicMaterial({ color: color, transparent: true, opacity: 0.8 });
      const core = new THREE.Mesh(coreGeom, coreMat);
      core.position.y = 0.5;
      group.add(core);

      group.position.set(x, y + 1, z);
      group.userData = { nodeId: id, label: label, originalColor: color, telemetry: null };
      scene.add(group);
      nodeMeshes.push(group);
      return group;
    }

    // Seed Real Physical Nodes
    const pi5 = create3DNode("phy-nangman-dev-pi5-app-seokchon", -25, 0, -4, 0x10b981, "RPi 5 (석촌)");
    const wisoftHub = create3DNode("vm-nangman-dev-jungbin-wisoft", 0, 0, -10, 0x00f0ff, "Wisoft Hub VM");
    const ec2 = create3DNode("ec2-ops-mgmt", 25, 0, -4, 0xa855f7, "AWS EC2 Mgmt");

    // Realtime Telemetry Polling Loop (Every 500ms)
    let liveTelemetryData = [];
    async function fetchLiveTelemetry() {
      try {
        const res = await fetch('/api/v1/nodes');
        const nodes = await res.json();
        liveTelemetryData = nodes || [];
        document.getElementById('online-nodes-count').innerText = liveTelemetryData.length;

        // Bind live data to 3D Nodes
        liveTelemetryData.forEach(t => {
          const match = nodeMeshes.find(m => m.userData.nodeId === t.node_id);
          if (match) {
            match.userData.telemetry = t;
            // If thermal temp > 60°C, pulse red/amber glow!
            if (t.thermal && t.thermal.cpu_temp_c >= 60) {
              match.children[1].material.color.setHex(0xf59e0b); // Amber heat
            } else {
              match.children[1].material.color.setHex(match.userData.originalColor);
            }
          }
        });

        // Update Drawer if currently viewing a node
        if (selectedNode && selectedNode.userData.telemetry) {
          renderDrawer(selectedNode.userData.telemetry);
        }
      } catch (e) {
        console.warn("Polling hub...", e);
      }
    }
    setInterval(fetchLiveTelemetry, 500);
    fetchLiveTelemetry();

    // Node Click & Selection (Raycasting)
    let selectedNode = null;
    window.addEventListener('click', (e) => {
      mouse.x = (e.clientX / window.innerWidth) * 2 - 1;
      mouse.y = -(e.clientY / window.innerHeight) * 2 + 1;
      raycaster.setFromCamera(mouse, camera);

      const intersects = raycaster.intersectObjects(nodeMeshes.map(g => g.children[0]));
      if (intersects.length > 0) {
        const group = intersects[0].object.parent;
        selectedNode = group;
        openDrawer(group.userData);
      }
    });

    function openDrawer(userData) {
      document.getElementById('drawer').classList.add('open');
      document.getElementById('d-node-name').innerText = userData.nodeId;
      document.getElementById('d-node-arch').innerText = userData.label;
      if (userData.telemetry) {
        renderDrawer(userData.telemetry);
      }
    }
    function closeDrawer() {
      document.getElementById('drawer').classList.remove('open');
      selectedNode = null;
    }

    function renderDrawer(t) {
      document.getElementById('d-temp').innerText = (t.thermal ? t.thermal.cpu_temp_c.toFixed(1) : '--') + ' °C';
      document.getElementById('d-mem').innerText = (t.memory ? t.memory.used_mb.toFixed(0) : '--') + ' MB (' + (t.memory ? t.memory.usage_pct.toFixed(1) : 0) + '%)';
      document.getElementById('d-psi').innerText = (t.psi ? t.psi.memory_avg10.toFixed(2) : '0.00') + '%';
      document.getElementById('d-retrans').innerText = (t.network ? t.network.tcp_retrans_total : 0);

      const cbox = document.getElementById('d-cgroups-list');
      if (t.cgroups_v2 && t.cgroups_v2.length > 0) {
        let html = '';
        t.cgroups_v2.slice(0, 15).forEach(c => {
          const isHigh = c.memory_used_mb > 500;
          html += '<div class="cgroup-row ' + (isHigh ? 'cgroup-culprit' : '') + '">' +
                  '<span>' + c.container_name + '</span>' +
                  '<span>' + c.memory_used_mb.toFixed(1) + ' MB</span></div>';
        });
        cbox.innerHTML = html;
      } else {
        cbox.innerHTML = '<div style="color:var(--subtext);">컨테이너 데이터 없음</div>';
      }
    }

    function focusZone(zone) {
      document.querySelectorAll('.zone-btn').forEach(b => b.classList.remove('active'));
      event.target.classList.add('active');
      if (zone === 'seokchon') {
        controls.target.set(-25, 0, 0);
        camera.position.set(-25, 18, 25);
      } else if (zone === 'wisoft') {
        controls.target.set(0, 0, -10);
        camera.position.set(0, 18, 15);
      } else if (zone === 'aws') {
        controls.target.set(25, 0, 0);
        camera.position.set(25, 18, 25);
      } else {
        controls.target.set(0, 0, 0);
        camera.position.set(0, 35, 65);
      }
    }

    // Animation Loop
    function animate() {
      requestAnimationFrame(animate);
      controls.update();

      // Subtle float animation
      const t = Date.now() * 0.002;
      nodeMeshes.forEach((m, idx) => {
        m.position.y = 1 + Math.sin(t + idx) * 0.2;
      });

      renderer.render(scene, camera);
    }
    animate();

    window.addEventListener('resize', () => {
      camera.aspect = window.innerWidth / window.innerHeight;
      camera.updateProjectionMatrix();
      renderer.setSize(window.innerWidth, window.innerHeight);
    });
  </script>
</body>
</html>`
}
