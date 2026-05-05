package services

import (
	"github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"fmt"
	"time"

	"gorm.io/gorm"
	"sync"
)

type TbVideoService struct {
	db   *gorm.DB
	lock sync.Mutex
}

func NewVideoService(db *gorm.DB) *TbVideoService {
	return &TbVideoService{
		db:   db,
		lock: sync.Mutex{},
	}
}

// SaveUrl 保存URL，如果存在则更新operation_type，不存在则保存到数据库
func (s *TbVideoService) SaveUrl(data *SaveUrlRequest) (*models.TbVideo, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 从URL提取videoId，如果请求中没有提供的话
	videoId := utils.ExtractVideoID(data.Url)

	// 转换OperationType从string到int

	// 检查是否已存在相同的videoId
	var existingUrl models.TbVideo
	dbErr := s.db.Where("video_id = ?", videoId).First(&existingUrl).Error

	if dbErr == nil {
		// 记录已存在，更新operation_type和其他字段
		fmt.Printf("=== 更新现有记录 ===\n")
		fmt.Printf("现有记录ID: %d\n", existingUrl.Id)
		fmt.Printf("更新前Title: %s\n", existingUrl.Title)
		fmt.Printf("更新前Description: %s\n", existingUrl.Description)

		// 使用 Select 确保所有字段都被更新，包括零值字段
		updates := models.TbVideo{
			URL:           data.Url,
			Title:         data.Title,
			PlaylistId:    data.PlaylistId,
			OperationType: data.OperationType,
			Description:   data.Description,
		}

		if err := s.db.Model(&existingUrl).Select("url", "title", "playlist_id", "operation_type", "description").Updates(updates).Error; err != nil {
			fmt.Printf("更新失败: %v\n", err)
			return nil, fmt.Errorf("更新URL失败: %v", err)
		}

		// 重新获取更新后的记录
		if err := s.db.First(&existingUrl, existingUrl.Id).Error; err != nil {
			return nil, fmt.Errorf("获取更新后的记录失败: %v", err)
		}

		fmt.Printf("更新后Title: %s\n", existingUrl.Title)
		fmt.Printf("更新后Description: %s\n", existingUrl.Description)
		fmt.Printf("==================\n")

		return &existingUrl, nil
	}

	if dbErr != gorm.ErrRecordNotFound {
		// 数据库查询错误
		return nil, fmt.Errorf("查询数据库失败: %v", dbErr)
	}

	// 记录不存在，创建新记录
	fmt.Printf("=== 创建新记录 ===\n")
	fmt.Printf("新记录VideoId: %s\n", videoId)
	fmt.Printf("新记录Title: %s\n", data.Title)
	fmt.Printf("新记录Description: %s\n", data.Description)

	saveUrl := &models.TbVideo{
		URL:           data.Url,
		Title:         data.Title,
		PlaylistId:    data.PlaylistId,
		VideoId:       videoId,
		Status:        "001", // 默认状态
		OperationType: data.OperationType,
		Description:   data.Description,
		CreatedAt:     time.Now(),
	}

	// 开始事务保存
	tx := s.db.Begin()
	if err := tx.Create(saveUrl).Error; err != nil {
		tx.Rollback()
		fmt.Printf("创建记录失败: %v\n", err)
		return nil, fmt.Errorf("保存URL失败: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}

	fmt.Printf("创建成功，记录ID: %d\n", saveUrl.Id)
	fmt.Printf("创建后Title: %s\n", saveUrl.Title)
	fmt.Printf("创建后Description: %s\n", saveUrl.Description)
	fmt.Printf("================\n")

	return saveUrl, nil
}

// GetUrlById 根据ID获取URL记录
func (s *TbVideoService) GetUrlById(id uint) (*models.TbVideo, error) {
	var saveUrl models.TbVideo
	if err := s.db.First(&saveUrl, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("记录不存在")
		}
		return nil, fmt.Errorf("查询失败: %v", err)
	}
	return &saveUrl, nil
}

// GetUrlByVideoId 根据VideoId获取URL记录
func (s *TbVideoService) GetUrlByVideoId(videoId string) (*models.TbVideo, error) {
	var saveUrl models.TbVideo
	if err := s.db.Where("video_id = ?", videoId).First(&saveUrl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("记录不存在")
		}
		return nil, fmt.Errorf("查询失败: %v", err)
	}
	return &saveUrl, nil
}

