package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var verifyIS = false
var ProxyPool []ProxyIp
var lock sync.Mutex
var mux2 sync.Mutex

var count int

func countAdd(i int) {
	mux2.Lock()
	count += i
	mux2.Unlock()

}
func countDel() {
	mux2.Lock()
	fmt.Printf("\r代理验证中: %d     ", count)
	count--
	mux2.Unlock()
}
func Verify(pi *ProxyIp, wg *sync.WaitGroup, ch chan int, first bool) {
	defer func() {
		wg.Done()
		countDel()
		<-ch
	}()

	pr := pi.Ip + ":" + pi.Port
	logDebug("开始验证代理: %s", pr)

	//是抓取验证，还是验证代理池内IP
	startT := time.Now()
	if first {
		if VerifyHttps(pr) {
			pi.Type = "HTTPS"
			logDebug("代理 %s 验证为HTTPS类型", pr)
		} else if VerifyHttp(pr) {
			pi.Type = "HTTP"
			logDebug("代理 %s 验证为HTTP类型", pr)
		} else if VerifySocket5(pr) {
			pi.Type = "SOCKET5"
			logDebug("代理 %s 验证为SOCKET5类型", pr)
		} else {
			logDebug("代理 %s 验证失败", pr)
			return
		}
		tc := time.Since(startT)
		pi.Time = time.Now().Format("2006-01-02 15:04:05")
		pi.Speed = tc.String()
		anonymity := Anonymity(pi, 0)
		if anonymity == "" {
			return
		}
		pi.Anonymity = anonymity
	} else {
		pi.RequestNum++
		if pi.Type == "HTTPS" {
			if VerifyHttps(pr) {
				pi.SuccessNum++
			}
		} else if pi.Type == "HTTP" {
			if VerifyHttp(pr) {
				pi.SuccessNum++
			}
		} else if pi.Type == "SOCKET5" {
			if VerifySocket5(pr) {
				pi.SuccessNum++
			}
		}
		tc := time.Since(startT)
		pi.Time = time.Now().Format("2006-01-02 15:04:05")
		pi.Speed = tc.String()
		return
	}
	tr := http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Timeout: 15 * time.Second, Transport: &tr}
	//处理返回结果
	res, err := client.Get("https://searchplugin.csdn.net/api/v1/ip/get?ip=" + pi.Ip)
	if err != nil {
		res, err = client.Get("https://searchplugin.csdn.net/api/v1/ip/get?ip=" + pi.Ip)
		if err != nil {
			return
		}
	}
	defer res.Body.Close()
	dataBytes, _ := io.ReadAll(res.Body)
	result := string(dataBytes)
	address := regexp.MustCompile("\"address\":\"(.+?)\",").FindAllStringSubmatch(result, -1)
	if len(address) != 0 {
		addresss := removeDuplication_map(strings.Split(address[0][1], " "))
		le := len(addresss)
		pi.Isp = strings.Split(addresss[le-1], "/")[0]
		for i := range addresss {
			if i == le-1 {
				break
			}
			switch i {
			case 0:
				pi.Country = addresss[0]
			case 1:
				pi.Province = addresss[1]
			case 2:
				pi.City = addresss[2]
			}
		}
	}

	pi.RequestNum = 1
	pi.SuccessNum = 1
	PIAdd(pi)
}
func VerifyHttp(pr string) bool {
	proxyUrl, proxyErr := url.Parse("http://" + pr)
	if proxyErr != nil {
		return false
	}
	tr := http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	tr.Proxy = http.ProxyURL(proxyUrl)
	client := http.Client{Timeout: 10 * time.Second, Transport: &tr}

	// 使用B站API进行验证
	request, _ := http.NewRequest("GET", "https://api.live.bilibili.com/xlive/web-room/v2/index/getRoomPlayInfo?protocol=1&format=1&codec=0&room_id=3", nil)
	request.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// 处理返回结果
	res, err := client.Do(request)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	// 读取响应内容
	dataBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false
	}

	// 验证响应内容是否包含预期的数据
	result := string(dataBytes)
	return strings.Contains(result, `"room_id": 23058`) || strings.Contains(result, `"room_id":23058`)
}
func VerifyHttps(pr string) bool {
	proxyUrl, proxyErr := url.Parse("http://" + pr)
	if proxyErr != nil {
		return false
	}
	tr := http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	tr.Proxy = http.ProxyURL(proxyUrl)
	client := http.Client{Timeout: 10 * time.Second, Transport: &tr}

	// 使用B站API进行验证
	request, _ := http.NewRequest("GET", "https://api.live.bilibili.com/xlive/web-room/v2/index/getRoomPlayInfo?protocol=1&format=1&codec=0&room_id=3", nil)
	request.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// 处理返回结果
	res, err := client.Do(request)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	// 读取响应内容
	dataBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false
	}

	// 验证响应内容是否包含预期的数据
	result := string(dataBytes)
	return strings.Contains(result, `"room_id": 23058`) || strings.Contains(result, `"room_id":23058`)
}

