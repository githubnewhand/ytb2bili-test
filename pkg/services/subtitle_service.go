package services

import (
	"time"
)

// Subtitle 字幕主结构体
type Subtitle struct {
	VideoID   string          `bson:"video_id"`   // 关联视频ID
	Language  string          `bson:"language"`   // 字幕语言代码(zh, en, ja等)
	Title     string          `bson:"title"`      // 字幕标题
	Entries   []SubtitleEntry `bson:"entries"`    // 字幕条目数组
	CreatedAt time.Time       `bson:"created_at"` // 创建时间
	UpdatedAt time.Time       `bson:"updated_at"` // 更新时间
	Version   int             `bson:"version"`    // 版本号，用于乐观锁

}

// SubtitleEntry 字幕条目
type SubtitleEntry struct {
	Index     int    `bson:"index"`      // 序号
	StartTime int64  `bson:"start_time"` // 开始时间(毫秒)
	EndTime   int64  `bson:"end_time"`   // 结束时间(毫秒)
	Text      string `bson:"text"`       // 字幕文本
}
