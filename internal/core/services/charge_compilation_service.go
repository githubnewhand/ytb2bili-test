package services

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	downloadedAwaitingClassificationStatus = "100"
	queuedPreparationStatus                = "101"
	chargePoolAvailableStatus              = "110"
)

type ChargePoolSummary struct {
	Tier      int   `json:"tier"`
	Available int64 `json:"available"`
	Reserved  int64 `json:"reserved"`
	Consumed  int64 `json:"consumed"`
	Total     int64 `json:"total"`
}

type LegacyMediaBackfillResult struct {
	Scanned  int      `json:"scanned"`
	Updated  int      `json:"updated"`
	Problems []string `json:"problems,omitempty"`
}

type ChargeDownloadReconcileResult struct {
	Scanned  int      `json:"scanned"`
	Repaired int      `json:"repaired"`
	Problems []string `json:"problems,omitempty"`
}

type ChargeCompilationService struct {
	DB          *gorm.DB
	Config      *types.AppConfig
	mu          sync.Mutex
	reconcileMu sync.Mutex
}

func NewChargeCompilationService(db *gorm.DB, config *types.AppConfig) *ChargeCompilationService {
	return &ChargeCompilationService{DB: db, Config: config}
}

func ChargeTierFromAudience(audience string) (int, bool) {
	switch audience {
	case "charge_30":
		return 30, true
	case "charge_50":
		return 50, true
	default:
		return 0, false
	}
}

func ChargeAudienceFromTier(tier int) (string, bool) {
	switch tier {
	case 30:
		return "charge_30", true
	case 50:
		return "charge_50", true
	default:
		return "", false
	}
}

