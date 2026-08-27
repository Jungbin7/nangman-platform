package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nangman-platform/agent/collector"
)

func main() {
	hostname, _ := os.Hostname()
	fmt.Println("==================================================================")
	fmt.Println("🚀 Nangman-Agent v1.0 (Kernel Telemetry & PSI Diagnostic Daemon)")
	fmt.Printf("📍 Node Hostname: %s | PID: %d\n", hostname, os.Getpid())
	fmt.Println("📡 Collecting /sys/thermal, /proc/pressure, /proc/meminfo (500ms interval)")
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

			// 2. JSON 직렬화 및 터미널 요약 출력
			jsonBytes, err := json.MarshalIndent(payload, "", "  ")
			if err == nil {
				fmt.Println(string(jsonBytes))
				fmt.Printf("--- [CPU: %.1f°C | Mem: %.1fMB(%.1f%%) | Disk: %.1fGB(%.1f%%, Inode: %.1f%%) | TCP Retrans: %d | Cgroups: %d] ---\n\n",
					thermal.CPUTempCelsius, mem.UsedMB, mem.UsagePct, disk.UsedGB, disk.UsagePct, disk.InodeUsedPct, net.TCPRetransTotal, len(cgroups))
			}
		}
	}
}
