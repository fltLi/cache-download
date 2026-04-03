package entry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type cacheItem struct {
	Path        string    `json:"path"`
	ModTime     time.Time `json:"time"`
	Description string    `json:"description"`
}

// 缓存条目管理 (并发安全)
// 初始化时从本地读取, 更新时写入 (发生错误不会导致缓存条目失效).
type Entry struct {
	path     string
	fileLock sync.RWMutex

	cacheDuration time.Duration
	entries       map[string]cacheItem // 缓存条目 URL -> 元数据 映射
	entryLock     sync.RWMutex
}

// 创建缓存条目管理.
func New(path string, cacheDuration time.Duration) (*Entry, error) {
	e := &Entry{
		path:          path,
		cacheDuration: cacheDuration,
		entries:       make(map[string]cacheItem),
	}
	err := e.Load()
	return e, err
}

// 尝试读取本地缓存条目.
func (e *Entry) Load() error {
	data, err := func() ([]byte, error) {
		e.fileLock.RLock()
		defer e.fileLock.RUnlock()
		return os.ReadFile(e.path)
	}()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("缓存条目读取失败: %w", err)
	}

	var entries map[string]cacheItem
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("缓存条目反序列化失败: %w", err)
	}

	func() {
		e.entryLock.Lock()
		defer e.entryLock.Unlock()
		e.entries = entries
	}()

	return nil
}

// 尝试写入本地缓存条目.
func (e *Entry) Store() error {
	data, err := func() ([]byte, error) {
		e.entryLock.RLock()
		defer e.entryLock.RUnlock()
		return json.MarshalIndent(e.entries, "", " ")
	}()
	if err != nil {
		return fmt.Errorf("缓存条目序列化失败: %w", err)
	}

	if err := func() error {
		e.fileLock.Lock()
		defer e.fileLock.Unlock()
		if err := os.MkdirAll(filepath.Dir(e.path), 0755); err != nil {
			return err
		}
		return os.WriteFile(e.path, data, 0644)
	}(); err != nil {
		return fmt.Errorf("缓存条目写入失败: %w", err)
	}

	return nil
}

// 添加条目并尝试写入本地.
func (e *Entry) Add(url string, path string, desc string) error {
	item := cacheItem{
		Path:        path,
		ModTime:     time.Now(), // 记录修改时间
		Description: desc,
	}

	func() {
		e.entryLock.Lock()
		defer e.entryLock.Unlock()
		e.entries[url] = item
	}()

	return e.Store()
}

// 查找缓存.
// 返回:
//   - string: 缓存 URL 对应的路径 (过期时也会返回).
//   - bool: 缓存是否存在且未过期.
//   - error: 文件过期移除后尝试写入本地时发生的错误.
func (e *Entry) Get(url string) (string, bool, error) {
	c, exists := func() (cacheItem, bool) {
		e.entryLock.RLock()
		defer e.entryLock.RUnlock()
		c, exists := e.entries[url]
		return c, exists
	}()

	if !exists {
		return "", false, nil
	}

	// 检查缓存是否过期, 并从条目移除
	if time.Since(c.ModTime) > e.cacheDuration {
		func() {
			e.entryLock.Lock()
			defer e.entryLock.Unlock()
			delete(e.entries, url)
		}()
		err := e.Store()
		return c.Path, false, err
	}

	return c.Path, true, nil
}
