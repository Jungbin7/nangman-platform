.PHONY: build build-all clean run

# 바이너리 이름
BINARY_NAME=nangman-agent

# 기본 로컬 빌드
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME) ./cmd/main.go

# 라즈베리파이(ARM64) 및 x86_64 멀티 아키텍처 크로스 컴파일
build-all:
	@echo "🔨 Building for Linux x86_64 (Odyssey, VMs, EC2)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME)-linux-amd64 ./cmd/main.go
	@echo "🔨 Building for Linux ARM64 (Raspberry Pi 5, RaspbX)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY_NAME)-linux-arm64 ./cmd/main.go
	@echo "✅ All binaries built successfully!"

# 로컬 즉시 실행
run:
	go run ./cmd/main.go

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-linux-arm64
