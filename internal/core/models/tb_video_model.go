package models

import "time"

const TableNameTbVideoModel = "tb_video_model"

type TbVideoModel struct {
	Id       string `gorm:"column:id;primaryKey;comment:参数主键" json:"id"` // 参数主键
	Title    string `gorm:"size:500;column:title;comment:状态" json:"title"`
	VideoUrl string `gorm:"size:255;column:video_url;comment:状态" json:"videoUrl"`
	VideoId  string `gorm:"size:255;column:video_id;comment:视频id" json:"videoId"`           // 参数主键
	MediaUrl string `gorm:"size:255;column:media_url;comment:视频 media_url" json:"mediaUrl"` // 参数主键

	VodFileId string `gorm:"size:255;column:vod_file_id;comment:点播vod_file_id" json:"vod_file_id"` // 参数主键
	ChannelId string `gorm:"size:64;column:channel_id;comment:频道ID" json:"channel_id"`             // 频道ID
	Status    string `gorm:"size:10;column:status;comment:状态" json:"status"`                       // 状态
	ImgURL    string `gorm:"size:255;column:img_url;comment:图片" json:"img_url"`

	CreateBy   string    `gorm:"size:32;column:create_by;comment:创建者" json:"create_by"` // 创建者
	CreateTime time.Time `gorm:"column:create_time;comment:创建时间" json:"create_time"`    // 创建时间
	UpdateBy   string    `gorm:"size:32;column:update_by;comment:更新者" json:"update_by"` // 更新者
	UpdateTime time.Time `gorm:"column:update_time;comment:更新时间" json:"update_time"`    // 更新时间
	Remark     string    `gorm:"size:255;column:remark;comment:备注" json:"remark"`
}

func (*TbVideoModel) TableName() string {
	return TableNameTbVideoModel
}
