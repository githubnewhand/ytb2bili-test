package bilibili

import (
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UploadClient ?????
type UploadClient struct {
	client         *Client
	uploadClient   *http.Client // ??????? HTTP ????????????
	loginInfo      *LoginInfo
	uploadProgress UploadProgressCallback
	uploadCDN      string
}

func buildUploadTransport(proxyURL string) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 10
	transport.MaxIdleConnsPerHost = 5
	transport.IdleConnTimeout = 30 * time.Second
	transport.DisableCompression = true
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = false
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 20 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second

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

	uploadClient := &http.Client{
		Timeout:   2 * time.Minute, // ???????15??
		Transport: buildUploadTransport(config.ProxyURL),
	}

	return &UploadClient{
		client:         NewClient(opts...),
		uploadClient:   uploadClient,
		loginInfo:      loginInfo,
		uploadProgress: config.UploadProgress,
		uploadCDN:      config.UploadCDN,
	}
}

// retryFunc ???????????????
func retryFunc(fn func() error) error {
	maxRetries := 5 // ??????
	wait := 2.0     // ????????

	for retries := maxRetries; retries > 0; retries-- {
		err := fn()
		if err == nil {
			return nil
		}

		// ?????????????
		if retries > 1 && IsNetworkError(err) {
			// ?????? + ????
			jitter := rand.Float64() * 2.0           // ??????
			waitTime := math.Min(jitter+wait, 120.0) // ????????

			log.Printf("?? Retry attempt #%d/%d. Network error detected, sleeping %.2fs before retry. Error: %v",
				maxRetries-retries+1, maxRetries, waitTime, err)

			time.Sleep(time.Duration(waitTime * float64(time.Second)))
			wait *= 1.8 // ?????????
		} else if retries > 1 {
			// ??????????
			log.Printf("? Quick retry #%d/%d for non-network error: %v", maxRetries-retries+1, maxRetries, err)
			time.Sleep(1 * time.Second)
		} else {
			return err
		}
	}
	return nil
}

// UploadVideo ??????
func (uc *UploadClient) UploadVideo(videoPath string) (*Video, error) {
	return uc.withUploadRoutes(func() (*Video, error) { return uc.uploadVideoOnce(videoPath) })
}
func (uc *UploadClient) uploadVideoOnce(videoPath string) (*Video, error) {
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
	return uc.withUploadRoutes(func() (*Video, error) { return uc.uploadVideoFromURLOnce(videoURL, fileName, fileSize) })
}
func (uc *UploadClient) uploadVideoFromURLOnce(videoURL, fileName string, fileSize int64) (*Video, error) {
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

func (uc *UploadClient) withUploadRoutes(fn func() (*Video, error)) (*Video, error) {
	if uc.uploadCDN != "" {
		return fn()
	}
	defer func() { uc.uploadCDN = "" }()
	var last error
	for index, route := range []string{"ws", "qn", "bda2"} {
		uc.uploadCDN = route
		video, err := fn()
		if err == nil {
			return video, nil
		}
		last = err
		text := strings.ToLower(err.Error())
		if !IsNetworkError(err) && !strings.Contains(text, "status 5") && !strings.Contains(text, "protocol_error") {
			return nil, err
		}
		log.Printf("Upload storage route %d failed; requesting another route", index+1)
	}
	return nil, last
}
