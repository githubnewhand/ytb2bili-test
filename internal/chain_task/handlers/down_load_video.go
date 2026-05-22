package handlers

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"gorm.io/gorm"
)

type DownloadVideo struct {
	base.BaseTask
	App               *core.AppServer
	DB                *gorm.DB
	SavedVideoService *services.SavedVideoService
}

type recentLineBuffer struct {
	mu    sync.Mutex
	limit int
	lines []string
}

const (
	downloadOverallTimeout = 4 * time.Hour
	downloadIdleTimeout    = 10 * time.Minute
	metadataTimeout        = 2 * time.Minute
)

var ytDlpProgressPattern = regexp.MustCompile(`\[download\]\s+([0-9]+(?:\.[0-9]+)?)%`)
var aria2ProgressPattern = regexp.MustCompile(`\(([0-9]+)%\)`)

func newRecentLineBuffer(limit int) *recentLineBuffer {
	return &recentLineBuffer{limit: limit}
}

func (b *recentLineBuffer) Add(line string) {
	if b == nil {
		return
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len([]rune(line)) > 300 {
		runes := []rune(line)
		line = string(runes[:300]) + "..."
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = append(b.lines, line)
	if b.limit > 0 && len(b.lines) > b.limit {
		b.lines = b.lines[len(b.lines)-b.limit:]
	}
}

func (b *recentLineBuffer) String() string {
	if b == nil {
		return ""
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	return strings.Join(b.lines, "\n")
}

func NewDownloadVideo(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *DownloadVideo {
	return &DownloadVideo{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

// findYtDlp 查找系统中的 yt-dlp 可执行文件
func (t *DownloadVideo) findYtDlp() (string, error) {
	// 从配置中获取安装目录
	var installDir string
	if t.App.Config != nil && t.App.Config.YtDlpPath != "" {
		installDir = t.App.Config.YtDlpPath
	}

	// 创建 yt-dlp 管理器
	manager := utils.NewYtDlpManager(t.App.Logger, installDir)

	// 检查是否已安装
	if manager.IsInstalled() {
		path := manager.GetBinaryPath()
		t.App.Logger.Debugf("找到 yt-dlp: %s", path)
		return path, nil
	}

	return "", fmt.Errorf("未找到 yt-dlp，请确保已正确安装")
}

func (t *DownloadVideo) findAria2() (string, bool) {
	if t.App == nil || t.App.Config == nil || !t.App.Config.Download.UseAria2 {
		return "", false
	}

	if t.App.Config.Download.Aria2Path != "" {
		if _, err := os.Stat(t.App.Config.Download.Aria2Path); err == nil {
			return t.App.Config.Download.Aria2Path, true
		}
		t.App.Logger.Warnf("⚠️ 配置的 aria2c 不存在: %s，将使用 yt-dlp 内置下载器", t.App.Config.Download.Aria2Path)
		return "", false
	}

	path, err := exec.LookPath("aria2c")
	if err != nil {
		t.App.Logger.Warn("⚠️ 未找到 aria2c，将使用 yt-dlp 内置下载器")
		return "", false
	}
	return path, true
}

// findLatestCookiesFile 查找最新的 cookies 文件
func (t *DownloadVideo) findLatestCookiesFile() string {
	// 1. 优先查找 data/cookies/ 目录下最新的用户提交的 cookies
	cookiesDir := filepath.Join(t.App.Config.DataPath, "cookies")

	// 确保路径是绝对路径
	if !filepath.IsAbs(cookiesDir) {
		absPath, err := filepath.Abs(cookiesDir)
		if err == nil {
			cookiesDir = absPath
		}
	}

	if entries, err := os.ReadDir(cookiesDir); err == nil {
		var latestFile string
		var latestTime int64

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !strings.HasPrefix(name, "cookies_") || !strings.HasSuffix(name, ".txt") {
				continue
			}

			filePath := filepath.Join(cookiesDir, name)
			if info, err := entry.Info(); err == nil {
				if info.ModTime().Unix() > latestTime {
					latestTime = info.ModTime().Unix()
					latestFile = filePath
				}
			}
		}

		if latestFile != "" {
			t.App.Logger.Infof("🍪 找到用户提交的最新 cookies 文件: %s", latestFile)
			return latestFile
		}
	} else {
		t.App.Logger.Warnf("⚠️ 无法读取 cookies 目录 %s: %v", cookiesDir, err)
	}

	// 2. 兼容旧逻辑：查找配置文件目录下的 cookies.txt
	configDir := filepath.Dir(t.App.Config.Path)
	cookiesPath := filepath.Join(configDir, "cookies.txt")

	// 确保是绝对路径
	if !filepath.IsAbs(cookiesPath) {
		absPath, err := filepath.Abs(cookiesPath)
		if err == nil {
			cookiesPath = absPath
		}
	}

	if _, err := os.Stat(cookiesPath); err == nil {
		t.App.Logger.Infof("🍪 找到配置目录下的 cookies 文件: %s", cookiesPath)
		return cookiesPath
	}

	// 3. 查找当前目录的 cookies.txt
	currentCookies := "cookies.txt"
	if _, err := os.Stat(currentCookies); err == nil {
		absPath, err := filepath.Abs(currentCookies)
		if err == nil {
			t.App.Logger.Infof("🍪 找到当前目录的 cookies 文件: %s", absPath)
			return absPath
		}
	}

	t.App.Logger.Warn("⚠️ 未找到任何可用的 cookies 文件")
	return ""
}

// getVideoURL 根据 VideoID 构建完整的视频 URL
func (t *DownloadVideo) getVideoURL() string {
	videoID := t.StateManager.VideoID

	// 如果已经是完整 URL，直接返回
	if strings.HasPrefix(videoID, "http://") || strings.HasPrefix(videoID, "https://") {
		return videoID
	}

	// YouTube 短 ID 格式
	if len(videoID) == 11 && !strings.Contains(videoID, "/") {
		return fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	}

	// Bilibili BV 号
	if strings.HasPrefix(videoID, "BV") {
		return fmt.Sprintf("https://www.bilibili.com/video/%s", videoID)
	}

	// 默认作为 YouTube ID 处理
	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
}

func (t *DownloadVideo) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Info("DownloadVideo Handler Version: with-cookies-support-v3") // 版本标记
	t.App.Logger.Infof("开始下载视频: %s", t.StateManager.VideoID)
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 1, "准备下载")

	// 1. 查找 yt-dlp 可执行文件
	ytdlpPath, err := t.findYtDlp()
	if err != nil {
		t.App.Logger.Errorf("❌ %v", err)
		context["error"] = err.Error()
		return false
	}
	types.ReportTaskProgress(context, 3, "检查下载工具")

	// 2. 确保下载目录存在
	if err := os.MkdirAll(t.StateManager.CurrentDir, 0755); err != nil {
		t.App.Logger.Errorf("❌ 创建下载目录失败: %v", err)
		context["error"] = err.Error()
		return false
	}
	types.ReportTaskProgress(context, 5, "准备下载目录")

	// 3. 尝试下载（先用代理，失败后不用代理重试）
	videoURL := t.getVideoURL()
	useProxy := t.App.Config != nil && t.App.Config.ProxyConfig != nil &&
		t.App.Config.ProxyConfig.UseProxy && t.App.Config.ProxyConfig.ProxyHost != ""

	// 第一次尝试：使用代理（如果配置了）
	if useProxy {
		t.App.Logger.Info("🔄 尝试使用代理下载...")
		if t.executeDownload(ytdlpPath, videoURL, true, context) {
			return true
		}
		t.App.Logger.Warn("⚠️ 代理下载失败，尝试不使用代理重试...")
	}

	// 第二次尝试：不使用代理
	t.App.Logger.Info("🔄 尝试不使用代理下载...")
	return t.executeDownload(ytdlpPath, videoURL, false, context)
}

// executeDownload 执行实际的下载操作
func (t *DownloadVideo) executeDownload(ytdlpPath, videoURL string, useProxy bool, context map[string]interface{}) bool {
	// 构建下载命令
	command := []string{
		ytdlpPath,
		"-P", t.StateManager.CurrentDir,
		"-o", "%(id)s.%(ext)s",
		"--merge-output-format", "mp4",
		"--newline",
		"--socket-timeout", "30",
		"--retries", "3",
		"--fragment-retries", "3",
	}

	if _, err := exec.LookPath("node"); err == nil {
		command = append(command, "--js-runtimes", "node")
	}

	if aria2Path, ok := t.findAria2(); ok {
		aria2Args := t.App.Config.Download.Aria2Args
		if aria2Args == "" {
			aria2Args = "-x 16 -s 16 -k 1M --file-allocation=none"
		}
		command = append(command,
			"--external-downloader", aria2Path,
			"--external-downloader-args", "aria2c:"+aria2Args,
		)
		t.App.Logger.Infof("🚀 使用 aria2c 加速下载: %s (%s)", aria2Path, aria2Args)
	}

	// 查找最新的 cookies 文件（优先使用用户提交的）
	cookiesPath := t.findLatestCookiesFile()

	if cookiesPath != "" {
		command = append(command, "--cookies", cookiesPath)
		t.App.Logger.Infof("🍪 使用 Cookies 文件: %s", cookiesPath)
	}

	// 添加代理配置（如果需要）
	if useProxy && t.App.Config != nil && t.App.Config.ProxyConfig != nil &&
		t.App.Config.ProxyConfig.UseProxy && t.App.Config.ProxyConfig.ProxyHost != "" {
		command = append(command, "--proxy", t.App.Config.ProxyConfig.ProxyHost)
		t.App.Logger.Infof("📡 使用代理: %s", t.App.Config.ProxyConfig.ProxyHost)
	} else if !useProxy {
		t.App.Logger.Info("🌐 不使用代理")
	}

	// 添加视频标识符和URL
	// command = append(command, "--", t.StateManager.VideoID)
	command = append(command, videoURL)

	t.App.Logger.Infof("执行命令: %s", strings.Join(command, " "))
	t.App.Logger.Infof("下载目录: %s", t.StateManager.CurrentDir)
	t.App.Logger.Infof("视频URL: %s", videoURL)

	// 创建命令并设置输出管道。yt-dlp 偶尔会卡在网络读取上，所以这里加上
	// 总超时和空闲超时，避免一个下载永久占住整个任务链。
	cmdCtx, cancel := stdctx.WithTimeout(stdctx.Background(), downloadOverallTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, command[0], command[1:]...)
	cmd.Dir = t.StateManager.CurrentDir

	// 捕获标准输出和标准错误
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.App.Logger.Errorf("❌ 创建标准输出管道失败: %v", err)
		context["error"] = err.Error()
		return false
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.App.Logger.Errorf("❌ 创建标准错误管道失败: %v", err)
		context["error"] = err.Error()
		return false
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		t.App.Logger.Errorf("❌ 启动下载命令失败: %v", err)
		context["error"] = err.Error()
		return false
	}
	types.ReportTaskProgress(context, 5, "开始下载")

	activityCh := make(chan struct{}, 1)

	recentOutput := newRecentLineBuffer(12)

	// 实时读取输出
	go t.logOutput(stdout, "INFO", activityCh, context, recentOutput)
	go t.logOutput(stderr, "ERROR", activityCh, context, recentOutput)

	// 等待命令完成
	if err := t.waitForDownload(cmd, cmdCtx, activityCh); err != nil {
		errMsg := t.formatDownloadError(err, recentOutput.String())
		t.App.Logger.Errorf("❌ 视频下载失败: %s", errMsg)
		context["error"] = errMsg
		return false
	}

	// 10. 验证下载的文件
	types.ReportTaskProgress(context, 90, "校验下载文件")
	downloadedFile := t.findDownloadedFile()
	if downloadedFile == "" {
		errMsg := "下载完成但未找到视频文件"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	// 11. 保存文件信息到 context
	context["downloaded_file"] = downloadedFile
	t.App.Logger.Infof("✓ 视频下载成功: %s", downloadedFile)

	// 12. 获取视频元数据（标题、描述等）
	t.App.Logger.Info("📋 获取视频元数据...")
	types.ReportTaskProgress(context, 95, "获取视频元数据")
	metadata, err := t.getVideoMetadata(ytdlpPath)
	if err != nil {
		t.App.Logger.Warnf("⚠️ 获取视频元数据失败: %v，将使用默认值", err)
	} else {
		context["original_title"] = metadata.Title
		context["original_description"] = metadata.Description
		t.App.Logger.Infof("✓ 原始标题: %s", metadata.Title)
		if metadata.Description != "" {
			t.App.Logger.Infof("✓ 原始描述: %s", t.truncateString(metadata.Description, 100))
		}

		// 保存到数据库
		if t.SavedVideoService != nil {
			savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
			if err == nil {
				savedVideo.Title = metadata.Title
				savedVideo.Description = metadata.Description
				if err := t.SavedVideoService.UpdateVideo(savedVideo); err != nil {
					t.App.Logger.Errorf("❌ 保存原始元数据到数据库失败: %v", err)
				} else {
					t.App.Logger.Info("✅ 原始元数据已保存到数据库")
				}
			}
		}
	}

	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 98, "下载步骤收尾")

	return true
}

func (t *DownloadVideo) waitForDownload(cmd *exec.Cmd, cmdCtx stdctx.Context, activityCh <-chan struct{}) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	idleTimer := time.NewTimer(downloadIdleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case err := <-done:
			if cmdCtx.Err() == stdctx.DeadlineExceeded {
				return fmt.Errorf("下载超过最长允许时间 %s，已终止", downloadOverallTimeout)
			}
			return err
		case <-activityCh:
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(downloadIdleTimeout)
		case <-idleTimer.C:
			if cmd.Process != nil {
				if err := cmd.Process.Kill(); err != nil {
					t.App.Logger.Warnf("终止空闲下载进程失败: %v", err)
				}
			}
			if err := <-done; err != nil {
				t.App.Logger.Debugf("空闲下载进程退出结果: %v", err)
			}
			return fmt.Errorf("下载超过 %s 没有输出，已终止", downloadIdleTimeout)
		}
	}
}

