package store

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// NewDatabase 创建数据库连接
func NewDatabase(config *types.AppConfig) (*gorm.DB, error) {
	// GORM配置
	gormConfig := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "tb_", // crypto_wallet prefix
			SingularTable: false,
		},
	}

	// 设置日志级别
	if config.Debug {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	} else {
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	}

	// 根据数据库类型创建连接
	var db *gorm.DB
	var err error

	switch config.Database.Type {
	case "postgres", "postgresql":
		dsn := config.Database.GetDSN()
		db, err = gorm.Open(postgres.Open(dsn), gormConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
	case "mysql":

		dsn := config.Database.GetDSN()
		db, err = gorm.Open(mysql.Open(dsn), gormConfig)

		if err != nil {
			return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s (supported: postgres, mysql)", config.Database.Type)
	}

	// 获取底层的sql.DB对象
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// AutoMigrate 自动迁移数据库表
func AutoMigrate(db *gorm.DB) error {
	// 导入所有模型并执行迁移
	return db.AutoMigrate(
		&model.User{},
		&model.SavedVideo{},
	)
}
