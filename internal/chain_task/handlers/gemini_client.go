package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiClient Gemini API 客户端
type GeminiClient struct {
	client    *genai.Client
	model     string
	timeout   time.Duration
	maxTokens int
}

// NewGeminiClient 创建新的 Gemini 客户端
func NewGeminiClient(apiKey string, model string, timeout int, maxTokens int) (*GeminiClient, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("创建 Gemini 客户端失败: %v", err)
	}

	if model == "" {
		model = "gemini-1.5-pro"
	}

	if timeout <= 0 {
		timeout = 120
	}

	if maxTokens <= 0 {
		maxTokens = 8000
	}

	return &GeminiClient{
		client:    client,
		model:     model,
		timeout:   time.Duration(timeout) * time.Second,
		maxTokens: maxTokens,
	}, nil
}

// Close 关闭客户端
func (g *GeminiClient) Close() error {
	return g.client.Close()
}

// UploadFile 上传文件到 Gemini
func (g *GeminiClient) UploadFile(ctx context.Context, filePath string, displayName string) (*genai.File, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	uploadedFile, err := g.client.UploadFile(ctx, "", file, &genai.UploadFileOptions{
		DisplayName: displayName,
	})
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %v", err)
	}

	return uploadedFile, nil
}

// WaitForFileProcessing 等待文件处理完成
func (g *GeminiClient) WaitForFileProcessing(ctx context.Context, file *genai.File) error {
	for {
		fileInfo, err := g.client.GetFile(ctx, file.Name)
		if err != nil {
			return fmt.Errorf("获取文件状态失败: %v", err)
		}

		if fileInfo.State == genai.FileStateActive {
			return nil
		}

		if fileInfo.State == genai.FileStateFailed {
			return fmt.Errorf("文件处理失败")
		}

		// 等待一段时间后重试
		time.Sleep(2 * time.Second)
	}
}

// GenerateMetadataFromVideo 从视频生成元数据（标题、描述、标签）
func (g *GeminiClient) GenerateMetadataFromVideo(ctx context.Context, videoFile *genai.File) (*VideoMetadata, error) {
	// 直接使用模型名称，SDK会自动处理
	model := g.client.GenerativeModel(g.model)

	// 设置生成参数
	model.SetMaxOutputTokens(int32(g.maxTokens))
	model.SetTemperature(0.7)

	prompt := `请作为一个专业的 Bilibili UP 主，分析这个视频并生成以下内容：

1. 一个吸引眼球的标题（严格控制在30个字以内，能够准确概括视频主题）
2. 一个精炼的视频介绍（严格控制在100个字以内，提炼视频的核心内容和亮点）
3. 3-5个相关的标签

要求：
- 必须使用中文
- 标题要简洁有力，吸引观众点击
- 介绍要精炼，突出重点，严格控制在100字以内
- 标签要准确反映视频内容
- 输出格式必须是JSON，格式如下：
{
  "title": "视频标题",
  "description": "视频介绍（100字以内）",
  "tags": ["标签1", "标签2", "标签3"]
}

请直接返回JSON格式的结果，不要包含任何其他说明文字。`

	resp, err := model.GenerateContent(ctx, genai.FileData{URI: videoFile.URI}, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("生成内容失败: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("未生成任何内容")
	}

	// 提取文本内容
	content := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	return parseMetadataJSON(content)
}

// GenerateMetadataFromText 从文本生成元数据（用于字幕）
func (g *GeminiClient) GenerateMetadataFromText(ctx context.Context, subtitleText string) (*VideoMetadata, error) {
	// 直接使用模型名称，SDK会自动处理
	model := g.client.GenerativeModel(g.model)

	// 设置生成参数
	model.SetMaxOutputTokens(int32(g.maxTokens))
	model.SetTemperature(0.7)

	prompt := fmt.Sprintf(`请根据以下视频字幕内容，生成一个吸引人的视频标题、精炼介绍和3-5个相关标签。

字幕内容：
%s

要求：
1. 标题要简洁有力，严格控制在30个字以内，能够准确概括视频主题
2. 介绍要精炼，严格控制在100个字以内，提炼视频的核心内容和亮点
3. 标签要准确反映视频内容，3-5个即可
4. 必须使用中文
5. 输出格式必须是JSON，格式如下：
{
  "title": "视频标题",
  "description": "视频介绍（100字以内）",
  "tags": ["标签1", "标签2", "标签3"]
}

请直接返回JSON格式的结果，不要包含任何其他说明文字。`, subtitleText)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("生成内容失败: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("未生成任何内容")
	}

	// 提取文本内容
	content := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	return parseMetadataJSON(content)
}

// parseMetadataJSON 解析 JSON 格式的元数据
func parseMetadataJSON(content string) (*VideoMetadata, error) {
	var metadata VideoMetadata

	// 清理可能的 markdown 代码块标记
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}
	content = strings.TrimSpace(content)

	// 使用 json.Unmarshal 解析
	if err := json.Unmarshal([]byte(content), &metadata); err != nil {
		return nil, fmt.Errorf("解析元数据JSON失败: %v, 内容: %s", err, content)
	}

	// 验证必填字段
	if metadata.Title == "" {
		return nil, fmt.Errorf("生成的标题为空")
	}

	return &metadata, nil
}
