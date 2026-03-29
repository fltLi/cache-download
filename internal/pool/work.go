package pool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type task struct {
	ctx    context.Context
	url    string
	path   string
	result chan<- error
}

// 下载工作线程.
func work(c *http.Client, tasks <-chan *task) {
	for t := range tasks {
		t.result <- download(c, t.ctx, t.url, t.path)
	}
}

// 下载数据到本地.
func download(c *http.Client, ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("请求创建失败: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("请求获取失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 错误: %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("文件所在目录创建失败: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("文件创建失败: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("数据写入失败: %w", err)
	}

	return nil
}
