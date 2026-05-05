package cos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/tencentyun/cos-go-sdk-v5"
)

// DownloadOptions 下载选项
type DownloadOptions struct {
	// 是否显示进度
	ShowProgress bool
	// 是否覆盖已存在的文件
	OverwriteExisting bool
	// 文件过滤器，只下载匹配的文件（可选）
	FileFilter func(string) bool
	// 进度回调函数
	ProgressCallback func(downloaded, total int)
}

// DownloadDirectoryWithOptions 带选项的目录下载
func (c *CosClient) DownloadDirectoryWithOptions(remoteDir, localDir string, options *DownloadOptions) error {
	if options == nil {
		options = &DownloadOptions{
			ShowProgress:      true,
			OverwriteExisting: false,
		}
	}

	// 确保远程目录格式正确
	if strings.HasPrefix(remoteDir, "/") {
		remoteDir = strings.TrimPrefix(remoteDir, "/")
	}
	if !strings.HasSuffix(remoteDir, "/") && remoteDir != "" {
		remoteDir += "/"
	}

	// 先统计总文件数
	totalFiles, err := c.countFiles(remoteDir, options.FileFilter)
	if err != nil {
		return fmt.Errorf("统计文件数量失败: %w", err)
	}

	if totalFiles == 0 {
		fmt.Printf("目录 %s 中没有找到文件\n", remoteDir)
		return nil
	}

	if options.ShowProgress {
		fmt.Printf("准备下载 %d 个文件从 %s 到 %s\n", totalFiles, remoteDir, localDir)
	}

	return c.downloadDirectoryWithProgress(remoteDir, localDir, options, totalFiles)
}

// countFiles 统计文件数量
func (c *CosClient) countFiles(remoteDir string, filter func(string) bool) (int, error) {
	ctx := context.Background()
	var marker string
	var count int

	for {
		opt := &cos.BucketGetOptions{
			Prefix:  remoteDir,
			Marker:  marker,
			MaxKeys: 1000,
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return 0, err
		}

		if len(resp.Contents) == 0 {
			break
		}

		for _, obj := range resp.Contents {
			if strings.HasSuffix(obj.Key, "/") {
				continue // 跳过目录
			}
			
			if filter != nil && !filter(obj.Key) {
				continue // 跳过不匹配的文件
			}
			
			count++
		}

		if resp.IsTruncated {
			marker = resp.NextMarker
		} else {
			break
		}
	}

	return count, nil
}

// downloadDirectoryWithProgress 带进度的目录下载
func (c *CosClient) downloadDirectoryWithProgress(remoteDir, localDir string, options *DownloadOptions, totalFiles int) error {
	ctx := context.Background()
	var marker string
	var downloaded int

	for {
		opt := &cos.BucketGetOptions{
			Prefix:  remoteDir,
			Marker:  marker,
			MaxKeys: 1000,
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return fmt.Errorf("列出文件失败: %w", err)
		}

		if len(resp.Contents) == 0 {
			break
		}

		for _, obj := range resp.Contents {
			if strings.HasSuffix(obj.Key, "/") {
				continue // 跳过目录
			}

			// 应用文件过滤器
			if options.FileFilter != nil && !options.FileFilter(obj.Key) {
				continue
			}

			// 构建本地文件路径
			relativePath := obj.Key
			if remoteDir != "" {
				relativePath = strings.TrimPrefix(obj.Key, remoteDir)
			}
			localFilePath := filepath.Join(localDir, relativePath)

			// 检查文件是否已存在
			if !options.OverwriteExisting {
				if _, err := os.Stat(localFilePath); err == nil {
					if options.ShowProgress {
						fmt.Printf("跳过已存在的文件: %s\n", localFilePath)
					}
					downloaded++
					if options.ProgressCallback != nil {
						options.ProgressCallback(downloaded, totalFiles)
					}
					continue
				}
			}

			// 下载文件
			if err := c.downloadSingleFileWithCheck(obj.Key, localFilePath); err != nil {
				fmt.Printf("下载文件失败 %s: %v\n", obj.Key, err)
				continue
			}

			downloaded++
			if options.ShowProgress {
				fmt.Printf("已下载 (%d/%d): %s -> %s\n", downloaded, totalFiles, obj.Key, localFilePath)
			}

			if options.ProgressCallback != nil {
				options.ProgressCallback(downloaded, totalFiles)
			}
		}

		if resp.IsTruncated {
			marker = resp.NextMarker
		} else {
			break
		}
	}

	if options.ShowProgress {
		fmt.Printf("下载完成！共下载 %d 个文件到 %s\n", downloaded, localDir)
	}
	return nil
}

// downloadSingleFileWithCheck 带检查的单文件下载
func (c *CosClient) downloadSingleFileWithCheck(remoteKey, localPath string) error {
	// 创建本地文件的目录
	localFileDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localFileDir, 0755); err != nil {
		return fmt.Errorf("创建本地文件目录失败: %w", err)
	}

	return c.downloadSingleFile(remoteKey, localPath)
}

// 预定义的文件过滤器
var (
	// ImageFilter 图片文件过滤器
	ImageFilter = func(filename string) bool {
		ext := strings.ToLower(filepath.Ext(filename))
		return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
	}

	// VideoFilter 视频文件过滤器
	VideoFilter = func(filename string) bool {
		ext := strings.ToLower(filepath.Ext(filename))
		return ext == ".mp4" || ext == ".avi" || ext == ".mov" || ext == ".mkv" || ext == ".flv"
	}

	// AudioFilter 音频文件过滤器
	AudioFilter = func(filename string) bool {
		ext := strings.ToLower(filepath.Ext(filename))
		return ext == ".mp3" || ext == ".wav" || ext == ".flac" || ext == ".aac" || ext == ".ogg"
	}

	// SubtitleFilter 字幕文件过滤器
	SubtitleFilter = func(filename string) bool {
		ext := strings.ToLower(filepath.Ext(filename))
		return ext == ".srt" || ext == ".vtt" || ext == ".ass" || ext == ".ssa"
	}
)
