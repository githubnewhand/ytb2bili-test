'use client';

import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, Clock, Play, CheckCircle, Upload, AlertCircle } from 'lucide-react';

interface Video {
  id: number;
  video_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
  task_steps?: TaskStep[];
  progress?: TaskProgress;
  bili_bvid?: string;
}

interface TaskProgress {
  total_steps?: number;
  completed_steps?: number;
  failed_steps?: number;
  current_step?: string;
  current_step_progress?: number;
  current_step_message?: string;
  is_running?: boolean;
  progress_percent?: number;
  progress_percentage?: number;
}

interface TaskStep {
  step_name: string;
  step_order: number;
  status: string;
  start_time: string;
  end_time: string;
  progress_percent?: number;
  progress_message?: string;
  error_msg: string;
  can_retry: boolean;
}

type TabType = 'all' | 'pending' | 'preparing' | 'ready' | 'uploading' | 'completed' | 'failed';

interface TaskQueueStatsProps {
  onVideoSelect?: (videoId: string) => void;
}

interface ProgressInfo {
  totalSteps: number;
  completedSteps: number;
  failedSteps: number;
  percent: number;
  currentStep: string;
  currentStepProgress: number;
  currentStepMessage: string;
  nextStep: string;
  isRunning: boolean;
}

const DEFAULT_STEP_COUNT = 6;

const getFallbackProgress = (status: string): ProgressInfo => {
  const fallbackByStatus: Record<string, Omit<ProgressInfo, 'failedSteps' | 'nextStep' | 'currentStepProgress' | 'currentStepMessage' | 'isRunning'>> = {
    '001': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 0, percent: 0, currentStep: '' },
    '002': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 0, percent: 0, currentStep: '准备任务链' },
    '200': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 4, percent: 67, currentStep: '' },
    '201': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 4, percent: 67, currentStep: '上传到Bilibili' },
    '299': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 4, percent: 67, currentStep: '' },
    '300': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 5, percent: 83, currentStep: '' },
    '301': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 5, percent: 83, currentStep: '上传字幕到Bilibili' },
    '399': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 5, percent: 83, currentStep: '' },
    '400': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 6, percent: 100, currentStep: '' },
    '999': { totalSteps: DEFAULT_STEP_COUNT, completedSteps: 0, percent: 0, currentStep: '' },
  };

  const fallback = fallbackByStatus[status] || fallbackByStatus['001'];
  return {
    ...fallback,
    failedSteps: ['299', '399', '999'].includes(status) ? 1 : 0,
    currentStepProgress: 0,
    currentStepMessage: '',
    nextStep: '',
    isRunning: ['002', '201', '301'].includes(status),
  };
};

const clampPercent = (value: number) => Math.max(0, Math.min(100, Math.round(value)));

const getProgressFromSteps = (steps: TaskStep[]): ProgressInfo => {
  const sortedSteps = [...steps].sort((a, b) => a.step_order - b.step_order);
  const totalSteps = sortedSteps.length;
  const completedSteps = sortedSteps.filter(step => step.status === 'completed').length;
  const failedSteps = sortedSteps.filter(step => step.status === 'failed').length;
  const runningStep = sortedSteps.find(step => step.status === 'running');
  const runningProgress = clampPercent(Number(runningStep?.progress_percent ?? 0));
  const nextStep = sortedSteps.find(step => step.status === 'pending')?.step_name || '';

  return {
    totalSteps,
    completedSteps,
    failedSteps,
    percent: totalSteps > 0 ? clampPercent(((completedSteps * 100) + runningProgress) / totalSteps) : 0,
    currentStep: runningStep?.step_name || '',
    currentStepProgress: runningProgress,
    currentStepMessage: runningStep?.progress_message || '',
    nextStep,
    isRunning: Boolean(runningStep),
  };
};

