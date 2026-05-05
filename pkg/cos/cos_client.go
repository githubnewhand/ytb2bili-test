package cos

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"context"
	"fmt"
	"github.com/tencentyun/cos-go-sdk-v5"
	"github.com/tencentyun/qcloud-cos-sts-sdk/go"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CosClient 腾讯云对象存储客户端
type CosClient struct {
	Client *cos.Client
	Config *types.AppConfig
}

//private，public-read，public-read-write

// Permission 权限配置
type Permission struct {
	LimitExt           bool     `json:"limitExt"`
	ExtWhiteList       []string `json:"extWhiteList"`
	LimitContentType   bool     `json:"limitContentType"`
	LimitContentLength bool     `json:"limitContentLength"`
}

// CosCredentialsResult 临时凭证结果
type CosCredentialsResult struct {
	Credentials struct {
		TmpSecretId     string `json:"tmpSecretId"`
		TmpSecretKey    string `json:"tmpSecretKey"`
		SecurityToken   string `json:"securityToken"`
		StartTime       int64  `json:"startTime"`
		ExpiredTime     int64  `json:"expiredTime"`
		RequestId       string `json:"requestId"`
		DurationSeconds int    `json:"durationSeconds"`
	} `json:"credentials"`
	Bucket string `json:"bucket"`
	Region string `json:"region"`
	Key    string `json:"key"`
}

// DirectUploadResult 直传结果
type DirectUploadResult struct {
	CosHost        string `json:"cosHost"`        // COS 主机地址
	CosKey         string `json:"cosKey"`         // COS 对象键
	Authorization  string `json:"authorization"`  // 预签名授权
	SecurityToken  string `json:"securityToken"`  // 安全令牌
	UploadUrl      string `json:"uploadUrl"`      // 完整上传URL
	ExpirationTime int64  `json:"expirationTime"` // 过期时间戳
}

// DirectUploadData 简化的直传数据结构
type DirectUploadData struct {
	Url           string `json:"url"`
	CosHost       string `json:"cosHost"`       // COS 主机地址
	CosKey        string `json:"cosKey"`        // COS 对象键
	CosUrl        string `json:"cosUrl"`        // COS 对象键
	Authorization string `json:"authorization"` // 预签名授权
	SecurityToken string `json:"securityToken"` // 安全令牌
}

// NewCosClient 创建腾讯云对象存储客户端
func NewCosClient(config *types.AppConfig) (*CosClient, error) {
	// 检查配置是否存在
	if config.TenCosConfig == nil {
		return nil, fmt.Errorf("TenCosConfig is not configured")
	}

	// 从配置中获取信息
	bucketURLStr := config.TenCosConfig.CosBucketURL
	secretID := config.TenCosConfig.CosSecretId
	secretKey := config.TenCosConfig.CosSecretKey

	if bucketURLStr == "" || secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("missing required COS configuration")
	}

	// 解析存储桶 URL
	u, err := url.Parse(bucketURLStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bucket URL: %w", err)
	}

	// 创建 COS 客户端
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	return &CosClient{
		Client: client,
		Config: config,
	}, nil
}

// generateCosKey 生成 COS 对象键
func generateCosKey(ext string) string {
	date := time.Now()
	m := int(date.Month())
	ymd := fmt.Sprintf("%d%02d%02d", date.Year(), m, date.Day())
	r := fmt.Sprintf("%06d", rand.Intn(1000000))

	// 确保扩展名以点号开头
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	cosKey := fmt.Sprintf("file/%s/%s_%s%s", ymd, ymd, r, ext)
	return cosKey
}

// getPermission 获取权限配置
func getPermission() Permission {
	permission := Permission{
		LimitExt:           true,
		ExtWhiteList:       []string{"jpg", "jpeg", "exe", "msi", "zip", "png", "gif", "bmp", "mp4", "avi", "mov", "wmv", "flv", "webm", "mkv", "mp3", "wav", "m4a", "aac", "srt", "vtt", "m3u8", "ts"},
		LimitContentType:   false,
		LimitContentLength: true,
	}
	return permission
}

