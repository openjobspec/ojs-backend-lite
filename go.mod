module github.com/openjobspec/ojs-backend-lite

go 1.24.0

require (
	github.com/go-chi/chi/v5 v5.2.5
	github.com/openjobspec/ojs-go-backend-common v0.0.0
	github.com/openjobspec/ojs-proto v0.0.0-00010101000000-000000000000
	github.com/robfig/cron/v3 v3.0.1
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
	modernc.org/sqlite v1.46.0
	nhooyr.io/websocket v1.8.17
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/openjobspec/ojs-go-backend-common => ../ojs-go-backend-common

replace github.com/openjobspec/ojs-proto => ../ojs-proto
