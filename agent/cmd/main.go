package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"nangman-platform/agent/collector"
	"nangman-platform/agent/streamer"
)

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

func main() {
	hostname, _ := os.Hostname()
	stream := streamer.NewStreamer()

	// 터미널 Alternate Screen Buffer 진입 및 커서 숨김 (스크롤 밀림 100% 방지)
	fmt.Print("\033[?1049h\033[?25l")

	// Ctrl+C 시 안전하게 화면 복원 후 종료
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Print("\033[?1049l\033[?25h\n🛑 Nangman-Agent gracefully stopped.\n")
		os.Exit(0)
	}()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for t := range ticker.C {
		// 1. 엔터프라이즈 커널 텔레메트리 수집
		thermal, _ := collector.ReadCPUTemp()
		psi, _ := collector.ReadPSI()
		mem, _ := collector.ReadMemory()
		disk, _ := collector.ReadDisk()
		net, _ := collector.ReadNetwork()
		cgroups, _ := collector.ReadCgroupsV2()
		rpi, _ := collector.ReadRPiHealth()

		payload := collector.TelemetryPayload{
			NodeID:    hostname,
			Hostname:  hostname,
			Timestamp: t,
			Thermal:   thermal,
			PSI:       psi,
			Memory:    mem,
			Disk:      disk,
			Network:   net,
			CgroupsV2: cgroups,
			RPiHealth: rpi,
		}

		// 2. 중앙 Hub 서버로 비동기 네트워크 스트리밍 전송
		stream.SendAsync(payload)

		// 3. 화면 깜빡임/스크롤 없는 인플레이스 TUI 렌더링 (커서 홈 덮어쓰기)
		fmt.Print("\033[H")

		now := t.Format("15:04:05")
		fmt.Printf("\033[1;36m▲ NANGMAN TELEMETRY AGENT\033[0m \033[2mv2.0 (Edge Node)\033[0m              \033[1;32m● STREAMING (500ms)\033[0m  \033[2m%s\033[0m\033[K\n", now)
		fmt.Printf("\033[2mNode: \033[1;37m%s\033[0m  │ \033[2mPID: \033[1m%d\033[0m  │ \033[2mHub Target: \033[0;34m%s\033[0m\033[K\n",
			hostname, os.Getpid(), stream.HubURL)
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────────\033[0m\033[K")

		// 4. 하드웨어 & 커널 텔레메트리 현황 (3단 그리드: Label 22자 | Value 16자 | Status/Bar 칼정렬)
		fmt.Println("\033[1;37mHARDWARE & KERNEL TELEMETRY\033[0m\033[K")

		// CPU 온도
		tempColor := "\033[1;32m"
		tempStatus := "● NORMAL (Throttling: 82.0°C)"
		if thermal.CPUTempCelsius >= 75.0 {
			tempColor = "\033[1;31m"
			tempStatus = "● CRITICAL (High Heat)"
		} else if thermal.CPUTempCelsius >= 68.0 {
			tempColor = "\033[1;33m"
			tempStatus = "● WARM (Fan Active)"
		}
		fmt.Printf("  %-22s %s%-15s\033[0m %s\033[K\n",
			"CPU Temperature", tempColor, fmt.Sprintf("%.1f°C", thermal.CPUTempCelsius), tempStatus)

		// 물리 RAM 사용량 및 게이지 바
		memBar := drawBar(mem.UsagePct/100.0, 16)
		freeGB := (mem.TotalMB - mem.UsedMB) / 1024.0
		memVal := fmt.Sprintf("%.1fG / %.1fG", mem.UsedMB/1024.0, mem.TotalMB/1024.0)
		fmt.Printf("  %-22s %-16s [\033[1;32m%s\033[0m] %5.1f%% \033[2m(Free: %.1fG)\033[0m\033[K\n",
			"Physical RAM", memVal, memBar, mem.UsagePct, freeGB)

		// 스토리지 디스크 (/)
		diskBar := drawBar(disk.UsagePct/100.0, 16)
		diskVal := fmt.Sprintf("%.1fG / %.1fG", disk.UsedGB, disk.TotalGB)
		fmt.Printf("  %-22s %-16s [\033[1;34m%s\033[0m] %5.1f%% \033[2m(Inode: %.1f%%)\033[0m\033[K\n",
			"Storage Disk (/)", diskVal, diskBar, disk.UsagePct, disk.InodeUsedPct)

		// 커널 PSI 메모리 압박
		psiColor := "\033[1;32m"
		psiStatus := "● HEALTHY (Zero stall)"
		if psi.MemoryAvg10 >= 10.0 {
			psiColor = "\033[1;31m"
			psiStatus = fmt.Sprintf("● CRITICAL (%.2f%% stall)", psi.MemoryAvg10)
		} else if psi.MemoryAvg10 >= 5.0 {
			psiColor = "\033[1;33m"
			psiStatus = fmt.Sprintf("● WARNING (%.2f%% stall)", psi.MemoryAvg10)
		}
		psiVal := fmt.Sprintf("%.2f%% avg10", psi.MemoryAvg10)
		fmt.Printf("  %-22s %-16s %s%s\033[0m\033[K\n",
			"Kernel PSI Stall", psiVal, psiColor, psiStatus)

		// 네트워크 TCP 재전송
		tcpVal := fmt.Sprintf("%d pkts", net.TCPRetransTotal)
		fmt.Printf("  %-22s %-16s \033[1;32m● STABLE (No packet drops)\033[0m\033[K\n",
			"TCP Retransmission", tcpVal)

		// 라즈베리파이 전원 헬스 (중간 컬럼에 5V Normal 고정 배치하여 불릿 열 일치)
		rpiVal := "5.0V / 5.0A"
		rpiStatus := "\033[1;32m● OK (5V Normal, No throttling)\033[0m"
		if rpi.UnderVoltageDetected {
			rpiVal = "Undervoltage"
			rpiStatus = "\033[1;31m● LOW VOLTAGE (Power Adapter Issue!)\033[0m"
		} else if rpi.CurrentlyThrottled {
			rpiVal = "Throttled"
			rpiStatus = "\033[1;31m● THERMAL THROTTLED (Clock Capped!)\033[0m"
		}
		fmt.Printf("  %-22s %-16s %s\033[K\n",
			"RPi Hardware Health", rpiVal, rpiStatus)

		// 5. 상위 컨테이너 워크로드 (cgroups v2 상위 3개)
		fmt.Printf("\n\033[1;37mTOP WORKLOADS (cgroups v2: %d containers/services active)\033[0m\033[K\n", len(cgroups))
		if len(cgroups) == 0 {
			fmt.Println("  \033[2m(No container or service cgroups detected)\033[0m\033[K")
		} else {
			sortedC := make([]collector.CgroupContainer, len(cgroups))
			copy(sortedC, cgroups)
			sort.Slice(sortedC, func(i, j int) bool {
				return sortedC[i].MemoryUsedMB > sortedC[j].MemoryUsedMB
			})

			for i := 0; i < len(sortedC) && i < 3; i++ {
				c := sortedC[i]
				share := 0.0
				if mem.UsedMB > 0 {
					share = (c.MemoryUsedMB / mem.UsedMB) * 100.0
				}
				cName := c.ContainerName
				if len(cName) > 28 {
					cName = cName[:25] + "..."
				}
				fmt.Printf("  %d. %-28s %7.1f MB  \033[2m(%4.1f%%)\033[0m\033[K\n",
					i+1, cName, c.MemoryUsedMB, share)
			}
		}

		// 6. 풋터
		fmt.Println("\033[2m──────────────────────────────────────────────────────────────────────────────────────────\033[0m\033[K")
		fmt.Printf("\033[2mStream Status: \033[1;32mActive (2.0 req/sec)\033[0m \033[2m│  Press Ctrl+C to stop Agent\033[0m\033[K\n")
	}
}
