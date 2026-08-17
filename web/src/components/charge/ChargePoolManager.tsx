'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  ArrowDown,
  ArrowUp,
  CheckCircle2,
  Clock3,
  Coins,
  Loader2,
  Play,
  RefreshCw,
  RotateCw,
  Shuffle,
  Trash2,
  Upload,
} from 'lucide-react';
import {
  ChargePoolSummary,
  ChargeTier,
  CompilationBatch,
  CompilationState,
} from '@/types';
import PreviewDurationSelect from '@/components/common/PreviewDurationSelect';

interface ApiEnvelope<T> {
  code: number;
  message: string;
  data: T;
}

const ACTIVE_STATES: CompilationState[] = [
  'queued',
  'merging',
  'processing',
  'upload_queued',
  'uploading',
];
const FAILED_STATES: CompilationState[] = ['merge_failed', 'processing_failed', 'upload_failed'];

const STATE_META: Record<CompilationState, { label: string; className: string; progress: number }> = {
  draft: { label: '待确认草稿', className: 'bg-amber-100 text-amber-800', progress: 5 },
  queued: { label: '等待拼接', className: 'bg-blue-100 text-blue-800', progress: 10 },
  merging: { label: '正在规范化拼接', className: 'bg-blue-100 text-blue-800', progress: 35 },
  processing: { label: '字幕/翻译/元数据处理中', className: 'bg-indigo-100 text-indigo-800', progress: 65 },
  ready: { label: '成片就绪', className: 'bg-green-100 text-green-800', progress: 90 },
  upload_queued: { label: '等待上传', className: 'bg-purple-100 text-purple-800', progress: 92 },
  uploading: { label: '正在上传', className: 'bg-purple-100 text-purple-800', progress: 96 },
  uploaded: { label: '已上传', className: 'bg-emerald-100 text-emerald-800', progress: 100 },
  merge_failed: { label: '拼接失败', className: 'bg-red-100 text-red-800', progress: 35 },
  processing_failed: { label: '处理失败', className: 'bg-red-100 text-red-800', progress: 65 },
  upload_failed: { label: '上传失败', className: 'bg-red-100 text-red-800', progress: 92 },
  cancelled: { label: '已取消', className: 'bg-gray-100 text-gray-600', progress: 0 },
};

