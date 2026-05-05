package models

import (
	"time"
)

const TableNameTbOAuthToken = "tb_oauth_token"

// TbOAuthToken OAuth2 令牌表
type TbOAuthToken struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement:true;comment:主键ID" json:"id"`                // 主键ID
	UserID       int64     `gorm:"column:user_id;not null;index;comment:用户ID" json:"user_id"`                     // 用户ID
	Provider     string    `gorm:"column:provider;not null;comment:提供商(youtube,github等)" json:"provider"`         // 提供商
	AccessToken  string    `gorm:"column:access_token;type:text;comment:访问令牌(加密)" json:"access_token,omitempty"` // 访问令牌
	RefreshToken string    `gorm:"column:refresh_token;type:text;comment:刷新令牌(加密)" json:"refresh_token,omitempty"` // 刷新令牌
	TokenType    string    `gorm:"column:token_type;comment:令牌类型" json:"token_type"`                             // 令牌类型
	Expiry       time.Time `gorm:"column:expiry;comment:过期时间" json:"expiry"`                                     // 过期时间
	Scope        string    `gorm:"column:scope;comment:授权范围" json:"scope"`                                       // 授权范围
	CreateTime   time.Time `gorm:"column:create_time;comment:创建时间" json:"create_time"`                           // 创建时间
	UpdateTime   time.Time `gorm:"column:update_time;comment:更新时间" json:"update_time"`                           // 更新时间
}

// TableName TbOAuthToken's table name
func (*TbOAuthToken) TableName() string {
	return TableNameTbOAuthToken
}
