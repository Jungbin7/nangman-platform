package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// ReadNetwork: /proc/net/dev(대역폭)와 /proc/net/snmp(TCP 재전송/에러)를 파싱합니다.
func ReadNetwork() (NetworkMetrics, error) {
	metrics := NetworkMetrics{}

	// 1. /proc/net/dev (인터페이스별 RX/TX 바이트 합산)
	if file, err := os.Open("/proc/net/dev"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, ":") {
				parts := strings.Split(line, ":")
				ifName := strings.TrimSpace(parts[0])
				if ifName == "lo" {
					continue // Loopback 제외
				}
				fields := strings.Fields(parts[1])
				if len(fields) >= 9 {
					rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
					txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
					metrics.RxBytesSec += rxBytes
					metrics.TxBytesSec += txBytes
				}
			}
		}
	}

	// 2. /proc/net/snmp (TCP RetransSegs & InErrs 파싱)
	if file, err := os.Open("/proc/net/snmp"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		var isTCPHeaderNext bool
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "Tcp:") {
				if !isTCPHeaderNext {
					isTCPHeaderNext = true
					continue
				}
				// 두 번째 Tcp: 라인이 실제 카운터 값
				fields := strings.Fields(line)
				if len(fields) >= 15 {
					// Index 12: RetransSegs, Index 14: InErrs
					retrans, _ := strconv.ParseUint(fields[12], 10, 64)
					inErrs, _ := strconv.ParseUint(fields[14], 10, 64)
					metrics.TCPRetransTotal = retrans
					metrics.TCPErrors = inErrs
				}
				break
			}
		}
	}

	return metrics, nil
}
