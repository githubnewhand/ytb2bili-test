package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/difyz9/ytb2bili/pkg/store/model"

	"gorm.io/gorm"
)

// TaskStepService 任务步骤服务
type TaskStepService struct {
	DB *gorm.DB
}

// NewTaskStepService 创建任务步骤服务实例
func NewTaskStepService(db *gorm.DB) *TaskStepService {
	return &TaskStepService{
		DB: db,
	}
}

// InitTaskSteps 初始化视频的任务步骤
func (s *TaskStepService) InitTaskSteps(videoID string) error {
	// 定义标准任务步骤
	steps := []struct {
		Name     string
		Order    int
		CanRetry bool
	}{
		{"下载视频", 1, true},
		{"生成字幕", 2, true},
		{"翻译字幕", 3, true},
		{"生成元数据", 4, true},
		{"上传到Bilibili", 5, true},
		{"上传字幕到Bilibili", 6, true},
	}

	// 幂等初始化：已存在的步骤跳过，缺失的步骤补齐
	for _, step := range steps {
		var count int64
		if err := s.DB.Model(&model.TaskStep{}).
			Where("video_id = ? AND step_name = ?", videoID, step.Name).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		taskStep := &model.TaskStep{
			VideoID:   videoID,
			StepName:  step.Name,
			StepOrder: step.Order,
			Status:    model.TaskStepStatusPending,
			CanRetry:  step.CanRetry,
		}

		if err := s.DB.Create(taskStep).Error; err != nil {
			return err
		}
	}

	return nil
}

// GetTaskStepsByVideoID 根据视频ID获取任务步骤列表
func (s *TaskStepService) GetTaskStepsByVideoID(videoID string) ([]model.TaskStep, error) {
	var steps []model.TaskStep
	err := s.DB.Where("video_id = ?", videoID).
		Order("step_order ASC").
		Find(&steps).Error
	return steps, err
}

// UpdateTaskStepStatus 更新任务步骤状态
func (s *TaskStepService) UpdateTaskStepStatus(videoID, stepName, status string, errorMsg ...string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// 设置时间
	now := time.Now()
	if status == model.TaskStepStatusRunning {
		updates["start_time"] = &now
		updates["end_time"] = nil
		updates["duration"] = 0
		updates["progress"] = 0
		updates["progress_msg"] = "开始执行"
		updates["error_msg"] = ""
	} else if status == model.TaskStepStatusCompleted || status == model.TaskStepStatusFailed {
		updates["end_time"] = &now
		if status == model.TaskStepStatusCompleted {
			updates["error_msg"] = ""
			updates["progress"] = 100
			updates["progress_msg"] = "已完成"
		} else {
			updates["progress_msg"] = "执行失败"
		}

		// 计算执行时长
		var step model.TaskStep
		if err := s.DB.Where("video_id = ? AND step_name = ?", videoID, stepName).First(&step).Error; err == nil {
			if step.StartTime != nil {
				duration := now.Sub(*step.StartTime).Milliseconds()
				updates["duration"] = duration
			}
		}
	}

	// 设置错误信息
	if len(errorMsg) > 0 {
		updates["error_msg"] = errorMsg[0]
	}

	return s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_name = ?", videoID, stepName).
		Updates(updates).Error
}

// UpdateTaskStepResult 更新任务步骤执行结果
func (s *TaskStepService) UpdateTaskStepResult(videoID, stepName string, resultData interface{}) error {
	var jsonData string
	if resultData != nil {
		if jsonBytes, err := json.Marshal(resultData); err == nil {
			jsonData = string(jsonBytes)
		}
	}

	return s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_name = ?", videoID, stepName).
		Update("result_data", jsonData).Error
}

// UpdateTaskStepProgress 更新运行中任务步骤的实时进度。
func (s *TaskStepService) UpdateTaskStepProgress(videoID, stepName string, percent int, message string) error {
	percent = clampProgressPercent(percent)
	updates := map[string]interface{}{
		"progress": percent,
	}
	if message != "" {
		updates["progress_msg"] = message
	}

	return s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_name = ?", videoID, stepName).
		Updates(updates).Error
}

// ResetTaskStep 重置任务步骤（用于重新执行）
func (s *TaskStepService) ResetTaskStep(videoID, stepName string) error {
	updates := map[string]interface{}{
		"status":       model.TaskStepStatusPending,
		"start_time":   nil,
		"end_time":     nil,
		"duration":     0,
		"progress":     0,
		"progress_msg": "",
		"error_msg":    "",
		"result_data":  "",
	}

	return s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_name = ?", videoID, stepName).
		Updates(updates).Error
}

// ResetDownstreamSteps 重置指定步骤之后的准备阶段步骤。
func (s *TaskStepService) ResetDownstreamSteps(videoID string, afterOrder, maxOrder int) error {
	if afterOrder >= maxOrder {
		return nil
	}

	updates := map[string]interface{}{
		"status":       model.TaskStepStatusPending,
		"start_time":   nil,
		"end_time":     nil,
		"duration":     0,
		"progress":     0,
		"progress_msg": "",
		"error_msg":    "",
		"result_data":  "",
	}

	return s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_order > ? AND step_order <= ?", videoID, afterOrder, maxOrder).
		Updates(updates).Error
}

