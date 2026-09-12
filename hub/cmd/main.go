package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nangman-platform/agent/collector"
	"nangman-platform/hub/metrics"
	"nangman-platform/hub/rca"
	"nangman-platform/hub/store"
)

var (
	incidentMu      sync.Mutex
	recentIncidents []string
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

		// 0.001초 RCA 엔진 실행 (PSI 이상 감지 시 인시던트 큐에 추가)
		if report := rca.AnalyzePSI(payload); report != nil {
			incidentMu.Lock()
			recentIncidents = append([]string{fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), report.Summary)}, recentIncidents...)
			if len(recentIncidents) > 6 {
				recentIncidents = recentIncidents[:6]
			}
			incidentMu.Unlock()
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

	// 4. 3D 공간 디지털 트윈 관제소 (Three.js 기반 3D UI)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(get3DDashboardHTML()))
	})

	// 5. 모던 ANSI TUI 대시보드 백그라운드 렌더러 (1초마다 터미널 고정 화면 리프레시)
	go renderTUIDashboard(clusterStore)

	port := ":8080"
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("❌ Hub server error: %v\n", err)
	}
}

// renderTUIDashboard: k9s / htop 스타일의 사이버펑크 터미널 관제 대시보드
func renderTUIDashboard(clusterStore *store.ClusterStore) {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		nodes := clusterStore.GetAllNodes()
		now := time.Now().Format("15:04:05")

		// ANSI Clear Screen & Cursor to Home
		fmt.Print("\033[H\033[2J")

		// 1. 헤더 배너
		fmt.Println("\033[1;36m╔════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗\033[0m")
		fmt.Printf("\033[1;36m║\033[0m \033[1;37m🌌 NANGMAN HYBRID CLUSTER TELEMETRY HUB v2.0\033[0m \033[2m(Production Telemetry)\033[0m                   \033[1;32m● LIVE\033[0m  \033[1;33m%s\033[0m \033[1;36m║\033[0m\n", now)
		fmt.Printf("\033[1;36m║\033[0m \033[2mHub Ingest:\033[0m \033[1m172.16.0.31:8080\033[0m  │ \033[2mActive Telemetry Nodes:\033[0m \033[1;32m%d Online\033[0m  │ \033[2m3D Spatial Viewer:\033[0m \033[1;35mhttp://localhost:8080\033[0m   \033[1;36m║\033[0m\n", len(nodes))
		fmt.Println("\033[1;36m╚════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝\033[0m")

		// 2. 노드 텔레메트리 테이블 헤더
		fmt.Println("\033[1m┌──────────────────────────────────┬──────────┬──────────────────┬──────────────┬────────────┬─────────────────────────────┬──────────┐\033[0m")
		fmt.Println("\033[1m│ NODE HOSTNAME                    │ CPU TEMP │ RAM USAGE        │ PSI MEM(10s) │ TCP RETR   │ TOP CULPRIT (cgroups v2)    │ HEALTH   │\033[0m")
		fmt.Println("\033[1m├──────────────────────────────────┼──────────┼──────────────────┼──────────────┼────────────┼─────────────────────────────┼──────────┤\033[0m")

		if len(nodes) == 0 {
			fmt.Println("│ \033[2m(No nodes connected. Waiting for nangman-agent telemetry stream on :8080...)\033[0m                                             │")
		} else {
			for _, n := range nodes {
				// 온도 색상
				tempStr := fmt.Sprintf("%.1f°C", n.Thermal.CPUTempCelsius)
				if n.Thermal.CPUTempCelsius >= 80.0 {
					tempStr = fmt.Sprintf("\033[1;31m%6.1f°C\033[0m", n.Thermal.CPUTempCelsius)
				} else if n.Thermal.CPUTempCelsius >= 70.0 {
					tempStr = fmt.Sprintf("\033[1;33m%6.1f°C\033[0m", n.Thermal.CPUTempCelsius)
				} else {
					tempStr = fmt.Sprintf("\033[1;32m%6.1f°C\033[0m", n.Thermal.CPUTempCelsius)
				}

				// 메모리 사용량
				memStr := fmt.Sprintf("%.0fMB (%.1f%%)", n.Memory.UsedMB, n.Memory.UsagePct)

				// PSI 메모리 색상
				psiVal := n.PSI.MemoryAvg10
				psiStr := fmt.Sprintf("%.2f%%", psiVal)
				if psiVal >= 10.0 {
					psiStr = fmt.Sprintf("\033[1;31m%6.2f%% CRIT\033[0m", psiVal)
				} else if psiVal >= 5.0 {
					psiStr = fmt.Sprintf("\033[1;33m%6.2f%% WARN\033[0m", psiVal)
				} else {
					psiStr = fmt.Sprintf("\033[1;32m%6.2f%% NORM\033[0m", psiVal)
				}

				// 1위 점유 cgroup
				culpritStr := "-"
				if len(n.CgroupsV2) > 0 {
					topC := n.CgroupsV2[0]
					for _, c := range n.CgroupsV2 {
						if c.MemoryUsedMB > topC.MemoryUsedMB {
							topC = c
						}
					}
					shortName := topC.ContainerName
					if len(shortName) > 16 {
						shortName = shortName[:13] + "..."
					}
					culpritStr = fmt.Sprintf("%s (%.0fMB)", shortName, topC.MemoryUsedMB)
				}

				// 상태 배지
				status := "\033[1;32m🟢 NORMAL\033[0m"
				if n.Thermal.IsThrottled || psiVal >= 10.0 {
					status = "\033[1;31m🔴 CRIT  \033[0m"
				} else if psiVal >= 5.0 || n.Thermal.CPUTempCelsius >= 75.0 {
					status = "\033[1;33m🟡 WARN  \033[0m"
				}

				hostname := n.Hostname
				if len(hostname) > 32 {
					hostname = hostname[:29] + "..."
				}

				fmt.Printf("│ %-32s │ %s   │ %-16s │ %s │ %-10d │ %-27s │ %s │\n",
					hostname, tempStr, memStr, psiStr, n.Network.TCPRetransTotal, culpritStr, status)
			}
		}

		fmt.Println("\033[1m└──────────────────────────────────┴──────────┴──────────────────┴──────────────┴────────────┴─────────────────────────────┴──────────┘\033[0m")

		// 3. 실시간 인시던트 및 RCA 감사 로그 (최근 6건)
		fmt.Println("\033[1;33m📋 REAL-TIME INCIDENT & RCA AUDIT LOG (Kernel Bottleneck Detection)\033[0m")
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────\033[0m")
		incidentMu.Lock()
		if len(recentIncidents) == 0 {
			fmt.Println(" \033[2m• No active kernel pressure incidents detected. Cluster PSI is running smoothly.\033[0m")
		} else {
			for _, inc := range recentIncidents {
				fmt.Printf(" • %s\n", inc)
			}
		}
		incidentMu.Unlock()
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────\033[0m")
		fmt.Printf("\033[2m[Tip] Open 3D Spatial Viewer at \033[1;35mhttp://172.16.0.31:8080\033[0m \033[2m| Press Ctrl+C to terminate Hub\033[0m\n")
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
