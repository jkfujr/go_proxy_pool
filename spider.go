package main

import (
	"bufio"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var wg sync.WaitGroup
var wg2 sync.WaitGroup
var ch2 = make(chan int, 50)

// 是否抓取中
var run = false

func spiderRun() {
	run = true
	defer func() {
		run = false
	}()

	count = 0
	logInfo("开始抓取代理...")
	for i := range conf.Spider {
		wg2.Add(1)
		go spider(&conf.Spider[i])
	}
	wg2.Wait()
	logInfo("代理抓取结束")

	count = 0
	logInfo("开始扩展抓取代理...")
	for i := range conf.SpiderPlugin {
		wg2.Add(1)
		go spiderPlugin(&conf.SpiderPlugin[i])
	}
	wg2.Wait()
	logInfo("扩展代理抓取结束")

	count = 0
	logInfo("开始文件抓取代理...")
	for i := range conf.SpiderFile {
		wg2.Add(1)
		go spiderFile(&conf.SpiderFile[i])
	}
	wg2.Wait()
	logInfo("文件代理抓取结束")

	//导出代理到文件
	export()
}

func spider(sp *Spider) {
	defer func() {
		wg2.Done()
		//mainLogger.Printf("%s 结束...",sp.Name)
	}()
	//mainLogger.Printf("%s 开始...", sp.Name)
	urls := strings.Split(sp.Urls, ",")
	var pis []ProxyIp
	for ui, v := range urls {
		if ui != 0 {
			time.Sleep(time.Duration(sp.Interval) * time.Second)
		}
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		if sp.ProxyIs {
			proxyUrl, parseErr := url.Parse("http://" + conf.Proxy.Host + ":" + conf.Proxy.Port)
			if parseErr != nil {
				mainErrorLogger.Println("代理地址错误: \n" + parseErr.Error())
				continue
			}
			tr.Proxy = http.ProxyURL(proxyUrl)
		}
		client := http.Client{Timeout: 20 * time.Second, Transport: tr}
		request, _ := http.NewRequest(sp.Method, v, strings.NewReader(sp.Body))
		//设置请求头
		SetHeadersConfig(sp.Headers, &request.Header)
		//处理返回结果
		res, err := client.Do(request)
		if err != nil {
			continue
		}
		dataBytes, _ := io.ReadAll(res.Body)
		result := string(dataBytes)
		ip := regexp.MustCompile(sp.Ip).FindAllStringSubmatch(result, -1)
		port := regexp.MustCompile(sp.Port).FindAllStringSubmatch(result, -1)
		if len(ip) == 0 {
			continue
		}
		for i := range ip {
			var _ip string
			var _port string
			_ip, _ = url.QueryUnescape(ip[i][1])
			_port, _ = url.QueryUnescape(port[i][1])
			_is := true
			for pi := range ProxyPool {
				if ProxyPool[pi].Ip == _ip && ProxyPool[pi].Port == _port {
					_is = false
					break
				}
			}
			if _is {
				pis = append(pis, ProxyIp{Ip: _ip, Port: _port, Source: sp.Name})
			}
		}
	}
	pis = uniquePI(pis)
	countAdd(len(pis))
	for i := range pis {
		wg.Add(1)
		ch2 <- 1
		go Verify(&pis[i], &wg, ch2, true)
	}
}

func spiderPlugin(spp *SpiderPlugin) {
	defer func() {
		wg2.Done()
	}()
	cmd := exec.Command(spp.Run)

	// 获取命令的标准输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logError("插件 %s 执行失败: %v", spp.Name, err)
		return
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		logError("插件 %s 启动失败: %v", spp.Name, err)
		return
	}

	// 读取输出
	output, err := io.ReadAll(stdout)
	if err != nil {
		logError("插件 %s 读取输出失败: %v", spp.Name, err)
		return
	}

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		logError("插件 %s 执行过程中出错: %v", spp.Name, err)
		return
	}

	// 处理输出
	var pis []ProxyIp
	lines := strings.Split(string(output), ",")
	for _, line := range lines {
		if line == "" {
			continue
		}

		split := strings.Split(strings.TrimSpace(line), ":")
		if len(split) != 2 {
			continue
		}

		_is := true
		for pi := range ProxyPool {
			if ProxyPool[pi].Ip == split[0] && ProxyPool[pi].Port == split[1] {
				_is = false
				break
			}
		}

		if _is {
			pis = append(pis, ProxyIp{Ip: split[0], Port: split[1], Source: spp.Name})
		}
	}

	pis = uniquePI(pis)
	countAdd(len(pis))

	for i := range pis {
		wg.Add(1)
		ch2 <- 1
		go Verify(&pis[i], &wg, ch2, true)
	}
}

func spiderFile(spp *SpiderFile) {
	defer func() {
		wg2.Done()
	}()
	var pis []ProxyIp
	fi, err := os.Open(spp.Path)
	if err != nil {
		logError("文件 %s 打开失败: %v", spp.Name, err)
		return
	}
	r := bufio.NewReader(fi) // 创建 Reader
	for {
		_is := true
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			split := strings.Split(strings.TrimSpace(string(line)), ":")
			for pi := range ProxyPool {
				if ProxyPool[pi].Ip == split[0] && ProxyPool[pi].Port == split[1] {
					_is = false
					break
				}
			}
			if _is {
				pis = append(pis, ProxyIp{Ip: split[0], Port: split[1], Source: spp.Name})
			}
		}
		if err != nil {
			break
		}
	}
	pis = uniquePI(pis)
	countAdd(len(pis))
	for i := range pis {
		wg.Add(1)
		ch2 <- 1
		go Verify(&pis[i], &wg, ch2, true)
	}
	wg.Wait()
}