func (s *ChargeCompilationService) ClassifyVideo(videoID, audience string, previewSeconds int, rightsVerified bool) (*model.SavedVideo, error) {
	if audience != "free" {
		if !s.compilationEnabled() {
			return nil, fmt.Errorf("充电视频拼接功能当前已禁用")
		}
		if _, ok := ChargeTierFromAudience(audience); !ok {
			return nil, fmt.Errorf("无效的发布范围")
		}
		if !rightsVerified {
			return nil, fmt.Errorf("充电素材必须确认拥有合法版权或完整授权")
		}
		if s.Config != nil && s.Config.BilibiliConfig != nil && s.Config.BilibiliConfig.Copyright == 2 {
			return nil, fmt.Errorf("当前B站投稿配置为转载，不能创建充电专属视频")
		}
		if previewSeconds < 1 || previewSeconds > 6*60*60 {
			return nil, fmt.Errorf("充电专属视频试看时间必须在1秒到6小时之间")
		}
	} else {
		previewSeconds = 0
	}

	if err := s.ensureLegacyReadyMedia(videoID); err != nil {
		return nil, err
	}

	var result model.SavedVideo
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var video model.SavedVideo
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("video_id = ?", videoID).
			First(&video).Error; err != nil {
			return err
		}
		if video.RecordType == model.RecordTypeCompilation {
			return fmt.Errorf("合成视频不能重新分类")
		}
		if video.BiliBVID != "" || video.BiliAID != 0 {
			return fmt.Errorf("视频已经上传，不能修改发布范围")
		}
		if video.ClassificationLockedAt != nil {
			return fmt.Errorf("视频已被处理任务或拼接批次锁定，不能修改发布范围")
		}
		if video.Status == "201" || video.Status == "300" || video.Status == "301" || video.Status == "400" {
			return fmt.Errorf("当前状态不允许修改发布范围")
		}

		now := time.Now()
		updates := map[string]interface{}{
			"publish_audience":       audience,
			"audience_selected_at":   &now,
			"upower_preview_seconds": previewSeconds,
			"rights_verified":        rightsVerified,
			"scheduled_upload_at":    nil,
			"record_type":            model.RecordTypeSource,
		}

		if audience == "free" {
			var poolItem model.ChargePoolItem
			poolErr := tx.Where("saved_video_id = ?", video.ID).First(&poolItem).Error
			if poolErr == nil {
				if poolItem.State != model.ChargePoolStateAvailable {
					return fmt.Errorf("素材已经被拼接批次使用，不能改为免费公开")
				}
				if err := tx.Delete(&poolItem).Error; err != nil {
					return err
				}
			} else if !errors.Is(poolErr, gorm.ErrRecordNotFound) {
				return poolErr
			}

			if strings.TrimSpace(video.MediaPath) != "" && (video.Status == downloadedAwaitingClassificationStatus || video.Status == chargePoolAvailableStatus) {
				updates["status"] = queuedPreparationStatus
				updates["workflow_state"] = model.WorkflowStatePreparing
			}
			if video.Status == "200" || video.Status == "299" {
				delay := s.freeUploadDelay()
				readyAt := now
				if video.ReadyAt != nil {
					readyAt = *video.ReadyAt
				}
				scheduledAt := readyAt.Add(delay)
				updates["ready_at"] = &readyAt
				updates["scheduled_upload_at"] = &scheduledAt
				updates["upload_policy"] = model.UploadPolicyScheduled
				updates["workflow_state"] = model.WorkflowStateReady
				updates["classification_locked_at"] = &now
			}
		} else {
			tier, _ := ChargeTierFromAudience(audience)
			if strings.TrimSpace(video.MediaPath) != "" {
				if err := s.upsertAvailablePoolItem(tx, video.ID, tier); err != nil {
					return err
				}
				updates["status"] = chargePoolAvailableStatus
				updates["workflow_state"] = model.WorkflowStatePoolAvailable
				updates["upload_policy"] = model.UploadPolicyManual
				updates["ready_at"] = nil
			} else {
				if isExistingMediaStatus(video.Status) {
					return fmt.Errorf("已处理视频的本地媒体文件不可用，已阻止重新下载")
				}
				updates["workflow_state"] = model.WorkflowStateDownloading
			}
		}

		if err := tx.Model(&model.SavedVideo{}).Where("id = ?", video.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&result, video.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ChargeCompilationService) BackfillLegacyReadyMedia(ctx context.Context) (LegacyMediaBackfillResult, error) {
	result := LegacyMediaBackfillResult{}
	var videos []model.SavedVideo
	if err := s.DB.
		Where("status IN ? AND (record_type = ? OR record_type = '' OR record_type IS NULL)", []string{
			downloadedAwaitingClassificationStatus,
			chargePoolAvailableStatus,
			"200",
			"299",
		}, model.RecordTypeSource).
		Where("media_path = '' OR media_path IS NULL OR media_duration_ms = 0").
		Order("created_at ASC").
		Find(&videos).Error; err != nil {
		return result, err
	}

	result.Scanned = len(videos)
	for index := range videos {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := s.ensureExistingMediaMetadata(ctx, &videos[index]); err != nil {
			result.Problems = append(result.Problems, fmt.Sprintf("%s: %v", videos[index].VideoID, err))
			continue
		}
		result.Updated++
	}
	return result, nil
}

// ReconcileCompletedChargeDownloads repairs charge source videos whose download
// step completed but whose durable pool transition was interrupted or skipped.
func (s *ChargeCompilationService) ReconcileCompletedChargeDownloads(ctx context.Context) (ChargeDownloadReconcileResult, error) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	result := ChargeDownloadReconcileResult{}
	completedDownloads := s.DB.Model(&model.TaskStep{}).
		Select("1").
		Where("tb_task_steps.video_id = tb_saved_videos.video_id").
		Where("tb_task_steps.step_name = ?", "\u4e0b\u8f7d\u89c6\u9891").
		Where("tb_task_steps.status = ?", model.TaskStepStatusCompleted)

	var videos []model.SavedVideo
	if err := s.DB.
		Where("publish_audience IN ?", []string{"charge_30", "charge_50"}).
		Where("record_type = ?", model.RecordTypeSource).
		Where("workflow_state = ?", model.WorkflowStateDownloading).
		Where("status = ?", "002").
		Where("rights_verified = ?", true).
		Where("bili_bv_id = '' AND bili_a_id = 0").
		Where("EXISTS (?)", completedDownloads).
		Order("created_at ASC").
		Find(&videos).Error; err != nil {
		return result, err
	}

	result.Scanned = len(videos)
	for index := range videos {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		video := &videos[index]
		if err := s.ensureExistingMediaMetadata(ctx, video); err != nil {
			result.Problems = append(result.Problems, fmt.Sprintf("%s: %v", video.VideoID, err))
			continue
		}
		route, err := s.RouteDownloadedVideo(video.VideoID, video.MediaPath, video.MediaDurationMS, video.MediaProbeJSON)
		if err != nil {
			result.Problems = append(result.Problems, fmt.Sprintf("%s: %v", video.VideoID, err))
			continue
		}
		if route != "charge" {
			result.Problems = append(result.Problems, fmt.Sprintf("%s: unexpected route %q", video.VideoID, route))
			continue
		}
		result.Repaired++
	}
	return result, nil
}

