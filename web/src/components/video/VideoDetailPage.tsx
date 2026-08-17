'use client';

import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, ExternalLink, RefreshCw, Download, Calendar, Clock, Image as ImageIcon, FileText, Play, Eye } from 'lucide-react';
import { VideoDetail, VideoFile, TASK_STEP_NAMES } from '@/types';
import { videoApi } from '@/lib/api';
import TaskStepList from './TaskStepList';
import StatusBadge from '@/components/ui/StatusBadge';
import VideoActions from './VideoActions';

interface VideoDetailPageProps {
  videoId: string;
  onBack: () => void;
}

const CHAPTER_STATUS_META: Record<string, {
  label: string;
  badgeClassName: string;
  panelClassName: string;
  iconClassName: string;
}> = {
  extracted: {
    label: '已提取',
    badgeClassName: 'bg-green-100 text-green-800',
    panelClassName: 'bg-green-50 border-green-200',
    iconClassName: 'text-green-600',
  },
  used_existing: {
    label: '沿用已有',
    badgeClassName: 'bg-cyan-100 text-cyan-800',
    panelClassName: 'bg-cyan-50 border-cyan-200',
    iconClassName: 'text-cyan-600',
  },
  no_match: {
    label: '未匹配',
    badgeClassName: 'bg-yellow-100 text-yellow-800',
    panelClassName: 'bg-yellow-50 border-yellow-200',
    iconClassName: 'text-yellow-600',
  },
  translation_failed: {
    label: '翻译失败',
    badgeClassName: 'bg-red-100 text-red-800',
    panelClassName: 'bg-red-50 border-red-200',
    iconClassName: 'text-red-600',
  },
  save_failed: {
    label: '保存失败',
    badgeClassName: 'bg-red-100 text-red-800',
    panelClassName: 'bg-red-50 border-red-200',
    iconClassName: 'text-red-600',
  },
  empty_result: {
    label: '结果为空',
    badgeClassName: 'bg-yellow-100 text-yellow-800',
    panelClassName: 'bg-yellow-50 border-yellow-200',
    iconClassName: 'text-yellow-600',
  },
  skipped: {
    label: '已跳过',
    badgeClassName: 'bg-gray-100 text-gray-800',
    panelClassName: 'bg-gray-50 border-gray-200',
    iconClassName: 'text-gray-500',
  },
  not_extracted: {
    label: '未提取',
    badgeClassName: 'bg-gray-100 text-gray-800',
    panelClassName: 'bg-gray-50 border-gray-200',
    iconClassName: 'text-gray-500',
  },
};

function getChaptersStatusMeta(status?: string, extracted?: boolean) {
  const normalizedStatus = status || (extracted ? 'extracted' : 'not_extracted');
  return CHAPTER_STATUS_META[normalizedStatus] || CHAPTER_STATUS_META.not_extracted;
}

