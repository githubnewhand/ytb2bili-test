'use client';

import { useState } from 'react';
import {
  AlertCircle,
  CheckCircle,
  ExternalLink,
  FileText,
  Loader2,
  LockKeyhole,
  Upload,
} from 'lucide-react';
import { PublishAudience } from '@/types';
import PreviewDurationSelect from '@/components/common/PreviewDurationSelect';

interface VideoActionsProps {
  videoId: string;
  status: string;
  biliBvid?: string;
  biliAid?: number;
  publishAudience?: PublishAudience;
  audienceSelectedAt?: string;
  upowerPreviewSeconds?: number;
  recordType?: 'source' | 'compilation';
  workflowState?: string;
  scheduledUploadAt?: string;
  rightsVerified?: boolean;
  onSuccess?: () => void;
}

const AUDIENCE_OPTIONS: Array<{
  value: Exclude<PublishAudience, ''>;
  label: string;
  description: string;
}> = [
  { value: 'free', label: '免费公开', description: '正常处理，准备完成1小时后自动上传，也可手动立即上传。' },
  { value: 'charge_30', label: '30元充电素材', description: '下载后进入30元素材池，不会单独上传。' },
  { value: 'charge_50', label: '50元充电素材', description: '下载后进入50元素材池，不会单独上传。' },
];

