export interface User {
  id: string;
  name: string;
  mid: string;
  avatar?: string;
  isLoggedIn: boolean;
}

export interface Video {
  id: number;
  video_id: string;
  title: string;
  url: string;
  status: VideoStatus;
  chapters?: string;
  chapters_status?: string;
  chapters_message?: string;
  chapters_extracted?: boolean;
  chapters_count?: number;
  created_at: string;
  updated_at: string;
  subtitles?: Subtitle[];
  upload_result?: UploadResult;
  bili_bvid?: string;
  bili_aid?: number;
  publish_audience?: PublishAudience;
  audience_selected_at?: string;
  upower_preview_seconds?: number;
  record_type?: 'source' | 'compilation';
  workflow_state?: string;
  media_duration_ms?: number;
  ready_at?: string;
  scheduled_upload_at?: string;
  upload_policy?: 'scheduled' | 'manual' | 'immediate';
  rights_verified?: boolean;
}

export interface TaskStep {
  id: number;
  video_id: string;
  step_name: string;
  step_order: number;
  status: TaskStepStatus;
  start_time?: string;
  end_time?: string;
  duration?: number; // 持续时间，毫秒
  progress_percent?: number;
  progress_message?: string;
  error_msg?: string;
  result_data?: any;
  can_retry: boolean;
  created_at: string;
  updated_at: string;
}

export interface TaskProgress {
  total_steps: number;
  completed_steps: number;
  failed_steps: number;
  progress_percent?: number;
  progress_percentage?: number;
  current_step?: string;
  current_step_progress?: number;
  current_step_message?: string;
  is_running?: boolean;
}

export interface VideoDetail {
  id: number;
  video_id: string;
  title: string;
  url: string;
  status: VideoStatus;
  created_at: string;
  updated_at: string;
  chapters?: string;
  chapters_status?: string;
  chapters_message?: string;
  chapters_extracted?: boolean;
  chapters_count?: number;
  generated_title?: string;
  generated_description?: string;
  generated_desc?: string;
  generated_tags?: string;
  bili_bvid?: string;
  bili_aid?: number;
  publish_audience?: PublishAudience;
  audience_selected_at?: string;
  upower_preview_seconds?: number;
  record_type?: 'source' | 'compilation';
  workflow_state?: string;
  media_duration_ms?: number;
  ready_at?: string;
  scheduled_upload_at?: string;
  upload_policy?: 'scheduled' | 'manual' | 'immediate';
  rights_verified?: boolean;
  cover_image?: string;
  task_steps: TaskStep[];
  progress: TaskProgress;
  files: VideoFile[];
}

export interface VideoFile {
  name: string;
  path: string;
  size: number;
  type: 'video' | 'subtitle' | 'cover' | 'metadata' | 'other';
  created_at: string;
}

export type TaskStepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';

export type VideoStatus = '001' | '002' | '100' | '101' | '110' | '200' | '201' | '299' | '300' | '301' | '399' | '400' | '999';

export type PublishAudience = '' | 'free' | 'charge_30' | 'charge_50';

export interface Subtitle {
  id?: number;
  text: string;
  duration: number;
  offset: number;
  lang: string;
}

export interface UploadResult {
  aid?: number;
  bvid?: string;
  video_url?: string;
  upload_time?: string;
}

export interface QRCodeResponse {
  qr_code_url: string;
  auth_code: string;
}

export interface LoginStatus {
  is_logged_in: boolean;
  user?: User;
  message?: string;
}

export interface ApiResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

export interface VideoSubmissionRequest {
  url: string;
  title: string;
  subtitles: Subtitle[];
}

export interface UploadValidation {
  is_valid: boolean;
  errors: string[];
  warnings: string[];
  video_info?: {
    title: string;
    duration: string;
    resolution: string;
    size: string;
  };
}

// 状态映射
export const VIDEO_STATUS_MAP = {
  '001': { label: '待处理', className: 'status-pending' },
  '002': { label: '处理中', className: 'status-processing' },
  '200': { label: '已完成', className: 'status-completed' },
  '999': { label: '失败', className: 'status-failed' },
} as const;

export const TASK_STEP_STATUS_MAP = {
  'pending': { label: '等待中', className: 'bg-gray-100 text-gray-800', color: 'gray' },
  'running': { label: '运行中', className: 'bg-blue-100 text-blue-800', color: 'blue' },
  'completed': { label: '已完成', className: 'bg-green-100 text-green-800', color: 'green' },
  'failed': { label: '失败', className: 'bg-red-100 text-red-800', color: 'red' },
  'skipped': { label: '已跳过', className: 'bg-yellow-100 text-yellow-800', color: 'yellow' },
} as const;

export const TASK_STEP_NAMES = {
  'download_video': '下载视频',
  'generate_subtitles': '生成字幕',
  'translate_subtitles': '翻译字幕',
  'generate_metadata': '生成元数据',
  'upload_to_bilibili': '上传到B站',
  'upload_subtitles': '上传字幕',
} as const;

export type ChargeTier = 30 | 50;
export type ChargePoolState = 'available' | 'reserved' | 'consumed' | 'error';
export type CompilationState =
  | 'draft'
  | 'queued'
  | 'merging'
  | 'processing'
  | 'ready'
  | 'upload_queued'
  | 'uploading'
  | 'uploaded'
  | 'merge_failed'
  | 'processing_failed'
  | 'upload_failed'
  | 'cancelled';

export interface ChargePoolSummary {
  tier: ChargeTier;
  available: number;
  reserved: number;
  consumed: number;
  total: number;
}

export interface ChargePoolItem {
  id: number;
  saved_video_id: number;
  tier: ChargeTier;
  state: ChargePoolState;
  reserved_batch_id?: number;
  saved_video?: Video;
  created_at: string;
  updated_at: string;
}

export interface CompilationItem {
  id: number;
  batch_id: number;
  source_saved_video_id: number;
  position: number;
  source_duration_ms: number;
  timeline_start_ms: number;
  timeline_end_ms: number;
  source_path_snapshot: string;
  source_video?: Video;
}

export interface CompilationBatch {
  id: number;
  batch_key: string;
  tier: ChargeTier;
  state: CompilationState;
  target_count: number;
  actual_count: number;
  random_seed: number;
  output_saved_video_id?: number;
  preview_seconds: number;
  total_duration_ms: number;
  output_path?: string;
  last_error?: string;
  retry_count: number;
  upload_policy: 'manual' | 'immediate';
  started_at?: string;
  completed_at?: string;
  items: CompilationItem[];
  output_saved_video?: Video;
  created_at: string;
  updated_at: string;
}
