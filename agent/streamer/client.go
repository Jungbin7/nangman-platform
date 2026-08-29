package streamer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"nangman-platform/agent/collector"
)

// Streamer: 중앙 Hub 서버로 텔레메트리를 전송하는 HTTP 클라이언트
type Streamer struct {
	HubURL     string
	HTTPClient *http.Client
}

// NewStreamer: 환경변수(HUB_URL) 또는 기본 로컬 주소로 스트리머를 초기화합니다.
func NewStreamer() *Streamer {
	hubURL := os.Getenv("HUB_URL")
	if hubURL == "" {
		hubURL = "http://localhost:8080/api/v1/telemetry"
	}

	return &Streamer{
		HubURL: hubURL,
		HTTPClient: &http.Client{
			Timeout: 1 * time.Second, // 타임아웃을 1초로 짧게 잡아 에이전트가 멈추지 않도록 보장
		},
	}
}

// SendAsync: 에이전트 메인 루프를 블로킹하지 않도록 고루틴(비동기)으로 전송합니다.
func (s *Streamer) SendAsync(payload collector.TelemetryPayload) {
	go func() {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return
		}

		req, err := http.NewRequest("POST", s.HubURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.HTTPClient.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()
}
