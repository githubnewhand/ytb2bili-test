package handlers

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/difyz9/ytb2bili/internal/bilibiliutil"
	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/internal/storage"
	"github.com/difyz9/ytb2bili/pkg/cos"
)

type UploadSubtitleToBilibili struct {
	base.BaseTask
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
}

const maxBilibiliSubtitleRunes = 60
const (
	bilibiliSubtitleLanguageChinese = "zh"
	bilibiliSubtitleLanguageEnglish = "en"
)

func NewUploadSubtitleToBilibili(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *UploadSubtitleToBilibili {
	return &UploadSubtitleToBilibili{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

func (t *UploadSubtitleToBilibili) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Info("开始上传字幕到 Bilibili")
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 5, "检查视频BVID")

	// 1. 检查是否有BVID（视频已上传成功）
	bvid, exists := context["bili_bvid"].(string)
	if !exists || bvid == "" {
		// 尝试从数据库获取BVID
		savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
		if err != nil || savedVideo.BiliBVID == "" {
			errMsg := "没有找到BVID，无法上传字幕"
			t.App.Logger.Warn("⚠️  " + errMsg)
			context["error"] = errMsg
			return false
		}
		bvid = savedVideo.BiliBVID
	}

	t.App.Logger.Infof("📺 视频BVID: %s", bvid)
	types.ReportTaskProgress(context, 15, "检查Bilibili登录")

	// 2. 检查登录信息
	loginStore := storage.GetDefaultStore()
	if !loginStore.IsValid() {
		t.App.Logger.Error("❌ 没有有效的 Bilibili 登录信息，无法上传字幕")
		context["error"] = "未登录 Bilibili"
		return false
	}

	loginInfo, err := loginStore.Load()
	if err != nil {
		t.App.Logger.Errorf("❌ 加载登录信息失败: %v", err)
		context["error"] = "加载登录信息失败"
		return false
	}

	// 3. 查找字幕文件
	subtitleFiles := t.findSubtitleFiles()
	if len(subtitleFiles) == 0 {
		errMsg := "未找到可上传字幕文件（需要 zh_bilibili.srt、zh_optimized.srt、zh.srt、en.srt 或视频ID.srt）"
		t.App.Logger.Warn("⚠️  " + errMsg)
		types.ReportTaskProgress(context, 0, "未找到可上传字幕文件")
		context["error"] = errMsg
		return false
	}
	types.ReportTaskProgress(context, 30, fmt.Sprintf("找到%d个字幕文件", len(subtitleFiles)))

	// 4. 创建 Bilibili 客户端和字幕上传器
	client := bilibili.NewClient(bilibiliutil.BuildOptions(t.App.Config, 2*time.Minute)...)
	uploader := bilibili.NewSubtitleUploader(client, loginInfo)

	// 5. 上传字幕文件
	uploadedCount := 0
	var uploadErrors []string
	for _, subtitleFile := range subtitleFiles {
		t.App.Logger.Infof("📝 正在上传字幕: %s", filepath.Base(subtitleFile.Path))
		types.ReportTaskProgress(context, 40+(uploadedCount*45/len(subtitleFiles)), fmt.Sprintf("上传字幕 %s", filepath.Base(subtitleFile.Path)))

		err := uploader.UploadSubtitle(bvid, subtitleFile.Path, subtitleFile.Language)
		if err != nil {
			if bilibili.IsSubtitleAlreadyUploadedError(err) {
				t.App.Logger.Infof("✅ 字幕已存在，跳过重复上传: %s (%s)", filepath.Base(subtitleFile.Path), subtitleFile.Language)
				uploadedCount++
				types.ReportTaskProgress(context, 40+(uploadedCount*45/len(subtitleFiles)), fmt.Sprintf("字幕已存在 %d/%d", uploadedCount, len(subtitleFiles)))
				continue
			}

			t.App.Logger.Errorf("❌ 上传字幕失败 %s: %v", subtitleFile.Path, err)
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: %v", filepath.Base(subtitleFile.Path), err))
			// 继续上传其他字幕文件，不因为一个失败就停止
			continue
		}

		t.App.Logger.Infof("✅ 字幕上传成功: %s (%s)", filepath.Base(subtitleFile.Path), subtitleFile.Language)
		uploadedCount++
		types.ReportTaskProgress(context, 40+(uploadedCount*45/len(subtitleFiles)), fmt.Sprintf("已上传字幕 %d/%d", uploadedCount, len(subtitleFiles)))
	}

	// 6. 记录结果
	if uploadedCount > 0 {
		t.App.Logger.Info("========================================")
		t.App.Logger.Infof("✅ 字幕上传完成！成功上传 %d 个字幕文件", uploadedCount)
		t.App.Logger.Infof("  视频链接: https://www.bilibili.com/video/%s", bvid)
		t.App.Logger.Info("========================================")

		context["subtitle_upload_count"] = uploadedCount
		types.ReportTaskProgress(context, 100, "字幕上传完成")
		return true
	} else {
		t.App.Logger.Error("❌ 没有成功上传任何字幕文件")
		if len(uploadErrors) > 0 {
			context["error"] = "字幕上传失败: " + strings.Join(uploadErrors, "; ")
		} else {
			context["error"] = "字幕上传失败"
		}
		return false
	}
}

// SubtitleFileInfo 字幕文件信息
type SubtitleFileInfo struct {
	Path     string
	Language string
}

// findSubtitleFiles 查找字幕文件
func (t *UploadSubtitleToBilibili) findSubtitleFiles() []SubtitleFileInfo {
	var subtitleFiles []SubtitleFileInfo

	if zhPath := t.findFirstExistingSubtitle([]string{"zh_bilibili.srt", "zh_optimized.srt", "zh.srt"}); zhPath != "" {
		uploadPath := zhPath
		if safePath, err := t.prepareBilibiliSafeSubtitle(zhPath); err != nil {
			t.App.Logger.Warnf("⚠️ 生成B站安全字幕失败，将使用原字幕: %v", err)
		} else {
			uploadPath = safePath
		}

		subtitleFiles = append(subtitleFiles, SubtitleFileInfo{
			Path:     uploadPath,
			Language: bilibiliSubtitleLanguageChinese,
		})
		t.App.Logger.Infof("🎯 找到中文字幕文件: %s", filepath.Base(uploadPath))
	}

	if enPath := t.findFirstExistingSubtitle([]string{"en.srt", fmt.Sprintf("%s.srt", t.StateManager.VideoID)}); enPath != "" {
		subtitleFiles = append(subtitleFiles, SubtitleFileInfo{
			Path:     enPath,
			Language: bilibiliSubtitleLanguageEnglish,
		})
		t.App.Logger.Infof("🎯 找到英文字幕文件: %s", filepath.Base(enPath))
	}

	return subtitleFiles
}

func (t *UploadSubtitleToBilibili) findFirstExistingSubtitle(filenames []string) string {
	for _, filename := range filenames {
		fullPath := filepath.Join(t.StateManager.CurrentDir, filename)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() && info.Size() > 0 {
			return fullPath
		}
	}
	return ""
}

type srtEntry struct {
	StartMs int64
	EndMs   int64
	Text    string
}

func (t *UploadSubtitleToBilibili) prepareBilibiliSafeSubtitle(sourcePath string) (string, error) {
	entries, err := parseSRTEntries(sourcePath)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("字幕文件为空")
	}

	var safeEntries []srtEntry
	changed := false
	for _, entry := range entries {
		parts := splitSubtitleText(entry.Text, maxBilibiliSubtitleRunes)
		if len(parts) > 1 {
			changed = true
		}
		safeEntries = append(safeEntries, splitEntryByText(entry, parts)...)
	}

	if !changed {
		return sourcePath, nil
	}

	outputPath := filepath.Join(filepath.Dir(sourcePath), "zh_bilibili.srt")
	if err := writeSRTEntries(outputPath, safeEntries); err != nil {
		return "", err
	}

	t.App.Logger.Infof("✅ 已生成B站安全字幕: %s (原%d条 -> 新%d条)", outputPath, len(entries), len(safeEntries))
	return outputPath, nil
}