export default function VideoDetailPage({ videoId, onBack }: VideoDetailPageProps) {
  const [video, setVideo] = useState<VideoDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchVideoDetail = useCallback(async (showRefreshing = false, silent = false) => {
    if (showRefreshing) setRefreshing(true);
    else if (!silent) setLoading(true);
    
    try {
      const response = await videoApi.getVideoDetail(videoId);
      if (response.code === 200 || response.code === 0) {
        setVideo(response.data);
        setError(null);
      } else {
        setError(response.message || '获取视频详情失败');
      }
    } catch (err: any) {
      console.error('获取视频详情失败:', err);
      setError(err.message || '网络错误，请重试');
    } finally {
      if (!silent) setLoading(false);
      if (showRefreshing) setRefreshing(false);
    }
  }, [videoId]);

  useEffect(() => {
    fetchVideoDetail();
  }, [fetchVideoDetail]);

  const hasRunningStep = video?.task_steps?.some(step => step.status === 'running') || false;
  const shouldAutoRefresh = Boolean(
    video && (video.progress?.is_running || hasRunningStep || ['002', '201', '301'].includes(video.status))
  );

  useEffect(() => {
    if (!shouldAutoRefresh) return;

    const interval = window.setInterval(() => {
      fetchVideoDetail(false, true);
    }, 2000);

    return () => window.clearInterval(interval);
  }, [fetchVideoDetail, shouldAutoRefresh]);

  const handleRetryStep = async (stepName: string) => {
    try {
      const response = await videoApi.retryTaskStep(videoId, stepName);
      if (response.code === 200 || response.code === 0) {
        // 重新获取视频详情以更新状态
        setTimeout(() => fetchVideoDetail(true), 1000);
      } else {
        alert(response.message || '重试失败');
      }
    } catch (err: any) {
      console.error('重试任务失败:', err);
      alert('重试失败，请稍后再试');
    }
  };

  const handleRefresh = () => {
    fetchVideoDetail(true);
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  };

  const getFileIcon = (type: VideoFile['type']) => {
    const className = "w-4 h-4";
    switch (type) {
      case 'video':
        return <Play className={className} />;
      case 'subtitle':
        return <FileText className={className} />;
      case 'cover':
        return <ImageIcon className={className} />;
      default:
        return <FileText className={className} />;
    }
  };

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const progressPercent = Math.max(0, Math.min(100, Number(
    video?.progress?.progress_percent ?? video?.progress?.progress_percentage ?? 0
  )));
  const currentStepProgress = Math.max(0, Math.min(100, Number(
    video?.progress?.current_step_progress ?? 0
  )));
  const currentStepMessage = video?.progress?.current_step_message || '';
  const currentStepName = video?.progress?.current_step
    ? TASK_STEP_NAMES[video.progress.current_step as keyof typeof TASK_STEP_NAMES] || video.progress.current_step
    : '';
  const isProgressRunning = Boolean(video?.progress?.is_running || currentStepName);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mb-4"></div>
          <p className="text-gray-600">加载视频详情...</p>
        </div>
      </div>
    );
  }

  if (error || !video) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-500 text-6xl mb-4">⚠️</div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">加载失败</h2>
          <p className="text-gray-600 mb-4">{error}</p>
          <div className="space-x-4">
            <button
              onClick={() => fetchVideoDetail()}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
            >
              重试
            </button>
            <button
              onClick={onBack}
              className="px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors"
            >
              返回
            </button>
          </div>
        </div>
      </div>
    );
  }

  const generatedDescription = video.generated_description || video.generated_desc || '';
  const chapterLines = (video.chapters || '')
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean);
  const chaptersExtracted = Boolean(video.chapters_extracted || chapterLines.length > 0);
  const chaptersCount = Math.max(video.chapters_count ?? 0, chapterLines.length);
  const chaptersStatusMeta = getChaptersStatusMeta(video.chapters_status, chaptersExtracted);
  const chaptersMessage = video.chapters_message || (
    chaptersExtracted ? `已提取 ${chaptersCount} 条章节` : '尚未提取到章节'
  );
  const chapterPreviewLines = chapterLines.slice(0, 8);

  return (
    <div className="min-h-screen bg-gray-50 py-6">
      <div className="container mx-auto px-4 max-w-6xl">
        {/* 头部导航 */}
        <div className="flex items-center justify-between mb-6">
          <button
            onClick={onBack}
            className="flex items-center space-x-2 text-gray-600 hover:text-gray-900 transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
            <span>返回视频列表</span>
          </button>

          <button
            onClick={handleRefresh}
            disabled={refreshing}
            className="flex items-center space-x-2 px-3 py-2 text-sm text-gray-600 hover:text-gray-900 transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
            <span>刷新</span>
          </button>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* 左侧：视频信息 */}
          <div className="lg:col-span-2 space-y-6">
            {/* 基本信息 */}
            <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
              <div className="flex items-start justify-between mb-4">
                <div className="flex-1">
                  <h1 className="text-2xl font-bold text-gray-900 mb-2">
                    {video.generated_title || video.title || '未设置标题'}
                  </h1>
                  {video.generated_title && video.generated_title !== video.title && (
                    <p className="text-gray-500 mb-2">
                      原标题: {video.title}
                    </p>
                  )}
                  <div className="flex items-center space-x-4 text-sm text-gray-500">
                    <span className="flex items-center space-x-1">
                      <Calendar className="w-4 h-4" />
                      <span>{formatDate(video.created_at)}</span>
                    </span>
                    <span>ID: {video.video_id}</span>
                  </div>
                </div>
                
                <div className="flex flex-col items-end space-y-2">
                  <StatusBadge status={video.status} />
                  
                  {video.cover_image && (
                    <a
                      href={video.cover_image}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-sm text-blue-600 hover:text-blue-800 flex items-center space-x-1"
                    >
                      <Eye className="w-4 h-4" />
                      <span>查看封面</span>
                    </a>
                  )}

                  {video.bili_bvid && (
                    <a
                      href={`https://www.bilibili.com/video/${video.bili_bvid}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-sm text-blue-600 hover:text-blue-800 flex items-center space-x-1"
                    >
                      <ExternalLink className="w-4 h-4" />
                      <span>打开B站稿件</span>
                    </a>
                  )}
                </div>
              </div>

              {/* 描述信息 */}
              {generatedDescription && (
                <div className="mb-4">
                  <h3 className="text-sm font-medium text-gray-900 mb-2">生成的描述</h3>
                  <div className="p-3 bg-gray-50 rounded-lg text-sm text-gray-700 whitespace-pre-wrap">
                    {generatedDescription}
                  </div>
                </div>
              )}

              {/* 标签 */}
              {video.generated_tags && (
                <div className="mb-4">
                  <h3 className="text-sm font-medium text-gray-900 mb-2">生成的标签</h3>
                  <div className="flex flex-wrap gap-2">
                    {video.generated_tags.split(',').map((tag, index) => (
                      <span
                        key={index}
                        className="inline-block px-2 py-1 text-sm bg-blue-100 text-blue-800 rounded"
                      >
                        {tag.trim()}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {/* 章节提取 */}
              <div className="mb-4">
                <div className="flex items-center justify-between gap-3 mb-2">
                  <h3 className="text-sm font-medium text-gray-900">章节提取</h3>
                  <span className={`inline-flex items-center px-2 py-1 text-xs font-medium rounded ${chaptersStatusMeta.badgeClassName}`}>
                    {chaptersStatusMeta.label}
                  </span>
                </div>
                <div className={`border rounded-lg p-3 ${chaptersStatusMeta.panelClassName}`}>
                  <div className="flex items-start gap-3">
                    <FileText className={`w-4 h-4 mt-0.5 flex-shrink-0 ${chaptersStatusMeta.iconClassName}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                        <span className="font-medium text-gray-900">
                          {chaptersCount > 0 ? `${chaptersCount} 条章节` : '暂无章节'}
                        </span>
                        {video.chapters_status && (
                          <span className="text-xs text-gray-500">
                            {video.chapters_status}
                          </span>
                        )}
                      </div>
                      <p className="mt-1 text-sm text-gray-700">
                        {chaptersMessage}
                      </p>
                      {chapterPreviewLines.length > 0 && (
                        <pre className="mt-3 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded border border-gray-200 bg-white/80 p-3 text-xs leading-5 text-gray-700">
                          {chapterPreviewLines.join('\n')}
                          {chapterLines.length > chapterPreviewLines.length ? '\n...' : ''}
                        </pre>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {/* 原始URL */}
              <div>
                <h3 className="text-sm font-medium text-gray-900 mb-2">视频来源</h3>
                <a
                  href={video.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center space-x-2 text-blue-600 hover:text-blue-800 text-sm"
                >
                  <span className="truncate max-w-md">{video.url}</span>
                  <ExternalLink className="w-4 h-4 flex-shrink-0" />
                </a>
              </div>
            </div>

            {/* 任务步骤 */}
            <TaskStepList
              steps={video.task_steps}
              onRetryStep={handleRetryStep}
              isRetrying={refreshing}
            />
          </div>

          {/* 右侧：进度和文件 */}
          <div className="space-y-6">
            {/* 手动操作按钮 */}
            <VideoActions 
              videoId={video.video_id} 
              status={video.status}
              biliBvid={video.bili_bvid}
              biliAid={video.bili_aid}
              publishAudience={video.publish_audience}
              audienceSelectedAt={video.audience_selected_at}
              upowerPreviewSeconds={video.upower_preview_seconds}
              onSuccess={handleRefresh}
            />

            {/* 任务进度 */}
            <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-4">处理进度</h3>
              
              <div className="space-y-4">
                {/* 进度条 */}
                <div>
                  <div className="flex justify-between text-sm text-gray-600 mb-2">
                    <span>总体进度</span>
                    <span>{progressPercent}%</span>
                  </div>
                  <div className="w-full bg-gray-200 rounded-full h-2">
                    <div
                      className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                      style={{ width: `${progressPercent}%` }}
                    ></div>
                  </div>
                </div>

                {/* 统计信息 */}
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div className="text-center p-3 bg-gray-50 rounded-lg">
                    <div className="text-2xl font-bold text-green-600">
                      {video.progress.completed_steps}
                    </div>
                    <div className="text-gray-600">已完成</div>
                  </div>
                  <div className="text-center p-3 bg-gray-50 rounded-lg">
                    <div className="text-2xl font-bold text-red-600">
                      {video.progress.failed_steps}
                    </div>
                    <div className="text-gray-600">失败</div>
                  </div>
                </div>

                {/* 当前步骤 */}
                {isProgressRunning && currentStepName && (
                  <div className="p-3 bg-blue-50 border border-blue-200 rounded-lg">
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center space-x-2 min-w-0">
                        <Clock className="w-4 h-4 text-blue-600 flex-shrink-0" />
                        <span className="text-sm text-blue-800 truncate">
                          正在执行: {currentStepName}
                        </span>
                      </div>
                      <span className="text-sm font-medium text-blue-800 flex-shrink-0">
                        {currentStepProgress}%
                      </span>
                    </div>
                    <div className="mt-2 h-1.5 w-full bg-blue-100 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-blue-600 rounded-full transition-all duration-500"
                        style={{ width: `${currentStepProgress}%` }}
                      ></div>
                    </div>
                    {currentStepMessage && (
                      <p className="mt-2 text-xs text-blue-700 truncate">
                        {currentStepMessage}
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* 文件列表 */}
            {video.files && video.files.length > 0 && (
              <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
                <h3 className="text-lg font-semibold text-gray-900 mb-4">相关文件</h3>
                
                <div className="space-y-3">
                  {video.files.map((file, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between p-3 border border-gray-200 rounded-lg hover:bg-gray-50 transition-colors"
                    >
                      <div className="flex items-center space-x-3 flex-1 min-w-0">
                        {getFileIcon(file.type)}
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-gray-900 truncate">
                            {file.name}
                          </p>
                          <p className="text-xs text-gray-500">
                            {formatFileSize(file.size)}
                          </p>
                        </div>
                      </div>
                      
                      <button
                        onClick={() => window.open(file.path, '_blank')}
                        className="p-1 text-gray-400 hover:text-blue-600 transition-colors"
                        title="下载文件"
                      >
                        <Download className="w-4 h-4" />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
