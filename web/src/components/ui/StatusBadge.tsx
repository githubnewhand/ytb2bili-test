'use client';

import { Clock, RefreshCw, CheckCircle, XCircle, AlertCircle, Upload, FileText } from 'lucide-react';

const STATUS_CONFIG = {
  '001': {
    label: '待处理',
    className: 'status-pending',
    icon: Clock,
    description: '视频已提交，等待处理'
  },
  '002': {
    label: '处理中',
    className: 'status-processing',
    icon: RefreshCw,
    description: '正在下载和处理视频'
  },
  '200': {
    label: '准备就绪',
    className: 'status-ready',
    icon: CheckCircle,
    description: '视频准备完成，等待上传'
  },
  '201': {
    label: '上传视频中',
    className: 'status-uploading',
    icon: Upload,
    description: '正在上传视频到Bilibili'
  },
  '299': {
    label: '上传失败',
    className: 'status-upload-failed',
    icon: XCircle,
    description: '视频上传失败'
  },
  '300': {
    label: '视频已上传',
    className: 'status-video-uploaded',
    icon: CheckCircle,
    description: '视频上传成功，等待上传字幕'
  },
  '301': {
    label: '上传字幕中',
    className: 'status-uploading-subtitle',
    icon: FileText,
    description: '正在上传字幕到Bilibili'
  },
  '399': {
    label: '字幕上传失败',
    className: 'status-subtitle-failed',
    icon: XCircle,
    description: '字幕上传失败'
  },
  '400': {
    label: '全部完成',
    className: 'status-all-completed',
    icon: CheckCircle,
    description: '所有任务已完成'
  },
  '999': {
    label: '任务失败',
    className: 'status-failed',
    icon: XCircle,
    description: '准备阶段失败'
  },
} as const;

interface StatusBadgeProps {
  status: string;
  showDescription?: boolean;
}

export default function StatusBadge({ status, showDescription = false }: StatusBadgeProps) {
  const config = STATUS_CONFIG[status as keyof typeof STATUS_CONFIG];
  
  if (!config) {
    return (
      <span className="bg-gray-100 text-gray-800 px-2 py-1 rounded-full text-sm flex items-center space-x-1">
        <AlertCircle className="w-3 h-3" />
        <span>未知状态</span>
      </span>
    );
  }

  const Icon = config.icon;

  const isAnimating = ['002', '201', '301'].includes(status);

  return (
    <div className="flex flex-col">
      <span className={`${config.className} flex items-center space-x-1`}>
        <Icon className={`w-3 h-3 ${isAnimating ? 'animate-spin' : ''}`} />
        <span>{config.label}</span>
      </span>
      {showDescription && (
        <span className="text-xs text-gray-500 mt-1">
          {config.description}
        </span>
      )}
    </div>
  );
}

export { STATUS_CONFIG };