// stringInSlice 检查字符串是否在切片中
func stringInSlice(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

// ValidateConfig 验证COS配置
func (c *CosClient) ValidateConfig() error {
	if c.Config.TenCosConfig == nil {
		return fmt.Errorf("腾讯云 COS 配置不存在")
	}

	if !c.Config.TenCosConfig.Enabled {
		return fmt.Errorf("腾讯云 COS 服务未启用")
	}

	if c.Config.TenCosConfig.CosSecretId == "" || c.Config.TenCosConfig.CosSecretKey == "" ||
		c.Config.TenCosConfig.CosBucket == "" || c.Config.TenCosConfig.CosRegion == "" {
		return fmt.Errorf("腾讯云 COS 配置不完整")
	}

	return nil
}

// ValidateFileExtension 验证文件扩展名
func (c *CosClient) ValidateFileExtension(ext string) error {
	permission := getPermission()

	// 移除扩展名前的点号（如果存在）用于验证
	cleanExt := strings.TrimPrefix(ext, ".")

	if permission.LimitExt {
		if cleanExt == "" || !stringInSlice(cleanExt, permission.ExtWhiteList) {
			return fmt.Errorf("不支持的文件类型: %s，支持的类型: %s",
				ext, strings.Join(permission.ExtWhiteList, ", "))
		}
	}

	return nil
}

// GetTempCredentials 获取临时凭证
func (c *CosClient) GetTempCredentials(key string) (*CosCredentialsResult, error) {
	// 创建 STS 客户端
	stsClient := sts.NewClient(
		c.Config.TenCosConfig.CosSecretId,
		c.Config.TenCosConfig.CosSecretKey,
		nil,
	)

	permission := getPermission()

	// 构建策略条件
	condition := make(map[string]map[string]interface{})

	if permission.LimitContentType {
		condition["string_like_if_exist"] = map[string]interface{}{
			"cos:content-type": "image/*",
		}
	}

	// 限制上传文件大小（100MB）
	if permission.LimitContentLength {
		condition["numeric_less_than_equal"] = map[string]interface{}{
			"cos:content-length": 100 * 1024 * 1024,
		}
	}

	// 构建策略选项
	opt := &sts.CredentialOptions{
		DurationSeconds: int64(1800), // 30分钟
		Region:          c.Config.TenCosConfig.CosRegion,
		Policy: &sts.CredentialPolicy{
			Version: "2.0",
			Statement: []sts.CredentialPolicyStatement{
				{
					Action: []string{
						"name/cos:PutObject",
						"name/cos:InitiateMultipartUpload",
						"name/cos:ListMultipartUploads",
						"name/cos:ListParts",
						"name/cos:UploadPart",
						"name/cos:CompleteMultipartUpload",
					},
					Effect: "allow",
					Resource: []string{
						fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s",
							c.Config.TenCosConfig.CosRegion, c.Config.TenCosConfig.SubAppId, c.Config.TenCosConfig.CosBucket, key),
					},
					Condition: condition,
				},
			},
		},
	}

	// 请求临时密钥
	res, err := stsClient.GetCredential(opt)
	if err != nil {
		return nil, fmt.Errorf("获取临时凭证失败: %w", err)
	}

	// 构建返回结果
	result := &CosCredentialsResult{
		Bucket: c.Config.TenCosConfig.CosBucket,
		Region: c.Config.TenCosConfig.CosRegion,
		Key:    key,
	}

	result.Credentials.TmpSecretId = res.Credentials.TmpSecretID
	result.Credentials.TmpSecretKey = res.Credentials.TmpSecretKey
	result.Credentials.SecurityToken = res.Credentials.SessionToken
	result.Credentials.StartTime = int64(res.StartTime)
	result.Credentials.ExpiredTime = int64(res.ExpiredTime)
	result.Credentials.RequestId = res.RequestId
	result.Credentials.DurationSeconds = int(opt.DurationSeconds)

	return result, nil
}

