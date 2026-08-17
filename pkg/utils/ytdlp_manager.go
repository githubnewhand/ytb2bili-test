package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

// YtDlpManager yt-dlp 管理器
type YtDlpManager struct {
	logger     *zap.SugaredLogger
	installDir string
	binaryPath string
}

// GitHubRelease GitHub发布信息
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// NewYtDlpManager 创建 yt-dlp 管理器
func NewYtDlpManager(logger *zap.SugaredLogger, installDir string) *YtDlpManager {
	if installDir == "" {
		// 默认安装目录
		homeDir, _ := os.UserHomeDir()
		installDir = filepath.Join(homeDir, "opt", "yt-dlp")
	}

	binaryPath := filepath.Join(installDir, "yt-dlp")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	return &YtDlpManager{
		logger:     logger,
		installDir: installDir,
		binaryPath: binaryPath,
	}
}

// CheckAndInstall 检查并自动安装 yt-dlp
func (m *YtDlpManager) CheckAndInstall() error {
	m.logger.Info("🔍 检查 yt-dlp 安装状态...")

	// 1. 检查是否已安装
	if m.IsInstalled() {
		m.logger.Info("✅ yt-dlp 已安装")
		return m.checkVersion()
	}

	m.logger.Info("⚠️  yt-dlp 未找到，开始自动安装...")

	// 2. 自动安装
	if err := m.Install(); err != nil {
		return fmt.Errorf("自动安装 yt-dlp 失败: %v", err)
	}

	m.logger.Info("✅ yt-dlp 安装完成")
	return nil
}

// IsInstalled 检查 yt-dlp 是否已安装
func (m *YtDlpManager) IsInstalled() bool {
	// 检查常见安装位置
	possiblePaths := []string{
		m.binaryPath,                            // 自定义安装路径
		"/usr/local/bin/yt-dlp",                 // Homebrew macOS
		"/opt/homebrew/bin/yt-dlp",              // Homebrew Apple Silicon
		"/usr/bin/yt-dlp",                       // 系统安装
		"C:\\Program Files\\yt-dlp\\yt-dlp.exe", // Windows
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			m.binaryPath = path
			m.logger.Debugf("找到 yt-dlp: %s", path)
			return true
		}
	}

	// 尝试从 PATH 查找
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		m.binaryPath = path
		m.logger.Debugf("通过 PATH 找到 yt-dlp: %s", path)
		return true
	}

	return false
}

// GetBinaryPath 获取 yt-dlp 二进制文件路径
func (m *YtDlpManager) GetBinaryPath() string {
	return m.binaryPath
}

// Version 获取当前 yt-dlp 版本号
func (m *YtDlpManager) Version() (string, error) {
	cmd := exec.Command(m.binaryPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Install 下载并安装 yt-dlp
func (m *YtDlpManager) Install() error {
	m.logger.Info("📥 开始下载 yt-dlp...")

	// 1. 获取最新版本信息
	release, err := m.getLatestRelease()
	if err != nil {
		return fmt.Errorf("获取最新版本信息失败: %v", err)
	}

	m.logger.Infof("🔄 最新版本: %s", release.TagName)

	// 2. 选择合适的下载链接
	downloadURL, err := m.getDownloadURL(release)
	if err != nil {
		return fmt.Errorf("获取下载链接失败: %v", err)
	}

	// 3. 创建安装目录
	if err := os.MkdirAll(m.installDir, 0755); err != nil {
		return fmt.Errorf("创建安装目录失败: %v", err)
	}

	// 4. 下载文件
	if err := m.downloadFile(downloadURL); err != nil {
		return fmt.Errorf("下载文件失败: %v", err)
	}

	// 5. 设置执行权限 (非 Windows)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(m.binaryPath, 0755); err != nil {
			return fmt.Errorf("设置执行权限失败: %v", err)
		}
	}

	// 6. 验证安装
	if !m.IsInstalled() {
		return fmt.Errorf("安装验证失败")
	}

	return nil
}