export default function VideoActions({
  videoId,
  status,
  biliBvid,
  biliAid,
  publishAudience = '',
  audienceSelectedAt,
  upowerPreviewSeconds = 180,
  recordType = 'source',
  workflowState = '',
  scheduledUploadAt,
  rightsVerified = false,
  onSuccess,
}: VideoActionsProps) {
  const [uploading, setUploading] = useState(false);
  const [selecting, setSelecting] = useState(false);
  const [selectedAudience, setSelectedAudience] = useState<PublishAudience>(publishAudience);
  const [previewSeconds, setPreviewSeconds] = useState(
    upowerPreviewSeconds > 0 ? upowerPreviewSeconds : 180,
  );
  const [hasRights, setHasRights] = useState(rightsVerified);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const alreadyUploaded = Boolean(biliBvid || biliAid || ['300', '301', '399', '400'].includes(status));
  const isChargeSource = recordType === 'source' && ['charge_30', 'charge_50'].includes(selectedAudience);
  const canClassify = recordType !== 'compilation'
    && !selectedAudience
    && ['100', '200', '299'].includes(status);
  const canUploadVideo = ['200', '299'].includes(status)
    && !alreadyUploaded
    && !isChargeSource
    && (recordType === 'compilation' || selectedAudience === 'free');
  const canUploadSubtitle = ['300', '399'].includes(status);
  const isPoolItem = workflowState === 'pool_available' || workflowState === 'pool_reserved' || status === '110';

  const handleSelectAudience = async (audience: Exclude<PublishAudience, ''>) => {
    const isCharge = audience !== 'free';
    if (isCharge && !hasRights) {
      setError('选择充电档位前，请先确认拥有视频版权或完整授权');
      return;
    }

    setSelecting(true);
    setError(null);
    setSuccess(null);
    try {
      const response = await fetch(`/api/v1/videos/${videoId}/publish-audience`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          audience,
          preview_seconds: isCharge ? previewSeconds : 0,
          rights_verified: isCharge ? hasRights : false,
        }),
      });
      const data = await response.json();
      if (!response.ok || data.code !== 200) {
        throw new Error(data.message || '保存发布方式失败');
      }
      setSelectedAudience(audience);
      setSuccess(
        audience === 'free'
          ? '已选择免费公开，系统将继续处理并在准备完成1小时后自动上传。'
          : `已进入${audience === 'charge_30' ? '30元' : '50元'}充电素材池。`,
      );
      onSuccess?.();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '网络错误，请重试');
    } finally {
      setSelecting(false);
    }
  };

  const handleManualUpload = async (taskType: 'video' | 'subtitle') => {
    if (taskType === 'video' && alreadyUploaded) {
      setError('视频已经上传到Bilibili，不能重复上传');
      return;
    }
    if (!window.confirm(taskType === 'video' ? '确定将该视频加入立即上传队列吗？' : '确定立即上传字幕吗？')) {
      return;
    }

    setUploading(true);
    setError(null);
    setSuccess(null);
    try {
      const response = await fetch(`/api/v1/videos/${videoId}/upload/${taskType}`, { method: 'POST' });
      const data = await response.json();
      if (!response.ok || data.code !== 200) {
        throw new Error(data.message || '创建上传任务失败');
      }
      setSuccess(taskType === 'video' ? '已加入立即上传队列，请勿重复操作。' : '字幕上传任务已启动。');
      window.setTimeout(() => onSuccess?.(), 1500);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '网络错误，请重试');
    } finally {
      setUploading(false);
    }
  };

  if (!canClassify && !canUploadVideo && !canUploadSubtitle && !alreadyUploaded && !isPoolItem) {
    return null;
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="mb-3 flex items-center text-sm font-medium text-gray-900">
        <Upload className="mr-2 h-4 w-4" />
        发布操作
      </h3>

      {success && (
        <div className="mb-3 flex items-start gap-2 rounded-lg border border-green-200 bg-green-50 p-3">
          <CheckCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-green-600" />
          <span className="text-sm text-green-800">{success}</span>
        </div>
      )}
      {error && (
        <div className="mb-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3">
          <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red-600" />
          <span className="text-sm text-red-800">{error}</span>
        </div>
      )}

      {alreadyUploaded && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-3">
          <p className="text-sm font-medium text-green-900">视频已上传，系统已阻止重复投稿。</p>
          {biliBvid && (
            <a
              href={`https://www.bilibili.com/video/${biliBvid}`}
              target="_blank"
              rel="noopener noreferrer"
              className="mt-1 inline-flex items-center gap-1 text-sm text-green-700 underline"
            >
              打开B站稿件 <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      )}

      {canClassify && (
        <div className="space-y-3">
          <div>
            <p className="text-sm font-medium text-gray-900">下载已完成，请选择发布方式</p>
            <p className="mt-1 text-xs text-gray-500">选择后立即分流；进入拼接批次后将不能更改档位。</p>
          </div>
          <div className="grid gap-2 md:grid-cols-3">
            {AUDIENCE_OPTIONS.map((option) => (
              <button
                key={option.value}
                type="button"
                disabled={selecting}
                onClick={() => handleSelectAudience(option.value)}
                className="rounded-lg border border-gray-200 p-3 text-left transition hover:border-blue-400 hover:bg-blue-50 disabled:opacity-50"
              >
                <span className="block text-sm font-medium text-gray-900">{option.label}</span>
                <span className="mt-1 block text-xs leading-5 text-gray-500">{option.description}</span>
              </button>
            ))}
          </div>
          <div className="rounded-lg border border-amber-200 bg-amber-50 p-3">
            <label className="flex items-start gap-2 text-sm text-amber-950">
              <input
                type="checkbox"
                checked={hasRights}
                onChange={(event) => setHasRights(event.target.checked)}
                className="mt-1"
              />
              <span>我确认充电素材为本人原创，或已取得允许付费传播的完整授权。</span>
            </label>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-amber-900">
              <span>默认试看</span>
              <PreviewDurationSelect
                value={previewSeconds}
                onChange={setPreviewSeconds}
                disabled={selecting}
              />
              <span>仅充电视频使用，最长6小时</span>
            </div>
          </div>
        </div>
      )}

      {isPoolItem && (
        <div className="flex items-start gap-2 rounded-lg border border-pink-200 bg-pink-50 p-3">
          <LockKeyhole className="mt-0.5 h-4 w-4 flex-shrink-0 text-pink-600" />
          <div>
            <p className="text-sm font-medium text-pink-900">
              {workflowState === 'pool_reserved' ? '素材已被拼接草稿锁定' : '素材正在充电池中等待选取'}
            </p>
            <p className="mt-1 text-xs text-pink-700">请前往页面上方“充电视频拼接”管理批次，不支持单视频投稿。</p>
          </div>
        </div>
      )}

      {canUploadVideo && (
        <div className="space-y-2">
          {scheduledUploadAt && recordType !== 'compilation' && (
            <p className="text-xs text-gray-500">计划自动上传：{scheduledUploadAt}</p>
          )}
          <button
            type="button"
            onClick={() => handleManualUpload('video')}
            disabled={uploading || selecting}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
            立即上传视频
          </button>
        </div>
      )}

      {canUploadSubtitle && (
        <button
          type="button"
          onClick={() => handleManualUpload('subtitle')}
          disabled={uploading}
          className="mt-2 flex w-full items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-white transition hover:bg-indigo-700 disabled:opacity-50"
        >
          {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileText className="h-4 w-4" />}
          立即上传字幕
        </button>
      )}

      {audienceSelectedAt && selectedAudience && (
        <p className="mt-3 text-xs text-gray-400">发布方式选择时间：{audienceSelectedAt}</p>
      )}
    </div>
  );
}
