package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	"github.com/difyz9/ytb2bili/pkg/utils"
)

const (
	bilibiliUploadAPITimeout       = 2 * time.Minute
	bilibiliVideoUploadMaxAttempts = 3
	bilibiliCoverUploadMaxAttempts = 2
	bilibiliSubmitMaxAttempts      = 3
)

// https://github.com/biliup/biliup/issues/65

// 参考分区表
// https://github.com/biliup/biliup/wiki

// fetchAndSaveMetadata 尝试从 YouTube 获取元数据并保存到数据库
func (t *UploadToBilibili) fetchAndSaveMetadata(videoID string) error {
	t.App.Logger.Infof("🔄 尝试补充获取视频元数据: %s", videoID)

	// 1. 找到 yt-dlp
	var installDir string
	if t.App.Config != nil && t.App.Config.YtDlpPath != "" {
		installDir = t.App.Config.YtDlpPath
	}
	manager := utils.NewYtDlpManager(t.App.Logger, installDir)
	if !manager.IsInstalled() {
		return fmt.Errorf("未找到 yt-dlp")
	}
	ytdlpPath := manager.GetBinaryPath()

	// 2. 构建命令
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	command := []string{
		ytdlpPath,
		"--dump-json",
		"--no-download",
		videoURL,
	}

	// 添加 cookies 支持
	configDir := filepath.Dir(t.App.Config.Path)
	cookiesPath := filepath.Join(configDir, "cookies.txt")
	// 如果配置文件目录下的 cookies.txt 不存在，尝试当前目录
	if _, err := os.Stat(cookiesPath); err != nil {
		cookiesPath = "cookies.txt"
	}
	if _, err := os.Stat(cookiesPath); err == nil {
		absPath, _ := filepath.Abs(cookiesPath)
		command = append(command, "--cookies", absPath)
	}

	// 添加代理
	if t.App.Config != nil && t.App.Config.ProxyConfig != nil && t.App.Config.ProxyConfig.UseProxy && t.App.Config.ProxyConfig.ProxyHost != "" {
		command = append(command, "--proxy", t.App.Config.ProxyConfig.ProxyHost)
	}

	// 3. 执行命令
	cmd := exec.Command(command[0], command[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("执行 yt-dlp 失败: %v", err)
	}

	// 4. 解析 JSON
	var metadata VideoMetadataInfo
	if err := json.Unmarshal(output, &metadata); err != nil {
		return fmt.Errorf("解析元数据失败: %v", err)
	}

	// 5. 更新数据库
	savedVideo, err := t.SavedVideoService.GetVideoByVideoID(videoID)
	if err != nil {
		return fmt.Errorf("获取视频记录失败: %v", err)
	}

	savedVideo.Title = metadata.Title
	savedVideo.Description = metadata.Description
	// 如果需要，也可以更新其他字段

	if err := t.SavedVideoService.UpdateVideo(savedVideo); err != nil {
		return fmt.Errorf("更新数据库失败: %v", err)
	}

	t.App.Logger.Infof("✅ 成功补充获取并保存元数据: %s", metadata.Title)
	return nil
}

type UploadToBilibili struct {
	base.BaseTask
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
	LoginStore        *storage.LoginStore // 可选：注入的登录存储
}

func NewUploadToBilibili(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *UploadToBilibili {
	return &UploadToBilibili{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

func (t *UploadToBilibili) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Info("开始上传视频到 Bilibili")
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 0, "检查Bilibili登录")

	// 1. 检查登录信息
	var loginStore *storage.LoginStore
	if t.LoginStore != nil {
		loginStore = t.LoginStore
	} else {
		loginStore = storage.GetDefaultStore()
	}

	if !loginStore.IsValid() {
		t.App.Logger.Error("❌ 没有有效的 Bilibili 登录信息，请先扫码登录")
		context["error"] = "未登录 Bilibili"
		return false
	}

	loginInfo, err := loginStore.Load()
	if err != nil {
		t.App.Logger.Errorf("❌ 加载登录信息失败: %v", err)
		context["error"] = fmt.Sprintf("加载登录信息失败: %v", err)
		return false
	}

	t.App.Logger.Infof("✓ 已加载登录信息，用户 MID: %d", loginInfo.TokenInfo.Mid)
	types.ReportTaskProgress(context, 0, "登录信息有效")

	// 2. 检查并准备元数据 (如果在之前的步骤中未获取到)
	types.ReportTaskProgress(context, 0, "检查投稿元数据")
	savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
	if err == nil && savedVideo != nil && savedVideo.Title == "" {
		t.App.Logger.Info("ℹ️ 视频标题为空，尝试补充获取元数据...")
		if err := t.fetchAndSaveMetadata(t.StateManager.VideoID); err != nil {
			t.App.Logger.Warnf("⚠️ 补充获取元数据失败: %v", err)
		} else {
			// 重新获取最新的视频信息
			savedVideo, _ = t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
		}
	} else if err != nil {
		t.App.Logger.Warnf("⚠️ 无法从数据库获取视频信息: %v", err)
	}

	// 3. 查找下载的视频文件
	videoFiles := t.findVideoFiles()
	if len(videoFiles) == 0 {
		errMsg := "未找到视频文件"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	videoPath := videoFiles[0] // 使用第一个视频文件
	t.App.Logger.Infof("📹 找到视频文件: %s", filepath.Base(videoPath))
	types.ReportTaskProgress(context, 0, "找到待上传视频文件")

	// 4. Upload video to Bilibili

	types.ReportTaskProgress(context, 0, "开始上传视频文件")
	video, err := t.uploadVideoWithRetry(loginInfo, videoPath, context)
	if err != nil {
		userFriendlyError := t.getUserFriendlyError(err, "上传视频")
		t.App.Logger.Errorf("❌ 上传视频失败: %v", err)
		context["error"] = userFriendlyError
		return false
	}

	t.App.Logger.Infof("✓ 视频上传成功！")
	t.App.Logger.Infof("  Filename: %s", video.Filename)
	t.App.Logger.Infof("  Title: %s", video.Title)
	types.ReportTaskProgress(context, 100, "视频文件上传完成")

	// 5. Upload cover when available
	coverURL := ""
	if coverImagePath, ok := context["cover_image_path"].(string); ok && coverImagePath != "" {
		t.App.Logger.Infof("📸 找到封面图片: %s", filepath.Base(coverImagePath))
		types.ReportTaskProgress(context, 100, "上传封面")

		uploadedCoverURL, err := t.uploadCoverWithRetry(loginInfo, coverImagePath)
		if err != nil {
			t.App.Logger.Errorf("❌ 上传封面失败: %v", err)
			t.App.Logger.Warn("⚠️ 将使用默认封面或截取视频画面")
		} else {
			coverURL = uploadedCoverURL
			t.App.Logger.Infof("✓ 封面上传成功: %s", coverURL)
		}
	}

	// 6. Build studio payload
	types.ReportTaskProgress(context, 100, "构建投稿信息")
	studio := t.buildStudioInfo(video, coverURL, context)

	// 7. Submit video to Bilibili
	t.App.Logger.Info("Submitting video metadata...")
	t.App.Logger.Debugf("Studio title: %s", studio.Title)
	t.App.Logger.Debugf("Studio category: %d", studio.Tid)
	types.ReportTaskProgress(context, 100, "提交投稿审核")

	result, err := t.submitVideoWithRetry(loginInfo, studio)
	if err != nil {
		userFriendlyError := t.getUserFriendlyError(err, "提交视频")
		t.App.Logger.Errorf("❌ 提交视频失败: %v", err)
		context["error"] = userFriendlyError
		return false
	}

	// 9. 检查提交结果
	if result.Code != 0 {
		errMsg := fmt.Sprintf("提交失败: code=%d, message=%s", result.Code, result.Message)
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	// 9. 保存上传结果到数据库
	context["bili_video"] = video
	context["bili_result"] = result

	// 10. 保存结果信息到数据库和context
	t.App.Logger.Info("💾 保存上传结果到数据库...")
	types.ReportTaskProgress(context, 100, "保存投稿结果")
	savedVideo, err = t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
	if err != nil {
		t.App.Logger.Errorf("❌ 获取视频记录失败: %v", err)
	} else {
		// 尝试从 result.Data 中解析 BVID 和 AID
		if result.Data != nil {
			if dataMap, ok := result.Data.(map[string]interface{}); ok {
				if bvid, exists := dataMap["bvid"]; exists {
					if bvidStr, ok := bvid.(string); ok {
						savedVideo.BiliBVID = bvidStr
						// 保存BVID到context供后续字幕上传使用
						context["bili_bvid"] = bvidStr
						t.App.Logger.Infof("📺 BVID: %s", bvidStr)
					}
				}
				if aid, exists := dataMap["aid"]; exists {
					if aidFloat, ok := aid.(float64); ok {
						savedVideo.BiliAID = int64(aidFloat)
						// 保存AID到context
						context["bili_aid"] = int64(aidFloat)
						t.App.Logger.Infof("🆔 AID: %d", int64(aidFloat))
					}
				}
			}
		}

		// 更新视频状态为 300 (已上传)
		savedVideo.Status = "300"
		if err := t.SavedVideoService.UpdateVideo(savedVideo); err != nil {
			t.App.Logger.Errorf("❌ 保存上传结果到数据库失败: %v", err)
		} else {
			t.App.Logger.Info("✅ 上传结果已保存到数据库，状态已更新为 300")
		}
	}

	// 10. 输出成功信息
	t.App.Logger.Info("========================================")
	t.App.Logger.Infof("✓ 视频投稿成功！")
	if savedVideo != nil && savedVideo.BiliBVID != "" {
		t.App.Logger.Infof("  BVID: %s", savedVideo.BiliBVID)
		t.App.Logger.Infof("  访问链接: https://www.bilibili.com/video/%s", savedVideo.BiliBVID)
	}
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 100, "视频上传步骤收尾")

	return true
}

// findVideoFiles 查找下载目录中的视频文件
func (t *UploadToBilibili) findVideoFiles() []string {
	var videoFiles []string
	videoExtensions := []string{".mp4", ".flv", ".mkv", ".webm", ".avi", ".mov"}

	files, err := os.ReadDir(t.StateManager.CurrentDir)
	if err != nil {
		t.App.Logger.Errorf("读取目录失败: %v", err)
		return videoFiles
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(file.Name()))
		for _, videoExt := range videoExtensions {
			if ext == videoExt {
				fullPath := filepath.Join(t.StateManager.CurrentDir, file.Name())
				videoFiles = append(videoFiles, fullPath)
				break
			}
		}
	}

	return videoFiles
}

// buildStudioInfo 构建投稿信息
func (t *UploadToBilibili) buildStudioInfo(video *bilibili.Video, coverURL string, context map[string]interface{}) *bilibili.Studio {
	// 默认值
	title := t.StateManager.VideoID
	desc := "自动上传的视频"
	tags := "视频"

	// 从数据库查询视频的标题和描述信息
	savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
	if err != nil {
		t.App.Logger.Warnf("⚠️ 无法从数据库获取视频信息: %v，将使用默认值", err)
	} else {
		// 此处不再重复调用 fetchAndSaveMetadata，已在 Execute 中处理

		// 清理标题中的标签（#hashtag）
		cleanTitle := func(title string) string {
			// 使用正则表达式移除 #标签
			re := regexp.MustCompile(`\s*#[^\s#]+`)
			cleaned := re.ReplaceAllString(title, "")
			// 清理多余的空格
			cleaned = strings.TrimSpace(cleaned)
			// 将多个连续空格替换为单个空格
			re2 := regexp.MustCompile(`\s+`)
			cleaned = re2.ReplaceAllString(cleaned, " ")
			return cleaned
		}

		// 根据配置选择标题来源
		biliConfig := t.App.Config.BilibiliConfig
		if biliConfig != nil && biliConfig.CustomTitleTemplate != "" {
			// 使用自定义标题模板
			title = biliConfig.CustomTitleTemplate
			// 清理原标题中的标签
			cleanedOriginalTitle := cleanTitle(savedVideo.Title)
			title = strings.ReplaceAll(title, "{original_title}", cleanedOriginalTitle)
			title = strings.ReplaceAll(title, "{ai_title}", savedVideo.GeneratedTitle)
			t.App.Logger.Infof("✓ 使用自定义标题模板: %s", title)
		} else if biliConfig != nil && !biliConfig.UseOriginalTitle {
			// 配置为使用AI生成标题
			if savedVideo.GeneratedTitle != "" {
				title = savedVideo.GeneratedTitle
				t.App.Logger.Infof("✓ 使用AI生成的标题: %s", title)
			} else if savedVideo.Title != "" {
				title = cleanTitle(savedVideo.Title)
				t.App.Logger.Infof("✓ AI标题不存在，回退使用原始标题（已清理标签）: %s", title)
			}
		} else {
			// 默认使用原始标题（YouTube原标题）
			if savedVideo.Title != "" {
				title = cleanTitle(savedVideo.Title)
				t.App.Logger.Infof("✓ 使用YouTube原始标题（已清理标签）: %s", title)
			} else if savedVideo.GeneratedTitle != "" {
				title = savedVideo.GeneratedTitle
				t.App.Logger.Infof("✓ 原始标题不存在，回退使用AI标题: %s", title)
			}
		}

		// B站标题长度限制（80个字符）
		const maxTitleLength = 80
		titleRunes := []rune(title)
		if len(titleRunes) > maxTitleLength {
			title = string(titleRunes[:maxTitleLength])
			t.App.Logger.Warnf("⚠️ 标题过长，已截断至 %d 字符: %s", maxTitleLength, title)
		}
		t.App.Logger.Infof("📝 标题长度: %d/%d 字符", len([]rune(title)), maxTitleLength)

		// 过滤无效的描述（YouTube的默认描述）
		isValidDescription := func(desc string) bool {
			if desc == "" {
				return false
			}
			// 过滤YouTube的默认描述
			invalidDescriptions := []string{
				"YouTube",
				"自动上传的视频",
				"Uploaded by",
				"Auto-generated",
			}
			for _, invalid := range invalidDescriptions {
				if strings.Contains(desc, invalid) && len(desc) < 50 {
					return false
				}
			}
			return true
		}

		// 根据配置选择描述来源
		if biliConfig != nil && biliConfig.CustomDescTemplate != "" {
			// 使用自定义模板
			desc = biliConfig.CustomDescTemplate
			desc = strings.ReplaceAll(desc, "{original_desc}", savedVideo.Description)
			desc = strings.ReplaceAll(desc, "{ai_desc}", savedVideo.GeneratedDesc)
			t.App.Logger.Infof("✓ 使用自定义描述模板")
		} else if biliConfig != nil && biliConfig.UseOriginalDesc {
			// 配置为使用原始描述
			if isValidDescription(savedVideo.Description) {
				desc = savedVideo.Description
				t.App.Logger.Infof("✓ 使用YouTube原始描述")
			} else if savedVideo.GeneratedDesc != "" {
				desc = savedVideo.GeneratedDesc
				t.App.Logger.Infof("✓ 原始描述无效，回退使用AI描述")
			} else {
				desc = ""
				t.App.Logger.Info("✓ 无有效描述，仅使用原视频链接")
			}
		} else {
			// 默认使用AI生成的描述 + 原视频简介
			aiIntro := ""
			originalDesc := ""

			// 获取AI生成的精炼介绍（100字以内）
			if savedVideo.GeneratedDesc != "" {
				aiIntro = savedVideo.GeneratedDesc
				t.App.Logger.Infof("✓ AI生成的精炼介绍: %s", aiIntro)
			}

			// 获取原视频简介
			if isValidDescription(savedVideo.Description) {
				originalDesc = savedVideo.Description
				t.App.Logger.Infof("✓ 原视频简介长度: %d 字符", len([]rune(originalDesc)))
			}

			// 拼接描述：AI介绍 + 分隔线 + 原视频简介
			if aiIntro != "" && originalDesc != "" {
				desc = fmt.Sprintf("%s\n\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n📄 原视频简介：\n%s", aiIntro, originalDesc)
				t.App.Logger.Info("✓ 使用AI介绍 + 原视频简介")
			} else if aiIntro != "" {
				desc = aiIntro
				t.App.Logger.Info("✓ 仅使用AI介绍")
			} else if originalDesc != "" {
				desc = originalDesc
				t.App.Logger.Info("✓ 仅使用原视频简介")
			} else {
				desc = ""
				t.App.Logger.Info("✓ 无有效描述，仅使用原视频链接")
			}
		}

		// 使用AI生成的标签
		if savedVideo.GeneratedTags != "" {
			tags = savedVideo.GeneratedTags
			t.App.Logger.Infof("✓ 使用数据库中AI生成的标签: %s", tags)
		}

		// B站简介字数限制（2000字）
		const maxDescLength = 2000

		// 在描述末尾添加原视频链接
		linkSuffix := ""
		if savedVideo.URL != "" {
			linkSuffix = fmt.Sprintf("\n\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n📺 原视频链接：%s\n🔄 本视频为转载内容，仅供学习交流使用", savedVideo.URL)
		}

		// 计算链接后缀的长度（字符数）
		linkSuffixLength := len([]rune(linkSuffix))
		t.App.Logger.Infof("🔗 原视频链接后缀长度: %d 字符", linkSuffixLength)

		// 预先截断描述，确保有足够空间给链接
		descRunes := []rune(desc)
		originalDescLength := len(descRunes)
		t.App.Logger.Infof("📄 原始描述长度: %d 字符", originalDescLength)

		// 计算可用的描述长度（留20个字符的安全缓冲）
		maxAllowedDescLength := maxDescLength - linkSuffixLength - 20
		if maxAllowedDescLength < 0 {
			maxAllowedDescLength = 0
		}

		// 如果描述超过可用长度，截断它
		if len(descRunes) > maxAllowedDescLength {
			if maxAllowedDescLength > 3 {
				desc = string(descRunes[:maxAllowedDescLength]) + "..."
				t.App.Logger.Warnf("⚠️ 描述过长，已截断至 %d 字符（原长度: %d）", maxAllowedDescLength, originalDescLength)
			} else {
				desc = ""
				t.App.Logger.Warn("⚠️ 空间不足，已清空描述内容，仅保留原视频链接")
			}
		}

		// 添加链接后缀
		if linkSuffix != "" {
			desc += linkSuffix
			t.App.Logger.Infof("✓ 已添加原视频链接到描述")
		}

		// 最终检查长度
		finalDescLength := len([]rune(desc))
		t.App.Logger.Infof("📝 最终描述长度: %d/%d 字符", finalDescLength, maxDescLength)

		// 最后的安全检查，如果还是超长，强制截断
		if finalDescLength > maxDescLength {
			desc = string([]rune(desc)[:maxDescLength])
			t.App.Logger.Errorf("❌ 描述仍然超长！强制截断至 %d 字符", maxDescLength)
		}
	}

	// 封面上传已移至 Execute 方法处理，此处仅接收 coverURL
	if coverURL != "" {
		t.App.Logger.Infof("🖼️ 使用封面URL: %s", coverURL)
	} else if context["cover_image_path"] != nil {
		t.App.Logger.Warn("⚠️ 有封面图片路径但未上传成功，视频可能使用默认截屏封面")
	}

	// 检查是否有中文字幕
	zhSRTPath := filepath.Join(t.StateManager.CurrentDir, "zh.srt")
	hasZhSubtitle := false
	if _, err := os.Stat(zhSRTPath); err == nil {
		hasZhSubtitle = true
		t.App.Logger.Info("✓ 检测到中文字幕文件")
	}

	// 更新video对象的Title为翻译后的标题
	video.Title = title
	t.App.Logger.Infof("✓ 设置视频Title为: %s", title)

	// 读取配置
	copyright := 1 // 默认自制
	noReprint := 1 // 默认禁止转载
	source := ""
	tid := 21                    // 默认分区
	dynamic := "发布了新视频！"         // 默认动态
	openElec := 0                // 默认关闭充电
	selectionReserve := int64(0) // 默认不参与活动
	upSelectionReply := 0        // 默认不展示推荐评论
	upCloseReply := 0            // 默认开启评论
	upCloseReward := 0           // 默认开启打赏

	if t.App.Config.BilibiliConfig != nil {
		if t.App.Config.BilibiliConfig.Copyright > 0 {
			copyright = t.App.Config.BilibiliConfig.Copyright
		}

		noReprint = t.App.Config.BilibiliConfig.NoReprint
		source = t.App.Config.BilibiliConfig.Source

		// 读取新增配置
		if t.App.Config.BilibiliConfig.Tid > 0 {
			tid = t.App.Config.BilibiliConfig.Tid
		}
		if t.App.Config.BilibiliConfig.Dynamic != "" {
			dynamic = t.App.Config.BilibiliConfig.Dynamic
		}
		openElec = t.App.Config.BilibiliConfig.OpenElec
		selectionReserve = t.App.Config.BilibiliConfig.SelectionReserve
		upSelectionReply = t.App.Config.BilibiliConfig.UpSelectionReply
		upCloseReply = t.App.Config.BilibiliConfig.UpCloseReply
		upCloseReward = t.App.Config.BilibiliConfig.UpCloseReward
	}

	// 如果是转载且没有提供来源，使用视频URL作为来源
	if copyright == 2 && source == "" {
		if savedVideo != nil {
			source = savedVideo.URL
		} else {
			// 如果无法获取URL，构建一个默认的YouTube URL
			source = fmt.Sprintf("https://www.youtube.com/watch?v=%s", t.StateManager.VideoID)
		}
	}

	studio := &bilibili.Studio{
		Copyright:     copyright,
		Title:         t.truncateTitle(title, 80), // B站标题最长80字符
		Desc:          desc,
		Tag:           tags,
		Tid:           tid,
		Cover:         coverURL, // 使用上传的封面URL
		Dynamic:       dynamic,
		OpenSubtitle:  hasZhSubtitle, // 如果有中文字幕则开启
		Interactive:   0,
		Dolby:         0,
		LosslessMusic: 0,
		NoReprint:     noReprint,
		OpenElec:      openElec,
		Videos: []bilibili.Video{
			*video,
		},
		Source: source,
	}

	// 记录暂不支持的高级配置（需要SDK更新）
	if selectionReserve > 0 {
		t.App.Logger.Warnf("⚠️ 参与活动功能(selection_reserve=%d)暂不被SDK支持，已忽略", selectionReserve)
	}
	if upSelectionReply > 0 {
		t.App.Logger.Warnf("⚠️ 推荐评论功能(up_selection_reply=%d)暂不被SDK支持，已忽略", upSelectionReply)
	}
	if upCloseReply > 0 {
		t.App.Logger.Warnf("⚠️ 关闭评论功能(up_close_reply=%d)暂不被SDK支持，已忽略", upCloseReply)
	}
	if upCloseReward > 0 {
		t.App.Logger.Warnf("⚠️ 关闭打赏功能(up_close_reward=%d)暂不被SDK支持，已忽略", upCloseReward)
	}

	t.App.Logger.Infof("📋 投稿信息:")
	t.App.Logger.Infof("  标题: %s", studio.Title)
	t.App.Logger.Infof("  简介: %s", t.truncateString(studio.Desc, 100))
	t.App.Logger.Infof("  标签: %s", studio.Tag)
	t.App.Logger.Infof("  分区: %d", studio.Tid)
	t.App.Logger.Infof("  封面: %s", studio.Cover)
	t.App.Logger.Infof("  字幕: %v", studio.OpenSubtitle)
	t.App.Logger.Infof("  类型: %d (1=自制, 2=转载)", studio.Copyright)
	if studio.Copyright == 2 {
		t.App.Logger.Infof("  来源: %s", studio.Source)
	}

	return studio
}

// truncateString 截断字符串用于日志显示

func (t *UploadToBilibili) newUploadClient(loginInfo *bilibili.LoginInfo, extraOptions ...bilibili.Option) *bilibili.UploadClient {
	options := bilibiliutil.BuildOptions(t.App.Config, bilibiliUploadAPITimeout)
	options = append(options, extraOptions...)
	return bilibili.NewUploadClient(loginInfo, options...)
}

func (t *UploadToBilibili) uploadVideoWithRetry(loginInfo *bilibili.LoginInfo, videoPath string, context map[string]interface{}) (*bilibili.Video, error) {
	var uploadedVideo *bilibili.Video
	err := t.retryUploadOperation("\u4e0a\u4f20\u89c6\u9891", bilibiliVideoUploadMaxAttempts, func() error {
		client := t.newUploadClient(loginInfo, bilibili.WithUploadProgress(func(progress bilibili.UploadProgress) {
			percent := int(progress.Percent + 0.5)
			if percent < 0 {
				percent = 0
			}
			if percent > 100 {
				percent = 100
			}

			message := fmt.Sprintf("上传视频文件 %d%%", percent)
			if progress.TotalChunks > 0 && progress.ChunkIndex > 0 {
				message = fmt.Sprintf("上传视频文件 %d%%（分片 %d/%d）", percent, progress.ChunkIndex, progress.TotalChunks)
			}
			types.ReportTaskProgress(context, percent, message)
		}))
		var uploadErr error
		uploadedVideo, uploadErr = client.UploadVideo(videoPath)
		return uploadErr
	})
	if err != nil {
		return nil, err
	}
	return uploadedVideo, nil
}

func (t *UploadToBilibili) uploadCoverWithRetry(loginInfo *bilibili.LoginInfo, coverImagePath string) (string, error) {
	var coverURL string
	err := t.retryUploadOperation("\u4e0a\u4f20\u5c01\u9762", bilibiliCoverUploadMaxAttempts, func() error {
		client := t.newUploadClient(loginInfo)
		var uploadErr error
		coverURL, uploadErr = client.UploadCover(coverImagePath)
		return uploadErr
	})
	return coverURL, err
}

func (t *UploadToBilibili) submitVideoWithRetry(loginInfo *bilibili.LoginInfo, studio *bilibili.Studio) (*bilibili.ResponseData, error) {
	var result *bilibili.ResponseData
	err := t.retryUploadOperation("\u63d0\u4ea4\u89c6\u9891", bilibiliSubmitMaxAttempts, func() error {
		client := t.newUploadClient(loginInfo)
		var submitErr error
		result, submitErr = client.SubmitVideo(studio)
		if submitErr != nil {
			return submitErr
		}
		if result == nil {
			return fmt.Errorf("submit returned empty result")
		}
		if result.Code != 0 && t.shouldRetrySubmitResult(result) {
			return fmt.Errorf("submit failed: code=%d, message=%s", result.Code, result.Message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *UploadToBilibili) retryUploadOperation(operation string, maxAttempts int, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			if attempt > 1 {
				t.App.Logger.Infof("\u2713 %s\u5728\u7b2c %d \u6b21\u5c1d\u8bd5\u540e\u6210\u529f", operation, attempt)
			}
			return nil
		}

		if attempt == maxAttempts || !t.shouldRetryUploadError(lastErr) {
			return lastErr
		}

		delay := time.Duration(attempt*attempt*3) * time.Second
		t.App.Logger.Warnf("\u26a0\ufe0f %s\u5931\u8d25\uff0c\u7b2c %d/%d \u6b21: %v\uff1b\u5c06\u5728 %s \u540e\u91cd\u8bd5", operation, attempt, maxAttempts, lastErr, delay)
		time.Sleep(delay)
	}

	return lastErr
}

func (t *UploadToBilibili) shouldRetryUploadError(err error) bool {
	if err == nil {
		return false
	}
	if bilibili.IsNetworkError(err) || bilibili.IsRateLimitError(err) {
		return true
	}

	errorStr := strings.ToLower(err.Error())
	retryableHints := []string{"timeout", "deadline exceeded", "temporarily unavailable", "service unavailable", "internal server error", "server error", "connection reset", "broken pipe", "eof", "too many requests", "try again", "dns", "no such host", "502", "503", "504"}
	for _, hint := range retryableHints {
		if strings.Contains(errorStr, hint) {
			return true
		}
	}

	return false
}

func (t *UploadToBilibili) shouldRetrySubmitResult(result *bilibili.ResponseData) bool {
	if result == nil {
		return false
	}

	message := strings.ToLower(result.Message)
	if result.Code == -799 {
		return true
	}

	retryableHints := []string{"timeout", "busy", "service unavailable", "server error", "too many requests"}
	for _, hint := range retryableHints {
		if strings.Contains(message, hint) {
			return true
		}
	}

	return false
}

func (t *UploadToBilibili) truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// truncateTitle 截断标题到指定长度
func (t *UploadToBilibili) truncateTitle(title string, maxLen int) string {
	runes := []rune(title)
	if len(runes) <= maxLen {
		return title
	}
	return string(runes[:maxLen-3]) + "..."
}

// getUserFriendlyError 将技术错误转换为用户友好的错误信息
func (t *UploadToBilibili) getUserFriendlyError(err error, operation string) string {
	errorStr := err.Error()

	// 网络相关错误
	if strings.Contains(errorStr, "broken pipe") || strings.Contains(errorStr, "connection reset") {
		return fmt.Sprintf("%s失败：网络连接中断，请检查网络状态后重试", operation)
	}

	if strings.Contains(errorStr, "timeout") || strings.Contains(errorStr, "deadline exceeded") {
		return fmt.Sprintf("%s失败：网络超时，请稍后重试", operation)
	}

	if strings.Contains(errorStr, "connection refused") {
		return fmt.Sprintf("%s失败：无法连接到B站服务器，请检查网络连接", operation)
	}

	if strings.Contains(errorStr, "no such host") || strings.Contains(errorStr, "dns") {
		return fmt.Sprintf("%s失败：网络域名解析失败，请检查网络设置", operation)
	}

	// 文件相关错误
	if strings.Contains(errorStr, "no such file") || strings.Contains(errorStr, "file not found") {
		return fmt.Sprintf("%s失败：找不到视频文件，请确认文件已正确下载", operation)
	}

	if strings.Contains(errorStr, "permission denied") {
		return fmt.Sprintf("%s失败：文件访问权限不足", operation)
	}

	if strings.Contains(errorStr, "file too large") {
		return fmt.Sprintf("%s失败：文件过大，超出B站上传限制", operation)
	}

	// B站API相关错误
	if strings.Contains(errorStr, "401") || strings.Contains(errorStr, "unauthorized") {
		return fmt.Sprintf("%s失败：登录状态已过期，请重新登录", operation)
	}

	if strings.Contains(errorStr, "403") || strings.Contains(errorStr, "forbidden") {
		return fmt.Sprintf("%s失败：账号权限不足或被限制", operation)
	}

	if strings.Contains(errorStr, "429") || strings.Contains(errorStr, "rate limit") {
		return fmt.Sprintf("%s失败：操作频率过快，请稍后再试", operation)
	}

	if strings.Contains(errorStr, "500") || strings.Contains(errorStr, "internal server error") {
		return fmt.Sprintf("%s失败：B站服务器临时异常，请稍后重试", operation)
	}

	if strings.Contains(errorStr, "upload chunks") {
		return fmt.Sprintf("%s失败：视频分片上传中断，可能是网络不稳定导致，请重试", operation)
	}

	// 通用错误处理
	if strings.Contains(errorStr, "failed to") {
		return fmt.Sprintf("%s失败：操作执行失败，请稍后重试", operation)
	}

	// 如果是未知错误，返回简化的错误信息
	return fmt.Sprintf("%s失败：发生未知错误，请重试或联系技术支持", operation)
}
