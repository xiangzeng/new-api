//go:build !linux

package model

import (
	"syscall"
	"time"
)

// setTCPUserTimeout 在非 Linux 平台不可用，仅依赖 TCP 保活做断线检测。
func setTCPUserTimeout(time.Duration) func(network, address string, conn syscall.RawConn) error {
	return nil
}