func (s *ChargeCompilationService) ensureLegacyReadyMedia(videoID string) error {
	var video model.SavedVideo
	if err := s.DB.Where("video_id = ?", videoID).First(&video).Error; err != nil {
		return err
	}
	if !isExistingMediaStatus(video.Status) {
		return nil
	}
	if usableVideoFile(video.MediaPath) && video.MediaDurationMS > 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.ensureExistingMediaMetadata(ctx, &video); err != nil {
		return fmt.Errorf("无法使用已经处理好的本地视频，系统不会重新下载: %w", err)
	}
	return nil
}

func (s *ChargeCompilationService) ensureExistingMediaMetadata(ctx context.Context, video *model.SavedVideo) error {
	if video == nil {
		return fmt.Errorf("视频记录为空")
	}
	mediaPath := strings.TrimSpace(video.MediaPath)
	if !usableVideoFile(mediaPath) {
		var err error
		mediaPath, err = findLegacyMediaFile(s.Config, video)
		if err != nil {
			return err
		}
	}
	absolutePath, err := filepath.Abs(mediaPath)
	if err != nil {
		return fmt.Errorf("解析本地视频路径失败: %w", err)
	}
	probe, err := utils.ProbeMediaFile(ctx, absolutePath)
	if err != nil {
		return fmt.Errorf("检测本地视频失败: %w", err)
	}
	updates := map[string]interface{}{
		"media_path":        absolutePath,
		"media_duration_ms": probe.DurationMS,
		"media_probe_json":  probe.RawJSON,
		"record_type":       model.RecordTypeSource,
	}
	if err := s.DB.Model(&model.SavedVideo{}).Where("id = ?", video.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("回填本地视频信息失败: %w", err)
	}
	video.MediaPath = absolutePath
	video.MediaDurationMS = probe.DurationMS
	video.MediaProbeJSON = probe.RawJSON
	video.RecordType = model.RecordTypeSource
	return nil
}

func findLegacyMediaFile(config *types.AppConfig, video *model.SavedVideo) (string, error) {
	if config == nil || strings.TrimSpace(config.FileUpDir) == "" {
		return "", fmt.Errorf("媒体根目录未配置")
	}
	if video == nil || strings.TrimSpace(video.VideoID) == "" {
		return "", fmt.Errorf("视频ID为空")
	}
	videoID := strings.TrimSpace(video.VideoID)
	if filepath.Base(videoID) != videoID || videoID == "." || videoID == ".." {
		return "", fmt.Errorf("视频ID包含非法路径字符")
	}
	root, err := filepath.Abs(config.FileUpDir)
	if err != nil {
		return "", fmt.Errorf("解析媒体根目录失败: %w", err)
	}

	directories := []string{filepath.Join(root, video.CreatedAt.Format("2006-01-02"), videoID)}
	matches, _ := filepath.Glob(filepath.Join(root, "*", videoID))
	sort.Strings(matches)
	directories = append(directories, matches...)

	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		for _, extension := range []string{".mp4", ".mkv", ".webm", ".mov", ".m4v", ".avi", ".flv"} {
			candidate := filepath.Join(directory, videoID+extension)
			if usableVideoFile(candidate) {
				return candidate, nil
			}
		}
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isVideoExtension(filepath.Ext(entry.Name())) {
				continue
			}
			candidate := filepath.Join(directory, entry.Name())
			if usableVideoFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("任务目录中找不到现成视频文件")
}