func (t *DownloadVideo) formatDownloadError(err error, recentOutput string) string {
	baseMsg := fmt.Sprintf("下载失败: %v", err)
	recentOutput = strings.TrimSpace(recentOutput)
	if recentOutput == "" {
		return baseMsg
	}

	lowerOutput := strings.ToLower(recentOutput)
	switch {
	case strings.Contains(lowerOutput, "sign in to confirm") && strings.Contains(lowerOutput, "not a bot"):
		return "下载失败: YouTube 要求登录验证，通常是当前 VPN 出口 IP 被风控。请更换非数据中心节点，或导入当前浏览器的 YouTube cookies 后重试。\n" + recentOutput
	case strings.Contains(lowerOutput, "proxy") || strings.Contains(lowerOutput, "connection refused"):
		return "下载失败: 代理连接异常，请检查代理地址和端口。\n" + recentOutput
	default:
		return baseMsg + "\n" + recentOutput
	}
}

// logOutput 实时输出日志
func (t *DownloadVideo) logOutput(reader io.Reader, level string, activityCh chan<- struct{}, context map[string]interface{}, recentOutput *recentLineBuffer) {
	buffer := make([]byte, 4096)
	var pending strings.Builder

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			t.markDownloadActivity(activityCh)
			for _, r := range string(buffer[:n]) {
				if r == '\r' || r == '\n' {
					t.handleDownloadOutputLine(pending.String(), level, context, recentOutput)
					pending.Reset()
					continue
				}

				pending.WriteRune(r)
				if pending.Len() >= 4096 {
					t.handleDownloadOutputLine(pending.String(), level, context, recentOutput)
					pending.Reset()
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				t.App.Logger.Debugf("读取下载输出结束: %v", err)
			}
			break
		}
	}

	t.handleDownloadOutputLine(pending.String(), level, context, recentOutput)
}

