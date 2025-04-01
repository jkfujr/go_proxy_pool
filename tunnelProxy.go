package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var lock2 sync.Mutex
var httpI []ProxyIp
var httpS []ProxyIp
var socket5 []ProxyIp

var httpIp string
var httpsIp string
var socket5Ip string

func httpSRunTunnelProxyServer() {
	httpsIp = getHttpsIp()
	httpIp = gethttpIp()

	logInfo("HTTP 隧道代理启动 - 监听IP端口 -> %s", conf.Config.Ip+":"+conf.Config.HttpTunnelPort)

	server := &http.Server{
		Addr:      conf.Config.Ip + ":" + conf.Config.HttpTunnelPort,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleTunneling(w, r)
			} else {
				handleHTTP(w, r)
			}
		}),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	err := server.ListenAndServe()
	if err != nil {
		logError("HTTP 隧道代理启动失败: %v", err)
	}
}

func socket5RunTunnelProxyServer() {
	socket5Ip = getSocket5Ip()
	logInfo("SOCKS5 隧道代理启动 - 监听IP端口 -> %s", conf.Config.Ip+":"+conf.Config.SocketTunnelPort)
	li, err := net.Listen("tcp", conf.Config.Ip+":"+conf.Config.SocketTunnelPort)
	if err != nil {
		logError("SOCKET5 隧道代理启动失败: %v", err)
	}
	for {
		clientConn, err := li.Accept()
		if err != nil {
			logError("SOCKET5 隧道代理接受连接失败: %v", err)
		}
		go func() {
			logInfo("隧道代理 | SOCKET5 请求 使用代理: %s", socket5Ip)
			if clientConn == nil {
				return
			}
			defer clientConn.Close()
			destConn, err := net.DialTimeout("tcp", socket5Ip, 30*time.Second)
			if err != nil {
				logError("SOCKET5 隧道代理连接到代理失败: %v", err)
				return
			}
			defer destConn.Close()

			go io.Copy(destConn, clientConn)
			io.Copy(clientConn, destConn)
		}()
	}
}

// MergeArray 合并数组
func MergeArray(dest []byte, src []byte) (result []byte) {
	result = make([]byte, len(dest)+len(src))
	//将第一个数组传入result
	copy(result, dest)
	//将第二个数组接在尾部，也就是 len(dest):
	copy(result[len(dest):], src)
	return
}

func gethttpIp() string {
	lock2.Lock()
	defer lock2.Unlock()
	if len(ProxyPool) == 0 {
		return ""
	}
	for _, v := range ProxyPool {
		if v.Type == "HTTP" {
			is := true
			for _, vv := range httpI {
				if v.Ip == vv.Ip && v.Port == vv.Port {
					is = false
				}
			}
			if is {
				httpI = append(httpI, v)
				return v.Ip + ":" + v.Port
			}
		}
	}
	var addr string
	if len(httpI) != 0 {
		addr = httpI[0].Ip + ":" + httpI[0].Port
	}
	httpI = make([]ProxyIp, 0)
	if addr == "" {
		addr = httpsIp
	}
	return addr
}

func getHttpsIp() string {
	lock2.Lock()
	defer lock2.Unlock()
	if len(ProxyPool) == 0 {
		return ""
	}
	for _, v := range ProxyPool {
		if v.Type == "HTTPS" {
			is := true
			for _, vv := range httpS {
				if v.Ip == vv.Ip && v.Port == vv.Port {
					is = false
				}
			}
			if is {
				httpS = append(httpS, v)
				return v.Ip + ":" + v.Port
			}
		}
	}
	var addr string
	if len(httpS) != 0 {
		addr = httpS[0].Ip + ":" + httpS[0].Port
	}
	httpS = make([]ProxyIp, 0)
	return addr
}
func getSocket5Ip() string {
	lock2.Lock()
	defer lock2.Unlock()
	if len(ProxyPool) == 0 {
		return ""
	}
	for _, v := range ProxyPool {
		if v.Type == "SOCKET5" {
			is := true
			for _, vv := range socket5 {
				if v.Ip == vv.Ip && v.Port == vv.Port {
					is = false
				}
			}
			if is {
				socket5 = append(socket5, v)
				return v.Ip + ":" + v.Port
			}
		}
	}
	var addr string
	if len(socket5) != 0 {
		addr = socket5[0].Ip + ":" + socket5[0].Port
	}
	socket5 = make([]ProxyIp, 0)
	return addr
}