// GenerateDirectUploadUrl 生成直传URL
func (c *CosClient) GenerateDirectUploadUrl(fileExtension string, durationSeconds int64) (DirectUploadData, error) {
	// 验证配置
	if err := c.ValidateConfig(); err != nil {
		return DirectUploadData{}, err
	}

	// 验证文件扩展名
	if err := c.ValidateFileExtension(fileExtension); err != nil {
		return DirectUploadData{}, err
	}

	// 创建 STS 客户端
	stsClient := sts.NewClient(
		c.Config.TenCosConfig.CosSecretId,
		c.Config.TenCosConfig.CosSecretKey,
		nil,
	)

	permission := getPermission()

	// 构建策略条件
	condition := make(map[string]map[string]interface{})

	if permission.LimitContentType {
		condition["string_like_if_exist"] = map[string]interface{}{
			"cos:content-type": "image/*",
		}
	}

	if permission.LimitContentLength {
		condition["numeric_less_than_equal"] = map[string]interface{}{
			"cos:content-length": 100 * 1024 * 1024, // 100MB
		}
	}

	// 生成COS对象键
	key := generateCosKey(fileExtension)

	// 使用传入的有效期，如果没传则默认30分钟
	if durationSeconds <= 0 {
		durationSeconds = 1800
	}

	// 构建STS策略选项
	opt := &sts.CredentialOptions{
		DurationSeconds: durationSeconds,
		Region:          c.Config.TenCosConfig.CosRegion,
		Policy: &sts.CredentialPolicy{
			Version: "2.0",
			Statement: []sts.CredentialPolicyStatement{
				{
					Action: []string{
						"name/cos:PutObject",
					},
					Effect: "allow",
					Resource: []string{
						fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s",
							c.Config.TenCosConfig.CosRegion, c.Config.TenCosConfig.SubAppId, c.Config.TenCosConfig.CosBucket, key),
					},
					Condition: condition,
				},
			},
		},
	}

	// 获取临时凭证
	res, err := stsClient.GetCredential(opt)
	if err != nil {
		return DirectUploadData{}, fmt.Errorf("获取临时凭证失败: %w", err)
	}

	// 生成COS主机地址
	host := fmt.Sprintf("%s.cos.%s.myqcloud.com", c.Config.TenCosConfig.CosBucket, c.Config.TenCosConfig.CosRegion)

	// 创建COS客户端用于生成签名
	u, _ := url.Parse("https://" + host)
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{})

	ctx := context.Background()
	opt2 := &cos.PresignedURLOptions{
		Query:  &url.Values{},
		Header: &http.Header{},
	}

	// 添加安全令牌到查询参数
	opt2.Query.Add("x-cos-security-token", res.Credentials.SessionToken)

	// 提取Authorization头
	signature := client.Object.GetSignature(ctx, http.MethodPut, key, res.Credentials.TmpSecretID, res.Credentials.TmpSecretKey, time.Hour, opt2, true)

	return DirectUploadData{
		Url:           fmt.Sprintf("https://%s/%s?x-cos-security-token=%s", host, key, res.Credentials.SessionToken),
		CosHost:       host,
		CosKey:        key,
		CosUrl:        fmt.Sprintf("https://%s/%s", host, key),
		Authorization: signature,
		SecurityToken: res.Credentials.SessionToken,
	}, nil
}

// GenerateFileUrl 生成文件访问URL
func (c *CosClient) GenerateFileUrl(key string) string {
	if strings.HasPrefix(key, "/") {
		key = strings.TrimPrefix(key, "/")
	}
	return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s",
		c.Config.TenCosConfig.CosBucket, c.Config.TenCosConfig.CosRegion, key)
}

// GenerateKey 生成COS对象键（公开方法）
func (c *CosClient) GenerateKey(ext string) string {
	return generateCosKey(ext)
}

// UploadAudioFromURL 从URL上传音频
func (c *CosClient) UploadAudioFromURL(audioURL, keyName string) (string, error) {

	// 发起 HTTP 请求下载音频文件
	resp, err := http.Get(audioURL)
	if err != nil {
		return keyName, err
	}
	defer resp.Body.Close()

	ctx := context.Background()
	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return keyName, fmt.Errorf("failed to download audio: %s", resp.Status)
	}

	// 上传音频文件到 COS
	_, err = c.Client.Object.Put(ctx, keyName, resp.Body, nil)
	if err != nil {
		return keyName, err
	}

	fmt.Printf("音频文件已成功上传到 COS，新名称为: %s\n", keyName)
	return keyName, err
}