func (t *DownloadVideo) handleDownloadOutputLine(line, level string, context map[string]interface{}, recentOutput *recentLineBuffer) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	if level == "ERROR" || strings.Contains(line, "ERROR:") || strings.Contains(line, "WARNING:") {
		recentOutput.Add(line)
	}

	if strings.Contains(line, "[download]") {
		if strings.Contains(line, "Destination:") {
			t.App.Logger.Infof("📥 %s", line)
			types.ReportTaskProgress(context, 6, "定位下载文件")
		} else if strings.Contains(line, "%") {
			t.App.Logger.Debugf("⏳ %s", line)
			if percent, ok := parseYtDlpProgressPercent(line); ok {
				mappedPercent := mapDownloadProgressPercent(percent)
				types.ReportTaskProgress(context, mappedPercent, fmt.Sprintf("下载中 %d%%", percent))
			}
		} else {
			t.App.Logger.Infof("📥 %s", line)
		}
		return
	}

	if strings.Contains(line, "[ffmpeg]") {
		t.App.Logger.Infof("🔄 %s", line)
		types.ReportTaskProgress(context, 92, "合并视频音频")
		return
	}

	if percent, ok := parseAria2ProgressPercent(line); ok {
		t.App.Logger.Debugf("🚀 aria2 %s", line)
		mappedPercent := mapDownloadProgressPercent(percent)
		types.ReportTaskProgress(context, mappedPercent, fmt.Sprintf("aria2下载中 %d%%", percent))
		return
	}

	if level == "ERROR" {
		t.App.Logger.Warnf("⚠️  %s", line)
	} else {
		t.App.Logger.Debugf("%s", line)
	}
}