func parseSRTEntries(path string) ([]srtEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []srtEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	var timeCode string
	var textLines []string
	stage := 0

	flush := func() error {
		if timeCode == "" || len(textLines) == 0 {
			timeCode = ""
			textLines = nil
			stage = 0
			return nil
		}
		start, end, err := parseTimeCode(timeCode)
		if err != nil {
			return err
		}
		text := normalizeSubtitleText(strings.Join(textLines, " "))
		if text != "" {
			entries = append(entries, srtEntry{StartMs: start, EndMs: end, Text: text})
		}
		timeCode = ""
		textLines = nil
		stage = 0
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}

		switch stage {
		case 0:
			if _, err := strconv.Atoi(line); err == nil {
				stage = 1
			}
		case 1:
			if strings.Contains(line, "-->") {
				timeCode = line
				stage = 2
			}
		default:
			textLines = append(textLines, line)
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func parseTimeCode(timeCode string) (int64, int64, error) {
	parts := strings.Split(timeCode, "-->")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("无效时间码: %s", timeCode)
	}

	start, err := parseSRTTimestamp(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}

	endPart := strings.TrimSpace(parts[1])
	if fields := strings.Fields(endPart); len(fields) > 0 {
		endPart = fields[0]
	}
	end, err := parseSRTTimestamp(endPart)
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		end = start + 1
	}

	return start, end, nil
}

