package manager

import (
	"fmt"
	"log"

	"github.com/difyz9/ytb2bili/internal/core/types"
)

// TaskChain 任务链
type TaskChain struct {
	Tasks   []types.Task
	Context map[string]interface{}
}

// NewTaskChain 创建任务链
func NewTaskChain() *TaskChain {
	return &TaskChain{
		Tasks:   make([]types.Task, 0),
		Context: make(map[string]interface{}),
	}
}

// AddTask 添加任务到链中
func (c *TaskChain) AddTask(task types.Task) *TaskChain {
	if err := task.InsertTask(); err != nil {
		log.Printf("添加任务到数据库失败: %v", err)
	}
	c.Tasks = append(c.Tasks, task)
	return c
}

// Run 执行任务链
func (c *TaskChain) Run(stopOnFailure bool) map[string]interface{} {
	for _, task := range c.Tasks {
		taskName := task.GetName()
		log.Printf("正在执行任务: %s", taskName)

		// 执行任务
		success := false
		var message string

		func() {
			defer func() {
				if r := recover(); r != nil {
					message = fmt.Sprintf("任务执行异常: %v", r)
					log.Printf("任务 %s 发生异常: %v", taskName, r)
				}
			}()

			success = task.Execute(c.Context)
		}()

		if !success {
			if message == "" {
				message = fmt.Sprintf("任务 %s 执行失败", taskName)
			}
			if errorMsg, exists := c.Context["error"]; !exists || errorMsg == nil || fmt.Sprint(errorMsg) == "" {
				c.Context["error"] = message
			}

			log.Printf("任务 %s 执行失败，终止链: %s", taskName, c.Context["error"])
			if stopOnFailure {
				break
			}
		}
	}

	return c.Context
}
