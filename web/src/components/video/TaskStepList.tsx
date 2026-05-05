'use client';

import { useState } from 'react';
import { Play, RotateCcw, Clock, CheckCircle, XCircle, AlertCircle, Loader2 } from 'lucide-react';
import { TaskStep, TaskStepStatus, TASK_STEP_STATUS_MAP, TASK_STEP_NAMES } from '@/types';

interface TaskStepListProps {
  steps: TaskStep[];
  onRetryStep: (stepName: string) => Promise<void>;
  isRetrying?: boolean;
}

export default function TaskStepList({ steps, onRetryStep, isRetrying = false }: TaskStepListProps) {
  const [retryingStep, setRetryingStep] = useState<string | null>(null);

  const getStepIcon = (status: TaskStepStatus) => {
    const className = "w-5 h-5";
    
    switch (status) {
      case 'pending':
        return <Clock className={`${className} text-gray-500`} />;
      case 'running':
        return <Loader2 className={`${className} text-blue-500 animate-spin`} />;
      case 'completed':
        return <CheckCircle className={`${className} text-green-500`} />;
      case 'failed':
        return <XCircle className={`${className} text-red-500`} />;
      case 'skipped':
        return <AlertCircle className={`${className} text-yellow-500`} />;
      default:
        return <Clock className={`${className} text-gray-500`} />;
    }
  };

  const formatDuration = (duration?: number) => {
    if (!duration) return '-';
    
    const seconds = Math.floor(duration / 1000);
    const minutes = Math.floor(seconds / 60);
    
    if (minutes > 0) {
      return `${minutes}分${seconds % 60}秒`;
    }
    return `${seconds}秒`;
  };

  const formatTime = (timeString?: string) => {
    if (!timeString) return '-';
    
    const date = new Date(timeString);
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  };

  const handleRetry = async (stepName: string) => {
    if (retryingStep || isRetrying) return;
    
    setRetryingStep(stepName);
    try {
      await onRetryStep(stepName);
    } finally {
      setRetryingStep(null);
    }
  };

  const canRetryStep = (step: TaskStep) => {
    return step.can_retry && (step.status === 'failed' || step.status === 'skipped');
  };

  const getStepProgress = (step: TaskStep) => {
    if (step.status === 'completed') return 100;
    const progress = Number(step.progress_percent ?? 0);
    return Math.max(0, Math.min(100, Math.round(progress)));
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

  // 按步骤顺序排序
  const sortedSteps = [...steps].sort((a, b) => a.step_order - b.step_order);

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200">
      <div className="px-6 py-4 border-b border-gray-200">
        <h3 className="text-lg font-semibold text-gray-900">任务步骤</h3>
        <p className="text-sm text-gray-500 mt-1">
          视频处理任务链的执行状态
        </p>
      </div>

      <div className="divide-y divide-gray-200">
        {sortedSteps.map((step, index) => {
          const statusInfo = TASK_STEP_STATUS_MAP[step.status] || TASK_STEP_STATUS_MAP['pending'];
          const stepName = TASK_STEP_NAMES[step.step_name as keyof typeof TASK_STEP_NAMES] || step.step_name;
          const isCurrentlyRetrying = retryingStep === step.step_name;
          const stepProgress = getStepProgress(step);
          const progressFillClass = step.status === 'completed'
            ? 'bg-green-500'
            : step.status === 'failed'
              ? 'bg-red-500'
              : step.status === 'pending'
                ? 'bg-gray-300'
                : 'bg-blue-500';

          return (
            <div key={step.id} className="px-6 py-4 hover:bg-gray-50 transition-colors">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-4 flex-1">
                  {/* 步骤序号 */}
                  <div className="flex-shrink-0 w-8 h-8 bg-gray-100 rounded-full flex items-center justify-center text-sm font-medium text-gray-600">
                    {step.step_order}
                  </div>

                  {/* 状态图标 */}
                  <div className="flex-shrink-0">
                    {isCurrentlyRetrying ? (
                      <Loader2 className="w-5 h-5 text-blue-500 animate-spin" />
                    ) : (
                      getStepIcon(step.status)
                    )}
                  </div>

                  {/* 步骤信息 */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center space-x-3">
                      <h4 className="text-sm font-medium text-gray-900">
                        {stepName}
                      </h4>
                      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusInfo.className}`}>
                        {isCurrentlyRetrying ? '重试中...' : statusInfo.label}
                      </span>
                    </div>

                    {/* 时间信息 */}
                    <div className="flex items-center space-x-4 mt-1 text-xs text-gray-500">
                      {step.start_time && (
                        <span>开始: {formatTime(step.start_time)}</span>
                      )}
                      {step.end_time && (
                        <span>结束: {formatTime(step.end_time)}</span>
                      )}
                      <span>耗时: {formatDuration(step.duration)}</span>
                    </div>

                    <div className="mt-3 max-w-xl">
                      <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
                        <span className="truncate pr-3">
                          {getStepProgressMessage(step)}
                        </span>
                        <span className="font-medium text-gray-700">{stepProgress}%</span>
                      </div>
                      <div className="h-1.5 w-full bg-gray-200 rounded-full overflow-hidden">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ${progressFillClass}`}
                          style={{ width: `${stepProgress}%` }}
                        ></div>
                      </div>
                    </div>

                    {/* 错误信息 */}
                    {step.error_msg && (
                      <div className="mt-2 p-2 bg-red-50 border border-red-200 rounded text-xs text-red-700">
                        {step.error_msg}
                      </div>
                    )}

                    {/* 结果信息 */}
                    {step.result_data && step.status === 'completed' && (
                      <div className="mt-2 text-xs text-gray-600">
                        <details className="cursor-pointer">
                          <summary className="font-medium hover:text-gray-800">
                            查看结果详情
                          </summary>
                          <pre className="mt-1 p-2 bg-gray-50 border border-gray-200 rounded overflow-x-auto whitespace-pre-wrap">
                            {typeof step.result_data === 'string' 
                              ? step.result_data 
                              : JSON.stringify(step.result_data, null, 2)}
                          </pre>
                        </details>
                      </div>
                    )}
                  </div>
                </div>

                {/* 重试按钮 */}
                <div className="flex-shrink-0 ml-4">
                  {canRetryStep(step) && (
                    <button
                      onClick={() => handleRetry(step.step_name)}
                      disabled={isCurrentlyRetrying || retryingStep !== null}
                      className="inline-flex items-center px-3 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {isCurrentlyRetrying ? (
                        <>
                          <Loader2 className="w-3 h-3 mr-1 animate-spin" />
                          重试中
                        </>
                      ) : (
                        <>
                          <RotateCcw className="w-3 h-3 mr-1" />
                          重试
                        </>
                      )}
                    </button>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* 空状态 */}
      {sortedSteps.length === 0 && (
        <div className="px-6 py-8 text-center">
          <Play className="w-12 h-12 mx-auto text-gray-400 mb-4" />
          <p className="text-gray-500">暂无任务步骤</p>
          <p className="text-sm text-gray-400 mt-1">
            任务尚未开始或步骤信息未加载
          </p>
        </div>
      )}
    </div>
  );
}
