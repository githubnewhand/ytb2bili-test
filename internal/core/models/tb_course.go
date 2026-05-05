package models

import (
	"gorm.io/gorm"
	"time"
)

// Course represents a course entity
type TbCourse struct {
	gorm.Model
	Title       string     `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Description string     `gorm:"column:description;type:text" json:"description"`
	IsActive    bool       `gorm:"column:is_active;default:true" json:"isActive"`
	ImageURL    string     `gorm:"column:image_url;type:varchar(500)" json:"imageUrl"`
	VideoURL    string     `gorm:"column:video_url;type:varchar(500)" json:"videoUrl"`
	Duration    int        `gorm:"column:duration;default:0" json:"duration"` // in seconds
	Level       string     `gorm:"column:level;type:varchar(50);default:'beginner'" json:"level"`
	Language    string     `gorm:"column:language;type:varchar(10);default:'zh'" json:"language"`
	Rating      float64    `gorm:"column:rating;default:0" json:"rating"`
	RatingCount int        `gorm:"column:rating_count;default:0" json:"ratingCount"`
	CategoryID  string     `gorm:"column:category_id" json:"categoryId"`
	Instructor  string     `gorm:"column:instructor" json:"instructor"`
	PublishedAt *time.Time `gorm:"column:published_at" json:"publishedAt"`
	Videos      []TbVideo  `gorm:"foreignKey:CourseID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"videos"`

	PlaylistID string `gorm:"column:playlist_id;type:varchar(50);default:''" json:"playlistId"`
	UserId     string `gorm:"column:user_id;type:varchar(50);default:''" json:"userId"`
}

// Category represents a course category

func (TbCourse) TableName() string {
	return "tb_course"
}