// UploadSrtToCOS 上传字幕到腾讯云对象存储
func (c *CosClient) UploadSrtToCOS(filePath, keyName string) (string, error) {

	if keyName == "" {
		keyName = c.ReplacePathPrefix(filePath)
	}

	if filePath == "" || keyName == "" {
		return keyName, fmt.Errorf("filePath and keyName cannot be empty")
	}

	// 打开音频文件
	f, err := os.Open(filePath)
	if err != nil {
		return keyName, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer f.Close()

	// 设置音频文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "audio/plain", // 根据实际字幕文件类型调整
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 使用注入的客户端上传文件
	_, err = c.Client.Object.Put(context.Background(), keyName, f, opt)
	if err != nil {
		return keyName, fmt.Errorf("failed to upload audio file: %w", err)
	}

	fmt.Println("Audio file uploaded successfully")
	return keyName, nil
}

// DownloadVideo 从 COS 下载视频到本地
func (c *CosClient) DownloadVideo(keyName, localPath string) (string, error) {
	// 构建完整的 COS 对象键
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 创建本地文件
	outFile, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer outFile.Close()

	// 从 COS 下载文件
	resp, err := c.Client.Object.Get(context.Background(), keyName, nil)
	if err != nil {
		return "", fmt.Errorf("从COS获取文件失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	// 将响应内容写入本地文件
	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	// 检查写入的字节数
	if written == 0 {
		return "", fmt.Errorf("下载的文件大小为0字节")
	}

	fmt.Printf("下载完成，文件大小: %d 字节\n", written)
	return keyName, nil
}

// UploadAudioToCOS 上传音频到腾讯云对象存储
func (c *CosClient) UploadAudioToCOS(filePath, keyName string) (string, error) {

	if keyName == "" {
		keyName = c.ReplacePathPrefix(filePath)
	}

	if filePath == "" || keyName == "" {
		return keyName, fmt.Errorf("filePath and keyName cannot be empty")
	}

	// 打开音频文件
	f, err := os.Open(filePath)
	if err != nil {
		return keyName, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer f.Close()

	// 设置音频文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "audio/mp3", // 根据实际音频文件类型调整
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 使用注入的客户端上传文件
	_, err = c.Client.Object.Put(context.Background(), keyName, f, opt)
	if err != nil {
		return keyName, fmt.Errorf("failed to upload audio file: %w", err)
	}

	fmt.Println("Audio file uploaded successfully")
	return keyName, nil
}

func (c *CosClient) UploadM3u8ToCOS(filePath, keyName, contentType string) (string, error) {
	if keyName == "" {
		keyName = c.ReplacePathPrefix(filePath)
	}

	fmt.Println("keyName - " + keyName)
	if filePath == "" || keyName == "" {
		return keyName, fmt.Errorf("filePath and keyName cannot be empty")
	}

	// 打开音频文件
	f, err := os.Open(filePath)
	if err != nil {
		return keyName, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer f.Close()

	//video/mp2t
	//audio/mpegurl
	// 设置音频文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: contentType, // 针对 MP4 文件设置
			//ContentType: "audio/mpegurl", // 针对 MP4 文件设置
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 使用注入的客户端上传文件
	_, err = c.Client.Object.Put(context.Background(), keyName, f, opt)
	if err != nil {
		return keyName, fmt.Errorf("上传 MP4 文件失败: %w", err)
	}

	fmt.Println("Audio file uploaded successfully")
	return keyName, nil
}

func (c *CosClient) UploadVideoToCOS(filePath, keyName string) (string, error) {
	if keyName == "" {
		keyName = c.ReplacePathPrefix(filePath)
	}

	fmt.Println("keyName - " + keyName)
	if filePath == "" || keyName == "" {
		return keyName, fmt.Errorf("filePath and keyName cannot be empty")
	}

	// 打开音频文件
	f, err := os.Open(filePath)
	if err != nil {
		return keyName, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer f.Close()

	// 设置音频文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "video/mp4", // 针对 MP4 文件设置
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 使用注入的客户端上传文件
	_, err = c.Client.Object.Put(context.Background(), keyName, f, opt)
	if err != nil {
		return keyName, fmt.Errorf("上传 MP4 文件失败: %w", err)
	}

	fmt.Println("Audio file uploaded successfully")
	return keyName, nil
}

// UploadVideoFromReader 从 io.Reader 上传视频文件到 COS，返回 key 和完整的访问 URL
func (c *CosClient) UploadVideoFromReader(reader io.Reader, fileName string) (string, string, error) {
	// 生成唯一的 key
	keyName := c.GenerateVideoKey(fileName)
	
	fmt.Printf("Uploading video to COS: key=%s, filename=%s\n", keyName, fileName)

	// 设置视频文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "video/mp4", // 默认设置为 MP4，可根据文件扩展名调整
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 上传文件
	_, err := c.Client.Object.Put(context.Background(), keyName, reader, opt)
	if err != nil {
		return "", "", fmt.Errorf("failed to upload video to COS: %w", err)
	}

	// 生成访问 URL
	videoURL := c.GenerateURL(keyName)

	fmt.Printf("Video uploaded to COS successfully: key=%s, url=%s\n", keyName, videoURL)
	return keyName, videoURL, nil
}

func (c *CosClient) UploadImageToCOS(filePath, keyName string) (string, error) {
	if keyName == "" {
		keyName = c.ReplacePathPrefix(filePath)
	}

	if filePath == "" || keyName == "" {
		return "", fmt.Errorf("filePath and keyName cannot be empty")
	}

	fmt.Println(keyName)
	// 打开图片文件
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open image file: %w", err)
	}
	defer f.Close()

	// 设置图片文件的 Content-Type
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "image/jpeg", // 针对 JPEG 文件设置，可根据实际情况修改
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: "public-read",
		},
	}

	// 使用注入的客户端上传文件
	_, err = c.Client.Object.Put(context.Background(), keyName, f, opt)
	if err != nil {
		return "", fmt.Errorf("上传图片文件失败: %w", err)
	}

	fmt.Println("Image file uploaded successfully")
	return keyName, nil
}

// deleteDirectory 递归删除指定目录下的所有文件
func (c *CosClient) DeleteDirectory(directory string) error {

	if strings.HasPrefix(directory, "/") {
		directory = strings.TrimPrefix(directory, "/")
	}
	// 确保目录以斜杠结尾
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}

	fmt.Println("删除目录 === ", directory)
	ctx := context.Background()
	var marker string
	var totalDeleted int

	for {
		// 列出目录下的文件（每次最多1000个）
		opt := &cos.BucketGetOptions{
			Prefix:  directory,
			Marker:  marker,
			MaxKeys: 1000,
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return fmt.Errorf("列出文件失败: %v", err)
		}

		// 如果没有文件，退出循环
		if len(resp.Contents) == 0 {
			break
		}

		// 准备要删除的对象列表
		var objectsToDelete []cos.Object
		for _, obj := range resp.Contents {
			objectsToDelete = append(objectsToDelete, cos.Object{Key: obj.Key})
		}

		// 批量删除文件
		deleteOpt := &cos.ObjectDeleteMultiOptions{
			Objects: objectsToDelete,
			Quiet:   false,
		}

		deleteResp, _, err := c.Client.Object.DeleteMulti(ctx, deleteOpt)
		if err != nil {
			return fmt.Errorf("删除文件失败: %v", err)
		}

		// 输出删除结果
		for _, deleted := range deleteResp.DeletedObjects {
			fmt.Printf("已删除: %s\n", deleted.Key)
			totalDeleted++
		}

		for _, errObj := range deleteResp.Errors {
			fmt.Printf("删除失败 - 文件: %s, 错误: %s\n", errObj.Key, errObj.Message)
		}

		// 如果还有更多文件，继续处理
		if resp.IsTruncated {
			marker = resp.NextMarker
		} else {
			break
		}
	}

	// 尝试删除目录本身（如果它是一个对象）
	_, err := c.Client.Object.Delete(ctx, directory)
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		fmt.Printf("警告: 删除目录对象 %s 失败: %v\n", directory, err)
	}

	fmt.Printf("目录 %s 删除完成，共删除 %d 个文件\n", directory, totalDeleted)
	return nil
}

// GeneratePresignedURL 生成预签名URL
func (c *CosClient) GeneratePresignedURL(keyName string) string {

	if keyName == "" {
		return ""
	}
	// 设置过期时间
	expiration := 1 * time.Hour

	// 使用注入的客户端生成预签名URL
	presignedURL, err := c.Client.Object.GetPresignedURL(
		context.Background(),
		http.MethodGet,
		keyName,
		c.Config.TenCosConfig.CosSecretId,
		c.Config.TenCosConfig.CosSecretKey,
		expiration,
		nil,
	)
	if err != nil {
		return ""
	}

	fmt.Printf("Presigned URL for the file: %s\n", presignedURL.String())
	return presignedURL.String()
}

// FileInfo 文件信息结构体
type FileInfo struct {
	Key  string // 文件路径
	Size int64  // 文件大小(bytes)
	Type string // 文件类型
	//Modified time.Time // 修改时间
}

// DirectoryInfo 目录信息结构体
type DirectoryInfo struct {
	Path      string       // 目录路径
	VideoID   string       // 视频ID(如果有)
	Files     []FileInfo   // 文件列表
	SubDirs   []string     // 子目录列表
	MediaInfo MediaDetails // 媒体文件详情
}

// MediaDetails 媒体文件详情
type MediaDetails struct {
	VideoFile  string // 视频文件路径
	AudioFile  string // 音频文件路径
	ImageFile  string // 图片文件路径
	Subtitle   string // 字幕文件路径
	ZhSubtitle string // 中文字幕文件路径
	M3U8File   string // m3u8文件路径
}

// printStructuredResult 打印结构化结果
func (c *CosClient) printStructuredResult(rootInfo DirectoryInfo) {
	fmt.Println("\n==================== 结构化结果 ====================")
	fmt.Printf("根目录: %s\n", rootInfo.Path)
	fmt.Printf("包含 %d 个一级子目录\n", len(rootInfo.SubDirs))

	for _, firstLevel := range rootInfo.SubDirs {
		fmt.Printf("\n┌── 一级目录: %s\n", firstLevel)

		// 获取一级目录下的文件
		files, err := c.getFiles(context.Background(), firstLevel)
		if err == nil && len(files) > 0 {
			fmt.Println("├── 文件列表:")
			for _, file := range files {
				fmt.Printf("│   ├── %s (%s, %d bytes)\n", file.Key, file.Type, file.Size)
			}
		}

		// 获取二级目录
		secondLevelDirs, err := c.getDirectories(context.Background(), firstLevel)
		if err == nil && len(secondLevelDirs) > 0 {
			fmt.Printf("└── 包含 %d 个二级子目录\n", len(secondLevelDirs))

			for _, secondLevel := range secondLevelDirs {
				videoID := extractVideoId(secondLevel)
				fmt.Printf("    ┌── 二级目录: %s (VideoID: %s)\n", secondLevel, videoID)

				// 获取二级目录下的文件
				files, err := c.getFiles(context.Background(), secondLevel)
				if err == nil && len(files) > 0 {
					fmt.Println("    ├── 文件列表:")
					for _, file := range files {
						fmt.Printf("    │   ├── %s (%s, %d bytes)\n", file.Key, file.Type, file.Size)
					}
				}

				// 检查是否有m3u8目录
				thirdLevelDirs, err := c.getDirectories(context.Background(), secondLevel)
				if err == nil {
					for _, thirdLevel := range thirdLevelDirs {
						if strings.Contains(thirdLevel, "m3u8") {
							fmt.Printf("    └── 发现m3u8目录: %s\n", thirdLevel)
						}
					}
				}
			}
		}
	}
}

// TraverseTwoLevelDirectories 遍历两层目录结构并返回结构化信息
func (c *CosClient) TraverseTwoLevelDirectories(ctx context.Context, rootPrefix string) (DirectoryInfo, error) {
	rootInfo := DirectoryInfo{
		Path: rootPrefix,
	}

	fmt.Printf("开始遍历根目录: %s\n", rootPrefix)

	// 获取第一层目录
	firstLevelDirs, err := c.getDirectories(ctx, rootPrefix)
	if err != nil {
		return rootInfo, fmt.Errorf("获取第一层目录失败: %v", err)
	}

	rootInfo.SubDirs = firstLevelDirs
	fmt.Printf("第一层目录数量: %d\n", len(firstLevelDirs))

	// 遍历每个第一层目录
	for _, firstDir := range firstLevelDirs {
		firstLevelInfo := DirectoryInfo{
			Path: firstDir,
		}

		// 列出第一层目录下的文件
		files, err := c.getFiles(ctx, firstDir)
		if err != nil {
			fmt.Printf("获取第一层目录 %s 下的文件失败: %v\n", firstDir, err)
			continue
		}
		firstLevelInfo.Files = files

		// 获取第二层目录
		secondLevelDirs, err := c.getDirectories(ctx, firstDir)
		if err != nil {
			fmt.Printf("获取第二层目录失败: %v\n", err)
			continue
		}

		firstLevelInfo.SubDirs = secondLevelDirs

		// 遍历每个第二层目录
		for _, secondDir := range secondLevelDirs {
			videoID := extractVideoId(secondDir)
			secondLevelInfo := DirectoryInfo{
				Path:    secondDir,
				VideoID: videoID,
			}

			// 列出第二层目录下的文件
			files, err := c.getFiles(ctx, secondDir)
			if err != nil {
				fmt.Printf("获取第二层目录 %s 下的文件失败: %v\n", secondDir, err)
				continue
			}
			secondLevelInfo.Files = files

			// 分类文件
			for _, file := range files {
				fileName := "/" + file.Key
				switch {
				case strings.HasSuffix(fileName, ".mp4"):
					secondLevelInfo.MediaInfo.VideoFile = fileName
				case strings.HasSuffix(fileName, ".mp3"):
					secondLevelInfo.MediaInfo.AudioFile = fileName
				case strings.HasSuffix(fileName, ".jpg"), strings.HasSuffix(fileName, ".jpeg"), strings.HasSuffix(fileName, ".png"):
					secondLevelInfo.MediaInfo.ImageFile = fileName
				case strings.HasSuffix(fileName, ".srt"):
					if strings.Contains(fileName, "zh") {
						secondLevelInfo.MediaInfo.ZhSubtitle = fileName
					} else {
						secondLevelInfo.MediaInfo.Subtitle = fileName
					}
				}
			}

			// 检查第三层子目录并查找m3u8文件
			thirdLevelDirs, err := c.getDirectories(ctx, secondDir)
			if err != nil {
				fmt.Printf("  获取第三层目录失败: %v\n", err)
			} else {
				for _, thirdDir := range thirdLevelDirs {
					if strings.Contains(thirdDir, "m3u8") {
						secondLevelInfo.MediaInfo.M3U8File = fmt.Sprintf("/%soutput.m3u8", thirdDir)
					}
				}
			}
		}
	}

	return rootInfo, nil
}

// getDirectories 获取指定前缀下的所有目录
func (c *CosClient) getDirectories(ctx context.Context, prefix string) ([]string, error) {
	var directories []string
	var marker string

	for {
		opt := &cos.BucketGetOptions{
			Prefix:    prefix,
			Marker:    marker,
			MaxKeys:   1000,
			Delimiter: "/",
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return nil, err
		}

		directories = append(directories, resp.CommonPrefixes...)

		if !resp.IsTruncated {
			break
		}
		marker = resp.NextMarker
	}

	return directories, nil
}

// getFiles 获取指定前缀下的所有文件
func (c *CosClient) getFiles(ctx context.Context, prefix string) ([]FileInfo, error) {
	var files []FileInfo
	var marker string

	for {
		opt := &cos.BucketGetOptions{
			Prefix:    prefix,
			Marker:    marker,
			MaxKeys:   1000,
			Delimiter: "/",
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, object := range resp.Contents {
			if object.Key != prefix && !isDirectory(object.Key, prefix) {
				fileType := getFileType(object.Key)
				files = append(files, FileInfo{
					Key:  object.Key,
					Size: object.Size,
					Type: fileType,
					//Modified: object.LastModified
				},
				)
			}
		}

		if !resp.IsTruncated {
			break
		}
		marker = resp.NextMarker
	}

	return files, nil
}

// getFileType 根据文件名获取文件类型
func getFileType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".mp4"):
		return "video"
	case strings.HasSuffix(filename, ".mp3"):
		return "audio"
	case strings.HasSuffix(filename, ".jpg"), strings.HasSuffix(filename, ".jpeg"), strings.HasSuffix(filename, ".png"):
		return "image"
	case strings.HasSuffix(filename, ".srt"):
		return "subtitle"
	case strings.HasSuffix(filename, ".m3u8"):
		return "playlist"
	default:
		return "other"
	}
}