func isExistingMediaStatus(status string) bool {
	switch status {
	case downloadedAwaitingClassificationStatus, chargePoolAvailableStatus, "200", "299":
		return true
	default:
		return false
	}
}

func usableVideoFile(path string) bool {
	if strings.TrimSpace(path) == "" || !isVideoExtension(filepath.Ext(path)) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func isVideoExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".avi", ".flv":
		return true
	default:
		return false
	}
}

// RouteDownloadedVideo persists the canonical media path and chooses the next
// durable route. It returns "awaiting", "free", or "charge".
func (s *ChargeCompilationService) RouteDownloadedVideo(videoID, mediaPath string, durationMS int64, probeJSON string) (string, error) {
	route := ""
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var video model.SavedVideo
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("video_id = ?", videoID).
			First(&video).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"media_path":        mediaPath,
			"media_duration_ms": durationMS,
			"media_probe_json":  probeJSON,
			"record_type":       model.RecordTypeSource,
		}
		switch video.PublishAudience {
		case "":
			route = "awaiting"
			updates["status"] = downloadedAwaitingClassificationStatus
			updates["workflow_state"] = model.WorkflowStateAwaitingClassification
		case "free":
			route = "free"
			updates["workflow_state"] = model.WorkflowStatePreparing
			now := time.Now()
			updates["classification_locked_at"] = &now
		case "charge_30", "charge_50":
			if !video.RightsVerified {
				return fmt.Errorf("充电素材尚未确认版权或授权")
			}
			tier, _ := ChargeTierFromAudience(video.PublishAudience)
			if err := s.upsertAvailablePoolItem(tx, video.ID, tier); err != nil {
				return err
			}
			route = "charge"
			updates["status"] = chargePoolAvailableStatus
			updates["workflow_state"] = model.WorkflowStatePoolAvailable
			updates["upload_policy"] = model.UploadPolicyManual
		default:
			return fmt.Errorf("未知发布范围: %s", video.PublishAudience)
		}
		return tx.Model(&model.SavedVideo{}).Where("id = ?", video.ID).Updates(updates).Error
	})
	return route, err
}

func (s *ChargeCompilationService) MarkFreeReady(videoID string) error {
	now := time.Now()
	scheduledAt := now.Add(s.freeUploadDelay())
	return s.DB.Model(&model.SavedVideo{}).
		Where("video_id = ?", videoID).
		Updates(map[string]interface{}{
			"status":                   "200",
			"workflow_state":           model.WorkflowStateReady,
			"ready_at":                 &now,
			"scheduled_upload_at":      &scheduledAt,
			"upload_policy":            model.UploadPolicyScheduled,
			"classification_locked_at": &now,
		}).Error
}

func (s *ChargeCompilationService) upsertAvailablePoolItem(tx *gorm.DB, savedVideoID uint, tier int) error {
	var item model.ChargePoolItem
	err := tx.Where("saved_video_id = ?", savedVideoID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.ChargePoolItem{
			SavedVideoID: savedVideoID,
			Tier:         tier,
			State:        model.ChargePoolStateAvailable,
			Version:      1,
		}).Error
	}
	if err != nil {
		return err
	}
	if item.State != model.ChargePoolStateAvailable {
		return fmt.Errorf("素材已被拼接批次锁定或使用")
	}
	return tx.Model(&item).Updates(map[string]interface{}{
		"tier":              tier,
		"reserved_batch_id": nil,
		"reserved_at":       nil,
		"version":           gorm.Expr("version + 1"),
	}).Error
}