func VerifySocket5(pr string) bool {
	// 首先验证SOCKS5代理连接是否可用
	dialer, err := proxy.SOCKS5("tcp", pr, nil, proxy.Direct)
	if err != nil {
		return false
	}

	// 创建一个使用SOCKS5代理的HTTP客户端
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Dial:            dialer.Dial,
		},
		Timeout: 10 * time.Second,
	}

	// 使用B站API进行验证
	request, _ := http.NewRequest("GET", "https://api.live.bilibili.com/xlive/web-room/v2/index/getRoomPlayInfo?protocol=1&format=1&codec=0&room_id=3", nil)
	request.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// 处理返回结果
	res, err := httpClient.Do(request)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	// 读取响应内容
	dataBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false
	}

	// 验证响应内容是否包含预期的数据
	result := string(dataBytes)
	return strings.Contains(result, `"room_id": 23058`) || strings.Contains(result, `"room_id":23058`)
}
func Anonymity(pr *ProxyIp, c int) string {
	c++
	host := "http://httpbin.org/get"
	proxy := ""
	if pr.Type == "SOCKET5" {
		proxy = "socks5://" + pr.Ip + ":" + pr.Port
	} else {
		proxy = "http://" + pr.Ip + ":" + pr.Port
	}
	proxyUrl, proxyErr := url.Parse(proxy)
	if proxyErr != nil {
		if c >= 3 {
			return ""
		}
		return Anonymity(pr, c)
	}
	tr := http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Timeout: 15 * time.Second, Transport: &tr}
	tr.Proxy = http.ProxyURL(proxyUrl)
	request, err := http.NewRequest("GET", host, nil)
	if err != nil {
		if c >= 3 {
			return ""
		}
		return Anonymity(pr, c)
	}
	request.Header.Add("Proxy-Connection", "keep-alive")
	//处理返回结果
	res, err := client.Do(request)
	if err != nil {
		if c >= 3 {
			return ""
		}
		return Anonymity(pr, c)
	}
	defer res.Body.Close()
	dataBytes, _ := io.ReadAll(res.Body)
	result := string(dataBytes)
	if !strings.Contains(result, `"url": "http://httpbin.org/`) {
		if c == 3 {
			return ""
		}
		c++
		return Anonymity(pr, c)
	}
	origin := regexp.MustCompile(`(\d+?\.\d+?.\d+?\.\d+?,.+\d+?\.\d+?.\d+?\.\d+?)`).FindAllStringSubmatch(result, -1)
	if len(origin) != 0 {
		return "透明"
	}
	if strings.Contains(result, "keep-alive") {
		return "普匿"
	}
	return "高匿"
}

func PIAdd(pi *ProxyIp) {
	lock.Lock()
	defer lock.Unlock()
	for i := range ProxyPool {
		if ProxyPool[i].Ip == pi.Ip && ProxyPool[i].Port == pi.Port {
			return
		}
	}
	ProxyPool = append(ProxyPool, *pi)
	ProxyPool = uniquePI(ProxyPool)
}

func VerifyProxy() {
	if run {
		logWarning("代理抓取中, 无法进行代理验证")
		return
	}
	verifyIS = true

	logInfo("开始验证代理存活情况, 验证次数是当前代理数的5倍: %d", len(ProxyPool)*5)
	for i := range ProxyPool {
		ProxyPool[i].RequestNum = 0
		ProxyPool[i].SuccessNum = 0
	}
	count = len(ProxyPool) * 5

	for io := 0; io < 5; io++ {
		for i := range ProxyPool {
			wg3.Add(1)
			ch1 <- 1
			go Verify(&ProxyPool[i], &wg3, ch1, false)
		}
		time.Sleep(15 * time.Second)
	}
	wg3.Wait()
	lock.Lock()
	var pp []ProxyIp
	for i := range ProxyPool {
		if ProxyPool[i].SuccessNum != 0 {
			pp = append(pp, ProxyPool[i])
		}
	}
	ProxyPool = pp
	export()
	lock.Unlock()
	logInfo("代理验证结束, 当前可用IP数: %d", len(ProxyPool))
	verifyIS = false
}

func removeDuplication_map(arr []string) []string {
	set := make(map[string]struct{}, len(arr))
	j := 0
	for _, v := range arr {
		_, ok := set[v]
		if ok {
			continue
		}
		set[v] = struct{}{}
		arr[j] = v
		j++
	}

	return arr[:j]
}