func parseYtDlpProgressPercent(line string) (int, bool) {
	matches := ytDlpProgressPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}

	percent, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, false
	}

	rounded := int(percent + 0.5)
	if rounded < 0 {
		return 0, true
	}
	if rounded > 100 {
		return 100, true
	}
	return rounded, true
}

func parseAria2ProgressPercent(line string) (int, bool) {
	if !strings.Contains(line, "DL:") && !strings.Contains(line, "ETA:") {
		return 0, false
	}

	matches := aria2ProgressPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}

	percent, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	if percent < 0 {
		return 0, true
	}
	if percent > 100 {
		return 100, true
	}
	return percent, true
}

func mapDownloadProgressPercent(percent int) int {
	mappedPercent := 5 + (percent * 85 / 100)
	if mappedPercent > 90 {
		return 90
	}
	if mappedPercent < 5 {
		return 5
	}
	return mappedPercent
}

func (t *DownloadVideo) markDownloadActivity(activityCh chan<- struct{}) {
	if activityCh == nil {
		return
	}

	select {
	case activityCh <- struct{}{}:
	default:
	}
}

// findDownloadedFile 查找下载的视频文件
func (t *DownloadVideo) findDownloadedFile() string {
	// 查找目录下的 mp4 文件
	files, err := filepath.Glob(filepath.Join(t.StateManager.CurrentDir, "*.mp4"))
	if err != nil || len(files) == 0 {
		// 尝试查找其他视频格式
		for _, ext := range []string{"*.webm", "*.mkv", "*.flv"} {
			files, err = filepath.Glob(filepath.Join(t.StateManager.CurrentDir, ext))
			if err == nil && len(files) > 0 {
				break
			}
		}
	}

	if len(files) > 0 {
		// 返回最新的文件
		latestFile := files[0]
		latestTime := int64(0)

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			if info.ModTime().Unix() > latestTime {
				latestTime = info.ModTime().Unix()
				latestFile = file
			}
		}

		return latestFile
	}

	return ""
}

