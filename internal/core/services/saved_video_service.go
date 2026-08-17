package services

import (
	"fmt"
	"time"

	"github.com/difyz9/ytb2bili/pkg/store/model"
	"gorm.io/gorm"
)

// SavedVideoService 保存视频服务
type SavedVideoService struct {
	DB *gorm.DB
}

// NewSavedVideoService 创建保存视频服务实例
func NewSavedVideoService(db *gorm.DB) *SavedVideoService {
	return &SavedVideoService{
		DB: db,
	}
}

// GetPendingVideos 获取待处理的视频列表（状态为 001 且 subtitles 不为空）
func (s *SavedVideoService) GetPendingVideos(limit int) ([]model.SavedVideo, error) {
	var videos []model.SavedVideo
	err := s.DB.Where("status IN ?", []string{"001", "101"}).
		Order("created_at ASC").
		Limit(limit).
		Find(&videos).Error
	return videos, err
}

// GetVideoByID 根据ID获取视频
func (s *SavedVideoService) GetVideoByID(id uint) (*model.SavedVideo, error) {
	var video model.SavedVideo
	err := s.DB.Where("id = ?", id).First(&video).Error
	if err != nil {
		return nil, err
	}
	return &video, nil
}

// GetVideoByVideoID 根据 VideoID 获取视频
func (s *SavedVideoService) GetVideoByVideoID(videoID string) (*model.SavedVideo, error) {
	var video model.SavedVideo
	err := s.DB.Where("video_id = ?", videoID).First(&video).Error
	if err != nil {
		return nil, err
	}
	return &video, nil
}

// UpdateStatus 更新视频状态
func (s *SavedVideoService) UpdateStatus(id uint, status string) error {
	return s.DB.Model(&model.SavedVideo{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// TryUpdateStatus 原子地将指定状态的视频切换到新状态。
// 返回 false 表示视频当前状态已经变化，调用方不应继续执行该任务。
func (s *SavedVideoService) TryUpdateStatus(id uint, fromStatus, toStatus string) (bool, error) {
	result := s.DB.Model(&model.SavedVideo{}).
		Where("id = ? AND status = ?", id, fromStatus).
		Update("status", toStatus)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 1 {
		return false, fmt.Errorf("unexpected rows affected while updating video status: %d", result.RowsAffected)
	}
	return result.RowsAffected == 1, nil
}

// SelectPublishAudience records the required human decision and starts the
// automatic-upload waiting period atomically.
func (s *SavedVideoService) SelectPublishAudience(id uint, audience string, previewSeconds int) (bool, error) {
	result := s.DB.Model(&model.SavedVideo{}).
		Where("id = ? AND status IN ? AND (bili_bv_id = ? OR bili_bv_id IS NULL) AND (bili_a_id = ? OR bili_a_id IS NULL)", id, []string{"200", "299"}, "", 0).
		Updates(map[string]interface{}{
			"publish_audience":       audience,
			"audience_selected_at":   time.Now(),
			"upower_preview_seconds": previewSeconds,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// UpdateVideo 更新视频信息
func (s *SavedVideoService) UpdateVideo(video *model.SavedVideo) error {
	return s.DB.Save(video).Error
}

// CreateVideo 创建新视频记录
func (s *SavedVideoService) CreateVideo(video *model.SavedVideo) error {
	return s.DB.Create(video).Error
}

// DeleteVideo 删除视频（软删除）
func (s *SavedVideoService) DeleteVideo(id uint) error {
	return s.DB.Delete(&model.SavedVideo{}, id).Error
}

// ListVideos 获取视频列表（支持分页和状态筛选）
func (s *SavedVideoService) ListVideos(page, pageSize int, status string) ([]model.SavedVideo, int64, error) {
	var videos []model.SavedVideo
	var total int64

	query := s.DB.Model(&model.SavedVideo{})

	// 如果指定状态，添加状态筛选
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&videos).Error

	return videos, total, err
}

// GetVideosByPlaylistID 根据播放列表ID获取视频列表
func (s *SavedVideoService) GetVideosByPlaylistID(playlistID string) ([]model.SavedVideo, error) {
	var videos []model.SavedVideo
	err := s.DB.Where("playlist_id = ?", playlistID).
		Order("created_at ASC").
		Find(&videos).Error
	return videos, err
}

// UpdateVideoStatus 批量更新视频状态
func (s *SavedVideoService) UpdateVideoStatus(ids []uint, status string) error {
	return s.DB.Model(&model.SavedVideo{}).
		Where("id IN ?", ids).
		Update("status", status).Error
}

// GetVideosPaginated 获取分页视频列表（用于前端显示）
func (s *SavedVideoService) GetVideosPaginated(offset, limit int) ([]model.SavedVideo, int, error) {
	var videos []model.SavedVideo
	var total int64

	// 获取总数
	if err := s.DB.Model(&model.SavedVideo{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := s.DB.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&videos).Error

	return videos, int(total), err
}

// GetByID 根据ID获取视频（别名方法，保持兼容性）
func (s *SavedVideoService) GetByID(id uint) (*model.SavedVideo, error) {
	return s.GetVideoByID(id)
}