// isDirectory 判断是否为目录
func isDirectory(key, prefix string) bool {
	relativePath := key[len(prefix):]
	return len(relativePath) > 0 && relativePath[len(relativePath)-1:] == "/"
}

// extractVideoId 从目录路径中提取 videoId
func extractVideoId(dirPath string) string {
	path := dirPath
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}

	return ""
}

// ReplacePathPrefix 替换路径前缀
func (c *CosClient) ReplacePathPrefix(fullPath string) string {
	if strings.HasPrefix(fullPath, c.Config.FileUpDir) {
		return strings.TrimPrefix(fullPath, c.Config.FileUpDir)
	}
	return fullPath
}

// DownloadDirectoryFromCOS 按照存储目录结构下载文件到本地
func (c *CosClient) DownloadDirectoryFromCOS(remoteDir, localDir string) error {
	// 确保远程目录格式正确
	if strings.HasPrefix(remoteDir, "/") {
		remoteDir = strings.TrimPrefix(remoteDir, "/")
	}
	if !strings.HasSuffix(remoteDir, "/") && remoteDir != "" {
		remoteDir += "/"
	}

	// 确保本地目录存在
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	ctx := context.Background()
	var marker string
	var totalDownloaded int

	fmt.Printf("开始下载目录: %s 到本地: %s\n", remoteDir, localDir)

	for {
		// 列出目录下的文件
		opt := &cos.BucketGetOptions{
			Prefix:  remoteDir,
			Marker:  marker,
			MaxKeys: 1000,
		}

		resp, _, err := c.Client.Bucket.Get(ctx, opt)
		if err != nil {
			return fmt.Errorf("列出文件失败: %w", err)
		}

		// 如果没有文件，退出循环
		if len(resp.Contents) == 0 {
			break
		}

		// 下载每个文件
		for _, obj := range resp.Contents {
			// 跳过目录对象（以/结尾的对象）
			if strings.HasSuffix(obj.Key, "/") {
				continue
			}

			// 构建本地文件路径
			relativePath := obj.Key
			if remoteDir != "" {
				relativePath = strings.TrimPrefix(obj.Key, remoteDir)
			}
			localFilePath := filepath.Join(localDir, relativePath)

			// 创建本地文件的目录
			localFileDir := filepath.Dir(localFilePath)
			if err := os.MkdirAll(localFileDir, 0755); err != nil {
				fmt.Printf("创建本地文件目录失败 %s: %v\n", localFileDir, err)
				continue
			}

			// 下载文件
			if err := c.downloadSingleFile(obj.Key, localFilePath); err != nil {
				fmt.Printf("下载文件失败 %s: %v\n", obj.Key, err)
				continue
			}

			fmt.Printf("已下载: %s -> %s\n", obj.Key, localFilePath)
			totalDownloaded++
		}

		// 如果还有更多文件，继续处理
		if resp.IsTruncated {
			marker = resp.NextMarker
		} else {
			break
		}
	}

	fmt.Printf("目录 %s 下载完成，共下载 %d 个文件到 %s\n", remoteDir, totalDownloaded, localDir)
	return nil
}

