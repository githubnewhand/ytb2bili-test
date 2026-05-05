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
  created_at: string;
  updated_at: string;
  subtitles?: Subtitle[];
  upload_result?: UploadResult;
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
  generated_title?: string;
  generated_description?: string;
  generated_tags?: string;
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

export type VideoStatus = '001' | '002' | '200' | '201' | '299' | '300' | '301' | '399' | '400' | '999';

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
