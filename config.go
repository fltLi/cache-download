package cachedl

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

const MinCacheDuration = 16 * time.Second

// 缓存下载器配置.
type Config struct {
	// 缓存管理
	Path          string        // 缓存目录
	CacheDuration time.Duration // 缓存有效期

	// 下载
	MaxWorkers int // 下载线程数

	// 辅助项
	Logger *slog.Logger // 日志器
}

func (c *Config) Validate() error {
	// Path
	info, err := os.Stat(c.Path)
	if err == nil && !info.IsDir() {
		return fmt.Errorf("缓存目录不是目录: %s", c.Path)
	}

	// CacheDuration
	if c.CacheDuration < MinCacheDuration {
		return fmt.Errorf("缓存有效期过短: %v", c.CacheDuration)
	}

	// MaxWorkers
	if c.MaxWorkers <= 0 {
		return fmt.Errorf("工作线程数必须为正整数: %d", c.MaxWorkers)
	}

	return nil
}