var srtTimestampPattern = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2}),(\d{3})$`)

func parseSRTTimestamp(value string) (int64, error) {
	matches := srtTimestampPattern.FindStringSubmatch(value)
	if len(matches) != 5 {
		return 0, fmt.Errorf("无效SRT时间: %s", value)
	}

	hours, _ := strconv.ParseInt(matches[1], 10, 64)
	minutes, _ := strconv.ParseInt(matches[2], 10, 64)
	seconds, _ := strconv.ParseInt(matches[3], 10, 64)
	millis, _ := strconv.ParseInt(matches[4], 10, 64)
	return (((hours*60)+minutes)*60+seconds)*1000 + millis, nil
}

func formatSRTTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3600000
	ms %= 3600000
	minutes := ms / 60000
	ms %= 60000
	seconds := ms / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func normalizeSubtitleText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func splitSubtitleText(text string, maxRunes int) []string {
	text = normalizeSubtitleText(text)
	if text == "" {
		return nil
	}
	if len([]rune(text)) <= maxRunes {
		return []string{text}
	}

	var parts []string
	var current []rune
	flush := func() {
		part := strings.TrimSpace(string(current))
		if part != "" {
			parts = append(parts, part)
		}
		current = nil
	}

	for _, r := range []rune(text) {
		current = append(current, r)
		if len(current) >= maxRunes || strings.ContainsRune("。！？!?；;，,", r) && len(current) >= maxRunes/2 {
			flush()
		}
	}
	flush()

	return parts
}

func splitEntryByText(entry srtEntry, parts []string) []srtEntry {
	if len(parts) <= 1 {
		return []srtEntry{entry}
	}

	duration := entry.EndMs - entry.StartMs
	if duration < int64(len(parts)) {
		duration = int64(len(parts))
	}

	result := make([]srtEntry, 0, len(parts))
	for i, part := range parts {
		start := entry.StartMs + duration*int64(i)/int64(len(parts))
		end := entry.StartMs + duration*int64(i+1)/int64(len(parts))
		if end <= start {
			end = start + 1
		}
		result = append(result, srtEntry{
			StartMs: start,
			EndMs:   end,
			Text:    part,
		})
	}
	return result
}

func writeSRTEntries(path string, entries []srtEntry) error {
	var builder strings.Builder
	for i, entry := range entries {
		builder.WriteString(strconv.Itoa(i + 1))
		builder.WriteString("\n")
		builder.WriteString(formatSRTTimestamp(entry.StartMs))
		builder.WriteString(" --> ")
		builder.WriteString(formatSRTTimestamp(entry.EndMs))
		builder.WriteString("\n")
		builder.WriteString(entry.Text)
		builder.WriteString("\n\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0644)
}
