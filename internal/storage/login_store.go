package storage

import (
	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LoginStore 登录信息存储管理器
type LoginStore struct {
	storePath string
	mu        sync.RWMutex
}

// StoredLoginInfo 存储的登录信息（包含保存时间）
type StoredLoginInfo struct {
	LoginInfo *bilibili.LoginInfo `json:"login_info"`
	UserInfo  *UserBasicInfo      `json:"user_info,omitempty"`  // 用户基本信息
	SavedAt   time.Time           `json:"saved_at"`
	ExpiresAt time.Time           `json:"expires_at"`
	UserMid   int64               `json:"user_mid"`
}

// UserBasicInfo 用户基本信息（简化版，用于本地存储）
type UserBasicInfo struct {
	Mid      int64  `json:"mid"`
	Name     string `json:"name"`
	Uname    string `json:"uname"`    // 用户名
	Sex      string `json:"sex"`
	Face     string `json:"face"`     // 头像URL
	Sign     string `json:"sign"`     // 个人签名
	Level    int    `json:"level"`    // 等级
	Birthday string `json:"birthday"` // 生日
	Rank     int    `json:"rank"`     // 排名
	// MyInfo API 扩展字段
	Coins     int  `json:"coins,omitempty"`     // 硬币数
	Fans      int  `json:"fans,omitempty"`      // 粉丝数
	Attention int  `json:"attention,omitempty"` // 关注数
	Friend    int  `json:"friend,omitempty"`    // 朋友数
	NickFree  bool `json:"nick_free,omitempty"` // 昵称可修改
	Silence   int  `json:"silence,omitempty"`   // 禁言状态
}

var (
	defaultStore *LoginStore
	once         sync.Once
)

// GetDefaultStore 获取默认的登录信息存储器
func GetDefaultStore() *LoginStore {
	once.Do(func() {
		// 默认存储在用户主目录下的 .bili_up 文件夹
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Printf("Warning: Failed to get user home directory: %v", err)
			homeDir = "."
		}

		storePath := filepath.Join(homeDir, ".bili_up", "login.json")
		defaultStore = NewLoginStore(storePath)

		// 确保存储目录存在
		if err := os.MkdirAll(filepath.Dir(storePath), 0700); err != nil {
			log.Printf("Warning: Failed to create storage directory: %v", err)
		}
	})
	return defaultStore
}

// NewLoginStore 创建新的登录信息存储器
func NewLoginStore(storePath string) *LoginStore {
	return &LoginStore{
		storePath: storePath,
	}
}

// Save 保存登录信息到本地
func (s *LoginStore) Save(loginInfo *bilibili.LoginInfo) error {
	return s.SaveWithUserInfo(loginInfo, nil)
}

// SaveWithUserInfo 保存登录信息和用户信息到本地
func (s *LoginStore) SaveWithUserInfo(loginInfo *bilibili.LoginInfo, userInfo *UserBasicInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if loginInfo == nil {
		return fmt.Errorf("login info is nil")
	}

	// 计算过期时间（access_token 的有效期）
	expiresIn := time.Duration(loginInfo.TokenInfo.ExpiresIn) * time.Second
	if expiresIn == 0 {
		// 默认有效期 30 天
		expiresIn = 30 * 24 * time.Hour
	}

	stored := &StoredLoginInfo{
		LoginInfo: loginInfo,
		UserInfo:  userInfo,
		SavedAt:   time.Now(),
		ExpiresAt: time.Now().Add(expiresIn),
		UserMid:   loginInfo.TokenInfo.Mid,
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal login info: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// 写入文件（使用临时文件 + 重命名确保原子性）
	tempPath := s.storePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write login info: %w", err)
	}

	if err := os.Rename(tempPath, s.storePath); err != nil {
		os.Remove(tempPath) // 清理临时文件
		return fmt.Errorf("failed to save login info: %w", err)
	}

	if userInfo != nil {
		log.Printf("Login info and user info saved successfully (Mid: %d, Name: %s, ExpiresAt: %s)", 
			stored.UserMid, userInfo.Name, stored.ExpiresAt.Format(time.RFC3339))
	} else {
		log.Printf("Login info saved successfully (Mid: %d, ExpiresAt: %s)", 
			stored.UserMid, stored.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

// Load 从本地加载登录信息
func (s *LoginStore) Load() (*bilibili.LoginInfo, error) {
	loginInfo, _, err := s.LoadWithUserInfo()
	if err != nil {
		return nil, err
	}
	return loginInfo, nil
}

// LoadWithUserInfo 从本地加载登录信息和用户信息
func (s *LoginStore) LoadWithUserInfo() (*bilibili.LoginInfo, *UserBasicInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 检查文件是否存在
	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("no saved login info found")
	}

	// 读取文件
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read login info: %w", err)
	}

	// 反序列化
	var stored StoredLoginInfo
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal login info: %w", err)
	}

	// 检查是否过期
	if time.Now().After(stored.ExpiresAt) {
		log.Printf("Login info expired (ExpiresAt: %s)", stored.ExpiresAt.Format(time.RFC3339))
		return nil, nil, fmt.Errorf("login info expired")
	}

	return stored.LoginInfo, stored.UserInfo, nil
}

