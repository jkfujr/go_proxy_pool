package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var wg3 sync.WaitGroup
var mux1 sync.Mutex
var ch1 = make(chan int, 50)

func main() {
	// 初始化日志系统
	err := initLoggers()
	if err != nil {
		fmt.Printf("初始化日志系统失败: %v\n", err)
		os.Exit(1)
	}

	logInfo("代理池服务启动中...")

	// 加载配置
	err = loadConfig()
	if err != nil {
		logError("加载配置失败: %v", err)
		os.Exit(1)
	}

	// 设置日志级别
	if conf.Config.Debug {
		SetLogLevel(LogLevelDebug)
		logDebug("调试模式已启用")
	}

	// 加载代理池数据
	err = loadProxyPool()
	if err != nil {
		logWarning("加载代理池数据失败: %v", err)
	}

	// 启动定时任务
	go func() {
		for {
			// 检查代理池大小
			if len(ProxyPool) < conf.Config.ProxyNum {
				if !run && !verifyIS {
					logInfo("代理池数量不足 %d，开始抓取代理", conf.Config.ProxyNum)
					go spiderRun()
				}
			}
			time.Sleep(5 * time.Minute)
		}
	}()

	// 定时验证代理
	go func() {
		for {
			time.Sleep(time.Duration(conf.Config.VerifyTime) * time.Second)
			if !run && !verifyIS && len(ProxyPool) > 0 {
				logInfo("开始定时验证代理")
				go VerifyProxy()
			}
		}
	}()

	// 启动隧道代理
	go httpSRunTunnelProxyServer()
	go socketRunTunnelProxyServer()

	// 启动反向代理
	for name, rp := range conf.ReverseProxy {
		if rp.Enable {
			go startReverseProxy(name, rp)
		}
	}

	// 启动Web API服务器
	Run()
}

// 初始化
func InitData() {
	//获取配置文件
	GetConfigData()
	//设置线程数量
	ch1 = make(chan int, conf.Config.ThreadNum)
	ch2 = make(chan int, conf.Config.ThreadNum)
	//是否需要抓代理
	if len(ProxyPool) < conf.Config.ProxyNum {
		//抓取代理
		spiderRun()
	}

	// 启动反代
	go startReverseProxyServers()

	//定时判断是否需要获取代理iP
	go func() {
		// 每 60 秒钟时执行一次
		ticker := time.NewTicker(60 * time.Second)
		for range ticker.C {
			if len(ProxyPool) < conf.Config.ProxyNum {
				if !run && !verifyIS {
					mainLogger.Printf("代理数量不足 %d\n", conf.Config.ProxyNum)
					//抓取代理
					spiderRun()
				}
			} else {
				//保存代理到本地
				export()
			}
		}
	}()

	//定时更换隧道IP
	go func() {
		tunnelTime := time.Duration(conf.Config.TunnelTime)
		ticker := time.NewTicker(tunnelTime * time.Second)
		for range ticker.C {
			if len(ProxyPool) != 0 {
				httpsIp = getHttpsIp()
				httpIp = gethttpIp()
				socket5Ip = getSocket5Ip()
			}
		}
	}()

	// 验证代理存活情况
	go func() {
		verifyTime := time.Duration(conf.Config.VerifyTime)
		ticker := time.NewTicker(verifyTime * time.Second)
		for range ticker.C {
			if !verifyIS && !run {
				VerifyProxy()
			}
		}
	}()
}

// 加载配置文件
func loadConfig() error {
	// 获取配置文件
	GetConfigData()

	// 设置线程数量
	ch1 = make(chan int, conf.Config.ThreadNum)
	ch2 = make(chan int, conf.Config.ThreadNum)

	return nil
}

// 加载代理池数据
func loadProxyPool() error {
	// 从文件中加载代理池数据
	import_()

	// 如果代理池为空，记录日志
	if len(ProxyPool) == 0 {
		logWarning("代理池为空，将在后台自动抓取")
	} else {
		logInfo("成功加载 %d 个代理", len(ProxyPool))
	}

	return nil
}

// 从文件导入代理池数据
func import_() error {
	// 尝试打开数据文件
	file, err := os.Open("data.json")
	if err != nil {
		return err
	}
	defer file.Close()

	// 读取文件内容
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// 解析JSON数据到代理池
	if len(data) > 0 {
		err = json.Unmarshal(data, &ProxyPool)
		if err != nil {
			return err
		}
	}

	return nil
}

// 启动反向代理服务器
func startReverseProxy(name string, rp ReverseProxyConfig) {
	// 调用已有的反向代理启动函数
	startReverseProxyServer(name, rp)
}

// 启动SOCKS5隧道代理服务器
func socketRunTunnelProxyServer() {
	// 调用已有的SOCKS5隧道代理启动函数
	socket5RunTunnelProxyServer()
}