// getLatestRelease 获取最新版本信息
func (m *YtDlpManager) getLatestRelease() (*GitHubRelease, error) {
	url := "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 请求失败: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// getDownloadURL 根据系统选择合适的下载链接
func (m *YtDlpManager) getDownloadURL(release *GitHubRelease) (string, error) {
	var targetName string

	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			targetName = "yt-dlp.exe"
		case "arm64":
			targetName = "yt-dlp_win_arm64.exe"
		case "386":
			targetName = "yt-dlp_win32.exe"
		default:
			targetName = "yt-dlp.exe" // 默认
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			targetName = "yt-dlp_macos"
		case "arm64":
			targetName = "yt-dlp_macos" // Apple Silicon 也使用同一个版本
		default:
			targetName = "yt-dlp_macos"
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			targetName = "yt-dlp_linux"
		case "arm64":
			targetName = "yt-dlp_linux_aarch64"
		case "arm":
			targetName = "yt-dlp_linux_armv7l"
		default:
			targetName = "yt-dlp_linux"
		}
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	// 查找匹配的资源
	for _, asset := range release.Assets {
		if asset.Name == targetName {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("未找到适合 %s/%s 的下载文件", runtime.GOOS, runtime.GOARCH)
}

// downloadFile 下载文件
func (m *YtDlpManager) downloadFile(url string) error {
	m.logger.Infof("📥 下载中: %s", url)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	// 创建临时文件
	tempFile := m.binaryPath + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	// 下载文件并显示进度
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(tempFile)
		return err
	}

	// 移动到最终位置
	if err := os.Rename(tempFile, m.binaryPath); err != nil {
		os.Remove(tempFile)
		return err
	}

	return nil
}

// checkVersion 检查版本信息
func (m *YtDlpManager) checkVersion() error {
	version, err := m.Version()
	if err != nil {
		m.logger.Warnf("⚠️  无法获取 yt-dlp 版本信息: %v", err)
		return nil
	}

	m.logger.Infof("📋 当前 yt-dlp 版本: %s", version)
	return nil
}

// CheckAndUpdateIfStale 在版本过旧时自动更新 yt-dlp。
// yt-dlp 的稳定版本号通常是 YYYY.MM.DD；无法解析时仅记录版本，不阻断下载。
func (m *YtDlpManager) CheckAndUpdateIfStale(maxAge time.Duration) error {
	if !m.IsInstalled() {
		return m.CheckAndInstall()
	}

	version, err := m.Version()
	if err != nil {
		m.logger.Warnf("⚠️  无法获取 yt-dlp 版本信息: %v", err)
		return nil
	}
	m.logger.Infof("📋 当前 yt-dlp 版本: %s", version)

	releaseDate, ok := parseYtDlpVersionDate(version)
	if !ok {
		m.logger.Warnf("⚠️  无法解析 yt-dlp 版本日期: %s", version)
		return nil
	}

	age := time.Since(releaseDate)
	if age <= maxAge {
		return nil
	}

	m.logger.Warnf("⚠️  yt-dlp 版本已超过 %.0f 天，尝试自动更新", age.Hours()/24)
	if err := m.Update(); err != nil {
		return fmt.Errorf("自动更新 yt-dlp 失败: %v", err)
	}
	return nil
}

func parseYtDlpVersionDate(version string) (time.Time, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return time.Time{}, false
	}

	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return time.Time{}, false
	}

	dateText := strings.Join(parts[:3], ".")
	parsed, err := time.Parse("2006.1.2", dateText)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// Update 更新 yt-dlp 到最新版本
func (m *YtDlpManager) Update() error {
	m.logger.Info("🔄 更新 yt-dlp...")

	// 备份当前版本
	backupPath := m.binaryPath + ".backup"
	if err := os.Rename(m.binaryPath, backupPath); err != nil {
		m.logger.Warnf("⚠️  无法备份当前版本: %v", err)
	}

	// 安装最新版本
	if err := m.Install(); err != nil {
		// 恢复备份
		if _, statErr := os.Stat(backupPath); statErr == nil {
			os.Rename(backupPath, m.binaryPath)
		}
		return err
	}

	// 删除备份
	if _, err := os.Stat(backupPath); err == nil {
		os.Remove(backupPath)
	}

	m.logger.Info("✅ yt-dlp 更新完成")
	return nil
}

// Validate 验证 yt-dlp 是否正常工作
func (m *YtDlpManager) Validate() error {
	if !m.IsInstalled() {
		return fmt.Errorf("yt-dlp 未安装")
	}

	// 运行简单的测试命令
	cmd := exec.Command(m.binaryPath, "--help")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp 运行测试失败: %v", err)
	}

	return nil
}