func (s *ChargeCompilationService) PoolSummary() ([]ChargePoolSummary, error) {
	results := make([]ChargePoolSummary, 0, 2)
	for _, tier := range []int{30, 50} {
		summary := ChargePoolSummary{Tier: tier}
		if err := s.DB.Model(&model.ChargePoolItem{}).Where("tier = ? AND state = ?", tier, model.ChargePoolStateAvailable).Count(&summary.Available).Error; err != nil {
			return nil, err
		}
		if err := s.DB.Model(&model.ChargePoolItem{}).Where("tier = ? AND state = ?", tier, model.ChargePoolStateReserved).Count(&summary.Reserved).Error; err != nil {
			return nil, err
		}
		if err := s.DB.Model(&model.ChargePoolItem{}).Where("tier = ? AND state = ?", tier, model.ChargePoolStateConsumed).Count(&summary.Consumed).Error; err != nil {
			return nil, err
		}
		summary.Total = summary.Available + summary.Reserved + summary.Consumed
		results = append(results, summary)
	}
	return results, nil
}

func (s *ChargeCompilationService) ListPool(tier int, state string) ([]model.ChargePoolItem, error) {
	if _, ok := ChargeAudienceFromTier(tier); !ok {
		return nil, fmt.Errorf("档位只能是30或50")
	}
	query := s.DB.Preload("SavedVideo").Where("tier = ?", tier)
	if state != "" {
		query = query.Where("state = ?", state)
	}
	var items []model.ChargePoolItem
	err := query.Order("created_at ASC").Find(&items).Error
	return items, err
}

