SET CGO_ENABLED=0
SET GOOS=windows
SET GOARCH=amd64
go build -ldflags "-s -w" -o ./build/goProxyPool-windows-amd64.exe
pause