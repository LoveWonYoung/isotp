# isotp

基于 ISO-TP 的 UDS 客户端实现，封装了 Toomoss CAN/CAN-FD 驱动的适配和 UDS 会话管理，提供可靠的请求/响应、负响应处理、自动重试和 Context 取消等能力。

本文档用法和配置命名参考了 Python 社区的 ISO-TP 库 [python-can-isotp](https://github.com/pylessard/python-can-isotp)，便于从其示例迁移到 Go。

## 主要特性
- 一站式 UDS 客户端：`NewUDSClient` 自动串联驱动、适配器、ISO-TP 协议栈及后台 goroutine。
- 负响应感知：将 `0x7F` 响应包装为 `UDSError`，支持 NRC 描述和可重试判定（`0x21`/`0x78` 自动重试）。
- 可配置请求：超时、最大重试次数、重试间隔可通过 `RequestOptions` 定制，支持 `Context` 取消。
- Response Pending 处理：收到 `0x78` 时自动延长等待时间，避免误报超时。
- 支持 CAN/CAN-FD：运行时可用 `SetFDMode` 切换 FD 模式。

## 安装
```bash
go get github.com/LoveWonYoung/isotp
```

## 快速开始
```go
package main

import (
	"log"
	"time"

	"github.com/LoveWonYoung/isotp/driver"
	"github.com/LoveWonYoung/isotp/tp"
	"github.com/LoveWonYoung/isotp/udsclient"
)

func main() {
	// 1) 初始化 CAN/CAN-FD 设备（示例为 Toomoss 驱动）
	dev := driver.NewCanMix(driver.CANFD)

	// 2) 配置寻址信息（11 位物理寻址，例：测试仪 0x7DF -> ECU 0x7E8）
	addr, err := tp.NewAddress(
		tp.Normal11Bit,
		tp.WithTxID(0x7DF),
		tp.WithRxID(0x7E8),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 3) 创建 UDS 客户端
	client, err := udsclient.NewUDSClient(dev, addr, tp.DefaultConfig())
	if err != nil {
		log.Fatalf("init uds client: %v", err)
	}
	defer client.Close()

	// 4) 发送 UDS 服务（例：ReadDataByIdentifier 0x22 F190）
	resp, err := client.RequestWithTimeout([]byte{0x22, 0xF1, 0x90}, 2*time.Second)
	if err != nil {
		log.Fatalf("uds request failed: %v", err)
	}
	log.Printf("response: % X", resp)
}
```

## API 速览
- `NewUDSClient(dev, addr, cfg)`：完成驱动启动、适配器连接和 ISO-TP 协议栈运行。
- `Request(payload []byte)` / `RequestWithTimeout`：简单请求；默认 `Timeout=500ms`、`MaxRetries=3`、`RetryDelay=100ms`。
- `RequestWithContext(ctx, payload, opts)`：可自定义超时/重试，并支持上层 `Context` 取消。
- `RequestFunctional` / `RequestFunctionalWithTimeout`：以功能寻址（Functional, 如 0x7DF 广播）发送请求。
- `UDSError`：负响应错误类型，包含 `ServiceID`、`NRC`、`Message`，`IsRetryable()` 用于判定是否自动重试。
- `SetFDMode(isFD bool)`：运行时切换 CAN FD。
- `Close()` / `IsClosed()`：关闭并回收资源。

### RequestOptions
```go
opts := udsclient.RequestOptions{
	Timeout:    2 * time.Second,
	MaxRetries: 5,
	RetryDelay: 200 * time.Millisecond,
}
resp, err := client.RequestWithContext(ctx, payload, opts)
```

## 寻址与协议栈配置
- 使用 `tp.NewAddress` 选择寻址模式（Normal/Extended/Mixed/29bit 等）并设置 Tx/Rx ID、TA/SA、地址扩展。
- `tp.DefaultConfig()` 提供标准超时和 Flow Control 参数，可按需要调整 `BlockSize`、`StMin`、`PaddingByte` 等。

## 对齐 python-can-isotp 的配置思路
- 寻址：`tp.NewAddress(tp.Normal11Bit, tp.WithTxID(...), tp.WithRxID(...))` 对应 python-can-isotp 的 `AddressingMode.Normal_11bits` + `txid`/`rxid`。
- 填充：`PaddingByte` 等价于 python-can-isotp 的 `tx_padding`/`rx_padding`。
- Flow Control：`BlockSize`、`StMin` 对应 python-can-isotp 的同名参数（`blocksize`、`stmin`）。
- 会话：Go 端通过 `tp.Transport` + `udsclient.UDSClient` 组合，等价于 python-can-isotp 的 `CanStack` + 上层 UDS 逻辑。

示例（与 python-can-isotp 示例的等价配置）：
```go
addr, _ := tp.NewAddress(
	tp.Normal11Bit, // python: AddressingMode.Normal_11bits
	tp.WithTxID(0x7DF),
	tp.WithRxID(0x7E8),
)
cfg := tp.DefaultConfig()
pad := byte(0x00)    // python: tx_padding=0x00
cfg.PaddingByte = &pad
cfg.BlockSize = 0    // python: blocksize=0
cfg.StMin = 20       // python: stmin=20 (ms)
client, _ := udsclient.NewUDSClient(dev, addr, cfg)
```

### 功能寻址（Functional Addressing）示例
```go
addr, _ := tp.NewAddress(
	tp.Normal11Bit,
	tp.WithTxID(0x7DF), // 功能寻址 ID
	tp.WithRxID(0x7E8), // 期望首个响应的物理 ID（如需）
)
client, _ := udsclient.NewUDSClient(dev, addr, tp.DefaultConfig())

// 以功能寻址方式发送 0x28 服务（通信控制），超时 1s
resp, err := client.RequestFunctionalWithTimeout([]byte{0x28, 0x00}, time.Second)
if err != nil {
	log.Fatalf("功能寻址请求失败: %v", err)
}
log.Printf("response: % X", resp)
```
> 功能寻址请求一般应为单帧（典型的广播诊断），本库会返回最先收到的一条响应。

## 测试
```bash
go test ./...
```
在非 Windows 平台会使用 Mock 驱动（`driver/mock_darwin.go`），无需真实硬件即可运行单元测试。
