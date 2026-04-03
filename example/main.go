// 使用示例
// 简易的缓存下载命令行工具.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github/fltLi/cache-download"
)

type task struct {
	url  string
	path string
}

func main() {
	fmt.Println("缓存下载器启动中...")

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "用法: %s <cache_dir>\n", os.Args[0])
		os.Exit(1)
	}
	cacheDir := os.Args[1]

	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建缓存目录失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志文件
	logPath := filepath.Join(cacheDir, time.Now().Format("2006-01-02")+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开日志文件失败: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 缓存下载器配置
	cfg := &cachedl.Config{
		Path:          cacheDir,
		CacheDuration: time.Hour,
		MaxWorkers:    3,
		Logger:        logger,
	}

	// 启动下载器
	client := &http.Client{}
	downloader, err := cachedl.New(client, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "缓存下载器启动失败: %v\n", err)
		os.Exit(1)
	}
	defer downloader.Close()

	// 主循环
	for {
		tasks := readTasks()
		if len(tasks) == 0 {
			continue
		}

		fmt.Printf("开始下载, 共 %d 项任务:\n", len(tasks))
		var wg sync.WaitGroup

		for idx, t := range tasks {
			wg.Add(1)
			go func(id int, t task) {
				defer wg.Done()
				if err := downloadFile(downloader, t); err != nil {
					fmt.Printf("%d. 报错: %v\n", id, err)
				} else {
					fmt.Printf("%d. 成功: %s\n", id, t.path)
				}
			}(idx+1, t)
		}
		wg.Wait()
	}
}

// readTasks 从标准输入读取 URL 和输出路径, 空行结束.
func readTasks() []task {
	fmt.Println("\n请依次输入 `URL 输出路径`, 空行结束:")
	var tasks []task
	scanner := bufio.NewScanner(os.Stdin)

	for i := 1; ; i++ {
		fmt.Printf("%d. ", i)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			fmt.Println("输入格式错误，请使用: URL 路径")
			continue
		}
		url := strings.TrimSpace(parts[0])
		rawPath := strings.TrimSpace(parts[1])
		if url == "" || rawPath == "" {
			fmt.Println("URL 和路径均不能为空")
			continue
		}
		absPath, err := filepath.Abs(rawPath)
		if err != nil {
			fmt.Println("路径不合法:", err)
			continue
		}
		tasks = append(tasks, task{url: url, path: absPath})
	}
	return tasks
}

// downloadFile 执行下载并写入目标文件.
func downloadFile(downloader *cachedl.Downloader, t task) error {
	ctx := context.Background()
	// 下载缓存文件 (获取只读句柄)
	cacheFile, err := downloader.Download(ctx, t.url, t.path)
	if err != nil {
		return err
	}
	defer cacheFile.Close()

	// 确保输出目录存在
	if err := os.MkdirAll(filepath.Dir(t.path), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 创建目标文件
	dstFile, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer dstFile.Close()

	// 拷贝内容
	if _, err := io.Copy(dstFile, cacheFile); err != nil {
		return fmt.Errorf("写入输出文件失败: %w", err)
	}
	return nil
}
