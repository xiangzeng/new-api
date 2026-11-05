package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// 消费日志异步写入（CONSUME_LOG_ASYNC=true 时启用）。
//
// 流式请求在客户端收到 message_stop 之后，handler 还要同步 INSERT 一条消费日志才返回，
// 连接也要等它写完才关。离数据库远的节点（美国从节点到香港库单次往返约 166ms，
// 一次 INSERT 实测 0.7–2.3s）会因此多挂一两秒。扣费早已在这之前完成，日志只是记录，
// 所以把这次 INSERT 交给后台写入协程。
//
// 不丢日志的约束：队列满时退回同步写；进程退出前 FlushConsumeLogs 把队列写完。

const (
	consumeLogQueueSize = 1000
	consumeLogWorkers   = 8
)

type pendingConsumeLog struct {
	log      *Log
	clientIp string
}

var (
	consumeLogQueue     chan pendingConsumeLog
	consumeLogStartOnce sync.Once
	consumeLogInFlight  sync.WaitGroup
)

func startConsumeLogWorkers() {
	consumeLogQueue = make(chan pendingConsumeLog, consumeLogQueueSize)
	for range consumeLogWorkers {
		go func() {
			for item := range consumeLogQueue {
				writeConsumeLogInBackground(item.log, item.clientIp)
				consumeLogInFlight.Done()
			}
		}()
	}
}

func enqueueConsumeLog(log *Log, clientIp string) {
	consumeLogStartOnce.Do(startConsumeLogWorkers)
	consumeLogInFlight.Add(1)
	select {
	case consumeLogQueue <- pendingConsumeLog{log: log, clientIp: clientIp}:
	default:
		// 队列满（数据库变慢或积压）：当场写，宁可这个请求慢一点也不丢日志
		writeConsumeLogInBackground(log, clientIp)
		consumeLogInFlight.Done()
	}
}

// writeConsumeLog 补上需要查用户设置才能决定的 IP 字段并落库。
func writeConsumeLog(log *Log, clientIp string) error {
	if settingMap, err := GetUserSetting(log.UserId, false); err == nil && settingMap.RecordIpLog {
		log.Ip = clientIp
	}
	return createLog(log)
}

// writeConsumeLogInBackground 用在拿不到请求 gin.Context 的地方，出错记系统日志（带请求 ID 便于追查）。
func writeConsumeLogInBackground(log *Log, clientIp string) {
	if err := writeConsumeLog(log, clientIp); err != nil {
		common.SysError(fmt.Sprintf("failed to record consume log (request %s, user %d, quota %d): %v",
			log.RequestId, log.UserId, log.Quota, err))
	}
}

// FlushConsumeLogs 等待已入队的消费日志全部写完，最多等 timeout。
// 在 HTTP 服务停止接收新请求之后调用；返回 false 表示超时，仍有日志没写完。
func FlushConsumeLogs(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		consumeLogInFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
