package bilibili

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UploadClient ?????
type UploadClient struct {
	client            *Client
	uploadClient      *http.Client // ??????? HTTP ????????????
	loginInfo         *LoginInfo
	uploadProgress    UploadProgressCallback
	uploadConcurrency int
}

func normalizeUploadConcurrency(concurrency int) int {
	if concurrency <= 0 {
		concurrency = 3
	}
	if concurrency > 8 {
		concurrency = 8
	}
	return concurrency
}

func buildUploadTransport(proxyURL string, concurrency int) *http.Transport {
	concurrency = normalizeUploadConcurrency(concurrency)
	maxPerHost := concurrency * 2
	if maxPerHost < 10 {
		maxPerHost = 10
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = maxPerHost * 4
	transport.MaxIdleConnsPerHost = maxPerHost
	transport.MaxConnsPerHost = maxPerHost
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = true

	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(parsedURL)
		}
	}

	return transport
}

// NewUploadClient ???????
func NewUploadClient(loginInfo *LoginInfo, opts ...Option) *UploadClient {
	config := DefaultConfig()
	config.ApplyOptions(opts...)

	transportConcurrency := config.UploadConcurrency
	if transportConcurrency <= 0 {
		transportConcurrency = 8
	}
	uploadClient := &http.Client{
		Timeout:   3 * time.Minute, // 单次分片请求上限 3 分钟，超时后由分片重试策略接管
		Transport: buildUploadTransport(config.ProxyURL, transportConcurrency),
	}

	return &UploadClient{
		client:            NewClient(opts...),
		uploadClient:      uploadClient,
		loginInfo:         loginInfo,
		uploadProgress:    config.UploadProgress,
		uploadConcurrency: config.UploadConcurrency,
	}
}

// retryFunc ???????????????
func retryFunc(fn func() error) error {
	return retryFuncWithPolicy(5, 2*time.Second, fn)
}

func retryFuncWithPolicy(maxRetries int, baseDelay time.Duration, fn func() error) error {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	if baseDelay <= 0 {
		baseDelay = time.Second
	}

	wait := baseDelay

	for retries := maxRetries; retries > 0; retries-- {
		err := fn()
		if err == nil {
			return nil
		}

		// ?????????????
		if retries > 1 && isRetryableUploadError(err) {
			// ?????? + ????
			jitter := time.Duration(rand.Float64() * float64(baseDelay))
			waitTime := wait + jitter
			maxWait := 120 * time.Second
			if waitTime > maxWait {
				waitTime = maxWait
			}

			log.Printf("Retry attempt #%d/%d for upload error, sleeping %s before retry. Error: %v",
				maxRetries-retries+1, maxRetries, waitTime, err)

			time.Sleep(waitTime)
			wait = time.Duration(math.Min(float64(wait)*1.8, float64(maxWait))) // ?????????
		} else if retries > 1 {
			// ??????????
			log.Printf("Quick retry #%d/%d for non-retryable-classified error: %v", maxRetries-retries+1, maxRetries, err)
			time.Sleep(baseDelay)
		} else {
			return err
		}
	}
	return nil
}

func isRetryableUploadError(err error) bool {
	if IsNetworkError(err) || IsRateLimitError(err) {
		return true
	}

	errorStr := strings.ToLower(err.Error())
	retryableHints := []string{
		"500 internal server error",
		"502 bad gateway",
		"503 service unavailable",
		"504 gateway timeout",
		"status 500",
		"status 502",
		"status 503",
		"status 504",
		"temporarily unavailable",
		"server error",
		"try again",
	}
	for _, hint := range retryableHints {
		if strings.Contains(errorStr, hint) {
			return true
		}
	}
	return false
}

// UploadVideo ??????
func (uc *UploadClient) UploadVideo(videoPath string) (*Video, error) {
	// 1. ??????
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	fileName := filepath.Base(videoPath)
	fileSize := fileInfo.Size()

	log.Printf("Uploading video: %s, size: %d bytes", fileName, fileSize)

	// 2. ???????
	preUploadInfo, err := uc.preUpload(fileName, fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to pre-upload: %v", err)
	}

	// 3. ????ID
	uploadID, err := uc.getUploadID(preUploadInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get upload ID: %v", err)
	}

	log.Printf("Got upload ID: %s", uploadID)

	// 4. ??????
	parts, err := uc.uploadChunks(videoPath, preUploadInfo, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to upload chunks: %v", err)
	}

	log.Printf("Uploaded %d chunks", len(parts))

	// 5. ????
	video, err := uc.completeUpload(preUploadInfo, uploadID, parts, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to complete upload: %v", err)
	}

	// ?? title ?????????????
	titleWithoutExt := fileName
	if ext := filepath.Ext(fileName); ext != "" {
		titleWithoutExt = strings.TrimSuffix(fileName, ext)
	}
	video.Title = titleWithoutExt

	log.Printf("Upload completed. Video filename: %s, title: %s", video.Filename, video.Title)

	return video, nil
}

// UploadVideoFromURL ? URL ??????? Bilibili?????????
func (uc *UploadClient) UploadVideoFromURL(videoURL, fileName string, fileSize int64) (*Video, error) {
	log.Printf("Uploading video from URL: %s, filename: %s, size: %d bytes", videoURL, fileName, fileSize)

	// 1. ???????
	preUploadInfo, err := uc.preUpload(fileName, fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to pre-upload: %v", err)
	}

	// 2. ????ID
	uploadID, err := uc.getUploadID(preUploadInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get upload ID: %v", err)
	}

	log.Printf("Got upload ID: %s", uploadID)

	// 3. ????????URL?????
	parts, err := uc.uploadChunksFromURL(videoURL, preUploadInfo, uploadID, fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to upload chunks from URL: %v", err)
	}

	log.Printf("Uploaded %d chunks from URL", len(parts))

	// 4. ????
	video, err := uc.completeUpload(preUploadInfo, uploadID, parts, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to complete upload: %v", err)
	}

	// ?? title ?????????????
	titleWithoutExt := fileName
	if ext := filepath.Ext(fileName); ext != "" {
		titleWithoutExt = strings.TrimSuffix(fileName, ext)
	}
	video.Title = titleWithoutExt

	log.Printf("Upload from URL completed. Video filename: %s, title: %s", video.Filename, video.Title)

	return video, nil
}
