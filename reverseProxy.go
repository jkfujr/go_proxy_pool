package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var reverseProxyLock sync.Mutex
var reverseProxyMap = make(map[string]string)
var reverseProxyLoggers = make(map[string]*log.Logger)
var reverseProxyErrorLoggers = make(map[string]*log.Logger)
var reverseProxyRequestCounts = make(map[string]int)

// 启动所有配置的反向代理服务器
func startReverseProxyServers() {
	// 创建日志目录
	err := os.MkdirAll("logs/RP", 0755)
	if err != nil {
		log.Printf("创建日志目录失败: %v\n", err)
	}

	// 遍历配置中的所有反向代理
	for name, rp := range conf.ReverseProxy {
		if rp.Enable {
			// 为每个反向代理创建日志目录
			rpLogDir := filepath.Join("logs/RP", name)
			err := os.MkdirAll(rpLogDir, 0755)
			if err != nil {
				log.Printf("创建反向代理 %s 日志目录失败: %v\n", name, err)
				continue
			}

			// 创建访问日志文件
			accessLogFile, err := os.OpenFile(
				filepath.Join(rpLogDir, fmt.Sprintf("%s.log", name)),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND,
				0644,
			)
			if err != nil {
				log.Printf("创建反向代理 %s 访问日志文件失败: %v\n", name, err)
				continue
			}

			// 创建错误日志文件
			errorLogFile, err := os.OpenFile(
				filepath.Join(rpLogDir, fmt.Sprintf("%s_error.log", name)),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND,
				0644,
			)
			if err != nil {
				log.Printf("创建反向代理 %s 错误日志文件失败: %v\n", name, err)
				accessLogFile.Close()
				continue
			}

			// 创建日志记录器
			reverseProxyLoggers[name] = log.New(accessLogFile, "", log.LstdFlags)
			reverseProxyErrorLoggers[name] = log.New(errorLogFile, "", log.LstdFlags)

			// 初始化请求计数
			reverseProxyRequestCounts[name] = 0

			go startReverseProxyServer(name, rp)
		}
	}
}

