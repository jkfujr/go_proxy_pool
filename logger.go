package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	// 主程序日志记录器
	mainLogger      *log.Logger
	mainErrorLogger *log.Logger
)

// 自定义日志格式
func logFormat(level string, format string, v ...interface{}) string {
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	message := fmt.Sprintf(format, v...)
	return fmt.Sprintf("%s [%s] %s", timestamp, level, message)
}

// 反向代理日志格式
func logReverseProxyFormat(name string, level string, format string, v ...interface{}) string {
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	message := fmt.Sprintf(format, v...)
	return fmt.Sprintf("%s [%s][反代][%s] %s", timestamp, level, name, message)
}

// 日志记录函数
func logInfo(format string, v ...interface{}) {
	mainLogger.Output(2, logFormat("INFO", format, v...))
}

func logError(format string, v ...interface{}) {
	mainErrorLogger.Output(2, logFormat("ERROR", format, v...))
}

func logWarning(format string, v ...interface{}) {
	mainLogger.Output(2, logFormat("WARN", format, v...))
}

// 日志级别常量
const (
	LogLevelDebug = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

// 当前日志级别，默认为 Info
var currentLogLevel = LogLevelInfo

// 设置日志级别
func SetLogLevel(level int) {
	currentLogLevel = level
}

// 调试日志函数，仅在调试级别时输出
func logDebug(format string, v ...interface{}) {
	if currentLogLevel <= LogLevelDebug {
		mainLogger.Output(2, logFormat("DEBUG", format, v...))
	}
}

// 反向代理日志函数
func logReverseProxyInfo(name string, format string, v ...interface{}) {
	mainLogger.Output(2, logReverseProxyFormat(name, "INFO", format, v...))
}

func logReverseProxyError(name string, format string, v ...interface{}) {
	mainErrorLogger.Output(2, logReverseProxyFormat(name, "ERROR", format, v...))
}

// 初始化日志系统
func initLoggers() error {
	// 创建日志目录
	err := os.MkdirAll("logs", 0755)
	if err != nil {
		return err
	}

	// 打开或创建主日志文件
	mainLogFile, err := os.OpenFile(
		filepath.Join("logs", "main.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return err
	}

	// 打开或创建错误日志文件
	mainErrorLogFile, err := os.OpenFile(
		filepath.Join("logs", "error.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		mainLogFile.Close()
		return err
	}

	// 创建多输出写入器，同时写入文件和标准输出
	mainLogMultiWriter := io.MultiWriter(os.Stdout, mainLogFile)
	mainErrorMultiWriter := io.MultiWriter(os.Stderr, mainErrorLogFile)

	// 初始化日志记录器 - 使用空格式，因为我们会自定义格式
	mainLogger = log.New(mainLogMultiWriter, "", 0)
	mainErrorLogger = log.New(mainErrorMultiWriter, "", 0)

	// 替换标准日志记录器
	log.SetOutput(mainLogMultiWriter)
	log.SetFlags(0)
	log.SetPrefix("")

	return nil
}
