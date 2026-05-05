package cos

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
)

// CosHandler COS处理器接口
type CosHandler interface {
	// UploadAudioFromURL 从URL上传音频
	UploadAudioFromURL(audioURL, newName string) (string, error)
	DownloadVideoFromCos(keyName, savePath string) (string, error)

	// UploadSrtToCOS 上传字幕到COS
	UploadSrtToCOS(filePath, keyName string) (string, error)

	// UploadAudioToCOS 上传音频到COS
	UploadAudioToCOS(filePath, keyName string) (string, error)

	// UploadAudioToCOS 上传音频到COS
	UploadVideoToCOS(filePath, keyName string) (string, error)

	// GeneratePresignedURL 生成预签名URL
	GeneratePresignedURL(keyName string) string
}

// CosHandlerImpl COS处理器实现
type CosHandlerImpl struct {
	client *CosClient
}

// NewCosHandler 创建COS处理器
func NewCosHandler(config *types.AppConfig) (CosHandler, error) {
	client, err := NewCosClient(config)
	if err != nil {
		return nil, err
	}

	return &CosHandlerImpl{
		client: client,
	}, nil
}

// UploadAudioFromURL 从URL上传音频
func (h *CosHandlerImpl) UploadAudioFromURL(audioURL, newName string) (string, error) {
	return h.client.UploadAudioFromURL(audioURL, newName)
}

func (h *CosHandlerImpl) DownloadVideoFromCos(keyName, savePath string) (string, error) {
	return h.client.DownloadVideo(keyName, savePath)
}

// UploadSrtToCOS 上传字幕到COS
func (h *CosHandlerImpl) UploadSrtToCOS(filePath, keyName string) (string, error) {
	return h.client.UploadSrtToCOS(filePath, keyName)
}

// UploadAudioToCOS 上传音频到COS
func (h *CosHandlerImpl) UploadAudioToCOS(filePath, keyName string) (string, error) {
	return h.client.UploadAudioToCOS(filePath, keyName)
}

// GeneratePresignedURL 生成预签名URL
func (h *CosHandlerImpl) GeneratePresignedURL(keyName string) string {
	return h.client.GeneratePresignedURL(keyName)
}
func (h *CosHandlerImpl) UploadVideoToCOS(filePath, keyName string) (string, error) {
	return h.client.UploadVideoToCOS(filePath, keyName)
}
