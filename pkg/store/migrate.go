package store

import (
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"gorm.io/gorm"
)

// MigrateDatabase 自动迁移数据库表
func MigrateDatabase(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.SavedVideo{},
		&model.TaskStep{},
		&model.AccountBinding{},
	)
}
