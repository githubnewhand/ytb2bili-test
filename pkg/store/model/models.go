package model

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 基础模型
type BaseModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AudioResult 音频处理结果
type AudioResult struct {
	SID            int     `json:"sid"`
	Text           string  `json:"text"`
	TranslatedText string  `json:"translated_text,omitempty"`
	AudioURL       string  `json:"audio_url"`
	Language       string  `json:"language"`
	Duration       float64 `json:"duration"`
}

// TranslationSettings 翻译设置
type TranslationSettings struct {
	SourceLanguage string  `json:"source_language"`
	TargetLanguage string  `json:"target_language"`
	Service        string  `json:"service"`
	Gender         string  `json:"gender"`
	Tier           string  `json:"tier"`
	VoiceName      string  `json:"voice_name"`
	VoiceSpeed     float64 `json:"voice_speed"`
}

// VideoProcessingRequest 视频处理请求（根据用户提供的JSON格式）
type VideoProcessingRequest struct {
	VideoID             string              `json:"video_id"`
	Platform            string              `json:"platform"`
	Subtitles           []SubtitleItem      `json:"subtitles"`
	TranslationSettings TranslationSettings `json:"translation_settings"`
}

// User 用户模型
type User struct {
	BaseModel
	Username    string     `gorm:"uniqueIndex;size:50;not null" json:"username"`
	Email       string     `gorm:"uniqueIndex;size:100" json:"email"`
	Phone       string     `gorm:"uniqueIndex;size:20" json:"phone"`
	Password    string     `gorm:"size:100;not null" json:"-"`
	Avatar      string     `gorm:"size:255" json:"avatar"`
	Status      int        `gorm:"default:1" json:"status"` // 1:正常 0:禁用
	LastLoginAt *time.Time `json:"last_login_at"`
}

type SubtitleItem struct {
	SID      int     `json:"sid" gorm:"column:sid"`           // 字幕ID
	From     float64 `json:"from" gorm:"column:from_time"`    // 开始时间
	To       float64 `json:"to" gorm:"column:to_time"`        // 结束时间
	Text     string  `json:"text" gorm:"column:content"`      // 字幕内容（兼容用户格式）
	Content  string  `json:"content" gorm:"column:content"`   // 字幕内容（兼容数据库格式）
	Location int     `json:"location" gorm:"column:location"` // 位置信息
}

// SavedVideoSubtitle 用户提交的字幕条目（用于API接收）
type SavedVideoSubtitle struct {
	Text     string  `json:"text"`     // 字幕文本
	Duration float64 `json:"duration"` // 持续时间
	Offset   float64 `json:"offset"`   // 偏移时间
	Lang     string  `json:"lang"`     // 语言
}

// SavedVideo 保存的视频信息
type SavedVideo struct {
	BaseModel
	VideoID                string     `gorm:"type:varchar(100);uniqueIndex;not null" json:"video_id"` // 视频ID（唯一）
	URL                    string     `gorm:"type:varchar(500);not null;index" json:"url"`            // 视频URL
	Title                  string     `gorm:"type:varchar(500)" json:"title"`                         // 视频标题
	Status                 string     `gorm:"type:varchar(20)" json:"status"`                         // 视频状态
	Description            string     `gorm:"type:text" json:"description"`                           // 视频描述
	Chapters               string     `gorm:"type:text" json:"chapters"`                              // 视频章节文本
	GeneratedTitle         string     `gorm:"type:varchar(500)" json:"generated_title"`               // AI生成的标题
	GeneratedDesc          string     `gorm:"type:text" json:"generated_desc"`                        // AI生成的描述
	GeneratedTags          string     `gorm:"type:varchar(1000)" json:"generated_tags"`               // AI生成的标签（逗号分隔）
	BiliBVID               string     `gorm:"type:varchar(50)" json:"bili_bvid"`                      // Bilibili BVID
	BiliAID                int64      `gorm:"type:bigint" json:"bili_aid"`                            // Bilibili AID
	OperationType          string     `gorm:"type:varchar(50)" json:"operation_type"`                 // 操作类型 (download/upload等)
	Subtitles              string     `gorm:"type:longtext" json:"subtitles"`                         // 字幕JSON字符串
	PlaylistID             string     `gorm:"type:varchar(100);index" json:"playlist_id"`             // 播放列表ID
	Timestamp              string     `gorm:"type:varchar(50)" json:"timestamp"`                      // 时间戳
	SavedAt                string     `gorm:"type:varchar(50)" json:"saved_at"`                       // 保存时间
	BiliTid                int        `gorm:"column:bili_tid;type:int;default:0" json:"biliTid"`      //保存视频自定义分区ID
	PublishAudience        string     `gorm:"type:varchar(20);not null;default:'';index" json:"publish_audience"`
	AudienceSelectedAt     *time.Time `gorm:"index" json:"audience_selected_at,omitempty"`
	UPowerPreviewSeconds   int        `gorm:"column:upower_preview_seconds;not null;default:0" json:"upower_preview_seconds"`
	RecordType             string     `gorm:"type:varchar(20);not null;default:'source';index" json:"record_type"`
	WorkflowState          string     `gorm:"type:varchar(40);not null;default:'';index" json:"workflow_state"`
	MediaPath              string     `gorm:"type:text" json:"media_path,omitempty"`
	MediaDurationMS        int64      `gorm:"type:bigint;not null;default:0" json:"media_duration_ms"`
	MediaProbeJSON         string     `gorm:"type:text" json:"-"`
	ContentHash            string     `gorm:"type:varchar(128);index" json:"content_hash,omitempty"`
	ReadyAt                *time.Time `gorm:"index" json:"ready_at,omitempty"`
	ScheduledUploadAt      *time.Time `gorm:"index" json:"scheduled_upload_at,omitempty"`
	UploadPolicy           string     `gorm:"type:varchar(20);not null;default:'scheduled';index" json:"upload_policy"`
	RightsVerified         bool       `gorm:"type:boolean;not null;default:false" json:"rights_verified"`
	ClassificationLockedAt *time.Time `gorm:"index" json:"classification_locked_at,omitempty"`
}

// TableName 指定表名
func (SavedVideo) TableName() string {
	return "tb_saved_videos"
}