func (s *ChargeCompilationService) CreateDraft(tier int) (*model.CompilationBatch, error) {
	if !s.compilationEnabled() {
		return nil, fmt.Errorf("充电视频拼接功能当前已禁用")
	}
	if _, ok := ChargeAudienceFromTier(tier); !ok {
		return nil, fmt.Errorf("档位只能是30或50")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var batch model.CompilationBatch
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var candidates []model.ChargePoolItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("SavedVideo").
			Where("tier = ? AND state = ?", tier, model.ChargePoolStateAvailable).
			Order("created_at ASC").
			Find(&candidates).Error; err != nil {
			return err
		}
		minItems, maxItems := s.selectionBounds()
		if len(candidates) < minItems {
			return fmt.Errorf("%d元素材池至少需要%d个可用视频，当前只有%d个", tier, minItems, len(candidates))
		}

		countLimit := maxItems
		if len(candidates) < countLimit {
			countLimit = len(candidates)
		}
		seed, err := secureRandomSeed()
		if err != nil {
			return err
		}
		rng := mathrand.New(mathrand.NewSource(seed))
		count := minItems
		if countLimit > minItems {
			count += rng.Intn(countLimit - minItems + 1)
		}
		rng.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})
		selected := candidates[:count]

		key, err := newBatchKey()
		if err != nil {
			return err
		}
		batch = model.CompilationBatch{
			BatchKey:       key,
			Tier:           tier,
			State:          model.CompilationStateDraft,
			TargetCount:    count,
			ActualCount:    count,
			RandomSeed:     seed,
			PreviewSeconds: s.defaultPreviewSeconds(),
			UploadPolicy:   s.chargeUploadPolicy(),
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}

		now := time.Now()
		for index, candidate := range selected {
			claim := tx.Model(&model.ChargePoolItem{}).
				Where("id = ? AND state = ?", candidate.ID, model.ChargePoolStateAvailable).
				Updates(map[string]interface{}{
					"state":             model.ChargePoolStateReserved,
					"reserved_batch_id": batch.ID,
					"reserved_at":       &now,
					"version":           gorm.Expr("version + 1"),
				})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected != 1 {
				return fmt.Errorf("素材 %d 已被其他批次占用", candidate.ID)
			}
			if err := tx.Model(&model.SavedVideo{}).Where("id = ?", candidate.SavedVideoID).
				Updates(map[string]interface{}{
					"workflow_state":           model.WorkflowStatePoolReserved,
					"classification_locked_at": &now,
				}).Error; err != nil {
				return err
			}
			item := model.CompilationItem{
				BatchID:            batch.ID,
				SourceSavedVideoID: candidate.SavedVideoID,
				Position:           index + 1,
				SourceDurationMS:   candidate.SavedVideo.MediaDurationMS,
				SourcePathSnapshot: candidate.SavedVideo.MediaPath,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetBatch(batch.ID)
}

func (s *ChargeCompilationService) GetBatch(id uint) (*model.CompilationBatch, error) {
	var batch model.CompilationBatch
	err := s.DB.
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Preload("Items.SourceSavedVideo").
		Preload("OutputSavedVideo").
		First(&batch, id).Error
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (s *ChargeCompilationService) ListBatches(limit int) ([]model.CompilationBatch, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var batches []model.CompilationBatch
	err := s.DB.
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Preload("Items.SourceSavedVideo").
		Preload("OutputSavedVideo").
		Order("created_at DESC").
		Limit(limit).
		Find(&batches).Error
	return batches, err
}

func (s *ChargeCompilationService) ReorderDraft(batchID uint, sourceVideoIDs []uint) (*model.CompilationBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var batch model.CompilationBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, batchID).Error; err != nil {
			return err
		}
		if batch.State != model.CompilationStateDraft {
			return fmt.Errorf("只有草稿批次可以调整顺序")
		}
		var items []model.CompilationItem
		if err := tx.Where("batch_id = ?", batchID).Find(&items).Error; err != nil {
			return err
		}
		if len(items) != len(sourceVideoIDs) {
			return fmt.Errorf("提交的素材数量与批次不一致")
		}
		known := make(map[uint]model.CompilationItem, len(items))
		for _, item := range items {
			known[item.SourceSavedVideoID] = item
		}
		for _, sourceID := range sourceVideoIDs {
			if _, ok := known[sourceID]; !ok {
				return fmt.Errorf("素材 %d 不属于该批次", sourceID)
			}
		}
		if err := tx.Model(&model.CompilationItem{}).Where("batch_id = ?", batchID).
			Update("position", gorm.Expr("-position")).Error; err != nil {
			return err
		}
		for index, sourceID := range sourceVideoIDs {
			if err := tx.Model(&model.CompilationItem{}).
				Where("batch_id = ? AND source_saved_video_id = ?", batchID, sourceID).
				Update("position", index+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetBatch(batchID)
}

func (s *ChargeCompilationService) StartBatch(batchID uint, previewSeconds int, uploadPolicy string) (*model.CompilationBatch, error) {
	if !s.compilationEnabled() {
		return nil, fmt.Errorf("充电视频拼接功能当前已禁用")
	}
	if previewSeconds < 1 || previewSeconds > 6*60*60 {
		return nil, fmt.Errorf("试看时间必须在1秒到6小时之间")
	}
	if uploadPolicy != model.UploadPolicyImmediate && uploadPolicy != model.UploadPolicyManual {
		return nil, fmt.Errorf("充电批次上传策略只能是 immediate 或 manual")
	}
	allowed := []string{
		model.CompilationStateDraft,
		model.CompilationStateMergeFailed,
		model.CompilationStateProcessFailed,
		model.CompilationStateUploadFailed,
	}
	result := s.DB.Model(&model.CompilationBatch{}).
		Where("id = ? AND state IN ?", batchID, allowed).
		Updates(map[string]interface{}{
			"state":           model.CompilationStateQueued,
			"preview_seconds": previewSeconds,
			"upload_policy":   uploadPolicy,
			"last_error":      "",
			"retry_count":     gorm.Expr("retry_count + 1"),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, fmt.Errorf("批次状态已变化，无法启动")
	}
	return s.GetBatch(batchID)
}

func (s *ChargeCompilationService) CancelBatch(batchID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var batch model.CompilationBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, batchID).Error; err != nil {
			return err
		}
		switch batch.State {
		case model.CompilationStateCancelled:
			return nil
		case model.CompilationStateDraft,
			model.CompilationStateMergeFailed,
			model.CompilationStateProcessFailed,
			model.CompilationStateUploadFailed:
			// Drafts and stopped failed batches no longer have active workers.
		default:
			return fmt.Errorf("只有草稿或已失败的批次可以取消")
		}
		var poolItems []model.ChargePoolItem
		if err := tx.Where("reserved_batch_id = ? AND state = ?", batch.ID, model.ChargePoolStateReserved).
			Find(&poolItems).Error; err != nil {
			return err
		}
		for _, item := range poolItems {
			if err := tx.Model(&item).Updates(map[string]interface{}{
				"state":             model.ChargePoolStateAvailable,
				"reserved_batch_id": nil,
				"reserved_at":       nil,
				"version":           gorm.Expr("version + 1"),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.SavedVideo{}).Where("id = ?", item.SavedVideoID).
				Updates(map[string]interface{}{
					"workflow_state":           model.WorkflowStatePoolAvailable,
					"classification_locked_at": nil,
				}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&batch).Updates(map[string]interface{}{
			"state":      model.CompilationStateCancelled,
			"last_error": "",
		}).Error
	})
}

func (s *ChargeCompilationService) compilationEnabled() bool {
	return s.Config == nil || s.Config.ChargeCompilation.Enabled
}

func (s *ChargeCompilationService) selectionBounds() (int, int) {
	minItems, maxItems := 3, 4
	if s.Config != nil {
		if s.Config.ChargeCompilation.MinItems > 0 {
			minItems = s.Config.ChargeCompilation.MinItems
		}
		if s.Config.ChargeCompilation.MaxItems >= minItems {
			maxItems = s.Config.ChargeCompilation.MaxItems
		}
	}
	if maxItems < minItems {
		maxItems = minItems
	}
	return minItems, maxItems
}

func (s *ChargeCompilationService) defaultPreviewSeconds() int {
	if s.Config != nil && s.Config.ChargeCompilation.DefaultPreviewSeconds > 0 {
		return s.Config.ChargeCompilation.DefaultPreviewSeconds
	}
	return 180
}

func (s *ChargeCompilationService) freeUploadDelay() time.Duration {
	minutes := 60
	if s.Config != nil && s.Config.ChargeCompilation.FreeAutoUploadDelayMinutes > 0 {
		minutes = s.Config.ChargeCompilation.FreeAutoUploadDelayMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func (s *ChargeCompilationService) chargeUploadPolicy() string {
	if s.Config != nil && s.Config.ChargeCompilation.ChargeUploadPolicy == model.UploadPolicyManual {
		return model.UploadPolicyManual
	}
	return model.UploadPolicyImmediate
}

func secureRandomSeed() (int64, error) {
	var buffer [8]byte
	if _, err := cryptorand.Read(buffer[:]); err != nil {
		return 0, fmt.Errorf("生成随机种子失败: %w", err)
	}
	return int64(binary.LittleEndian.Uint64(buffer[:]) & ((1 << 63) - 1)), nil
}

func newBatchKey() (string, error) {
	var buffer [5]byte
	if _, err := cryptorand.Read(buffer[:]); err != nil {
		return "", fmt.Errorf("生成批次编号失败: %w", err)
	}
	return fmt.Sprintf("cmp_%d_%x", time.Now().UnixMilli(), buffer[:]), nil
}
