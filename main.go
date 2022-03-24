package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
)

/**
 * @Author  Flagship
 * @Date  2022/3/24 13:17
 * @Description
 */

const (
	kUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.82 Safari/537.36"
	ProgressBarStr = "\rDownload progress: [%s] %.2f%%"
)

type HttpDownloader struct {
	url           string
	filename      string
	contentLength int
	acceptRanges  bool // 是否支持断点续传
	numRoutines   int  // 同时下载线程数
}

//NewDownloader 下载器构造方法，传入下载链接和线程数
func NewDownloader(url string, numThreads int) *HttpDownloader {
	// 得到文件名
	filename := path.Base(url)

	// 请求url获取header
	resp, err := http.Head(url)
	if err != nil {
		log.Fatal("请求失败")
		return nil
	}

	httpDownload := &HttpDownloader{}
	httpDownload.url = url
	httpDownload.contentLength = int(resp.ContentLength)
	httpDownload.numRoutines = numThreads
	httpDownload.filename = filename

	// 是否支持断点续传
	httpDownload.acceptRanges = len(resp.Header["Accept-Ranges"]) != 0 && resp.Header["Accept-Ranges"][0] == "bytes"

	return httpDownload
}

//Download 下载文件方法
func (h *HttpDownloader) Download() {
	// 创建文件
	f, err := os.Create(h.filename)
	if err != nil {
		log.Fatalf("Failed to create file, err: %s", err.Error())
	}
	defer f.Close()

	// 根据是否支持断点下载进行不同的下载方式
	if h.acceptRanges == false {
		fmt.Println("This file does not support multi-coroutine download, now download with common way...")
		resp, err := http.Get(h.url)
		if err != nil {
			log.Fatalf("Failed to request the url[%s], err: %s", h.url, err.Error())
		}
		save2file(h.filename, 0, resp)
	} else {
		fmt.Println("Downloading with multi goroutine...")
		var wg sync.WaitGroup
		success := make(chan bool, h.numRoutines)
		for _, ranges := range h.Split() {
			// 分配任务
			wg.Add(1)
			go func(start, end int) {
				defer func() {
					success <- true
					wg.Done()
				}()
				h.download(start, end)
			}(ranges[0], ranges[1])
		}

		// 下载进度
		go func() {
			fmt.Printf(ProgressBarStr, strings.Repeat(" ", 100), 0.0)
			countSuccess := 0
			for range success {
				countSuccess++
				var currProgress = float64(countSuccess) / float64(h.numRoutines) * 100
				// 进度条
				str := strings.Repeat("=", int(currProgress)) + strings.Repeat(" ", 100-int(currProgress))
				fmt.Printf(ProgressBarStr, str, currProgress)
			}
			fmt.Printf(ProgressBarStr, strings.Repeat("=", 100), 100.0)
		}()

		// 等待所有任务完成
		wg.Wait()
		close(success)
	}
}

//Split 分割下载任务
func (h *HttpDownloader) Split() [][]int {
	var ranges [][]int
	// 每个小任务的大小
	blockSize := h.contentLength / h.numRoutines
	for i := 0; i < h.numRoutines; i++ {
		start := i * blockSize
		end := (i+1)*blockSize - 1
		// 最后一个任务要全部下载完
		if i == h.numRoutines-1 {
			end = h.contentLength - 1
		}
		ranges = append(ranges, []int{start, end})
	}
	return ranges
}

//download 指定文件区间下载
func (h *HttpDownloader) download(start, end int) {
	req, err := http.NewRequest("GET", h.url, nil)
	if err != nil {
		log.Fatalf("Create the download request failed, err: %s", err.Error())
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%v-%v", start, end))
	req.Header.Set("User-Agent", kUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Failed to execute download request, err: %s", err.Error())
	}
	defer resp.Body.Close()

	save2file(h.filename, int64(start), resp)
}

//save2file 保存Body内容到指定文件区间
func save2file(filename string, offset int64, resp *http.Response) {
	f, err := os.OpenFile(filename, os.O_WRONLY, 0660)
	if err != nil {
		log.Fatalf("Open file failed, err: %s", err.Error())
	}
	// 从文件起点开始进行偏移
	if _, err := f.Seek(offset, 0); err != nil {
		log.Fatalf("Seek on a file failed, err: %s", err.Error())
	}
	defer f.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Read content from response body failed, err: %s", err.Error())
	}

	if _, err := f.Write(content); err != nil {
		log.Fatalf("Write content to file failed, err: %s", err.Error())
	}
}

func main() {
	// "https://dldir1.qq.com/qqfile/qq/PCQQ9.5.9/QQ9.5.9.28625.exe"
	var downloadUrl string
	var numRoutines int
	flag.StringVar(&downloadUrl, "u", "", "The file download url.")
	flag.IntVar(&numRoutines, "n", 6, "The num of go routines, default is 6.")
	flag.Parse()

	if _, err := url.ParseRequestURI(downloadUrl); err != nil {
		log.Fatal("The file download url is invalid.")
	}

	downloader := NewDownloader(downloadUrl, numRoutines)

	fmt.Println("Start the download task...")
	downloader.Download()
}
