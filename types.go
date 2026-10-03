package watchstats

// Event 是上层统一提供的播放事件。
// 时间与位置均为整数毫秒，位置处于视频时长范围内。
type Event struct {
	ViewerID  string // 观众标识
	SessionID string // 会话标识，同一会话始终属于同一观众
	EventID   string // 事件标识，在会话内唯一
	TimeMs    int64  // 事件发生时刻
	PosMs     int64  // 事件执行后的播放位置
	Playing   bool   // 事件执行后是否正在播放
}

// Summary 是视频维度的汇总统计。
type Summary struct {
	Viewers      int     // 已看时长大于 0 的观众数
	AvgRatio     float64 // 这些观众的平均观看比例（分母为完整视频时长）
	Milestone25  int     // 覆盖比例 >= 25% 的观众数
	Milestone50  int     // 覆盖比例 >= 50% 的观众数
	Milestone75  int     // 覆盖比例 >= 75% 的观众数
	Milestone100 int     // 覆盖比例 >= 100% 的观众数
}
