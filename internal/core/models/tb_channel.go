package models

import (
	"time"
)

const TableNameTbChanel = "tb_channel"

// TbChannel mapped from table <tb_channel>
type TbChannel struct {
	ChannelId    string `gorm:"column:channel_id;primaryKey;comment:频道ID" json:"channel_id"`       // 频道ID
	Title        string `gorm:"size:500;column:title;comment:频道标题" json:"title"`                   // 频道标题
	Description  string `gorm:"column:description;comment:频道描述" json:"description"`                // 频道描述
	CustomUrl    string `gorm:"size:500;column:custom_url;comment:自定义URL" json:"custom_url"`       // 自定义URL
	ThumbnailUrl string `gorm:"size:500;column:thumbnail_url;comment:缩略图URL" json:"thumbnail_url"` // 缩略图URL
	UserId       string `gorm:"column:user_id;comment:用户ID" json:"user_id"`                        // 关联的用户ID

	Status     string    `gorm:"size:10;column:status;comment:状态" json:"status"`        // 状态
	CreateBy   string    `gorm:"size:20;column:create_by;comment:创建者" json:"create_by"` // 创建者
	CreateTime time.Time `gorm:"column:create_time;comment:创建时间" json:"create_time"`    // 创建时间
	UpdateBy   string    `gorm:"size:20;column:update_by;comment:更新者" json:"update_by"` // 更新者
	UpdateTime time.Time `gorm:"column:update_time;comment:更新时间" json:"update_time"`    // 更新时间
	Remark     string    `gorm:"size:255;column:remark;comment:备注" json:"remark"`       // 备注
}

// TableName TbChannel's table name
func (*TbChannel) TableName() string {
	return TableNameTbChanel
}
