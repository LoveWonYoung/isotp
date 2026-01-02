package main

import (
	"log"
	"time"

	"github.com/LoveWonYoung/isotp/driver"
	"github.com/LoveWonYoung/isotp/tp"
	"github.com/LoveWonYoung/isotp/udsclient"
)

func main() {

	dev := driver.NewCanMix(driver.CANFD) // 或 driver.NewToomossCan()
	addr := tp.NewAddress(tp.Normal11bits, 0x7DF, 0x7E8, 0, 0, 0, 0, 0, false, false)
	var params = tp.NewParams()
	var len_ int = 8
	params.TxDataMinLength = &len_
	client, err := udsclient.NewUDSClient(dev, addr, params)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.RequestWithTimeout([]byte{0x22, 0xF1, 0x90}, 2*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("response: % X", resp)
}

/*
StMin: 发送 CF 间隔（ms，0..0xFF）。
BlockSize: 发送多少个 CF 后等待对端 FC（0 表示一直发）。
OverrideReceiverStMin: 可覆盖对端 FC 的 StMin（秒）；nil 表示按对端要求。
RxFlowControlTimeoutMs: 发送 FF 后等待对端 FC 的超时（ms）。
RxConsecutiveTimeoutMs: 等待下一帧 CF 的超时（ms）。
TxPadding: 可选填充字节（0..0xFF）；nil 不填充。
WftMax: 接收端允许的连续 WAIT(0x31) 次数。
TxDataLength: 每个 CAN 帧的有效数据长度上限（8/12/16/20/24/32/48/64）。
TxDataMinLength: 可选最小长度（同上集合）；nil 不强制。
MaxFrameSize: 允许的最大 ISO-TP 负载长度（字节）。
CanFD: 是否使用 CAN FD。
BitrateSwitch: FD 下是否启用 BRS。
DefaultTargetType: 默认寻址类型（tp.Physical 或 tp.Functional）。
RateLimitMaxBitrate: 速率限制的平均比特率上限（bit/s）。
RateLimitWindowSize: 速率限制的窗口大小（秒）。
RateLimitEnable: 是否启用速率限制。
ListenMode: 仅监听，不发送。
BlockingSend: Send 是否阻塞直到发送完成/超时。
WaitFunc: 用于内部 sleep 的可替换函数（默认 time.Sleep）。
*/
