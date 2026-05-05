import axios from 'axios';
import { getApiBaseUrl } from '@/lib/apiBase';
import type { 
  ApiResponse, 
  Video, 
  VideoDetail,
  TaskStep,
  VideoFile,
  QRCodeResponse, 
  LoginStatus, 
  VideoSubmissionRequest,
  UploadValidation 
} from '@/types';

const API_BASE_URL = getApiBaseUrl();

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    // 可以在这里添加 token 等认证信息
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    return response.data;
  },
  (error) => {
    console.error('API Error:', error);
    return Promise.reject(error);
  }
);

// 认证相关 API
export const authApi = {
  // 获取登录二维码
  getQRCode: (): Promise<ApiResponse<QRCodeResponse>> => {
    return api.get('/auth/qrcode');
  },

  // 检查登录状态
  checkLoginStatus: (authCode: string): Promise<ApiResponse<LoginStatus>> => {
    return api.post('/auth/poll', { auth_code: authCode });
  },

  // 获取当前用户状态
  getUserStatus: (): Promise<ApiResponse<LoginStatus>> => {
    return api.get('/auth/status');
  },

  // 退出登录
  logout: (): Promise<ApiResponse> => {
    return api.post('/auth/logout');
  },
};

// 视频相关 API
export const videoApi = {
  // 获取视频列表
  getVideos: (page = 1, limit = 10): Promise<ApiResponse<{ videos: Video[], total: number }>> => {
    return api.get(`/videos?page=${page}&limit=${limit}`);
  },

  // 获取单个视频详情
  getVideo: (id: number): Promise<ApiResponse<Video>> => {
    return api.get(`/videos/${id}`);
  },

  // 获取视频详细信息（包含任务步骤）
  getVideoDetail: (id: string): Promise<ApiResponse<VideoDetail>> => {
    return api.get(`/videos/${id}`);
  },

  // 获取视频文件列表
  getVideoFiles: (id: string): Promise<ApiResponse<VideoFile[]>> => {
    return api.get(`/videos/${id}/files`);
  },

  // 重试任务步骤
  retryTaskStep: (videoId: string, stepName: string): Promise<ApiResponse> => {
    return api.post(`/videos/${videoId}/steps/${stepName}/retry`);
  },

  // 提交新视频
  submitVideo: (data: VideoSubmissionRequest): Promise<ApiResponse<Video>> => {
    return api.post('/submit', data);
  },

  // 验证视频上传
  validateUpload: (videoId: string): Promise<ApiResponse<UploadValidation>> => {
    return api.post('/upload/validate', { video_id: videoId });
  },

  // 删除视频
  deleteVideo: (id: number): Promise<ApiResponse> => {
    return api.delete(`/videos/${id}`);
  },
};

// 字幕相关 API
export const subtitleApi = {
  // 获取视频字幕
  getSubtitles: (videoId: string): Promise<ApiResponse<any>> => {
    return api.get(`/subtitles/${videoId}`);
  },

  // 更新字幕
  updateSubtitles: (videoId: string, subtitles: any): Promise<ApiResponse> => {
    return api.put(`/subtitles/${videoId}`, { subtitles });
  },
};

export default api;