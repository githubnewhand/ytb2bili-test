package models

import "time"

const TableNameTbUpload = "tb_upload"

type TbUpload struct {
	Id       string `gorm:"column:id;primaryKey;comment:主键" json:"id"` // 参数主键
	FileName string `gorm:"size:500;column:file_name;comment:文件名" json:"file_name"`
	FileUrl  string `gorm:"size:255;column:file_url;comment:文件地址" json:"file_url"`
	UserId   string `gorm:"size:255;column:user_id;comment:文件id" json:"user_id"`   // 参数主键
	VideoId  string `gorm:"size:255;column:video_id;comment:文件id" json:"video_id"` // 参数主键

	HashKey  string `gorm:"size:255;column:hash_key;comment:文件 hash_key" json:"hash_key"`    // 参数主键
	MediaUrl string `gorm:"size:255;column:media_url;comment:文件 media_url" json:"media_url"` // 参数主键

	Status     string    `gorm:"size:10;column:status;comment:状态" json:"status"`        // 状态
	CreateBy   string    `gorm:"size:32;column:create_by;comment:创建者" json:"create_by"` // 创建者
	CreateTime time.Time `gorm:"column:create_time;comment:创建时间" json:"create_time"`    // 创建时间
	UpdateBy   string    `gorm:"size:32;column:update_by;comment:更新者" json:"update_by"` // 更新者
	UpdateTime time.Time `gorm:"column:update_time;comment:更新时间" json:"update_time"`    // 更新时间
}

func (*TbUpload) TableName() string {
	return TableNameTbUpload
}
