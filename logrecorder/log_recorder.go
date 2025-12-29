package logrecorder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// NowString 返回当前时间格式为 "20060102 1504" 的字符串
func NowString() string {
	return time.Now().Format("20060102_1504")
}

// MakeDir 创建以日期命名的目录（如：2025_04_25）
func MakeDir() (string, error) {
	now := time.Now()
	dirName := fmt.Sprintf("%d_%02d_%02d", now.Year(), now.Month(), now.Day())
	fullPath := filepath.Join(".", dirName)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return "", fmt.Errorf("创建文件夹失败: %w", err)
		}
		fmt.Println("文件夹已创建:", fullPath)
	}

	return fullPath, nil
}

// RecorderAsNameInit 初始化日志记录器，name为日志文件前缀名
func RecorderAsNameInit(name string) error {
	log.SetPrefix("")
	log.SetFlags(log.Lmicroseconds)

	dir, err := MakeDir()
	if err != nil {
		return err
	}

	logPath := filepath.Join(dir, fmt.Sprintf("%s.log", name))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	log.SetOutput(f)
	return nil
}

// InitAndRotate 初始化日志记录器，并每10分钟轮换一次日志文件
func InitAndRotate(logName string) {
	MakeDir()
	// 立即执行一次，以创建初始日志文件
	err := RecorderAsNameInit(logName + NowString())
	if err != nil {
		log.Printf("初始日志记录器初始化失败: %v", err)
	}

	// 启动一个goroutine来处理日志轮换
	go func() {
		// 创建一个每10分钟触发一次的定时器
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// 无限循环，等待定时器触发
		for range ticker.C {
			// 每次触发时，使用新的时间戳重新初始化日志记录器
			err := RecorderAsNameInit(logName + NowString())
			if err != nil {
				// 如果重新初始化失败，记录错误信息到当前的日志输出
				log.Printf("日志轮换失败: %v", err)
			}
		}
	}()
}

func Init(logName string) {
	MakeDir()
	RecorderAsNameInit(logName + NowString())
}
