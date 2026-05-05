package subtitle

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// YtdlpSubtitleDownloader yt-dlp字幕下载器
type YtdlpSubtitleDownloader struct {
	logger *zap.SugaredLogger
}

// SubtitleInfo 字幕信息
type SubtitleInfo struct {
	Language     string `json:"language"`      // 语言代码 (如: en, zh-Hans)
	LanguageName string `json:"language_name"` // 语言名称 (如: English, Chinese)
	Ext          string `json:"ext"`           // 格式 (如: vtt, srt, json3)
	URL          string `json:"url"`           // 字幕URL
	IsAutomatic  bool   `json:"is_automatic"`  // 是否为自动生成
}

// VideoSubtitles 视频字幕列表
type VideoSubtitles struct {
	VideoID       string                   `json:"video_id"`
	Title         string                   `json:"title"`
	Duration      float64                  `json:"duration"`
	Subtitles     map[string][]SubtitleInfo `json:"subtitles"`      // 手动字幕
	AutoSubtitles map[string][]SubtitleInfo `json:"auto_subtitles"` // 自动生成字幕
}

// NewYtdlpSubtitleDownloader 创建yt-dlp字幕下载器
func NewYtdlpSubtitleDownloader(logger *zap.SugaredLogger) *YtdlpSubtitleDownloader {
	return &YtdlpSubtitleDownloader{
		logger: logger,
	}
}

// ListSubtitles 列出视频所有可用字幕
func (d *YtdlpSubtitleDownloader) ListSubtitles(videoURL string) (*VideoSubtitles, error) {
	d.logger.Infof("获取视频字幕列表: %s", videoURL)

	// 使用yt-dlp获取视频信息（包含字幕列表）
	cmd := exec.Command("yt-dlp",
		"--dump-json",
		"--skip-download",
		videoURL,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("执行yt-dlp失败: %w, 输出: %s", err, string(output))
	}

	// 解析JSON
	var videoInfo struct {
		ID            string                              `json:"id"`
		Title         string                              `json:"title"`
		Duration      float64                             `json:"duration"`
		Subtitles     map[string][]map[string]interface{} `json:"subtitles"`
		AutoSubtitles map[string][]map[string]interface{} `json:"automatic_captions"`
	}

	if err := json.Unmarshal(output, &videoInfo); err != nil {
		return nil, fmt.Errorf("解析视频信息失败: %w", err)
	}

	result := &VideoSubtitles{
		VideoID:       videoInfo.ID,
		Title:         videoInfo.Title,
		Duration:      videoInfo.Duration,
		Subtitles:     make(map[string][]SubtitleInfo),
		AutoSubtitles: make(map[string][]SubtitleInfo),
	}

	// 转换手动字幕
	for lang, subs := range videoInfo.Subtitles {
		var subtitleList []SubtitleInfo
		for _, sub := range subs {
			subtitleList = append(subtitleList, SubtitleInfo{
				Language:     lang,
				LanguageName: getString(sub, "name"),
				Ext:          getString(sub, "ext"),
				URL:          getString(sub, "url"),
				IsAutomatic:  false,
			})
		}
		result.Subtitles[lang] = subtitleList
	}

	// 转换自动生成字幕
	for lang, subs := range videoInfo.AutoSubtitles {
		var subtitleList []SubtitleInfo
		for _, sub := range subs {
			subtitleList = append(subtitleList, SubtitleInfo{
				Language:     lang,
				LanguageName: getString(sub, "name"),
				Ext:          getString(sub, "ext"),
				URL:          getString(sub, "url"),
				IsAutomatic:  true,
			})
		}
		result.AutoSubtitles[lang] = subtitleList
	}

	d.logger.Infof("找到 %d 种手动字幕，%d 种自动字幕",
		len(result.Subtitles), len(result.AutoSubtitles))

	return result, nil
}

