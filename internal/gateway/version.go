package gateway

// VersionInfo 用于前端版本检测与更新提示。
type VersionInfo struct {
	// Version 一般使用服务启动时间戳，前端用它判断是否需要刷新。
	Version int64 `json:"version"`
	// VersionName 为展示用版本名。
	VersionName string `json:"versionName"`
	// Changelog 为更新内容说明。
	Changelog string `json:"changelog"`
}
