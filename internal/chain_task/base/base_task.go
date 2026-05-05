package base

import (
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/pkg/cos"
)

// BaseTask 基础任务实现
type BaseTask struct {
	Name         string
	StateManager *manager.StateManager
	Client       *cos.CosClient
}

// TaskOption 定义选项函数类型
type TaskOption func(*BaseTask)

// WithName 是一个选项函数，用于设置任务名称
func WithName(name string) TaskOption {
	return func(task *BaseTask) {
		task.Name = name
	}
}

// GetName 获取任务名称
func (t *BaseTask) GetName() string {
	return t.Name
}

// InsertTask 插入任务记录
func (t *BaseTask) InsertTask() error {

	return nil
}

// UpdateStatus 更新任务状态
func (t *BaseTask) UpdateStatus(status, message string) error {
	//return t.StateManager.UpdateTBVideo(t.Name, status, message)
	return nil
}