// loadStoredInfo 私有方法：从本地加载完整的存储信息
func (s *LoginStore) loadStoredInfo() (*StoredLoginInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 检查文件是否存在
	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no saved login info found")
	}

	// 读取文件
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read login info: %w", err)
	}

	// 反序列化
	var stored StoredLoginInfo
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("failed to unmarshal login info: %w", err)
	}

	// 检查是否过期
	if time.Now().After(stored.ExpiresAt) {
		log.Printf("Login info expired (ExpiresAt: %s)", stored.ExpiresAt.Format(time.RFC3339))
		return nil, fmt.Errorf("login info expired")
	}

	if stored.UserInfo != nil {
		log.Printf("Login info and user info loaded successfully (Mid: %d, Name: %s, ValidUntil: %s)", 
			stored.UserMid, stored.UserInfo.Name, stored.ExpiresAt.Format(time.RFC3339))
	} else {
		log.Printf("Login info loaded successfully (Mid: %d, ValidUntil: %s)", 
			stored.UserMid, stored.ExpiresAt.Format(time.RFC3339))
	}
	
	return &stored, nil
}

// GetUserInfo 获取保存的用户信息
func (s *LoginStore) GetUserInfo() (*UserBasicInfo, error) {
	_, userInfo, err := s.LoadWithUserInfo()
	if err != nil {
		return nil, err
	}
	return userInfo, nil
}

// Delete 删除保存的登录信息
func (s *LoginStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.storePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete login info: %w", err)
	}

	log.Println("Login info deleted successfully")
	return nil
}

// IsValid 检查保存的登录信息是否有效
func (s *LoginStore) IsValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 检查文件是否存在
	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		return false
	}

	// 读取文件
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return false
	}

	// 反序列化
	var stored StoredLoginInfo
	if err := json.Unmarshal(data, &stored); err != nil {
		return false
	}

	// 检查是否过期
	return time.Now().Before(stored.ExpiresAt)
}

// GetStorePath 获取存储路径
func (s *LoginStore) GetStorePath() string {
	return s.storePath
}

// ConvertMyInfoToUserInfo 将MyInfo响应转换为存储格式
func ConvertMyInfoToUserInfo(myInfo *bilibili.MyInfoResponse) *UserBasicInfo {
	if myInfo == nil {
		return nil
	}
	
	return &UserBasicInfo{
		Mid:       myInfo.Mid,
		Name:      myInfo.Uname,
		Uname:     myInfo.Uname,
		Sex:       myInfo.Sex,
		Face:      myInfo.Face,
		Sign:      myInfo.Sign,
		Level:     myInfo.Level,
		Birthday:  myInfo.GetBirthdayString(),
		Rank:      parseRankString(myInfo.GetRankString()), // 将字符串转换为整数
		Coins:     myInfo.GetCoins(),
		Fans:      myInfo.Fans,
		Attention: myInfo.Attention,
		Friend:    myInfo.Friend,
		NickFree:  false, // SDK中没有这个字段
		Silence:   myInfo.Silence,
	}
}

// ConvertBasicInfoToUserInfo 将基本用户信息转换为存储格式
func ConvertBasicInfoToUserInfo(basicInfo *bilibili.UserBasicInfo) *UserBasicInfo {
	if basicInfo == nil {
		return nil
	}
	
	return &UserBasicInfo{
		Mid:      basicInfo.Mid,
		Name:     basicInfo.Name,
		Uname:    basicInfo.Name,
		Sex:      basicInfo.Sex,
		Face:     basicInfo.Face,
		Sign:     basicInfo.Sign,
		Level:    basicInfo.Level,
		Birthday: basicInfo.Birthday,
		Rank:     basicInfo.Rank,
	}
}

// parseRankString 解析等级称谓字符串为整数（如果可能）
func parseRankString(rankStr string) int {
	// 简单的映射，可以根据需要扩展
	switch rankStr {
	case "见习":
		return 1
	case "正式会员":
		return 2
	case "高级会员":
		return 3
	case "VIP":
		return 4
	case "年度大会员":
		return 5
	default:
		return 0
	}
}
