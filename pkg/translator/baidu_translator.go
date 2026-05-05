package translator

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	logger2 "github.com/difyz9/ytb2bili/pkg/logger"
)

var logger = logger2.GetLogger()

// BaiduTranslator 百度翻译器
type BaiduTranslator struct {
	appId     string
	secretKey string
	endpoint  string
	client    *http.Client
}

// BaiduTranslationResponse 百度翻译API响应结构
type BaiduTranslationResponse struct {
	From        string                       `json:"from"`
	To          string                       `json:"to"`
	TransResult []BaiduTranslationResultItem `json:"trans_result"`
	ErrorCode   string                       `json:"error_code,omitempty"`
	ErrorMsg    string                       `json:"error_msg,omitempty"`
}

// BaiduTranslationResultItem 百度翻译结果项
type BaiduTranslationResultItem struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

// BaiduLanguageDetectionResponse 百度语言检测响应
type BaiduLanguageDetectionResponse struct {
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	Data      struct {
		Src string `json:"src"`
	} `json:"data"`
}

// NewBaiduTranslator 创建百度翻译器实例
func NewBaiduTranslator(config map[string]interface{}) (Translator, error) {
	appId, ok := config["app_id"].(string)
	if !ok || appId == "" {
		return nil, fmt.Errorf("baidu translator app_id is required")
	}

	secretKey, ok := config["secret_key"].(string)
	if !ok || secretKey == "" {
		return nil, fmt.Errorf("baidu translator secret_key is required")
	}

	endpoint, ok := config["endpoint"].(string)
	if !ok || endpoint == "" {
		endpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"
	}

	return &BaiduTranslator{
		appId:     appId,
		secretKey: secretKey,
		endpoint:  endpoint,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Translate 翻译文本
func (bt *BaiduTranslator) Translate(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	startTime := time.Now()

	// 验证参数
	if req.Text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// 转换语言代码
	sourceLang := bt.convertLanguageCode(req.SourceLang)
	targetLang := bt.convertLanguageCode(req.TargetLang)

	// 生成随机数
	salt := strconv.FormatInt(time.Now().UnixNano(), 10)

	// 生成签名
	sign := bt.generateSign(req.Text, salt)

	// 构建请求参数
	params := url.Values{}
	params.Set("q", req.Text)
	params.Set("from", sourceLang)
	params.Set("to", targetLang)
	params.Set("appid", bt.appId)
	params.Set("salt", salt)
	params.Set("sign", sign)

	// 发起请求
	resp, err := bt.client.PostForm(bt.endpoint, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// 解析响应
	var apiResp BaiduTranslationResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查错误
	if apiResp.ErrorCode != "" && apiResp.ErrorCode != "52000" {
		return nil, fmt.Errorf("baidu API error: %s - %s", apiResp.ErrorCode, apiResp.ErrorMsg)
	}

	// 检查翻译结果
	if len(apiResp.TransResult) == 0 {
		return nil, fmt.Errorf("no translation result returned")
	}

	// 构建结果
	duration := time.Since(startTime)
	result := &TranslationResult{
		OriginalText:   req.Text,
		TranslatedText: apiResp.TransResult[0].Dst,
		SourceLang:     bt.revertLanguageCode(apiResp.From),
		TargetLang:     bt.revertLanguageCode(apiResp.To),
		Provider:       "baidu",
		Usage: &Usage{
			Characters: len(req.Text),
			Duration:   duration.Milliseconds(),
		},
	}

	logger.Infof("Baidu translation completed: %s -> %s (%dms)",
		req.SourceLang, req.TargetLang, result.Usage.Duration)

	return result, nil
}

// BatchTranslate 批量翻译
func (bt *BaiduTranslator) BatchTranslate(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	startTime := time.Now()

	if len(req.Texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	// 百度API支持用换行符分隔多段文本
	combinedText := strings.Join(req.Texts, "\n")

	// 调用单文本翻译接口
	singleReq := &TranslationRequest{
		Text:       combinedText,
		SourceLang: req.SourceLang,
		TargetLang: req.TargetLang,
	}

	singleResult, err := bt.Translate(ctx, singleReq)
	if err != nil {
		return nil, err
	}

	// 按换行符分割翻译结果
	translatedTexts := strings.Split(singleResult.TranslatedText, "\n")

	// 确保结果数量匹配
	if len(translatedTexts) != len(req.Texts) {
		// 如果数量不匹配，尝试逐个翻译
		return bt.batchTranslateIndividually(ctx, req)
	}

	// 构建结果
	results := make([]*TranslationResult, len(req.Texts))
	totalChars := 0

	for i, originalText := range req.Texts {
		results[i] = &TranslationResult{
			OriginalText:   originalText,
			TranslatedText: translatedTexts[i],
			SourceLang:     req.SourceLang,
			TargetLang:     req.TargetLang,
			Provider:       "baidu",
			Usage: &Usage{
				Characters: len(originalText),
				Duration:   0, // 批量翻译中单个项目的耗时不单独计算
			},
		}
		totalChars += len(originalText)
	}

	duration := time.Since(startTime)
	batchResult := &BatchTranslationResult{
		Results: results,
		Usage: &Usage{
			Characters: totalChars,
			Duration:   duration.Milliseconds(),
		},
	}

	logger.Infof("Baidu batch translation completed: %d texts, %d chars (%dms)",
		len(req.Texts), totalChars, batchResult.Usage.Duration)

	return batchResult, nil
}

// batchTranslateIndividually 逐个翻译（备用方法）
func (bt *BaiduTranslator) batchTranslateIndividually(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	startTime := time.Now()
	results := make([]*TranslationResult, len(req.Texts))
	totalChars := 0

	for i, text := range req.Texts {
		singleReq := &TranslationRequest{
			Text:       text,
			SourceLang: req.SourceLang,
			TargetLang: req.TargetLang,
		}

		result, err := bt.Translate(ctx, singleReq)
		if err != nil {
			return nil, fmt.Errorf("failed to translate text %d: %v", i+1, err)
		}

		results[i] = result
		totalChars += len(text)

		// 添加小延迟避免频率限制
		time.Sleep(100 * time.Millisecond)
	}

	duration := time.Since(startTime)
	return &BatchTranslationResult{
		Results: results,
		Usage: &Usage{
			Characters: totalChars,
			Duration:   duration.Milliseconds(),
		},
	}, nil
}

// GetSupportedLanguages 获取支持的语言列表
func (bt *BaiduTranslator) GetSupportedLanguages(ctx context.Context) ([]LanguageInfo, error) {
	// 百度翻译支持的常见语种
	languages := []LanguageInfo{
		{Code: "auto", Name: "自动检测", NativeName: "Auto Detect"},
		{Code: "zh", Name: "中文(简体)", NativeName: "中文(简体)"},
		{Code: "en", Name: "英语", NativeName: "English"},
		{Code: "jp", Name: "日语", NativeName: "日本語"},
		{Code: "kor", Name: "韩语", NativeName: "한국어"},
		{Code: "fra", Name: "法语", NativeName: "Français"},
		{Code: "spa", Name: "西班牙语", NativeName: "Español"},
		{Code: "th", Name: "泰语", NativeName: "ไทย"},
		{Code: "ara", Name: "阿拉伯语", NativeName: "العربية"},
		{Code: "ru", Name: "俄语", NativeName: "Русский"},
		{Code: "pt", Name: "葡萄牙语", NativeName: "Português"},
		{Code: "de", Name: "德语", NativeName: "Deutsch"},
		{Code: "it", Name: "意大利语", NativeName: "Italiano"},
		{Code: "el", Name: "希腊语", NativeName: "Ελληνικά"},
		{Code: "nl", Name: "荷兰语", NativeName: "Nederlands"},
		{Code: "pl", Name: "波兰语", NativeName: "Polski"},
		{Code: "bul", Name: "保加利亚语", NativeName: "Български"},
		{Code: "est", Name: "爱沙尼亚语", NativeName: "Eesti"},
		{Code: "dan", Name: "丹麦语", NativeName: "Dansk"},
		{Code: "fin", Name: "芬兰语", NativeName: "Suomi"},
		{Code: "cs", Name: "捷克语", NativeName: "Čeština"},
		{Code: "rom", Name: "罗马尼亚语", NativeName: "Română"},
		{Code: "slo", Name: "斯洛文尼亚语", NativeName: "Slovenščina"},
		{Code: "swe", Name: "瑞典语", NativeName: "Svenska"},
		{Code: "hu", Name: "匈牙利语", NativeName: "Magyar"},
		{Code: "cht", Name: "中文(繁体)", NativeName: "中文(繁體)"},
		{Code: "vie", Name: "越南语", NativeName: "Tiếng Việt"},
		{Code: "yue", Name: "中文(粤语)", NativeName: "中文(粵語)"},
		{Code: "wyw", Name: "中文(文言文)", NativeName: "中文(文言文)"},
		{Code: "hi", Name: "印地语", NativeName: "हिन्दी"},
		{Code: "id", Name: "印尼语", NativeName: "Bahasa Indonesia"},
		{Code: "may", Name: "马来语", NativeName: "Bahasa Melayu"},
		{Code: "bur", Name: "缅甸语", NativeName: "မြန်မာ"},
		{Code: "nor", Name: "挪威语", NativeName: "Norsk"},
		{Code: "swe", Name: "瑞典语", NativeName: "Svenska"},
		{Code: "ice", Name: "冰岛语", NativeName: "Íslenska"},
		{Code: "tr", Name: "土耳其语", NativeName: "Türkçe"},
		{Code: "ukr", Name: "乌克兰语", NativeName: "Українська"},
		{Code: "wel", Name: "威尔士语", NativeName: "Cymraeg"},
		{Code: "urd", Name: "乌尔都语", NativeName: "اردو"},
		{Code: "heb", Name: "希伯来语", NativeName: "עברית"},
		{Code: "arm", Name: "亚美尼亚语", NativeName: "Հայերեն"},
	}

	logger.Infof("Baidu translator supports %d languages", len(languages))
	return languages, nil
}

// DetectLanguage 检测语言
func (bt *BaiduTranslator) DetectLanguage(ctx context.Context, text string) (string, float64, error) {
	if text == "" {
		return "", 0, fmt.Errorf("text cannot be empty")
	}

	// 百度翻译通过将源语言设为auto来自动检测
	req := &TranslationRequest{
		Text:       text,
		SourceLang: "auto",
		TargetLang: "en", // 目标语言设为英语
	}

	result, err := bt.Translate(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to detect language: %v", err)
	}

	// 置信度设为0.95（百度API不直接提供置信度）
	confidence := 0.95

	logger.Infof("Baidu language detection: %s (confidence: %.2f)", result.SourceLang, confidence)
	return result.SourceLang, confidence, nil
}

// IsHealthy 健康检查
func (bt *BaiduTranslator) IsHealthy(ctx context.Context) error {
	// 使用简单的翻译请求进行健康检查
	req := &TranslationRequest{
		Text:       "hello",
		SourceLang: "en",
		TargetLang: "zh",
	}

	_, err := bt.Translate(ctx, req)
	if err != nil {
		return fmt.Errorf("baidu translator health check failed: %v", err)
	}

	logger.Info("Baidu translator health check passed")
	return nil
}

// GetInfo 获取翻译器信息
func (bt *BaiduTranslator) GetInfo() *TranslatorInfo {
	return &TranslatorInfo{
		Name:               "Baidu Translate",
		Provider:           "baidu",
		Version:            "1.0.0",
		MaxTextLength:      6000, // 百度翻译支持最大6000字符
		SupportedLanguages: []string{"zh", "en", "jp", "kor", "fra", "spa", "th", "ara", "ru", "pt", "de", "it"},
		Features:           []string{"translate", "batch_translate", "language_detect"},
		IsOnline:           true,
	}
}

// generateSign 生成签名
func (bt *BaiduTranslator) generateSign(query, salt string) string {
	// 按照百度API文档：appid+q+salt+密钥
	signStr := bt.appId + query + salt + bt.secretKey

	// 计算MD5
	hash := md5.Sum([]byte(signStr))
	return fmt.Sprintf("%x", hash)
}

// convertLanguageCode 转换语言代码到百度格式
func (bt *BaiduTranslator) convertLanguageCode(code string) string {
	// 标准化语言代码映射
	mapping := map[string]string{
		"zh-cn": "zh",
		"zh-tw": "cht",
		"zh-hk": "yue",
		"ja":    "jp",
		"ko":    "kor",
		"fr":    "fra",
		"es":    "spa",
		"ar":    "ara",
		"vi":    "vie",
		"ms":    "may",
		"my":    "bur",
		"no":    "nor",
		"sv":    "swe",
		"is":    "ice",
		"uk":    "ukr",
		"cy":    "wel",
		"ur":    "urd",
		"he":    "heb",
		"hy":    "arm",
	}

	if mapped, exists := mapping[code]; exists {
		return mapped
	}
	return code
}

// revertLanguageCode 将百度语言代码转换回标准格式
func (bt *BaiduTranslator) revertLanguageCode(code string) string {
	// 反向映射
	mapping := map[string]string{
		"cht": "zh-tw",
		"yue": "zh-hk",
		"jp":  "ja",
		"kor": "ko",
		"fra": "fr",
		"spa": "es",
		"ara": "ar",
		"vie": "vi",
		"may": "ms",
		"bur": "my",
		"nor": "no",
		"swe": "sv",
		"ice": "is",
		"ukr": "uk",
		"wel": "cy",
		"urd": "ur",
		"heb": "he",
		"arm": "hy",
	}

	if mapped, exists := mapping[code]; exists {
		return mapped
	}
	return code
}
