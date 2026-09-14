package store

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var legacyPreparationSteps = []string{
	"下载视频",
	"生成字幕",
	"翻译字幕",
	"生成元数据",
}

// recoverLegacyPublicationState stops unfinished compilation/upload work left by
// the removed paid-video feature and returns source videos to the public flow.
// Historical tables and media files are retained; this migration deletes nothing.
func recoverLegacyPublicationState(db *gorm.DB) error {
	if !db.Migrator().HasTable("tb_charge_pool_items") {
		return nil
	}

	type legacyVideo struct {
		ID            uint
		VideoID       string
		Status        string
		OperationType string
		RecordType    string
	}

	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		var videos []legacyVideo
		if err := tx.Table("tb_saved_videos").
			Select("id, video_id, status, operation_type, record_type").
			Where(
				"publish_audience IN ? OR status IN ?",
				[]string{"charge_30", "charge_50"},
				[]string{"100", "101", "110"},
			).
			Find(&videos).Error; err != nil {
			return fmt.Errorf("读取旧发布任务失败: %w", err)
		}

		for _, video := range videos {
			// Published rows are historical records. Keep them unchanged and never
			// enqueue them again while removing the paid-video workflow.
			if video.Status == "300" || video.Status == "301" || video.Status == "400" {
				continue
			}

			isCompilation := strings.EqualFold(video.RecordType, "compilation") ||
				strings.EqualFold(video.OperationType, "compilation")
			if isCompilation {
				if err := tx.Table("tb_saved_videos").
					Where("id = ?", video.ID).
					Updates(map[string]interface{}{
						"status":                   "999",
						"updated_at":               now,
						"publish_audience":         "",
						"audience_selected_at":     nil,
						"upower_preview_seconds":   0,
						"workflow_state":           "",
						"ready_at":                 nil,
						"scheduled_upload_at":      nil,
						"upload_policy":            "manual",
						"rights_verified":          false,
						"classification_locked_at": nil,
					}).Error; err != nil {
					return fmt.Errorf("停止旧拼接成品 %s 失败: %w", video.VideoID, err)
				}
				continue
			}

			var completed int64
			if err := tx.Table("tb_task_steps").
				Distinct("step_name").
				Where(
					"video_id = ? AND step_name IN ? AND status = ?",
					video.VideoID,
					legacyPreparationSteps,
					"completed",
				).
				Count(&completed).Error; err != nil {
				return fmt.Errorf("检查旧任务 %s 处理进度失败: %w", video.VideoID, err)
			}

			updates := legacyPublicVideoUpdates(now, completed)
			if err := tx.Table("tb_saved_videos").
				Where("id = ?", video.ID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("恢复旧任务 %s 到公开流程失败: %w", video.VideoID, err)
			}
		}

		if tx.Migrator().HasTable("tb_compilation_batches") {
			if err := tx.Table("tb_compilation_batches").
				Where("state <> ?", "uploaded").
				Updates(map[string]interface{}{
					"state":      "cancelled",
					"last_error": "充电视频功能已撤回，批次已停止",
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("停止旧拼接批次失败: %w", err)
			}
		}
		if tx.Migrator().HasTable("tb_charge_pool_items") {
			if err := tx.Table("tb_charge_pool_items").
				Where("state IN ?", []string{"available", "reserved"}).
				Updates(map[string]interface{}{
					"state":      "error",
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("停用旧充电池条目失败: %w", err)
			}
		}
		if tx.Migrator().HasTable("tb_upload_jobs") {
			if err := tx.Table("tb_upload_jobs").
				Where(
					"compilation_batch_id IS NOT NULL AND status IN ?",
					[]string{"queued", "running"},
				).
				Updates(map[string]interface{}{
					"status":      "cancelled",
					"last_error":  "充电视频功能已撤回，上传作业已停止",
					"finished_at": now,
					"updated_at":  now,
				}).Error; err != nil {
				return fmt.Errorf("停止旧拼接上传作业失败: %w", err)
			}
		}
		return nil
	})
}

func legacyPublicVideoUpdates(now time.Time, completed int64) map[string]interface{} {
	status := "001"
	if completed == int64(len(legacyPreparationSteps)) {
		status = "200"
	}
	return map[string]interface{}{
		"status":                   status,
		"updated_at":               now,
		"publish_audience":         "",
		"audience_selected_at":     nil,
		"upower_preview_seconds":   0,
		"record_type":              "source",
		"workflow_state":           "",
		"ready_at":                 nil,
		"scheduled_upload_at":      nil,
		"upload_policy":            "scheduled",
		"rights_verified":          false,
		"classification_locked_at": nil,
	}
}

func hasDatabaseColumn(db *gorm.DB, table, column string) bool {
	columns, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		return false
	}
	for _, candidate := range columns {
		if strings.EqualFold(candidate.Name(), column) {
			return true
		}
	}
	return false
}
