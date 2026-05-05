package models

import "time"

type TbVideo struct {
	Id      uint   `gorm:"primarykey;column:id" json:"id"`
	URL     string `gorm:"column:url;type:varchar(255);not null" json:"url"`
	Title   string `gorm:"column:title;type:text;not null" json:"title"`
	VideoId string `gorm:"column:video_id;type:varchar(255);not null" json:"videoId"`
	Status  string `gorm:"column:status;type:varchar(10);not null" json:"status"`
	VcosKey string `gorm:"column:vcos_key;type:varchar(255)" json:"vcosKey"`
	AcosKey string `gorm:"column:acos_key;type:varchar(255)" json:"acosKey"`
	M3u8    string `gorm:"column:m3u8;type:varchar(255)" json:"m3u8"`
	CosKey  string `gorm:"column:cos_key;type:varchar(255)" json:"cosKey"`
	ImgURL  string `gorm:"column:img_url;comment:图片" json:"imgUrl"` // 图片

	Duration      float64 `gorm:"column:duration;type:float;default:0" json:"duration"`
	OperationType int     `gorm:"column:operation_type;type:int" json:"operationType"`
	Description   string  `gorm:"column:description;type:text" json:"description"`
	SortOrder     int     `gorm:"not null;default:0" json:"sorOrder"` // 视频排序
	BiliTid       int     `gorm:"column:bili_tid;type:int;default:0" json:"biliTid"`// B站分区ID

	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	PlaylistId string    `gorm:"column:playlist_id;type:varchar(255)" json:"playlistId"`
	CourseID   uint      `gorm:"not null;index" json:"courseId"`
	Course     TbCourse  `gorm:"foreignKey:CourseID" json:"course"`
}

// TableName TbChannel's table name
func (*TbVideo) TableName() string {
	return "tb_video"
}