// VideoMetadataInfo 视频元数据信息
type VideoMetadataInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Uploader    string `json:"uploader"`
	Duration    int    `json:"duration"`
}

// getVideoMetadata 使用 yt-dlp 获取视频元数据（带代理回退）
func (t *DownloadVideo) getVideoMetadata(ytdlpPath string) (*VideoMetadataInfo, error) {
	videoURL := t.getVideoURL()

	// 构建基础命令参数
	baseArgs := []string{"--dump-json", "--no-download"}
	if _, err := exec.LookPath("node"); err == nil {
		baseArgs = append(baseArgs, "--js-runtimes", "node")
	}

	// 添加 cookies 支持（使用最新的用户提交的 cookies）
	cookiesPath := t.findLatestCookiesFile()

	if cookiesPath != "" {
		baseArgs = append(baseArgs, "--cookies", cookiesPath)
		t.App.Logger.Debugf("🍪 使用 Cookies 文件获取元数据: %s", cookiesPath)
	} else {
		t.App.Logger.Debug("No cookies file found; fetching metadata without cookies")
	}

	// 尝试使用代理
	useProxy := t.App.Config != nil && t.App.Config.ProxyConfig != nil &&
		t.App.Config.ProxyConfig.UseProxy && t.App.Config.ProxyConfig.ProxyHost != ""

	args := append([]string{}, baseArgs...)
	if useProxy {
		args = append(args, "--proxy", t.App.Config.ProxyConfig.ProxyHost)
		t.App.Logger.Debugf("📡 使用代理获取元数据: %s", t.App.Config.ProxyConfig.ProxyHost)
	}

	args = append(args, videoURL)

	// 第一次尝试（可能带代理）
	output, err := t.runYtDlpOutput(ytdlpPath, args, metadataTimeout)

	// 如果使用代理失败，尝试不使用代理
	if err != nil && useProxy {
		t.App.Logger.Warnf("⚠️ 使用代理获取元数据失败，尝试不使用代理...")
		argsNoProxy := append([]string{}, baseArgs...)
		argsNoProxy = append(argsNoProxy, videoURL)
		output, err = t.runYtDlpOutput(ytdlpPath, argsNoProxy, metadataTimeout)
		if err != nil {
			return nil, fmt.Errorf("获取元数据失败: %v", err)
		}
		t.App.Logger.Info("✓ 不使用代理成功获取元数据")
	} else if err != nil {
		return nil, fmt.Errorf("获取元数据失败: %v", err)
	}

	var metadata VideoMetadataInfo
	if err := json.Unmarshal(output, &metadata); err != nil {
		return nil, fmt.Errorf("解析元数据失败: %v", err)
	}

	return &metadata, nil
}

func (t *DownloadVideo) runYtDlpOutput(ytdlpPath string, args []string, timeout time.Duration) ([]byte, error) {
	cmdCtx, cancel := stdctx.WithTimeout(stdctx.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, ytdlpPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if cmdCtx.Err() == stdctx.DeadlineExceeded {
		return nil, fmt.Errorf("yt-dlp 命令超过 %s 未返回", timeout)
	}
	if err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return nil, fmt.Errorf("%w: %s", err, errText)
		}
	}
	return output, err
}

// truncateString 截断字符串用于日志显示
func (t *DownloadVideo) truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
