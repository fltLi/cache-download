package pool

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// 下载池 (并发安全)
// 包装 `http.Client`, 控制线程数.
type Downloader struct {
	closed atomic.Bool
	tasks  chan<- *task
	wg     sync.WaitGroup
}

// 创建下载池.
func New(client *http.Client, maxWorkers int) (*Downloader, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP 客户端不可为空")
	}
	if maxWorkers <= 0 {
		return nil, fmt.Errorf("工作线程数必须为正整数: %d", maxWorkers)
	}

	tasks := make(chan *task)
	d := &Downloader{
		tasks: tasks,
	}

	// 添加工作线程
	for range maxWorkers {
		d.wg.Go(func() {
			work(client, tasks)
		})
	}

	return d, nil
}

// 等待任务完成并关闭下载池.
func (d *Downloader) Close() {
	if d.closed.Swap(true) {
		return
	}

	close(d.tasks)
	d.wg.Wait()
}

// 创建下载任务并等待完成.
func (d *Downloader) Download(ctx context.Context, url, path string) error {
	if d.closed.Load() {
		return fmt.Errorf("下载器已关闭")
	}

	result := make(chan error)
	task := &task{
		ctx:    ctx,
		url:    url,
		path:   path,
		result: result,
	}

	// 发送任务并等待完成
	d.tasks <- task
	return <-result
}
