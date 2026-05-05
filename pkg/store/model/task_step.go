package model

import (
	"time"
)

// TaskStep 任务步骤记录
type TaskStep struct {
	BaseModel
	VideoID     string     `gorm:"type:varchar(100);not null;index" json:"video_id"` // 关联的视频ID
	StepName    string     `gorm:"type:varchar(100);not null" json:"step_name"`      // 步骤名称
	StepOrder   int        `gorm:"type:int;not null" json:"step_order"`              // 步骤顺序
	Status      string     `gorm:"type:varchar(20);not null" json:"status"`          // 步骤状态: pending, running, completed, failed, skipped
	StartTime   *time.Time `gorm:"type:datetime" json:"start_time"`                  // 开始时间
	EndTime     *time.Time `gorm:"type:datetime" json:"end_time"`                    // 结束时间
	Duration    int64      `gorm:"type:bigint" json:"duration"`                      // 执行时长（毫秒）
	Progress    int        `gorm:"type:int;default:0" json:"progress_percent"`       // 当前步骤进度百分比
	ProgressMsg string     `gorm:"type:varchar(255)" json:"progress_message"`        // 当前步骤进度说明
	ErrorMsg    string     `gorm:"type:text" json:"error_msg"`                       // 错误信息
	ResultData  string     `gorm:"type:longtext" json:"result_data"`                 // 步骤执行结果数据（JSON）
	CanRetry    bool       `gorm:"type:boolean;default:true" json:"can_retry"`       // 是否可以重试
}

// TableName 指定表名
func (TaskStep) TableName() string {
	return "tb_task_steps"
}

// TaskStepStatus 任务步骤状态常量
const (
	TaskStepStatusPending   = "pending"   // 待执行
	TaskStepStatusRunning   = "running"   // 执行中
	TaskStepStatusCompleted = "completed" // 已完成
	TaskStepStatusFailed    = "failed"    // 失败
	TaskStepStatusSkipped   = "skipped"   // 跳过
)
