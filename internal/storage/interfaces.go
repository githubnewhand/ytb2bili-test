package storage

import "github.com/difyz9/bilibili-go-sdk/bilibili"

// LoginStoreInterface 登录信息存储接口
type LoginStoreInterface interface {
	Save(loginInfo *bilibili.LoginInfo) error
	Load() (*bilibili.LoginInfo, error)
	Delete() error
	IsValid() bool
	GetStorePath() string
	
	// 新增用户信息相关方法
	SaveWithUserInfo(loginInfo *bilibili.LoginInfo, userInfo *UserBasicInfo) error
	LoadWithUserInfo() (*bilibili.LoginInfo, *UserBasicInfo, error)
	GetUserInfo() (*UserBasicInfo, error)
}

// 确保LoginStore实现了LoginStoreInterface接口
var _ LoginStoreInterface = (*LoginStore)(nil)