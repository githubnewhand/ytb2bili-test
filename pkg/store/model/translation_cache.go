package model

import "time"

// TranslationCache stores validated, reusable subtitle translations.
// CacheKey includes every setting that can affect the result, so prompt or
// model changes never silently reuse an incompatible translation.
type TranslationCache struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	CacheKey       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"cache_key"`
	SourceText     string    `gorm:"type:text;not null" json:"source_text"`
	Translation    string    `gorm:"type:text;not null" json:"translation"`
	SourceLanguage string    `gorm:"type:varchar(16);not null" json:"source_language"`
	TargetLanguage string    `gorm:"type:varchar(16);not null" json:"target_language"`
	Provider       string    `gorm:"type:varchar(32);not null" json:"provider"`
	Model          string    `gorm:"type:varchar(128);not null" json:"model"`
	PromptVersion  string    `gorm:"type:varchar(64);not null" json:"prompt_version"`
	HitCount       int64     `gorm:"not null;default:0" json:"hit_count"`
	LastUsedAt     time.Time `gorm:"index;not null" json:"last_used_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (TranslationCache) TableName() string {
	return "tb_translation_caches"
}
