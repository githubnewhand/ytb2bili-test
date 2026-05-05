package utils

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ImageQuality string

const (
	QualityDefault  ImageQuality = "default"       // 120x90
	QualityMedium   ImageQuality = "mqdefault"     // 320x180
	QualityHigh     ImageQuality = "hqdefault"     // 480x360
	QualityStandard ImageQuality = "sddefault"     // 640x480
	QualityMax      ImageQuality = "maxresdefault" // 1280x720
)

var QualityPriority = []ImageQuality{
	QualityMax,
	QualityStandard,
	QualityHigh,
	QualityMedium,
	QualityDefault,
}

type DownloadResult struct {
	Success      bool
	FilePath     string
	Quality      string
	ErrorMessage string
	FileSize     int64
}

type DownloadOptions struct {
	SavePath         string
	FilenameTemplate string
	Timeout          time.Duration
	MaxRetries       int
	QualityFallback  bool
	CreateDirs       bool
	Overwrite        bool
}

type YouTubeThumbnailDownloader struct {
	Options DownloadOptions
}

func NewYouTubeThumbnailDownloader(opt DownloadOptions) *YouTubeThumbnailDownloader {
	if opt.FilenameTemplate == "" {
		opt.FilenameTemplate = "{quality}"
	}
	if opt.Timeout == 0 {
		opt.Timeout = 10 * time.Second
	}
	if opt.MaxRetries == 0 {
		opt.MaxRetries = 3
	}
	return &YouTubeThumbnailDownloader{Options: opt}
}

func (d *YouTubeThumbnailDownloader) buildFilePath(videoID string, quality ImageQuality, customFilename string) (string, error) {
	dir := d.Options.SavePath
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if d.Options.CreateDirs {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return "", err
		}
	}
	var filename string
	if customFilename != "" {
		filename = customFilename + ".jpg"
	} else {
		tmpl := d.Options.FilenameTemplate
		filename = strings.ReplaceAll(tmpl, "{quality}", string(quality)) + ".jpg"
	}
	return filepath.Join(dir, filename), nil
}

func (d *YouTubeThumbnailDownloader) downloadSingleQuality(videoID string, quality ImageQuality, customFilename string) DownloadResult {
	url := fmt.Sprintf("https://img.youtube.com/vi/%s/%s.jpg", videoID, quality)
	filePath, err := d.buildFilePath(videoID, quality, customFilename)
	if err != nil {
		return DownloadResult{Success: false, ErrorMessage: err.Error(), Quality: string(quality)}
	}
	if !d.Options.Overwrite {
		if fi, err := os.Stat(filePath); err == nil && fi.Size() > 0 {
			return DownloadResult{Success: true, FilePath: filePath, Quality: string(quality), FileSize: fi.Size()}
		}
	}
	var lastErr error
	for attempt := 0; attempt < d.Options.MaxRetries; attempt++ {
		log.Printf("尝试下载 %s 质量图片，第 %d 次: %s", quality, attempt+1, url)
		client := &http.Client{Timeout: d.Options.Timeout}
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image") {
				return DownloadResult{Success: false, ErrorMessage: "响应内容不是图片", Quality: string(quality)}
			}
			f, err := os.Create(filePath)
			if err != nil {
				return DownloadResult{Success: false, ErrorMessage: err.Error(), Quality: string(quality)}
			}
			defer f.Close()
			sz, err := io.Copy(f, resp.Body)
			if err != nil {
				return DownloadResult{Success: false, ErrorMessage: err.Error(), Quality: string(quality)}
			}
			return DownloadResult{Success: true, FilePath: filePath, Quality: string(quality), FileSize: sz}
		} else if resp.StatusCode == 404 {
			return DownloadResult{Success: false, ErrorMessage: "图片不存在 (404)", Quality: string(quality)}
		} else {
			lastErr = errors.New(resp.Status)
		}
	}
	return DownloadResult{Success: false, ErrorMessage: fmt.Sprintf("下载失败: %v", lastErr), Quality: string(quality)}
}

func (d *YouTubeThumbnailDownloader) downloadMultipleQualities(videoID string, qualities []ImageQuality, customFilename string) map[string]DownloadResult {
	results := make(map[string]DownloadResult)
	for _, q := range qualities {
		results[string(q)] = d.downloadSingleQuality(videoID, q, customFilename)
	}
	return results
}

func (d *YouTubeThumbnailDownloader) downloadBestAvailable(videoID string, preferred []ImageQuality, customFilename string) DownloadResult {
	qualities := preferred
	if len(qualities) == 0 {
		qualities = QualityPriority
	}
	for _, q := range qualities {
		res := d.downloadSingleQuality(videoID, q, customFilename)
		if res.Success {
			return res
		}
		if d.Options.QualityFallback {
			continue
		} else {
			return res
		}
	}
	return DownloadResult{Success: false, ErrorMessage: "所有质量都下载失败"}
}

func (d *YouTubeThumbnailDownloader) downloadAllAvailable(videoID string, customFilename string) map[string]DownloadResult {
	return d.downloadMultipleQualities(videoID, []ImageQuality{
		QualityMax, QualityStandard, QualityHigh, QualityMedium, QualityDefault,
	}, customFilename)
}

// 便利函数
type QualityInput interface{}

func DownloadYouTubeThumbnail(videoID string, quality QualityInput, opt DownloadOptions, filename string) interface{} {
	downloader := NewYouTubeThumbnailDownloader(opt)
	switch v := quality.(type) {
	case string:
		if v == "best" {
			return downloader.downloadBestAvailable(videoID, nil, filename)
		} else if v == "all" {
			return downloader.downloadAllAvailable(videoID, filename)
		} else {
			return downloader.downloadSingleQuality(videoID, ImageQuality(v), filename)
		}
	case []ImageQuality:
		return downloader.downloadMultipleQualities(videoID, v, filename)
	case []string:
		var qualities []ImageQuality
		for _, s := range v {
			qualities = append(qualities, ImageQuality(s))
		}
		return downloader.downloadMultipleQualities(videoID, qualities, filename)
	default:
		return downloader.downloadSingleQuality(videoID, QualityDefault, filename)
	}
}

//func main() {
//	opt := DownloadOptions{
//		SavePath:         "./thumbnails",
//		FilenameTemplate: "{video_id}_{quality}",
//		Timeout:          10 * time.Second,
//		MaxRetries:       3,
//		QualityFallback:  true,
//		CreateDirs:       true,
//		Overwrite:        false,
//	}
//	videoID := "SYy8_z-qsRo"
//	// 示例1: 下载多种质量
//	qualities := []ImageQuality{QualityMax, QualityHigh, QualityMedium}
//	results := DownloadYouTubeThumbnail(videoID, qualities, opt, "").(map[string]DownloadResult)
//	for k, v := range results {
//		if v.Success {
//			fmt.Printf("下载成功: %s - %s (%d bytes)\n", k, v.FilePath, v.FileSize)
//		} else {
//			fmt.Printf("下载失败: %s - %s\n", k, v.ErrorMessage)
//		}
//	}
//}