const getVideoProgress = (video: Video): ProgressInfo => {
  if (video.task_steps && video.task_steps.length > 0) {
    return getProgressFromSteps(video.task_steps);
  }

  if (video.progress) {
    const totalSteps = Number(video.progress.total_steps || DEFAULT_STEP_COUNT);
    const completedSteps = Number(video.progress.completed_steps || 0);
    const failedSteps = Number(video.progress.failed_steps || 0);
    const rawPercent = video.progress.progress_percent ?? video.progress.progress_percentage;
    const percent = rawPercent !== undefined
      ? clampPercent(Number(rawPercent))
      : totalSteps > 0
        ? clampPercent((completedSteps / totalSteps) * 100)
        : 0;

    return {
      totalSteps,
      completedSteps,
      failedSteps,
      percent,
      currentStep: video.progress.current_step || '',
      currentStepProgress: clampPercent(Number(video.progress.current_step_progress || 0)),
      currentStepMessage: video.progress.current_step_message || '',
      nextStep: '',
      isRunning: Boolean(video.progress.is_running || video.progress.current_step),
    };
  }

  return getFallbackProgress(video.status);
};

export default function TaskQueueStats({ onVideoSelect }: TaskQueueStatsProps) {
  const [videos, setVideos] = useState<Video[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<TabType>('all');
  const [refreshing, setRefreshing] = useState(false);
  const [expandedVideoId, setExpandedVideoId] = useState<number | null>(null);
  const [detailedVideo, setDetailedVideo] = useState<Video | null>(null);
  const [isDetailLoading, setIsDetailLoading] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const itemsPerPage = 10;
  const apiPageSize = 100;

  const fetchVideos = useCallback(async (showRefreshing = true) => {
    try {
      if (showRefreshing) setRefreshing(true);
      let page = 1;
      let total = 0;
      const allVideos: Video[] = [];

      while (true) {
        const response = await fetch(`/api/v1/videos?page=${page}&limit=${apiPageSize}`);
        const data = await response.json();

        if (!((data.code === 0 || data.code === 200) && data.data)) {
          break;
        }

        const batch: Video[] = data.data.videos || [];
        total = Number(data.data.total || 0);
        allVideos.push(...batch);

        if (allVideos.length >= total || batch.length < apiPageSize) {
          break;
        }

        page += 1;
      }

      setVideos(allVideos);
    } catch (error) {
      console.error('Failed to fetch video list:', error);
    } finally {
      setLoading(false);
      if (showRefreshing) setRefreshing(false);
    }
  }, []);

  const fetchVideoDetail = useCallback(async (videoId: number, showLoading = true) => {
    if (showLoading) setIsDetailLoading(true);
    try {
      const response = await fetch(`/api/v1/videos/${videoId}`);
      const data = await response.json();
      if (data.code === 200 || data.code === 0) {
        setDetailedVideo(data.data);
      } else {
        console.error('Failed to fetch video details:', data.message);
      }
    } catch (error) {
      console.error('Error fetching video details:', error);
    } finally {
      if (showLoading) setIsDetailLoading(false);
    }
  }, []);

  const hasLiveTasks = videos.some(video => (
    video.progress?.is_running ||
    Boolean(video.progress?.current_step) ||
    ['002', '201', '301'].includes(video.status)
  ));

  useEffect(() => {
    fetchVideos(false);
    const interval = window.setInterval(() => fetchVideos(false), hasLiveTasks ? 2000 : 30000);
    return () => window.clearInterval(interval);
  }, [fetchVideos, hasLiveTasks]);

  useEffect(() => {
    if (!expandedVideoId || !detailedVideo) return;
    const progress = getVideoProgress(detailedVideo);
    if (!progress.isRunning && !['002', '201', '301'].includes(detailedVideo.status)) return;

    const interval = window.setInterval(() => fetchVideoDetail(expandedVideoId, false), 2000);
    return () => window.clearInterval(interval);
  }, [detailedVideo, expandedVideoId, fetchVideoDetail]);

  const handleToggleDetails = async (videoId: number) => {
    if (expandedVideoId === videoId) {
      setExpandedVideoId(null);
      setDetailedVideo(null);
    } else {
      setExpandedVideoId(videoId);
      fetchVideoDetail(videoId);
    }
  };

  const handleRetryStep = async (videoId: number, stepName: string) => {
    try {
      const response = await fetch(`/api/v1/videos/${videoId}/steps/${stepName}/retry`, {
        method: 'POST',
      });
      const data = await response.json();
      if (data.code === 200 || data.code === 0) {
        // 刷新详情
        fetchVideoDetail(videoId, false);
        fetchVideos(false);
      } else {
        console.error('Failed to retry step:', data.message);
      }
    } catch (error) {
      console.error('Error retrying step:', error);
    }
  };
  
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed':
        return 'bg-green-100 text-green-800';
      case 'failed':
        return 'bg-red-100 text-red-800';
      case 'running':
        return 'bg-blue-100 text-blue-800';
      case 'pending':
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  // 状态映射
  const getStatusInfo = (status: string) => {
    const statusMap: { [key: string]: { label: string; color: string; icon: any; category: TabType } } = {
      '001': { label: '待处理', color: 'bg-gray-100 text-gray-700', icon: Clock, category: 'pending' },
      '002': { label: '处理中', color: 'bg-blue-100 text-blue-700', icon: Play, category: 'preparing' },
      '200': { label: '准备就绪', color: 'bg-green-100 text-green-700', icon: CheckCircle, category: 'ready' },
      '201': { label: '上传视频中', color: 'bg-purple-100 text-purple-700', icon: Upload, category: 'uploading' },
      '299': { label: '上传失败', color: 'bg-red-100 text-red-700', icon: AlertCircle, category: 'failed' },
      '300': { label: '视频已上传', color: 'bg-cyan-100 text-cyan-700', icon: CheckCircle, category: 'uploading' },
      '301': { label: '上传字幕中', color: 'bg-indigo-100 text-indigo-700', icon: Upload, category: 'uploading' },
      '399': { label: '字幕上传失败', color: 'bg-orange-100 text-orange-700', icon: AlertCircle, category: 'failed' },
      '400': { label: '全部完成', color: 'bg-emerald-100 text-emerald-700', icon: CheckCircle, category: 'completed' },
      '999': { label: '任务失败', color: 'bg-red-100 text-red-700', icon: AlertCircle, category: 'failed' },
    };
    return statusMap[status] || { label: '未知', color: 'bg-gray-100 text-gray-700', icon: AlertCircle, category: 'all' };
  };

  // 获取当前阶段描述
  const getStageDescription = (status: string) => {
    const stageMap: { [key: string]: string } = {
      '001': '等待开始处理',
      '002': '正在执行准备任务链（下载视频→生成字幕→翻译字幕→生成元数据）',
      '200': '准备阶段完成，等待视频上传（每小时上传1个）',
      '201': '正在上传视频到Bilibili',
      '299': '视频上传失败，需要重试',
      '300': '视频已上传，等待1小时后上传字幕',
      '301': '正在上传字幕到Bilibili',
      '399': '字幕上传失败，需要重试',
      '400': '所有任务已完成',
      '999': '准备阶段失败，需要检查任务步骤',
    };
    return stageMap[status] || '未知状态';
  };

  // 分类视频
  const categorizeVideos = () => {
    return {
      pending: videos.filter(v => v.status === '001'),
      preparing: videos.filter(v => v.status === '002'),
      ready: videos.filter(v => v.status === '200'),
      uploading: videos.filter(v => ['201', '300', '301'].includes(v.status)),
      completed: videos.filter(v => v.status === '400'),
      failed: videos.filter(v => ['299', '399', '999'].includes(v.status)),
    };
  };

  const categories = categorizeVideos();
  const filteredVideos = activeTab === 'all' ? videos : categories[activeTab as keyof typeof categories];

  // 分页逻辑
  const totalItems = filteredVideos.length;
  const totalPages = Math.ceil(totalItems / itemsPerPage);
  const paginatedVideos = filteredVideos.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  );

  const handlePageChange = (page: number) => {
    if (page >= 1 && page <= totalPages) {
      setCurrentPage(page);
    }
  };

  useEffect(() => {
    setCurrentPage(1);
  }, [activeTab]);

  const tabs = [
    { key: 'all', label: '全部', count: videos.length },
    { key: 'pending', label: '待处理', count: categories.pending.length },
    { key: 'preparing', label: '准备中', count: categories.preparing.length },
    { key: 'ready', label: '准备就绪', count: categories.ready.length },
    { key: 'uploading', label: '上传中', count: categories.uploading.length },
    { key: 'completed', label: '已完成', count: categories.completed.length },
    { key: 'failed', label: '失败', count: categories.failed.length },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mb-4"></div>
          <p className="text-gray-600">加载任务数据...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 标题栏 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900">任务管理</h2>
        <button
          onClick={() => fetchVideos(true)}
          disabled={refreshing}
          className="flex items-center space-x-2 px-4 py-2 text-sm text-gray-600 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
          <span>刷新</span>
        </button>
      </div>

      {/* 标签页 */}
      <div className="border-b border-gray-200">
        <div className="flex space-x-8">
          {tabs.map(tab => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key as TabType)}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === tab.key
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              {tab.label}
              {tab.count > 0 && (
                <span className={`ml-2 text-xs px-2 py-0.5 rounded ${
                  activeTab === tab.key ? 'bg-blue-100' : 'bg-gray-100'
                }`}>
                  {tab.count}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* 视频处理任务链 */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900">视频处理任务链</h3>
        </div>
        <div className="bg-white rounded-lg border border-gray-200">
          {paginatedVideos.length === 0 ? (
            <div className="p-12 text-center text-gray-500">
              暂无任务数据
            </div>
          ) : (
            <div className="divide-y divide-gray-200">
              {paginatedVideos.map(video => {
                const statusInfo = getStatusInfo(video.status);
                const Icon = statusInfo.icon;
                const detailForVideo = expandedVideoId === video.id && detailedVideo?.id === video.id ? detailedVideo : video;
                const progressInfo = getVideoProgress(detailForVideo);
                return (
                  <div key={video.id}>
                    <div 
                      className="p-4 hover:bg-gray-50 transition-colors cursor-pointer"
                      onClick={() => handleToggleDetails(video.id)}
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex-1">
                          <div className="flex items-center space-x-3 mb-2">
                            <h4 className="font-medium text-gray-900">
                              {video.title || video.video_id}
                            </h4>
                            <span className={`flex items-center space-x-1 text-xs px-2 py-1 rounded ${statusInfo.color}`}>
                              <Icon className="w-3 h-3" />
                              <span>{statusInfo.label}</span>
                            </span>
                            {video.bili_bvid && (
                              <a
                                href={`https://www.bilibili.com/video/${video.bili_bvid}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-xs text-blue-600 hover:underline"
                              >
                                {video.bili_bvid}
                              </a>
                            )}
                          </div>
                          <p className="text-sm text-gray-600 mb-3">
                            {getStageDescription(video.status)}
                          </p>
                          <TaskProgressBar progress={progressInfo} />
                          <div className="flex items-center space-x-6 text-xs text-gray-500">
                            <span>视频ID: {video.video_id}</span>
                            <span>创建: {new Date(video.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</span>
                            <span>更新: {new Date(video.updated_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</span>
                          </div>
                        </div>
                        <button 
                          className="ml-4 px-4 py-2 text-sm text-blue-600 hover:bg-blue-50 rounded transition-colors"
                        >
                          {expandedVideoId === video.id ? '收起详情' : '查看详情'}
                        </button>
                      </div>
                    </div>
                    {expandedVideoId === video.id && (
                      <div className="p-4 border-t border-gray-200 bg-gray-50">
                        {isDetailLoading ? (
                          <div className="text-center text-gray-500">加载任务步骤...</div>
                        ) : detailedVideo && detailedVideo.task_steps ? (
                          <TaskStepDetail 
                            steps={detailedVideo.task_steps} 
                            onRetry={(stepName) => handleRetryStep(video.id, stepName)}
                          />
                        ) : (
                          <div className="text-center text-gray-500">无任务步骤信息</div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* 分页控件 */}
      {totalPages > 1 && (
        <div className="flex justify-center items-center space-x-2 mt-6">
          <button
            onClick={() => handlePageChange(currentPage - 1)}
            disabled={currentPage === 1}
            className="px-3 py-1 text-sm text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
          >
            上一页
          </button>
          <span className="text-sm text-gray-700">
            第 {currentPage} 页 / 共 {totalPages} 页
          </span>
          <button
            onClick={() => handlePageChange(currentPage + 1)}
            disabled={currentPage === totalPages}
            className="px-3 py-1 text-sm text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
          >
            下一页
          </button>
        </div>
      )}

      {/* 自动化调度说明 */}
      <div>
        <h3 className="text-lg font-semibold text-gray-900 mb-4">自动化调度策略</h3>
        <div className="bg-white rounded-lg border border-gray-200">
          <div className="p-6 space-y-4">
            <div className="flex items-start space-x-3">
              <div className="flex-shrink-0 w-8 h-8 bg-blue-100 rounded-lg flex items-center justify-center">
                <Play className="w-4 h-4 text-blue-600" />
              </div>
              <div className="flex-1">
                <h4 className="font-medium text-gray-900 mb-1">准备阶段任务链</h4>
                <p className="text-sm text-gray-600">
                  每5秒检查一次待处理任务，依次执行：下载视频 → 生成字幕 → 翻译字幕 → 生成元数据
                </p>
                <div className="mt-2 text-xs text-gray-500">
                  当前准备中: {categories.preparing.length} 个 | 待处理: {categories.pending.length} 个
                </div>
              </div>
            </div>

            <div className="flex items-start space-x-3">
              <div className="flex-shrink-0 w-8 h-8 bg-purple-100 rounded-lg flex items-center justify-center">
                <Upload className="w-4 h-4 text-purple-600" />
              </div>
              <div className="flex-1">
                <h4 className="font-medium text-gray-900 mb-1">视频上传调度</h4>
                <p className="text-sm text-gray-600">
                  每5分钟检查一次，每小时自动上传1个准备就绪的视频到Bilibili
                </p>
                <div className="mt-2 text-xs text-gray-500">
                  准备就绪: {categories.ready.length} 个 | 上传中: {categories.uploading.length} 个
                </div>
              </div>
            </div>

            <div className="flex items-start space-x-3">
              <div className="flex-shrink-0 w-8 h-8 bg-indigo-100 rounded-lg flex items-center justify-center">
                <Upload className="w-4 h-4 text-indigo-600" />
              </div>
              <div className="flex-1">
                <h4 className="font-medium text-gray-900 mb-1">字幕上传调度</h4>
                <p className="text-sm text-gray-600">
                  视频上传完成1小时后，自动上传对应的字幕文件
                </p>
                <div className="mt-2 text-xs text-gray-500">
                  等待上传字幕: {videos.filter(v => v.status === '300').length} 个
                </div>
              </div>
            </div>

            {categories.failed.length > 0 && (
              <div className="flex items-start space-x-3 p-3 bg-red-50 rounded-lg">
                <div className="flex-shrink-0 w-8 h-8 bg-red-100 rounded-lg flex items-center justify-center">
                  <AlertCircle className="w-4 h-4 text-red-600" />
                </div>
                <div className="flex-1">
                  <h4 className="font-medium text-red-900 mb-1">失败任务提醒</h4>
                  <p className="text-sm text-red-700">
                    当前有 {categories.failed.length} 个任务失败，请查看详情并手动重试
                  </p>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* 统计汇总 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="text-sm text-gray-600 mb-1">总任务数</div>
          <div className="text-2xl font-bold text-gray-900">{videos.length}</div>
        </div>
        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="text-sm text-gray-600 mb-1">进行中</div>
          <div className="text-2xl font-bold text-blue-600">
            {categories.preparing.length + categories.uploading.length}
          </div>
        </div>
        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="text-sm text-gray-600 mb-1">已完成</div>
          <div className="text-2xl font-bold text-green-600">{categories.completed.length}</div>
        </div>
        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="text-sm text-gray-600 mb-1">失败</div>
          <div className="text-2xl font-bold text-red-600">{categories.failed.length}</div>
        </div>
      </div>
    </div>
  );
}

const TaskProgressBar = ({ progress }: { progress: ProgressInfo }) => {
  const isComplete = progress.totalSteps > 0 && progress.completedSteps >= progress.totalSteps;
  const hasFailed = progress.failedSteps > 0;
  const fillClass = hasFailed
    ? 'bg-red-500'
    : isComplete
      ? 'bg-emerald-500'
      : 'bg-blue-500';
  const statusText = progress.currentStep
    ? `当前: ${progress.currentStep}${progress.isRunning ? ` ${progress.currentStepProgress}%` : ''}`
    : progress.nextStep && !isComplete
      ? `下一步: ${progress.nextStep}`
      : isComplete
        ? '全部完成'
        : '等待调度';

  return (
    <div className="mb-3 max-w-3xl">
      <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
        <span className="min-w-0 truncate pr-3">
          已完成 {progress.completedSteps}/{progress.totalSteps}
          {progress.failedSteps > 0 ? ` · 失败 ${progress.failedSteps}` : ''}
          {statusText ? ` · ${statusText}` : ''}
        </span>
        <span className="font-medium text-gray-700">{progress.percent}%</span>
      </div>
      <div className="h-2 w-full bg-gray-200 rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-500 ${fillClass}`}
          style={{ width: `${progress.percent}%` }}
        />
      </div>
      {progress.isRunning && progress.currentStepMessage && (
        <div className="mt-1 text-xs text-blue-600 truncate">
          {progress.currentStepMessage}
        </div>
      )}
    </div>
  );
};

const TaskStepDetail = ({ steps, onRetry }: { steps: TaskStep[], onRetry: (stepName: string) => void }) => {
  const progressInfo = getProgressFromSteps(steps);

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed':
        return 'bg-green-100 text-green-800';
      case 'failed':
        return 'bg-red-100 text-red-800';
      case 'running':
        return 'bg-blue-100 text-blue-800';
      case 'pending':
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  const getStepProgressMessage = (step: TaskStep) => {
    if (step.progress_message) return step.progress_message;
    switch (step.status) {
      case 'completed':
        return '已完成';
      case 'running':
        return '执行中';
      case 'failed':
        return '执行失败';
      case 'skipped':
        return '已跳过';
      default:
        return '等待执行';
    }
  };

  return (
    <div className="space-y-3">
      <h5 className="font-semibold text-gray-800">任务步骤</h5>
      <TaskProgressBar progress={progressInfo} />
      <ul className="space-y-2">
        {steps.sort((a, b) => a.step_order - b.step_order).map(step => {
          const stepProgress = step.status === 'completed'
            ? 100
            : Math.max(0, Math.min(100, Math.round(Number(step.progress_percent || 0))));
          const progressFillClass = step.status === 'completed'
            ? 'bg-green-500'
            : step.status === 'failed'
              ? 'bg-red-500'
              : step.status === 'pending'
                ? 'bg-gray-300'
                : 'bg-blue-500';

          return (
            <li key={step.step_name} className="p-3 bg-white rounded-lg border border-gray-200">
              <div className="flex items-center justify-between">
                <div className="flex-1">
                  <div className="flex items-center space-x-2">
                    <span className="font-medium text-gray-700">{step.step_name}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full ${getStatusColor(step.status)}`}>
                      {step.status}
                    </span>
                  </div>
                  {step.error_msg && (
                    <p className="text-xs text-red-600 mt-1">错误: {step.error_msg}</p>
                  )}
                  <div className="mt-2 max-w-lg">
                    <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
                      <span className="truncate pr-3">{getStepProgressMessage(step)}</span>
                      <span className="font-medium text-gray-700">{stepProgress}%</span>
                    </div>
                    <div className="h-1.5 w-full bg-gray-200 rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full transition-all duration-500 ${progressFillClass}`}
                        style={{ width: `${stepProgress}%` }}
                      />
                    </div>
                  </div>
                </div>
                {step.can_retry && (step.status === 'failed' || step.status === 'skipped') && (
                  <button
                    onClick={() => onRetry(step.step_name)}
                    className="px-3 py-1 text-xs text-blue-600 bg-blue-100 hover:bg-blue-200 rounded"
                  >
                    重试
                  </button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
};