// 添加隧道代理日志函数
func logTunnelInfo(tunnelType string, format string, v ...interface{}) {
	mainLogger.Output(2, logFormat("TUNNEL", fmt.Sprintf("[%s] %s", tunnelType, fmt.Sprintf(format, v...))))
}

func logTunnelError(tunnelType string, format string, v ...interface{}) {
	mainErrorLogger.Output(2, logFormat("TUNNEL", fmt.Sprintf("[%s] %s", tunnelType, fmt.Sprintf(format, v...))))
}

// 处理 HTTPS 隧道连接
func handleTunneling(w http.ResponseWriter, r *http.Request) {
	logTunnelInfo("HTTPS", "请求：%s 使用代理: %s", r.URL.String(), httpsIp)

	// 连接到目标代理服务器
	destConn, err := net.DialTimeout("tcp", httpsIp, 20*time.Second)
	if err != nil {
		logTunnelError("HTTPS", "连接代理失败: %v", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// 设置读取超时
	destConn.SetReadDeadline(time.Now().Add(20 * time.Second))

	// 构建请求
	var req []byte
	req = MergeArray([]byte(fmt.Sprintf("%s %s %s%s", r.Method, r.Host, r.Proto, []byte{13, 10})), []byte(fmt.Sprintf("Host: %s%s", r.Host, []byte{13, 10})))

	// 添加请求头
	for k, v := range r.Header {
		req = MergeArray(req, []byte(fmt.Sprintf(
			"%s: %s%s", k, v[0], []byte{13, 10})))
	}

	// 添加空行，表示头部结束
	req = MergeArray(req, []byte{13, 10})

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err == nil && len(body) > 0 {
		req = MergeArray(req, body)
	}

	// 发送请求到代理服务器
	destConn.Write(req)

	// 设置响应状态码
	w.WriteHeader(http.StatusOK)

	// 获取底层连接
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		logTunnelError("HTTPS", "不支持 Hijacker 接口")
		http.Error(w, "不支持隧道连接", http.StatusInternalServerError)
		return
	}

	// 接管连接
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		logTunnelError("HTTPS", "Hijack 失败: %v", err)
		return
	}

	// 设置客户端连接超时
	clientConn.SetReadDeadline(time.Now().Add(20 * time.Second))

	// 先读取一次，清空缓冲区
	destConn.Read(make([]byte, 1024))

	// 双向转发数据
	go io.Copy(destConn, clientConn)
	go io.Copy(clientConn, destConn)
}

// 处理普通 HTTP 请求
func handleHTTP(w http.ResponseWriter, r *http.Request) {
	logTunnelInfo("HTTP", "请求：%s 使用代理: %s", r.URL.String(), httpIp)

	// 创建 HTTP 客户端
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// 配置代理
	proxyUrl, parseErr := url.Parse("http://" + httpIp)
	if parseErr != nil {
		logTunnelError("HTTP", "解析代理URL失败: %v", parseErr)
		http.Error(w, "代理配置错误", http.StatusInternalServerError)
		return
	}
	tr.Proxy = http.ProxyURL(proxyUrl)

	// 创建客户端
	client := &http.Client{Timeout: 20 * time.Second, Transport: tr}

	// 创建新请求
	request, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		logTunnelError("HTTP", "创建请求失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 复制请求头
	request.Header = r.Header

	// 发送请求
	res, err := client.Do(request)
	if err != nil {
		logTunnelError("HTTP", "请求失败: %v", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer res.Body.Close()

	// 复制响应头
	for k, vv := range res.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// 读取响应体
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		logTunnelError("HTTP", "读取响应体失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应状态码
	w.WriteHeader(res.StatusCode)

	// 写入响应体
	w.Write(bodyBytes)
}