// 启动单个反向代理服务器
func startReverseProxyServer(name string, rp ReverseProxyConfig) {
	targetURL, err := url.Parse(rp.URL)
	if err != nil {
		mainErrorLogger.Printf("反向代理 %s 目标URL解析错误: %v\n", name, err)
		return
	}

	// 初始化代理IP
	updateReverseProxyIP(name, rp)

	// 如果配置了基于时间的切换，则启动定时器
	if rp.TunnelTime > 0 {
		go func() {
			tunnelTime := time.Duration(rp.TunnelTime)
			ticker := time.NewTicker(tunnelTime * time.Second)
			for range ticker.C {
				if len(ProxyPool) != 0 {
					updateReverseProxyIP(name, rp)
					// 重置请求计数
					reverseProxyLock.Lock()
					reverseProxyRequestCounts[name] = 0
					reverseProxyLock.Unlock()
				}
			}
		}()
	}

	// 使用主日志记录器，而不是可能尚未初始化的特定反向代理日志记录器
	mainLogger.Printf("[INFO][反代][%s] 反向代理启动 - 监听端口: %s, 目标URL: %s", name, rp.ProxyPort, rp.URL)

	// 创建反向代理处理器
	director := func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// 如果目标URL有路径，则添加到请求路径前面
		if targetURL.Path != "" {
			if !strings.HasSuffix(targetURL.Path, "/") && !strings.HasPrefix(req.URL.Path, "/") {
				req.URL.Path = targetURL.Path + "/" + req.URL.Path
			} else {
				req.URL.Path = targetURL.Path + req.URL.Path
			}
		}

		// 保留原始请求的查询参数
		if targetURL.RawQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetURL.RawQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetURL.RawQuery + "&" + req.URL.RawQuery
		}
	}

	// 创建反向代理
	proxy := &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy: func(req *http.Request) (*url.URL, error) {
				// 获取当前代理IP
				proxyIP := getReverseProxyIP(name)
				if proxyIP == "" {
					return nil, nil
				}

				// 根据代理类型构建代理URL
				var proxyURL *url.URL
				var err error

				if strings.HasPrefix(proxyIP, "socks5://") {
					// 已经包含协议前缀的SOCKS5代理
					proxyURL, err = url.Parse(proxyIP)
				} else if strings.Contains(proxyIP, ".") {
					// 假设是HTTP/HTTPS代理
					proxyURL, err = url.Parse("http://" + proxyIP)
				} else {
					return nil, fmt.Errorf("无效的代理地址: %s", proxyIP)
				}

				if err != nil {
					mainErrorLogger.Printf("解析代理URL失败: %v\n", err)
					return nil, err
				}

				return proxyURL, nil
			},
			// 设置请求超时
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second, // 连接超时
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second, // TLS握手超时
			ResponseHeaderTimeout: 5 * time.Second, // 响应头超时
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 使用 mainErrorLogger 替代可能未初始化的 reverseProxyErrorLoggers[name]
			mainErrorLogger.Printf("[反代][%s] 请求错误: %v, URL: %s, 方法: %s, 来源: %s",
				name, err, r.URL.String(), r.Method, r.RemoteAddr)

			// 获取当前使用的代理IP
			currentProxyIP := getReverseProxyIP(name)

			// 检查是否是代理相关错误或超时错误
			errStr := err.Error()
			if strings.Contains(errStr, "proxyconnect") ||
				strings.Contains(errStr, "connection refused") ||
				strings.Contains(errStr, "no route to host") ||
				strings.Contains(errStr, "connection reset") ||
				strings.Contains(errStr, "i/o timeout") ||
				strings.Contains(errStr, "forbidden") ||
				strings.Contains(errStr, "Forbidden") ||
				strings.Contains(errStr, "TLS handshake timeout") ||
				strings.Contains(errStr, "EOF") ||
				strings.Contains(errStr, "forcibly closed") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "context canceled") {

				// 从代理池中删除当前代理
				if currentProxyIP != "" {
					// 使用 mainLogger 替代可能未初始化的专用日志记录器
					mainLogger.Printf("[INFO][反代][%s] 检测到代理错误，正在从代理池中删除代理: %s",
						name, currentProxyIP)
					removeProxyFromPool(currentProxyIP)
				}

				// 立即更新代理IP
				mainLogger.Printf("[INFO][反代][%s] 检测到代理错误，正在切换代理...", name)
				updateReverseProxyIP(name, rp)

				// 重置请求计数
				reverseProxyLock.Lock()
				reverseProxyRequestCounts[name] = 0
				reverseProxyLock.Unlock()

				// 返回错误信息给客户端，建议重试
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte("代理连接失败，请重试请求"))
				return
			}

			// 对于其他类型的错误，返回通用错误信息
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("服务器内部错误"))
		},
		ModifyResponse: func(resp *http.Response) error {
			// 记录成功的请求
			startTime := time.Now()
			clientIP := resp.Request.RemoteAddr
			method := resp.Request.Method
			path := resp.Request.URL.Path
			if resp.Request.URL.RawQuery != "" {
				path += "?" + resp.Request.URL.RawQuery
			}
			statusCode := resp.StatusCode
			contentLength := resp.ContentLength

			// 使用 mainLogger 而不是可能未初始化的专用日志记录器
			mainLogger.Printf("[反代][%s] %s - [%s] \"%s %s\" %d %d \"%.3fs\" \"%s\"",
				name,
				clientIP,
				startTime.Format("02/Jan/2006:15:04:05 -0700"),
				method,
				path,
				statusCode,
				contentLength,
				time.Since(startTime).Seconds(),
				resp.Request.UserAgent(),
			)

			// 只有在响应状态码为成功时才增加请求计数
			if statusCode >= 200 && statusCode < 400 && rp.RequestCount > 0 {
				reverseProxyLock.Lock()
				reverseProxyRequestCounts[name]++
				count := reverseProxyRequestCounts[name]
				reverseProxyLock.Unlock()

				mainLogger.Printf("[INFO][反代][%s] 成功请求计数: %d/%d", name, count, rp.RequestCount)

				if count >= rp.RequestCount {
					mainLogger.Printf("[INFO][反代][%s] 请求次数达到 %d，正在切换代理...", name, rp.RequestCount)
					updateReverseProxyIP(name, rp)

					// 重置请求计数
					reverseProxyLock.Lock()
					reverseProxyRequestCounts[name] = 0
					reverseProxyLock.Unlock()
				}
			}

			return nil
		},
	}

	// 启动HTTP服务器
	server := &http.Server{
		Addr:    conf.Config.Ip + ":" + rp.ProxyPort,
		Handler: proxy,
	}

	err = server.ListenAndServe()
	if err != nil {
		mainErrorLogger.Printf("反向代理 %s 启动失败: %v\n", name, err)
	}
}