// GetTaskStepByName 根据视频ID和步骤名称获取特定步骤
func (s *TaskStepService) GetTaskStepByName(videoID, stepName string) (*model.TaskStep, error) {
	var step model.TaskStep
	err := s.DB.Where("video_id = ? AND step_name = ?", videoID, stepName).First(&step).Error
	if err != nil {
		return nil, err
	}
	return &step, nil
}

// GetTaskProgress 获取任务进度信息
func (s *TaskStepService) GetTaskProgress(videoID string) (map[string]interface{}, error) {
	var steps []model.TaskStep
	if err := s.DB.Where("video_id = ?", videoID).Order("step_order ASC").Find(&steps).Error; err != nil {
		return nil, err
	}

	totalSteps := len(steps)
	completedSteps := 0
	failedSteps := 0
	currentStep := ""
	currentStepProgress := 0
	currentStepMessage := ""
	isRunning := false

	for _, step := range steps {
		switch step.Status {
		case model.TaskStepStatusCompleted:
			completedSteps++
		case model.TaskStepStatusFailed:
			failedSteps++
		case model.TaskStepStatusRunning:
			currentStep = step.StepName
			currentStepProgress = clampProgressPercent(step.Progress)
			currentStepMessage = step.ProgressMsg
			isRunning = true
		}
	}

	progress := map[string]interface{}{
		"total_steps":           totalSteps,
		"completed_steps":       completedSteps,
		"failed_steps":          failedSteps,
		"current_step":          currentStep,
		"current_step_progress": currentStepProgress,
		"current_step_message":  currentStepMessage,
		"is_running":            isRunning,
		"progress_percent":      0,
	}

	if totalSteps > 0 {
		progress["progress_percent"] = ((completedSteps * 100) + currentStepProgress) / totalSteps
	}

	return progress, nil
}

// ResetAllRunningTasks 重置所有运行中的任务
func (s *TaskStepService) ResetAllRunningTasks() error {
	// 开始事务
	tx := s.DB.Begin()

	stepUpdates := map[string]interface{}{
		"status":       model.TaskStepStatusPending,
		"start_time":   nil,
		"end_time":     nil,
		"duration":     0,
		"progress":     0,
		"progress_msg": "",
		"error_msg":    "",
		"result_data":  "",
	}

	result := tx.Model(&model.TaskStep{}).
		Where("status = ?", model.TaskStepStatusRunning).
		Updates(stepUpdates)

	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to reset running task steps: %v", result.Error)
	}

	taskStepsAffected := result.RowsAffected

	videoResult := tx.Model(&model.SavedVideo{}).
		Where("status = ?", "002").
		Update("status", "001")

	if videoResult.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to reset running video status: %v", videoResult.Error)
	}

	videosAffected := videoResult.RowsAffected

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	log.Printf("Reset %d running task steps and %d videos (from processing to pending status)", taskStepsAffected, videosAffected)
	return nil
}

// GetPendingSteps 获取所有状态为pending的任务步骤
func (s *TaskStepService) GetPendingSteps() ([]*model.TaskStep, error) {
	var candidates []*model.TaskStep

	result := s.DB.Table("tb_task_steps").
		Select("tb_task_steps.*").
		Joins("INNER JOIN tb_saved_videos ON tb_task_steps.video_id = tb_saved_videos.video_id").
		Where("tb_task_steps.status = ?", model.TaskStepStatusPending).
		Where("tb_task_steps.step_order <= ?", 4).
		Where("tb_task_steps.deleted_at IS NULL").
		Where("tb_saved_videos.deleted_at IS NULL").
		Where("tb_saved_videos.status IN ?", []string{"002", "999"}).
		Order("tb_task_steps.video_id ASC, tb_task_steps.step_order ASC").
		Find(&candidates)

	if result.Error != nil {
		return nil, fmt.Errorf("查询待重试步骤失败: %v", result.Error)
	}

	steps := make([]*model.TaskStep, 0, len(candidates))
	for _, step := range candidates {
		ok, err := s.arePreviousStepsCompleted(step.VideoID, step.StepOrder)
		if err != nil {
			return nil, fmt.Errorf("检查步骤依赖失败: %v", err)
		}
		if ok {
			steps = append(steps, step)
		}
	}

	return steps, nil
}

func (s *TaskStepService) arePreviousStepsCompleted(videoID string, stepOrder int) (bool, error) {
	if stepOrder <= 1 {
		return true, nil
	}

	var blockingCount int64
	err := s.DB.Model(&model.TaskStep{}).
		Where("video_id = ? AND step_order < ? AND status <> ?", videoID, stepOrder, model.TaskStepStatusCompleted).
		Count(&blockingCount).Error
	if err != nil {
		return false, err
	}

	return blockingCount == 0, nil
}

// DeleteTaskStepsByVideoID 删除指定视频的所有任务步骤（软删除）
func (s *TaskStepService) DeleteTaskStepsByVideoID(videoID string) error {
	result := s.DB.Where("video_id = ?", videoID).Delete(&model.TaskStep{})
	if result.Error != nil {
		return fmt.Errorf("删除任务步骤失败: %v", result.Error)
	}
	return nil
}

func clampProgressPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