const formatDuration = (milliseconds = 0) => {
  const seconds = Math.max(0, Math.round(milliseconds / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
    : `${minutes}:${String(remainder).padStart(2, '0')}`;
};

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const payload = await response.json() as ApiEnvelope<T>;
  if (!response.ok || ![0, 200, 201].includes(payload.code)) {
    throw new Error(payload.message || '请求失败');
  }
  return payload.data;
}

export default function ChargePoolManager() {
  const [summaries, setSummaries] = useState<ChargePoolSummary[]>([]);
  const [batches, setBatches] = useState<CompilationBatch[]>([]);
  const [draft, setDraft] = useState<CompilationBatch | null>(null);
  const [previewSeconds, setPreviewSeconds] = useState(180);
  const [uploadPolicy, setUploadPolicy] = useState<'immediate' | 'manual'>('immediate');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [activeRequest, setActiveRequest] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const fetchWorkspace = useCallback(async (showSpinner = false) => {
    if (showSpinner) setRefreshing(true);
    try {
      const [nextSummaries, nextBatches] = await Promise.all([
        apiRequest<ChargePoolSummary[]>('/api/v1/charge/pools/summary'),
        apiRequest<CompilationBatch[]>('/api/v1/charge/batches?limit=30'),
      ]);
      setSummaries(nextSummaries);
      setBatches(nextBatches);
      setDraft((current) => {
        if (!current) return null;
        return nextBatches.find((batch) => batch.id === current.id && batch.state === 'draft') || null;
      });
      setError('');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '加载充电池失败');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  const hasActiveBatch = useMemo(
    () => batches.some((batch) => ACTIVE_STATES.includes(batch.state)),
    [batches],
  );

  useEffect(() => {
    fetchWorkspace();
  }, [fetchWorkspace]);

  useEffect(() => {
    const interval = window.setInterval(
      () => fetchWorkspace(false),
      hasActiveBatch ? 5000 : 15000,
    );
    return () => window.clearInterval(interval);
  }, [fetchWorkspace, hasActiveBatch]);

  const runAction = async (key: string, action: () => Promise<void>) => {
    setActiveRequest(key);
    setError('');
    setNotice('');
    try {
      await action();
      await fetchWorkspace(false);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '操作失败');
    } finally {
      setActiveRequest('');
    }
  };

  const createDraft = (tier: ChargeTier) => runAction(`draft-${tier}`, async () => {
    const nextDraft = await apiRequest<CompilationBatch>('/api/v1/charge/batches/draft', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tier }),
    });
    setDraft(nextDraft);
    setPreviewSeconds(nextDraft.preview_seconds || 180);
    setUploadPolicy(nextDraft.upload_policy || 'immediate');
    setNotice(`已从${tier}元素材池随机选取${nextDraft.actual_count}个视频。`);
  });

  const rerollDraft = () => {
    if (!draft) return Promise.resolve();
    return runAction('reroll', async () => {
      const nextDraft = await apiRequest<CompilationBatch>(`/api/v1/charge/batches/${draft.id}/reroll`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      });
      setDraft(nextDraft);
      setPreviewSeconds(nextDraft.preview_seconds || previewSeconds);
      setNotice('已释放原草稿并重新随机选片。');
    });
  };

  const moveDraftItem = (index: number, direction: -1 | 1) => {
    if (!draft) return Promise.resolve();
    const destination = index + direction;
    if (destination < 0 || destination >= draft.items.length) return Promise.resolve();
    const items = [...draft.items];
    [items[index], items[destination]] = [items[destination], items[index]];
    const sourceVideoIds = items.map((item) => item.source_saved_video_id);
    return runAction(`order-${index}-${direction}`, async () => {
      const updated = await apiRequest<CompilationBatch>(`/api/v1/charge/batches/${draft.id}/order`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source_video_ids: sourceVideoIds }),
      });
      setDraft(updated);
      setNotice('拼接顺序已保存。');
    });
  };

  const cancelDraft = () => {
    if (!draft || !window.confirm('取消草稿后，已选素材会全部释放回素材池。确定继续吗？')) {
      return Promise.resolve();
    }
    return runAction('cancel', async () => {
      await apiRequest<never>(`/api/v1/charge/batches/${draft.id}/cancel`, { method: 'POST' });
      setDraft(null);
      setNotice('草稿已取消，素材已释放。');
    });
  };

  const startDraft = () => {
    if (!draft) return Promise.resolve();
    return runAction('start', async () => {
      await apiRequest<CompilationBatch>(`/api/v1/charge/batches/${draft.id}/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          preview_seconds: previewSeconds,
          upload_policy: uploadPolicy,
        }),
      });
      setDraft(null);
      setNotice('批次已进入拼接处理队列。');
    });
  };

  const retryBatch = (batch: CompilationBatch) => runAction(`retry-${batch.id}`, async () => {
    await apiRequest<CompilationBatch>(`/api/v1/charge/batches/${batch.id}/retry`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        preview_seconds: batch.preview_seconds || 180,
        upload_policy: batch.upload_policy || 'immediate',
      }),
    });
    setNotice(batch.state === 'upload_failed' ? '已复用成片重新加入上传队列。' : '批次已重新加入处理队列。');
  });

  const uploadReadyBatch = (batch: CompilationBatch) => runAction(`upload-${batch.id}`, async () => {
    const videoId = batch.output_saved_video?.video_id;
    if (!videoId) throw new Error('批次输出视频尚未生成');
    await apiRequest<unknown>(`/api/v1/videos/${videoId}/upload/video`, { method: 'POST' });
    setNotice('成片已加入立即上传队列。');
  });

  if (loading) {
    return (
      <div className="flex min-h-48 items-center justify-center rounded-lg border border-gray-200 bg-white">
        <Loader2 className="mr-2 h-5 w-5 animate-spin text-pink-600" />
        <span className="text-sm text-gray-600">正在加载充电素材池...</span>
      </div>
    );
  }

  return (
    <section className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-xl font-bold text-gray-900">
            <Coins className="h-5 w-5 text-pink-600" />
            充电视频拼接
          </h2>
          <p className="mt-1 text-sm text-gray-500">30元与50元素材完全隔离；每次随机锁定3至4个视频，确认后才开始拼接。</p>
        </div>
        <button
          type="button"
          onClick={() => fetchWorkspace(true)}
          disabled={refreshing}
          className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 disabled:opacity-50"
        >
          <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
          刷新
        </button>
      </div>

      {notice && (
        <div className="flex items-start gap-2 rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-800">
          <CheckCircle2 className="mt-0.5 h-4 w-4 flex-shrink-0" /> {notice}
        </div>
      )}
      {error && (
        <div className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" /> {error}
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        {([30, 50] as ChargeTier[]).map((tier) => {
          const summary = summaries.find((item) => item.tier === tier)
            || { tier, available: 0, reserved: 0, consumed: 0, total: 0 };
          const busy = activeRequest === `draft-${tier}`;
          return (
            <div key={tier} className="rounded-xl border border-pink-100 bg-gradient-to-br from-white to-pink-50 p-5">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="text-lg font-bold text-gray-900">{tier}元素材池</p>
                  <p className="mt-1 text-xs text-gray-500">可用素材达到3个即可创建随机草稿</p>
                </div>
                <span className="rounded-full bg-pink-100 px-3 py-1 text-sm font-semibold text-pink-700">
                  可用 {summary.available}
                </span>
              </div>
              <div className="my-4 grid grid-cols-3 gap-2 text-center">
                <div className="rounded-lg bg-white p-2"><p className="text-lg font-semibold">{summary.available}</p><p className="text-xs text-gray-500">可用</p></div>
                <div className="rounded-lg bg-white p-2"><p className="text-lg font-semibold">{summary.reserved}</p><p className="text-xs text-gray-500">已锁定</p></div>
                <div className="rounded-lg bg-white p-2"><p className="text-lg font-semibold">{summary.consumed}</p><p className="text-xs text-gray-500">已使用</p></div>
              </div>
              <button
                type="button"
                onClick={() => createDraft(tier)}
                disabled={summary.available < 3 || Boolean(activeRequest) || Boolean(draft)}
                className="flex w-full items-center justify-center gap-2 rounded-lg bg-pink-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-pink-700 disabled:cursor-not-allowed disabled:bg-gray-300"
              >
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Shuffle className="h-4 w-4" />}
                随机选取3至4个
              </button>
            </div>
          );
        })}
      </div>

      {draft && (
        <div className="rounded-xl border-2 border-amber-300 bg-amber-50/50 p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-lg font-bold text-gray-900">{draft.tier}元拼接草稿</h3>
              <p className="mt-1 text-xs text-gray-500">批次 {draft.batch_key} · 共 {draft.actual_count} 个素材</p>
            </div>
            <button
              type="button"
              onClick={rerollDraft}
              disabled={Boolean(activeRequest)}
              className="flex items-center gap-1 rounded-lg border border-amber-300 bg-white px-3 py-2 text-sm text-amber-800 hover:bg-amber-100 disabled:opacity-50"
            >
              <RotateCw className="h-4 w-4" /> 重新随机
            </button>
          </div>

          <ol className="my-4 space-y-2">
            {draft.items.map((item, index) => (
              <li key={item.id} className="flex items-center gap-3 rounded-lg border border-gray-200 bg-white p-3">
                <span className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-pink-100 text-sm font-bold text-pink-700">
                  {index + 1}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-gray-900">
                    {item.source_video?.title || item.source_video?.video_id || `素材 #${item.source_saved_video_id}`}
                  </p>
                  <p className="mt-0.5 text-xs text-gray-500">
                    时长 {formatDuration(item.source_duration_ms || item.source_video?.media_duration_ms)}
                  </p>
                </div>
                <div className="flex gap-1">
                  <button
                    type="button"
                    aria-label="上移素材"
                    onClick={() => moveDraftItem(index, -1)}
                    disabled={index === 0 || Boolean(activeRequest)}
                    className="rounded p-2 text-gray-500 hover:bg-gray-100 disabled:opacity-25"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    aria-label="下移素材"
                    onClick={() => moveDraftItem(index, 1)}
                    disabled={index === draft.items.length - 1 || Boolean(activeRequest)}
                    className="rounded p-2 text-gray-500 hover:bg-gray-100 disabled:opacity-25"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </button>
                </div>
              </li>
            ))}
          </ol>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="text-sm text-gray-700">
              <span className="block">试看时长</span>
              <div className="mt-1">
                <PreviewDurationSelect
                  value={previewSeconds}
                  onChange={setPreviewSeconds}
                  disabled={Boolean(activeRequest)}
                />
              </div>
            </div>
            <label className="text-sm text-gray-700">
              成片处理完成后
              <select
                value={uploadPolicy}
                onChange={(event) => setUploadPolicy(event.target.value as 'immediate' | 'manual')}
                className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2"
              >
                <option value="immediate">立即加入上传队列</option>
                <option value="manual">等待手动上传</option>
              </select>
            </label>
          </div>

          <div className="mt-4 flex flex-wrap justify-end gap-2">
            <button
              type="button"
              onClick={cancelDraft}
              disabled={Boolean(activeRequest)}
              className="flex items-center gap-1 rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              <Trash2 className="h-4 w-4" /> 取消并释放
            </button>
            <button
              type="button"
              onClick={startDraft}
              disabled={Boolean(activeRequest) || previewSeconds < 1 || previewSeconds > 21600}
              className="flex items-center gap-2 rounded-lg bg-blue-600 px-5 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {activeRequest === 'start' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
              确认并开始拼接
            </button>
          </div>
        </div>
      )}

      <div className="space-y-3">
        <h3 className="text-base font-semibold text-gray-900">最近拼接批次</h3>
        {batches.length === 0 ? (
          <div className="rounded-lg border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500">
            暂无拼接批次
          </div>
        ) : batches.map((batch) => {
          const meta = STATE_META[batch.state];
          return (
            <article key={batch.id} className="rounded-lg border border-gray-200 bg-white p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-gray-900">{batch.tier}元 · {batch.actual_count}段拼接</p>
                    <span className={`rounded-full px-2 py-0.5 text-xs ${meta.className}`}>{meta.label}</span>
                  </div>
                  <p className="mt-1 truncate text-xs text-gray-500">
                    {batch.batch_key} · {new Date(batch.created_at).toLocaleString('zh-CN')}
                    {batch.total_duration_ms > 0 ? ` · 成片 ${formatDuration(batch.total_duration_ms)}` : ''}
                  </p>
                </div>
                <div className="flex gap-2">
                  {FAILED_STATES.includes(batch.state) && (
                    <button
                      type="button"
                      onClick={() => retryBatch(batch)}
                      disabled={Boolean(activeRequest)}
                      className="flex items-center gap-1 rounded-lg bg-red-50 px-3 py-2 text-xs font-medium text-red-700 hover:bg-red-100 disabled:opacity-50"
                    >
                      {activeRequest === `retry-${batch.id}` ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RotateCw className="h-3.5 w-3.5" />}
                      重试
                    </button>
                  )}
                  {batch.state === 'ready' && batch.upload_policy === 'manual' && (
                    <button
                      type="button"
                      onClick={() => uploadReadyBatch(batch)}
                      disabled={Boolean(activeRequest)}
                      className="flex items-center gap-1 rounded-lg bg-purple-600 px-3 py-2 text-xs font-medium text-white hover:bg-purple-700 disabled:opacity-50"
                    >
                      {activeRequest === `upload-${batch.id}` ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
                      立即上传
                    </button>
                  )}
                </div>
              </div>
              <div className="mt-3 h-2 overflow-hidden rounded-full bg-gray-100">
                <div
                  className={`h-full rounded-full transition-all ${FAILED_STATES.includes(batch.state) ? 'bg-red-500' : 'bg-pink-500'}`}
                  style={{ width: `${meta.progress}%` }}
                />
              </div>
              {batch.last_error && (
                <div className="mt-3 flex items-start gap-2 rounded-md bg-red-50 p-2 text-xs text-red-700">
                  <AlertCircle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0" /> {batch.last_error}
                </div>
              )}
              {ACTIVE_STATES.includes(batch.state) && (
                <p className="mt-2 flex items-center gap-1 text-xs text-gray-500">
                  <Clock3 className="h-3.5 w-3.5" /> 页面会每5秒自动刷新进度
                </p>
              )}
            </article>
          );
        })}
      </div>
    </section>
  );
}
