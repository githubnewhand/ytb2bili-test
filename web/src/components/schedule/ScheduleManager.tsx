'use client';

import { useState, useEffect } from 'react';
import { RefreshCw, Clock, Upload, Play, CheckCircle, AlertCircle, Calendar } from 'lucide-react';

interface Video {
  id: number;
  video_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
  bili_bvid?: string;
  bili_aid?: number;
}

interface ScheduleTask {
  id: number;
  video: Video;
  type: 'video' | 'subtitle';
  status: 'pending' | 'running' | 'completed' | 'failed';
  scheduled_time?: string;
  last_run_time?: string;
}

interface ScheduleManagerProps {
  onVideoSelect?: (videoId: string) => void;
}

export default function ScheduleManager({ onVideoSelect }: ScheduleManagerProps) {
  const [videos, setVideos] = useState<Video[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [executingTasks, setExecutingTasks] = useState<Set<string>>(new Set());
  const [videoPage, setVideoPage] = useState(1);
  const [subtitlePage, setSubtitlePage] = useState(1);
  const itemsPerPage = 10;
  const apiPageSize = 100;
  useEffect(() => {
    fetchVideos();
    const interval = setInterval(fetchVideos, 30000);
    return () => clearInterval(interval);
  }, []);

  const fetchVideos = async () => {
    try {
      setRefreshing(true);
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
      setRefreshing(false);
    }
  };

  // 获取待上传视频的任务
  const getVideoUploadTasks = (): ScheduleTask[] => {
    return videos
      .filter(v => v.status === '200') // 准备就绪的视频
      .map(v => ({
        id: v.id,
        video: v,
        type: 'video',
        status: 'pending',
      }));
  };

  // 获取待上传字幕的任务
  const getSubtitleUploadTasks = (): ScheduleTask[] => {
    return videos
      .filter(v => v.status === '300' || v.status === '399') // 300待上传，399失败待重试
      .map(v => ({
        id: v.id,
        video: v,
        type: 'subtitle',
        status: v.status === '399' ? 'failed' : 'pending',
      }));
  };

  // 手动执行上传任务
  const handleManualUpload = async (videoId: number, taskType: 'video' | 'subtitle') => {
    const taskKey = `${videoId}-${taskType}`;
    
    if (executingTasks.has(taskKey)) {
      return; // 防止重复执行
    }

    try {
      setExecutingTasks(prev => new Set(prev).add(taskKey));

      const response = await fetch(`/api/v1/videos/${videoId}/upload/${taskType}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      const data = await response.json();

      if (data.code === 0 || data.code === 200) {
        alert(`${taskType === 'video' ? '视频' : '字幕'}上传任务已成功触发！`);
        // 刷新列表
        await fetchVideos();
      } else {
        throw new Error(data.message || '上传失败');
      }
    } catch (error) {
      console.error('手动上传失败:', error);
      alert(`上传失败: ${error instanceof Error ? error.message : '未知错误'}`);
    } finally {
      setExecutingTasks(prev => {
        const newSet = new Set(prev);
        newSet.delete(taskKey);
        return newSet;
      });
    }
  };

  const videoUploadTasks = getVideoUploadTasks();
  const subtitleUploadTasks = getSubtitleUploadTasks();

  // 分页逻辑
  const paginatedVideoTasks = videoUploadTasks.slice(
    (videoPage - 1) * itemsPerPage,
    videoPage * itemsPerPage
  );
  const totalVideoPages = Math.ceil(videoUploadTasks.length / itemsPerPage);

  const paginatedSubtitleTasks = subtitleUploadTasks.slice(
    (subtitlePage - 1) * itemsPerPage,
    subtitlePage * itemsPerPage
  );
  const totalSubtitlePages = Math.ceil(subtitleUploadTasks.length / itemsPerPage);

  useEffect(() => {
    setVideoPage(1);
  }, [videoUploadTasks.length]);

  useEffect(() => {
    setSubtitlePage(1);
  }, [subtitleUploadTasks.length]);
  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mb-4"></div>
          <p className="text-gray-600">加载定时任务...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 标题栏 */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">定时上传管理</h2>
          <p className="text-sm text-gray-600 mt-1">手动触发或查看自动上传任务</p>
        </div>
        <button
          onClick={fetchVideos}
          disabled={refreshing}
          className="flex items-center space-x-2 px-4 py-2 text-sm text-gray-600 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
          <span>刷新</span>
        </button>
      </div>

      {/* 调度策略说明 */}
      <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
        <div className="flex items-start space-x-3">
          <Clock className="w-5 h-5 text-blue-600 flex-shrink-0 mt-0.5" />
          <div className="flex-1">
            <h3 className="font-medium text-blue-900 mb-2">自动上传策略</h3>
            <div className="text-sm text-blue-800 space-y-1">
              <p>• <strong>视频上传</strong>：每小时自动上传1个准备就绪的视频到Bilibili</p>
              <p>• <strong>字幕上传</strong>：仅手动触发；失败后可点击“重试上传字幕”</p>
              <p>• <strong>手动触发</strong>：您可以随时点击下方的&quot;立即上传&quot;按钮手动触发上传任务</p>
            </div>
          </div>
        </div>
      </div>

      {/* 视频上传任务 */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900 flex items-center space-x-2">
            <Upload className="w-5 h-5 text-purple-600" />
            <span>视频上传队列</span>
            <span className="text-sm font-normal text-gray-500">({videoUploadTasks.length} 个待上传)</span>
          </h3>
        </div>

        <div className="bg-white rounded-lg border border-gray-200">
          {videoUploadTasks.length === 0 ? (
            <div className="p-12 text-center text-gray-500">
              <Upload className="w-12 h-12 mx-auto mb-3 text-gray-300" />
              <p>暂无待上传的视频</p>
              <p className="text-sm mt-1">所有准备就绪的视频将显示在这里</p>
            </div>
          ) : (
            <div className="divide-y divide-gray-200">
              {paginatedVideoTasks.map(task => {
                const taskKey = `${task.id}-video`;
                const isExecuting = executingTasks.has(taskKey);
                
                return (
                  <div key={task.id} className="p-4 hover:bg-gray-50 transition-colors">
                    <div className="flex items-center justify-between">
                      <div className="flex-1">
                        <div className="flex items-center space-x-3 mb-2">
                          <h4 className="font-medium text-gray-900">
                            {task.video.title || task.video.video_id}
                          </h4>
                          <span className="text-xs px-2 py-1 rounded bg-green-100 text-green-700">
                            准备就绪
                          </span>
                        </div>
                        <div className="flex items-center space-x-6 text-xs text-gray-500">
                          <span>视频ID: {task.video.video_id}</span>
                          <span>准备完成: {new Date(task.video.updated_at).toLocaleString('zh-CN')}</span>
                        </div>
                      </div>
                      <button
                        onClick={() => handleManualUpload(task.id, 'video')}
                        disabled={isExecuting}
                        className="ml-4 flex items-center space-x-2 px-4 py-2 text-sm text-white bg-purple-600 hover:bg-purple-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {isExecuting ? (
                          <>
                            <RefreshCw className="w-4 h-4 animate-spin" />
                            <span>上传中...</span>
                          </>
                        ) : (
                          <>
                            <Play className="w-4 h-4" />
                            <span>立即上传</span>
                          </>
                        )}
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
        {totalVideoPages > 1 && (
          <PaginationControls
            currentPage={videoPage}
            totalPages={totalVideoPages}
            onPageChange={setVideoPage}
          />
        )}
      </div>

      {/* 字幕上传任务 */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900 flex items-center space-x-2">
            <Upload className="w-5 h-5 text-indigo-600" />
            <span>字幕上传队列</span>
            <span className="text-sm font-normal text-gray-500">({subtitleUploadTasks.length} 个待处理)</span>
          </h3>
        </div>

        <div className="bg-white rounded-lg border border-gray-200">
          {subtitleUploadTasks.length === 0 ? (
            <div className="p-12 text-center text-gray-500">
              <Upload className="w-12 h-12 mx-auto mb-3 text-gray-300" />
              <p>暂无待上传的字幕</p>
              <p className="text-sm mt-1">视频上传完成1小时后，字幕任务将显示在这里</p>
            </div>
          ) : (
            <div className="divide-y divide-gray-200">
              {paginatedSubtitleTasks.map(task => {
                const taskKey = `${task.id}-subtitle`;
                const isExecuting = executingTasks.has(taskKey);
                
                return (
                  <div key={task.id} className="p-4 hover:bg-gray-50 transition-colors">
                    <div className="flex items-center justify-between">
                      <div className="flex-1">
                        <div className="flex items-center space-x-3 mb-2">
                          <h4 className="font-medium text-gray-900">
                            {task.video.title || task.video.video_id}
                          </h4>
                          <span className={`text-xs px-2 py-1 rounded ${task.video.status === '399' ? 'bg-red-100 text-red-700' : 'bg-cyan-100 text-cyan-700'}`}>
                            {task.video.status === '399' ? '字幕上传失败' : '视频已上传'}
                          </span>
                          {task.video.bili_bvid && (
                            <a
                              href={`https://www.bilibili.com/video/${task.video.bili_bvid}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-xs text-blue-600 hover:underline"
                            >
                              {task.video.bili_bvid}
                            </a>
                          )}
                        </div>
                        <div className="flex items-center space-x-6 text-xs text-gray-500">
                          <span>视频ID: {task.video.video_id}</span>
                          <span>视频上传: {new Date(task.video.updated_at).toLocaleString('zh-CN')}</span>
                        </div>
                      </div>
                      <button
                        onClick={() => handleManualUpload(task.id, 'subtitle')}
                        disabled={isExecuting}
                        className={`ml-4 flex items-center space-x-2 px-4 py-2 text-sm text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${task.video.status === '399' ? 'bg-red-600 hover:bg-red-700' : 'bg-indigo-600 hover:bg-indigo-700'}`}
                      >
                        {isExecuting ? (
                          <>
                            <RefreshCw className="w-4 h-4 animate-spin" />
                            <span>上传中...</span>
                          </>
                        ) : (
                          <>
                            <Play className="w-4 h-4" />
                            <span>{task.video.status === '399' ? '重试上传字幕' : '立即上传字幕'}</span>
                          </>
                        )}
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
        {totalSubtitlePages > 1 && (
          <PaginationControls
            currentPage={subtitlePage}
            totalPages={totalSubtitlePages}
            onPageChange={setSubtitlePage}
          />
        )}
      </div>

      {/* 统计信息 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-green-100 rounded-lg">
              <CheckCircle className="w-5 h-5 text-green-600" />
            </div>
            <div>
              <div className="text-sm text-gray-600">准备就绪</div>
              <div className="text-xl font-bold text-gray-900">{videoUploadTasks.length}</div>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-cyan-100 rounded-lg">
              <Upload className="w-5 h-5 text-cyan-600" />
            </div>
            <div>
              <div className="text-sm text-gray-600">等待上传字幕</div>
              <div className="text-xl font-bold text-gray-900">{subtitleUploadTasks.length}</div>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-lg border border-gray-200 p-4">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-blue-100 rounded-lg">
              <Calendar className="w-5 h-5 text-blue-600" />
            </div>
            <div>
              <div className="text-sm text-gray-600">总待处理任务</div>
              <div className="text-xl font-bold text-gray-900">
                {videoUploadTasks.length + subtitleUploadTasks.length}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 使用说明 */}
      <div className="bg-gray-50 rounded-lg p-4 border border-gray-200">
        <h4 className="font-medium text-gray-900 mb-3">使用说明</h4>
        <div className="text-sm text-gray-600 space-y-2">
          <p>1. <strong>自动上传</strong>：系统每5分钟检查一次，按照策略自动执行上传任务</p>
          <p>2. <strong>手动上传</strong>：点击&quot;立即上传&quot;按钮可以立即触发上传，无需等待自动调度</p>
          <p>3. <strong>上传限制</strong>：为避免被限流，建议手动上传时注意频率控制</p>
          <p>4. <strong>任务状态</strong>：上传完成后，任务会自动从队列中移除</p>
        </div>
      </div>
    </div>
  );
}

const PaginationControls = ({ currentPage, totalPages, onPageChange }: { currentPage: number, totalPages: number, onPageChange: (page: number) => void }) => {
  return (
    <div className="flex justify-center items-center space-x-2 mt-4">
      <button
        onClick={() => onPageChange(currentPage - 1)}
        disabled={currentPage === 1}
        className="px-3 py-1 text-sm text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
      >
        上一页
      </button>
      <span className="text-sm text-gray-700">
        第 {currentPage} 页 / 共 {totalPages} 页
      </span>
      <button
        onClick={() => onPageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
        className="px-3 py-1 text-sm text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
      >
        下一页
      </button>
    </div>
  );
};
