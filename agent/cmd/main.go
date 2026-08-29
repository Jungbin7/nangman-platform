package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nangman-platform/agent/collector"
	"nangman-platform/agent/streamer"
)

func main() {
	hostname, _ := os.Hostname()
	stream := streamer.NewStreamer()

	fmt.Println("==================================================================")
	fmt.Println("🚀 Nangman-Agent v1.1 (Kernel Telemetry & Live Streaming Daemon)")
	fmt.Printf("📍 Node Hostname: %s | PID: %d\n", hostname, os.Getpid())
	fmt.Printf("📡 Streaming to Hub Target: %s (500ms interval)\n", stream.HubURL)
	fmt.Println("==================================================================")

	// Ctrl+C 시 안전하게 종료 처리
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\n🛑 Nangman-Agent safely stopped.")
			return
		case t := <-ticker.C:
			// 1. 엔터프라이즈 커널 텔레메트리 수집
			thermal, _ := collector.ReadCPUTemp()
			psi, _ := collector.ReadPSI()
			mem, _ := collector.ReadMemory()
			disk, _ := collector.ReadDisk()
			net, _ := collector.ReadNetwork()
			cgroups, _ := collector.ReadCgroupsV2()
			rpi, _ := collector.ReadRPiHealth()

			payload := collector.TelemetryPayload{
				NodeID:     hostname,
				Hostname:   hostname,
				Timestamp:  t,
				Thermal:    thermal,
				PSI:        psi,
				Memory:     mem,
				Disk:       disk,
				Network:    net,
				CgroupsV2:  cgroups,
				RPiHealth:  rpi,
			}

			// 2. 중앙 Hub 서버로 비동기 네트워크 스트리밍 전송
			stream.SendAsync(payload)

			// 3. 로컬 터미널 1줄 요약 출력
			fmt.Printf("[%s] CPU: %.1f°C | Mem: %.1fMB(%.1f%%) | Disk: %.1fGB(%.1f%%) | Retrans: %d | Cgroups: %d | 📡 Streaming OK\n",
				t.Format("15:04:05.000"), thermal.CPUTempCelsius, mem.UsedMB, mem.UsagePct, disk.UsedGB, disk.UsagePct, net.TCPRetransTotal, len(cgroups))
		}
	}
}
