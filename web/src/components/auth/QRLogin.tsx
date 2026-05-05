'use client';

/* eslint-disable @next/next/no-img-element */

import { useCallback, useEffect, useRef, useState } from 'react';
import { CheckCircle, QrCode, RefreshCw, XCircle } from 'lucide-react';
import { getApiUrl, resolveApiResourceUrl } from '@/lib/apiBase';

interface LoginUser {
  id: string;
  name: string;
  mid: string;
  avatar?: string;
}

interface QRLoginProps {
  onLoginSuccess?: (user: LoginUser) => void;
  onRefreshStatus?: () => void | Promise<void>;
}

interface QRCodeResponse {
  code: number;
  message: string;
  qr_code_url?: string;
  auth_code?: string;
}

interface PollResponse {
  code: number;
  message: string;
  login_info?: {
    token_info?: {
      mid?: number;
      uname?: string;
      face?: string;
    };
  };
}

type QRLoginStatus = 'idle' | 'loading' | 'scanning' | 'success' | 'error';

const POLL_INTERVAL_MS = 3000;
const QR_EXPIRE_MS = 5 * 60 * 1000;

export default function QRLogin({ onLoginSuccess, onRefreshStatus }: QRLoginProps) {
  const [qrCodeUrl, setQrCodeUrl] = useState('');
  const [status, setStatus] = useState<QRLoginStatus>('idle');
  const [message, setMessage] = useState('准备生成二维码...');

  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const expireTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isPollingRef = useRef(false);

  const clearPolling = useCallback(() => {
    isPollingRef.current = false;

    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current);
      pollIntervalRef.current = null;
    }

    if (expireTimeoutRef.current) {
      clearTimeout(expireTimeoutRef.current);
      expireTimeoutRef.current = null;
    }
  }, []);

  const finishLogin = useCallback(async (loginInfo?: PollResponse['login_info']) => {
    const tokenInfo = loginInfo?.token_info;

    await onRefreshStatus?.();

    onLoginSuccess?.({
      id: tokenInfo?.mid?.toString() || '',
      name: tokenInfo?.uname || 'Bilibili 用户',
      mid: tokenInfo?.mid?.toString() || '',
      avatar: tokenInfo?.face || '',
    });
  }, [onLoginSuccess, onRefreshStatus]);

  const startPolling = useCallback((authCode: string) => {
    if (!authCode || isPollingRef.current) {
      return;
    }

    clearPolling();
    isPollingRef.current = true;

    pollIntervalRef.current = setInterval(async () => {
      try {
        const response = await fetch(getApiUrl('/auth/poll'), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ auth_code: authCode }),
        });
        const data = (await response.json()) as PollResponse;

        if (response.ok && data.code === 0 && data.login_info) {
          clearPolling();
          setStatus('success');
          setMessage('登录成功，正在刷新页面...');
          await finishLogin(data.login_info);
          return;
        }

        if (response.status === 400 || response.status === 500) {
          clearPolling();
          setStatus('error');
          setMessage(data.message || '二维码已过期，请重新生成');
        }
      } catch (error) {
        console.error('检查登录状态失败:', error);
      }
    }, POLL_INTERVAL_MS);

    expireTimeoutRef.current = setTimeout(() => {
      if (!isPollingRef.current) {
        return;
      }

      clearPolling();
      setStatus('error');
      setMessage('二维码已过期，请重新生成');
    }, QR_EXPIRE_MS);
  }, [clearPolling, finishLogin]);

  const generateQRCode = useCallback(async () => {
    clearPolling();
    setQrCodeUrl('');
    setStatus('loading');
    setMessage('正在生成二维码...');

    try {
      const response = await fetch(getApiUrl('/auth/qrcode'), {
        cache: 'no-store',
      });
      const data = (await response.json()) as QRCodeResponse;

      if (!response.ok || data.code !== 0 || !data.auth_code || !data.qr_code_url) {
        throw new Error(data.message || '生成二维码失败');
      }

      setQrCodeUrl(resolveApiResourceUrl(data.qr_code_url));
      setStatus('scanning');
      setMessage('请使用 Bilibili 手机客户端扫描二维码');
      startPolling(data.auth_code);
    } catch (error) {
      console.error('生成二维码失败:', error);
      setStatus('error');
      setMessage(`生成二维码失败：${error instanceof Error ? error.message : '未知错误'}`);
    }
  }, [clearPolling, startPolling]);

  useEffect(() => {
    void generateQRCode();

    return () => {
      clearPolling();
    };
  }, [clearPolling, generateQRCode]);

  const handleRefresh = () => {
    void generateQRCode();
  };

  const renderContent = () => {
    if (status === 'loading') {
      return (
        <div className="flex flex-col items-center space-y-2">
          <RefreshCw className="h-8 w-8 animate-spin text-blue-500" />
          <span className="text-sm text-gray-500">生成中...</span>
        </div>
      );
    }

    if (status === 'success') {
      return (
        <div className="flex flex-col items-center space-y-2">
          <CheckCircle className="h-12 w-12 text-green-500" />
          <span className="text-sm font-medium text-green-600">登录成功</span>
        </div>
      );
    }

    if (status === 'error') {
      return (
        <div className="flex flex-col items-center space-y-3 px-4 text-center">
          <XCircle className="h-12 w-12 text-red-500" />
          <span className="text-sm text-red-600">{message}</span>
        </div>
      );
    }

    if (status === 'scanning' && qrCodeUrl) {
      return (
        <img
          src={qrCodeUrl}
          alt="登录二维码"
          width={240}
          height={240}
          className="rounded"
          onError={() => {
            clearPolling();
            setStatus('error');
            setMessage('二维码图片加载失败，请刷新后重试');
          }}
        />
      );
    }

    return (
      <div className="flex flex-col items-center space-y-2 text-gray-400">
        <QrCode className="h-12 w-12" />
        <span className="text-sm">等待生成二维码</span>
      </div>
    );
  };

  return (
    <div className="flex flex-col items-center justify-center space-y-6 p-8">
      <div className="text-center">
        <h2 className="mb-2 text-2xl font-bold text-gray-900">Bilibili 扫码登录</h2>
        <p className="text-gray-600">使用 Bilibili 手机客户端扫描下方二维码完成登录</p>
      </div>

      <div className="relative">
        <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-gray-200 bg-white">
          {renderContent()}
        </div>

        {status === 'scanning' && (
          <div className="absolute -right-2 -top-2">
            <div className="h-4 w-4 animate-pulse rounded-full bg-blue-500" />
          </div>
        )}
      </div>

      <div className="text-center">
        <p className={`text-sm ${status === 'success' ? 'text-green-600' : status === 'error' ? 'text-red-600' : 'text-gray-600'}`}>
          {message}
        </p>
      </div>

      {(status === 'error' || status === 'idle' || status === 'scanning') && (
        <button
          type="button"
          onClick={handleRefresh}
          className={`flex items-center space-x-2 rounded-lg px-4 py-2 text-white transition-colors ${status === 'scanning' ? 'bg-gray-500 hover:bg-gray-600' : 'bg-blue-500 hover:bg-blue-600'}`}
        >
          <RefreshCw className="h-4 w-4" />
          <span>{status === 'scanning' ? '刷新二维码' : '重新生成'}</span>
        </button>
      )}

      <div className="max-w-sm text-center text-xs text-gray-500">
        <p>打开 Bilibili 手机客户端，点击右上角扫一扫图标，扫描上方二维码即可快速登录。</p>
      </div>
    </div>
  );
}