// UpdateUrl 更新URL记录
func (s *TbVideoService) UpdateUrl(id uint, data *SaveUrlRequest) (*models.TbVideo, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var saveUrl models.TbVideo
	if err := s.db.First(&saveUrl, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("记录不存在")
		}
		return nil, fmt.Errorf("查询失败: %v", err)
	}

	// 更新字段
	updates := map[string]interface{}{
		"url":            data.Url,
		"title":          data.Title,
		"operation_type": data.OperationType,
		"status":         data.Status,
		"description":    data.Description,
	}

	if err := s.db.Model(&saveUrl).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新失败: %v", err)
	}

	return &saveUrl, nil
}

// UpdateStatus 更新状态
func (s *TbVideoService) UpdateStatus(id uint, status string) error {
	if err := s.db.Model(&models.TbVideo{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		return fmt.Errorf("更新状态失败: %v", err)
	}
	return nil

}

func (s *TbVideoService) BatchUpdateStatus(ids []uint, status string) error {
	// 使用 GORM 的 Updates 方法批量更新
	if err := s.db.Model(&models.TbVideo{}).
		Where("id IN ?", ids).
		Update("status", status).Error; err != nil {
		return fmt.Errorf("批量更新状态失败: %v", err)
	}
	return nil
}

// UpdateStatus 更新状态
func (s *TbVideoService) UpdateItem(item *models.TbVideo) error {
	// 使用 GORM 的 Updates 方法，仅更新非空字段
	if err := s.db.Model(&models.TbVideo{}).Where("id = ?", item.Id).Updates(item).Error; err != nil {
		return err
	}
	return nil
}

// DeleteUrl 删除URL记录
func (s *TbVideoService) DeleteUrl(id uint) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.db.Delete(&models.TbVideo{}, id).Error; err != nil {
		return fmt.Errorf("删除失败: %v", err)
	}
	return nil
}

// ListUrls 分页获取URL列表
func (s *TbVideoService) ListUrls(page, pageSize int, status string) ([]*models.TbVideo, int64, error) {
	var urls []*models.TbVideo
	var total int64

	query := s.db.Model(&models.TbVideo{})

	// 如果指定了状态，添加状态过滤
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取总数失败: %v", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&urls).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %v", err)
	}

	return urls, total, nil
}

// GetUrlsByPlaylistId 根据播放列表ID获取URL列表
func (s *TbVideoService) GetUrlsByPlaylistId(playlistId string) ([]*models.TbVideo, error) {
	var urls []*models.TbVideo
	if err := s.db.Where("playlist_id = ?", playlistId).Order("created_at DESC").Find(&urls).Error; err != nil {
		return nil, fmt.Errorf("查询播放列表URL失败: %v", err)
	}
	return urls, nil
}

// SaveUrlRequest 保存URL请求结构
type SaveUrlRequest struct {
	Url           string `json:"url" binding:"required"`
	Title         string `json:"title"`
	PlaylistId    string `json:"playlistId"`
	Status        string `json:"status"`
	OperationType int    `json:"operationType"` // 改为string类型，与前端保持一致
	Description   string `json:"description"`
}

// UpdateUrlRequest 更新URL请求结构
type UpdateUrlRequest struct {
	Url           string `json:"url"`
	Title         string `json:"title"`
	PlaylistId    string `json:"playlistId"`
	OperationType string `json:"operationType"`
	Description   string `json:"description"`
	Status        string `json:"status"`
}
