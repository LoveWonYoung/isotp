/*
 * @Author: LoveWonYoung leeseoimnida@gmail.com
 * @Date: 2025-05-19 16:01:40
 * @LastEditors: LoveWonYoung leeseoimnida@gmail.com
 * @LastEditTime: 2025-06-26 11:09:14
 * @FilePath: \src\main.go
 * @Description: 这是默认设置,请设置`customMade`, 打开koroFileHeader查看配置 进行设置: https://github.com/OBKoro1/koro1FileHeader/wiki/%E9%85%8D%E7%BD%AE
 */
package main

import (
	"fmt"
	"gitee.com/lovewonyoung/CanMix/driver"
	"gitee.com/lovewonyoung/CanMix/log_recorder"
)

func main() {
	log_recorder.InitAndRotate("can_log_")

	// 1. 初始化CAN设备
	var canDevice driver.CANDriver = driver.NewCanMix(driver.CANFD)
	err := canDevice.Init()
	if err != nil {
		fmt.Println("CAN Init failed:", err)
		return
	}

	canDevice.Start()
	defer canDevice.Stop()

	// 2. 启动一个goroutine来处理接收到的CAN报文
	go func() {
		rxChan := canDevice.RxChan()
		for range rxChan {
		}
		fmt.Println("接收通道已关闭。")
	}()

	fmt.Println("CAN设备已启动，正在监听报文。程序将保持运行。")

	select {}
}