// DownloadSubtitle 下载指定语言的字幕
// language: 语言代码，如 "en", "zh-Hans", "zh-CN" 等
// format: 字幕格式，如 "srt", "vtt", "json3"
// outputPath: 输出路径（不含扩展名）
func (d *YtdlpSubtitleDownloader) DownloadSubtitle(videoURL, language, format, outputPath string) (string, error) {
	d.logger.Infof("下载字幕: 语言=%s, 格式=%s", language, format)

	// 确保输出目录存在
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 构建yt-dlp命令
	args := []string{
		"--skip-download",           // 跳过视频下载
		"--write-subs",              // 写入字幕
		"--sub-langs", language,     // 指定语言
		"--sub-format", format,      // 指定格式
		"--convert-subs", format,    // 转换为指定格式
		"-o", outputPath + ".%(ext)s", // 输出路径模板
		videoURL,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("下载字幕失败: %w, 输出: %s", err, string(output))
	}

	// 查找生成的字幕文件
	subtitleFile := fmt.Sprintf("%s.%s.%s", outputPath, language, format)
	if _, err := os.Stat(subtitleFile); os.IsNotExist(err) {
		// 尝试其他可能的文件名格式
		possibleFiles := []string{
			fmt.Sprintf("%s.%s", outputPath, format),
			fmt.Sprintf("%s.%s.%s", outputPath, strings.Split(language, "-")[0], format),
		}

		for _, file := range possibleFiles {
			if _, err := os.Stat(file); err == nil {
				subtitleFile = file
				break
			}
		}

		if _, err := os.Stat(subtitleFile); os.IsNotExist(err) {
			return "", fmt.Errorf("字幕文件未生成: %s", subtitleFile)
		}
	}

	d.logger.Infof("字幕已下载: %s", subtitleFile)
	return subtitleFile, nil
}

// DownloadAllSubtitles 下载所有可用字幕
func (d *YtdlpSubtitleDownloader) DownloadAllSubtitles(videoURL, format, outputPath string) ([]string, error) {
	d.logger.Infof("下载所有字幕: 格式=%s", format)

	// 确保输出目录存在
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 构建yt-dlp命令
	args := []string{
		"--skip-download",           // 跳过视频下载
		"--write-subs",              // 写入字幕
		"--write-auto-subs",         // 包含自动生成字幕
		"--all-subs",                // 下载所有字幕
		"--sub-format", format,      // 指定格式
		"--convert-subs", format,    // 转换为指定格式
		"-o", outputPath + ".%(ext)s", // 输出路径模板
		videoURL,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("下载字幕失败: %w, 输出: %s", err, string(output))
	}

	// 查找生成的所有字幕文件
	pattern := filepath.Join(outputDir, fmt.Sprintf("%s*.%s", filepath.Base(outputPath), format))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("查找字幕文件失败: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("未找到任何字幕文件")
	}

	d.logger.Infof("已下载 %d 个字幕文件", len(files))
	return files, nil
}

// DownloadEnglishSubtitle 下载英文字幕（优先手动字幕，其次自动字幕）
func (d *YtdlpSubtitleDownloader) DownloadEnglishSubtitle(videoURL, format, outputPath string) (string, error) {
	d.logger.Info("下载英文字幕...")

	// 尝试下载顺序: en -> en-US -> en-GB
	languages := []string{"en", "en-US", "en-GB"}

	for _, lang := range languages {
		file, err := d.DownloadSubtitle(videoURL, lang, format, outputPath)
		if err == nil {
			return file, nil
		}
		d.logger.Warnf("未找到 %s 字幕，尝试下一个", lang)
	}

	return "", fmt.Errorf("未找到英文字幕")
}

// DownloadChineseSubtitle 下载中文字幕
func (d *YtdlpSubtitleDownloader) DownloadChineseSubtitle(videoURL, format, outputPath string) (string, error) {
	d.logger.Info("下载中文字幕...")

	// 尝试下载顺序: zh-Hans -> zh-CN -> zh-TW -> zh
	languages := []string{"zh-Hans", "zh-CN", "zh-TW", "zh"}

	for _, lang := range languages {
		file, err := d.DownloadSubtitle(videoURL, lang, format, outputPath)
		if err == nil {
			return file, nil
		}
		d.logger.Warnf("未找到 %s 字幕，尝试下一个", lang)
	}

	return "", fmt.Errorf("未找到中文字幕")
}

// CheckYtdlpInstalled 检查yt-dlp是否已安装
func CheckYtdlpInstalled() error {
	cmd := exec.Command("yt-dlp", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp 未安装或不在PATH中: %w", err)
	}
	version := strings.TrimSpace(string(output))
	fmt.Printf("yt-dlp 版本: %s\n", version)
	return nil
}

// getString 安全获取map中的字符串值
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
