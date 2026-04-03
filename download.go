package cachedl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github/fltLi/cache-download/internal/entry"
	"github/fltLi/cache-download/internal/pool"
	syncutils "github/fltLi/cache-download/internal/sync"
)

const EntryFile = "entry.json"

// 缓存下载器 (并发安全)
type Downloader struct {
	// 缓存管理
	path  string
	entry *entry.Entry

	// 下载任务
	taskLock sync.RWMutex
	tasks    map[string]*syncutils.Broadcaster[error] // 正在运行的下载任务
	pool     *pool.Downloader

	// 辅助项
	closed atomic.Bool
	logger *slog.Logger
}

// 创建缓存下载器.
func New(client *http.Client, cfg *Config) (*Downloader, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP 客户端不可为空")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	entryPath := filepath.Join(cfg.Path, EntryFile)
	entry, entryErr := entry.New(entryPath, cfg.CacheDuration) // 不致命错误稍后写日志静默处理
	if entry == nil {
		return nil, entryErr
	}

	pool, poolErr := pool.New(client, cfg.MaxWorkers)
	if pool == nil {
		return nil, poolErr
	}

	d := &Downloader{
		path:   cfg.Path,
		entry:  entry,
		tasks:  make(map[string]*syncutils.Broadcaster[error]),
		pool:   pool,
		logger: cfg.Logger,
	}

	if entryErr != nil {
		d.log(slog.LevelError, entryErr.Error())
	}
	if poolErr != nil {
		d.log(slog.LevelError, poolErr.Error())
	}

	return d, nil
}

// 等待所有任务完成并关闭.
func (d *Downloader) Close() {
	if d.closed.Swap(true) {
		return
	}

	d.pool.Close() // 之后 d.tasks 也会失效
}

// 下载并获取文件只读句柄.
func (d *Downloader) Download(ctx context.Context, url, desc string) (*os.File, error) {
	if d.closed.Load() {
		d.logContext(ctx, slog.LevelError, "缓存下载器已关闭, 拒收任务", slog.String("url", url), slog.String("description", desc))
		return nil, fmt.Errorf("缓存下载器已关闭")
	}

	// 尝试复用缓存
	file, err := d.tryLoadCache(url)
	if file != nil {
		d.logContext(ctx, slog.LevelInfo, "复用缓存", slog.String("url", url), slog.String("description", desc))
		return file, err
	}

	// 尝试等待下载中任务
	file, err = d.tryWaitTask(url)
	if file != nil {
		d.logContext(ctx, slog.LevelInfo, "复用缓存", slog.String("url", url), slog.String("description", desc))
		return file, err
	}

	// 创建新任务并等待
	d.startTask(ctx, url, desc)
	file, err = d.tryWaitTask(url)
	d.logContext(ctx, slog.LevelInfo, "缓存下载成功", slog.String("url", url), slog.String("description", desc))
	return file, err
}

//////// internal ////////

func (d *Downloader) log(level slog.Level, msg string, attrs ...slog.Attr) {
	d.logContext(context.Background(), level, msg, attrs...)
}

func (d *Downloader) logContext(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if d.logger != nil {
		d.logger.LogAttrs(ctx, level, msg, attrs...)
	}
}

func (d *Downloader) tryLoadCache(url string) (*os.File, error) {
	path, exists, err := d.entry.Get(url)
	if err != nil {
		d.log(slog.LevelWarn, err.Error())
	}

	if !exists {
		// 清理过期缓存
		if path != "" {
			d.log(slog.LevelInfo, "清理过期缓存",
				slog.String("url", url),
				slog.String("path", path))
			cleanErr := os.Remove(filepath.Join(d.path, path))
			if cleanErr != nil {
				d.log(slog.LevelWarn, "清理过期缓存失败",
					slog.String("url", url),
					slog.String("path", path),
					slog.String("error", cleanErr.Error()))
				err = cleanErr
			}
		}

		return nil, err
	}

	// 打开缓存项
	file, err := os.Open(filepath.Join(d.path, path))
	if err != nil {
		d.log(slog.LevelError, "读取缓存时出错", slog.String("error", err.Error()))
		return nil, err
	}

	return file, nil
}

func (d *Downloader) tryWaitTask(url string) (*os.File, error) {
	result, exists := func() (*syncutils.Broadcaster[error], bool) {
		d.taskLock.RLock()
		defer d.taskLock.RUnlock()
		result, exists := d.tasks[url]
		return result, exists
	}()

	if !exists {
		return nil, nil
	}

	if err := result.Load(); err != nil {
		return nil, err
	}

	// 从缓存条目中查找
	return d.tryLoadCache(url)
}

// 生成随机名称.
func generateCacheName(data string) string {
	hashPart := sha256.Sum256([]byte(data))
	randPart := rand.Intn(1 << 24)
	return fmt.Sprintf("%s_%06d.cache", hex.EncodeToString(hashPart[:12]), randPart)
}

// 创建下载任务.
func (d *Downloader) startTask(ctx context.Context, url, desc string) {
	// 任务列表占位
	bc := syncutils.NewBroadcaster[error]()
	func() {
		d.taskLock.Lock()
		defer d.taskLock.Unlock()
		d.tasks[url] = bc
	}()

	removeTask := func() {
		d.taskLock.Lock()
		defer d.taskLock.Unlock()
		delete(d.tasks, url)
	}

	// 启动下载任务
	go func() {
		defer removeTask()

		name := generateCacheName(fmt.Sprintf("%s|%s|%v", url, desc, time.Now()))
		path := filepath.Join(d.path, name)

		d.logContext(
			ctx,
			slog.LevelInfo,
			"创建下载任务",
			slog.String("url", url),
			slog.String("description", desc),
			slog.String("path", name),
		)

		if err := d.pool.Download(ctx, url, path); err != nil {
			d.logContext(
				ctx,
				slog.LevelError,
				"下载出错",
				slog.String("url", url),
				slog.String("description", desc),
				slog.String("path", name),
				slog.String("error", err.Error()),
			)
			bc.Store(err)
			return
		}

		if err := d.entry.Add(url, name, desc); err != nil {
			d.logContext(ctx, slog.LevelWarn, "写入缓存条目出错", slog.String("error", err.Error()))
		}
		bc.Store(nil)
	}()
}
