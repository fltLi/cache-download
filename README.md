# cache-download

基于 JSON 元数据管理的并发安全缓存下载库.

## :sparkles: 特性

- **智能缓存**: 自动**缓存下载内容**, 相同 URL 重复请求直接复用缓存, 提升效率.

- **灵活配置**: 支持自定义缓存有效期, 并发下载线程数, 并允许注入日志器.

- **并发安全**: 保证多 goroutine 甚至多进程环境下的安全访问 (多进程小概率发生重复下载).

## :package: 安装

```bash
go get github.com/fltLi/cache-download
```

## :rocket: 使用

### 基本用法

```go
// 准备配置文件
cfg := &cachedl.Config{
    Path:          "./cache",      // 缓存目录
    CacheDuration: 24 * time.Hour, // 缓存有效期
    MaxWorkers:    5,              // 下载线程数
    Logger:        slog.Default(), // 自定义日志器 (可选)
}

// 创建 HTTP 客户端和下载器
client := &http.Client{Timeout: 30 * time.Second}
downloader, err := cachedl.New(client, cfg)
if downloader == nil {
    panic(err)
}
defer downloader.Close() // 阻塞完成剩余任务

// 下载文件, 获取只读的缓存文件句柄
file, err := downloader.Download(
    context.Background(),
    "https://example.com/file.zip",
    "example file", // 描述信息, 方便人工查询 entry.json
)
if file == nil {
    panic(err)
}
defer file.Close()
```

### 示例程序

项目附带一个简易的缓存下载命令行工具 [example/main.go](example/main.go), 支持批量任务输入:

```bash
go run example/main.go ./cache
```

## :gear: 配置说明

| 字段            | 类型            | 说明                      |
| --------------- | --------------- | ------------------------- |
| `Path`          | `string`        | 缓存目录                  |
| `CacheDuration` | `time.Duration` | 缓存有效期 (最小为 16 秒) |
| `MaxWorkers`    | `int`           | 下载线程数                |
| `Logger`        | `*slog.Logger`  | 可选日志器                |

## :book: API 概述

### `New(client *http.Client, cfg *Config) (*Downloader, error)`

创建缓存下载器实例.

- 配置将接受 `(c *Config) Validate() error` 校验.
- 成功创建缓存下载器时, 可能因缓存条目 JSON 多进程访问冲突额外返回一个 error, 忽略即可.

### `(d *Downloader) Download(ctx context.Context, url, desc string) (*os.File, error)`

下载指定 URL 的内容, 返回只读的缓存文件句柄.

- 如果缓存命中且未过期, 直接打开缓存文件.
- 如果同一 URL 正在下载中, 等待该任务完成并复用缓存.

- `desc` 为描述信息, 仅用于日志和缓存元数据记录.

### `(d *Downloader) Close()`

关闭下载器, 等待所有进行中的任务完成并释放资源.
调用后无法再提交新任务.

- 成功下载时, 可能因缓存条目 JSON 多进程访问冲突额外返回一个 error, 忽略即可.

## :page_facing_up: 许可证

Code: MIT, 2026, fltLi
