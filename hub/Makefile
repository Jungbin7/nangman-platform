.PHONY: run build build-linux

# 허브 로컬 즉시 실행
run:
	go run ./cmd/main.go

# 허브 바이너리 로컬 빌드
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o nangman-hub ./cmd/main.go

# Wisoft 연구실 VM / x86_64 리눅스 서버용 바이너리 빌드
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o nangman-hub-linux-amd64 ./cmd/main.go