// downloadSingleFile 下载单个文件
func (c *CosClient) downloadSingleFile(remoteKey, localPath string) error {
	ctx := context.Background()

	// 获取文件
	resp, err := c.Client.Object.Get(ctx, remoteKey, nil)
	if err != nil {
		return fmt.Errorf("获取远程文件失败: %w", err)
	}
	defer resp.Body.Close()

	// 创建本地文件
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer localFile.Close()

	// 复制文件内容
	_, err = localFile.ReadFrom(resp.Body)
	if err != nil {
		return fmt.Errorf("写入本地文件失败: %w", err)
	}

	return nil
}

// DownloadFileFromCOS 下载单个文件
func (c *CosClient) DownloadFileFromCOS(remoteKey, localPath string) error {
	// 确保本地文件的目录存在
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	return c.downloadSingleFile(remoteKey, localPath)
}

// GenerateVideoKey 生成视频文件的唯一 key
func (c *CosClient) GenerateVideoKey(fileName string) string {
	// 生成基于时间戳的唯一 key
	timestamp := time.Now().Unix()
	randomSuffix := rand.Intn(10000)
	
	// 获取文件扩展名
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = ".mp4" // 默认扩展名
	}
	
	// 生成格式: videos/2024/01/15/video_1642384000_1234.mp4
	key := fmt.Sprintf("videos/%s/video_%d_%d%s", 
		time.Now().Format("2006/01/02"), timestamp, randomSuffix, ext)
	
	return key
}

// GenerateURL 根据 key 生成完整的访问 URL
func (c *CosClient) GenerateURL(key string) string {
	// 获取 COS 配置的域名
	baseURL := c.Client.BaseURL.BucketURL.String()
	
	// 确保 baseURL 以 / 结尾，key 不以 / 开头
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	if strings.HasPrefix(key, "/") {
		key = key[1:]
	}
	
	return baseURL + key
}
