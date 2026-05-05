"use client";

import { useState, useEffect } from 'react';
import AppLayout from '@/components/layout/AppLayout';
import QRLogin from '@/components/auth/QRLogin';
import { useAuth } from '@/hooks/useAuth';
import { Settings } from 'lucide-react';
import { APP_NAME_WITH_VERSION } from '@/lib/appVersion';

export default function SettingsPage() {
  const { user, loading, handleLoginSuccess, handleRefreshStatus, handleLogout } = useAuth();
  const [autoUpload, setAutoUpload] = useState<boolean>(() => {
    if (typeof window !== 'undefined') {
      try {
        const v = localStorage.getItem('biliup:autoUpload');
        return v === '1';
      } catch {
        return false;
      }
    }
    return false;
  });

  useEffect(() => {
    if (typeof window !== 'undefined') {
      try {
        localStorage.setItem('biliup:autoUpload', autoUpload ? '1' : '0');
      } catch {
        // ignore
      }
    }
  }, [autoUpload]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mb-4"></div>
          <p className="text-gray-600">加载中...</p>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100">
        <div className="container mx-auto px-4 py-16">
          <div className="max-w-md mx-auto">
            <div className="text-center mb-8">
              <h1 className="text-3xl font-bold text-gray-900 mb-2">
                {APP_NAME_WITH_VERSION}
              </h1>
              <p className="text-gray-600">
                Bilibili 视频管理平台
              </p>
            </div>
            
            <div className="bg-white rounded-lg shadow-lg">
              <QRLogin 
                onLoginSuccess={handleLoginSuccess}
                onRefreshStatus={handleRefreshStatus}
              />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <AppLayout user={user} onLogout={handleLogout}>
      <div className="bg-white rounded-lg shadow-md">
        <div className="p-6 border-b border-gray-200">
          <div className="flex items-center space-x-3">
            <Settings className="w-5 h-5 text-gray-600" />
            <h2 className="text-lg font-medium text-gray-900">设置</h2>
          </div>
        </div>

        <div className="p-6">
          <div className="space-y-4">
            <label className="flex items-center justify-between bg-gray-50 p-4 rounded-md">
              <div>
                <div className="text-sm font-medium">自动上传</div>
                <div className="text-xs text-gray-500">视频提交后自动开始上传任务</div>
              </div>
              <input
                type="checkbox"
                checked={autoUpload}
                onChange={(e) => setAutoUpload(e.target.checked)}
                className="w-5 h-5"
              />
            </label>

            <div className="bg-blue-50 p-4 rounded-md">
              <div className="text-sm text-blue-800">
                <strong>提示：</strong> 更多设置项将在后续版本中添加。
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
