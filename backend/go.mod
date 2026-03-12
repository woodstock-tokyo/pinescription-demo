module github.com/woodstock-tokyo/pinescription-demo

go 1.22

// To swap in the real pinescription library once available:
// replace github.com/tsuz/go-pine => github.com/woodstock-tokyo/pinescription v<version>

require (
	github.com/gorilla/websocket v1.5.3
	github.com/tsuz/go-pine v0.0.0-20230617070221-e9734852d2af
)

require (
	github.com/pkg/errors v0.9.1 // indirect
	github.com/twinj/uuid v1.0.0 // indirect
)
