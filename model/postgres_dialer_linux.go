//go:build linux

package model

import (
	"syscall"
	"time"
)

// setTCPUserTimeout 设置 TCP_USER_TIMEOUT：已发出的数据超过 timeout 仍未被确认就断开连接。
func setTCPUserTimeout(timeout time.Duration) func(network, address string, conn syscall.RawConn) error {
	return func(network, address string, conn syscall.RawConn) error {
		var sockErr error
		err := conn.Control(func(fd uintptr) {
			// 0x12 = TCP_USER_TIMEOUT（linux/tcp.h），syscall 包未导出该常量
			sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 0x12, int(timeout.Milliseconds()))
		})
		if err != nil {
			return err
		}
		return sockErr
	}
}
