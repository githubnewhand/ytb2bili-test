package model

import "time"

const (
	RecordTypeSource      = "source"
	RecordTypeCompilation = "compilation"

	WorkflowStateDownloading            = "downloading"
	WorkflowStateAwaitingClassification = "awaiting_classification"
	WorkflowStatePreparing              = "preparing"
	WorkflowStatePoolAvailable          = "pool_available"
	WorkflowStatePoolReserved           = "pool_reserved"
	WorkflowStateReady                  = "ready"
	WorkflowStateUploading              = "uploading"
	WorkflowStateUploaded               = "uploaded"
	WorkflowStateFailed                 = "failed"

	UploadPolicyScheduled = "scheduled"
	UploadPolicyManual    = "manual"
	UploadPolicyImmediate = "immediate"

	ChargePoolStateAvailable = "available"
	ChargePoolStateReserved  = "reserved"
	ChargePoolStateConsumed  = "consumed"
	ChargePoolStateError     = "error"

	CompilationStateDraft         = "draft"
	CompilationStateQueued        = "queued"
	CompilationStateMerging       = "merging"
	CompilationStateProcessing    = "processing"
	CompilationStateReady         = "ready"
	CompilationStateUploadQueued  = "upload_queued"
	CompilationStateUploading     = "uploading"
	CompilationStateUploaded      = "uploaded"
	CompilationStateMergeFailed   = "merge_failed"
	CompilationStateProcessFailed = "processing_failed"
	CompilationStateUploadFailed  = "upload_failed"
	CompilationStateCancelled     = "cancelled"

	UploadJobStatusQueued    = "queued"
	UploadJobStatusRunning   = "running"
	UploadJobStatusSucceeded = "succeeded"
	UploadJobStatusFailed    = "failed"
	UploadJobStatusUnknown   = "submission_unknown"
	UploadJobStatusCancelled = "cancelled"
)

type ChargePoolItem struct {
	BaseModel
	SavedVideoID    uint       `gorm:"not null;uniqueIndex" json:"saved_video_id"`
	Tier            int        `gorm:"not null;index:idx_charge_pool_pick,priority:1" json:"tier"`
	State           string     `gorm:"type:varchar(20);not null;index:idx_charge_pool_pick,priority:2" json:"state"`
	ReservedBatchID *uint      `gorm:"index" json:"reserved_batch_id,omitempty"`
	ReservedAt      *time.Time `json:"reserved_at,omitempty"`
	ConsumedAt      *time.Time `json:"consumed_at,omitempty"`
	Version         uint       `gorm:"not null;default:1" json:"version"`
	SavedVideo      SavedVideo `gorm:"foreignKey:SavedVideoID" json:"saved_video,omitempty"`
}

func (ChargePoolItem) TableName() string {
	return "tb_charge_pool_items"
}

type CompilationBatch struct {
	BaseModel
	BatchKey           string            `gorm:"type:varchar(64);not null;uniqueIndex" json:"batch_key"`
	Tier               int               `gorm:"not null;index" json:"tier"`
	State              string            `gorm:"type:varchar(30);not null;index" json:"state"`
	TargetCount        int               `gorm:"not null" json:"target_count"`
	ActualCount        int               `gorm:"not null" json:"actual_count"`
	RandomSeed         int64             `gorm:"not null" json:"random_seed"`
	OutputSavedVideoID *uint             `gorm:"index" json:"output_saved_video_id,omitempty"`
	PreviewSeconds     int               `gorm:"not null;default:180" json:"preview_seconds"`
	TotalDurationMS    int64             `gorm:"not null;default:0" json:"total_duration_ms"`
	OutputPath         string            `gorm:"type:text" json:"output_path,omitempty"`
	ManifestJSON       string            `gorm:"type:text" json:"manifest_json,omitempty"`
	LastError          string            `gorm:"type:text" json:"last_error,omitempty"`
	RetryCount         int               `gorm:"not null;default:0" json:"retry_count"`
	UploadPolicy       string            `gorm:"type:varchar(20);not null;default:'immediate'" json:"upload_policy"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
	Items              []CompilationItem `gorm:"foreignKey:BatchID" json:"items,omitempty"`
	OutputSavedVideo   *SavedVideo       `gorm:"foreignKey:OutputSavedVideoID" json:"output_saved_video,omitempty"`
}

func (CompilationBatch) TableName() string {
	return "tb_compilation_batches"
}

type CompilationItem struct {
	BaseModel
	BatchID            uint       `gorm:"not null;uniqueIndex:idx_batch_source;uniqueIndex:idx_batch_position" json:"batch_id"`
	SourceSavedVideoID uint       `gorm:"not null;uniqueIndex:idx_batch_source" json:"source_saved_video_id"`
	Position           int        `gorm:"not null;uniqueIndex:idx_batch_position" json:"position"`
	SourceDurationMS   int64      `gorm:"not null;default:0" json:"source_duration_ms"`
	TimelineStartMS    int64      `gorm:"not null;default:0" json:"timeline_start_ms"`
	TimelineEndMS      int64      `gorm:"not null;default:0" json:"timeline_end_ms"`
	SourcePathSnapshot string     `gorm:"type:text;not null" json:"source_path_snapshot"`
	SourceSavedVideo   SavedVideo `gorm:"foreignKey:SourceSavedVideoID" json:"source_video,omitempty"`
}

func (CompilationItem) TableName() string {
	return "tb_compilation_items"
}

type UploadJob struct {
	BaseModel
	SavedVideoID       uint       `gorm:"not null;index:idx_upload_job_claim,priority:2" json:"saved_video_id"`
	CompilationBatchID *uint      `gorm:"index" json:"compilation_batch_id,omitempty"`
	JobType            string     `gorm:"type:varchar(20);not null" json:"job_type"`
	TriggerType        string     `gorm:"type:varchar(20);not null" json:"trigger_type"`
	ScheduledAt        time.Time  `gorm:"not null;index:idx_upload_job_claim,priority:1" json:"scheduled_at"`
	Status             string     `gorm:"type:varchar(30);not null;index:idx_upload_job_claim,priority:0" json:"status"`
	Attempts           int        `gorm:"not null;default:0" json:"attempts"`
	LeaseUntil         *time.Time `gorm:"index" json:"lease_until,omitempty"`
	LastError          string     `gorm:"type:text" json:"last_error,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
}

func (UploadJob) TableName() string {
	return "tb_upload_jobs"
}
