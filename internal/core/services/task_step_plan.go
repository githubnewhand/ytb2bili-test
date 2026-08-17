package services

import (
	"fmt"

	"github.com/difyz9/ytb2bili/pkg/store/model"
)

// InitTaskPlan adds or reorders the steps needed by one concrete workflow.
// Existing historical steps are retained so old progress records stay visible.
func (s *TaskStepService) InitTaskPlan(videoID string, stepNames []string) error {
	if videoID == "" {
		return fmt.Errorf("videoID不能为空")
	}
	for index, name := range stepNames {
		if name == "" {
			return fmt.Errorf("任务步骤名称不能为空")
		}
		order := index + 1
		var count int64
		if err := s.DB.Model(&model.TaskStep{}).
			Where("video_id = ? AND step_name = ?", videoID, name).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			if err := s.DB.Model(&model.TaskStep{}).
				Where("video_id = ? AND step_name = ?", videoID, name).
				Updates(map[string]interface{}{
					"step_order": order,
					"can_retry":  true,
				}).Error; err != nil {
				return err
			}
			continue
		}
		if err := s.DB.Create(&model.TaskStep{
			VideoID:   videoID,
			StepName:  name,
			StepOrder: order,
			Status:    model.TaskStepStatusPending,
			CanRetry:  true,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