// 更新反向代理使用的代理IP
func updateReverseProxyIP(name string, rp ReverseProxyConfig) {
	reverseProxyLock.Lock()
	defer reverseProxyLock.Unlock()

	// 获取当前使用的代理IP，以便避免重复选择
	currentProxyIP := reverseProxyMap[name]

	var proxyIP string
	var attempts int = 0
	maxAttempts := 3 // 最多尝试3次获取不同的代理

	for proxyIP == "" || (proxyIP == currentProxyIP && attempts < maxAttempts) {
		attempts++

		switch rp.ProxyType {
		case "HTTP":
			proxyIP = gethttpIp()
		case "HTTPS":
			proxyIP = getHttpsIp()
		case "SOCKS5":
			proxyIP = getSocket5Ip()
		case "ALL":
			// 尝试按优先级获取代理
			proxyIP = getHttpsIp()
			if proxyIP == "" {
				proxyIP = gethttpIp()
			}
			if proxyIP == "" {
				proxyIP = getSocket5Ip()
			}
		}

		// 如果尝试多次后仍然获取到相同的代理，就接受它
		if attempts >= maxAttempts {
			break
		}
	}

	if proxyIP != "" {
		if proxyIP != currentProxyIP {
			logReverseProxyInfo(name, "更新代理IP: %s -> %s", currentProxyIP, proxyIP)
		} else {
			logReverseProxyInfo(name, "无法获取新代理，继续使用: %s", proxyIP)
		}
		reverseProxyMap[name] = proxyIP
	} else {
		logReverseProxyError(name, "无可用代理IP")
	}
}

// 获取反向代理当前使用的代理IP
func getReverseProxyIP(name string) string {
	reverseProxyLock.Lock()
	defer reverseProxyLock.Unlock()
	return reverseProxyMap[name]
}

// 从代理池中删除指定的代理
func removeProxyFromPool(proxyIP string) {
	if proxyIP == "" {
		return
	}

	// 分割代理IP和端口
	parts := strings.Split(proxyIP, ":")
	if len(parts) != 2 {
		mainErrorLogger.Printf("无效的代理格式: %s\n", proxyIP)
		return
	}

	ip := parts[0]
	port := parts[1]

	// 遍历代理池，删除匹配的代理
	for i := 0; i < len(ProxyPool); i++ {
		if ProxyPool[i].Ip == ip && ProxyPool[i].Port == port {
			mainLogger.Printf("从代理池中删除代理: %s:%s\n", ip, port)

			// 删除代理
			if i+1 < len(ProxyPool) {
				ProxyPool = append(ProxyPool[:i], ProxyPool[i+1:]...)
			} else {
				ProxyPool = ProxyPool[:i]
			}

			// 保存更新后的代理池到文件
			export()
			return
		}
	}

	mainLogger.Printf("代理 %s 未在代理池中找到\n", proxyIP)
}
