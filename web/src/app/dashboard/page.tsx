"use client";

import { useState } from 'react';
import AppLayout from '@/components/layout/AppLayout';
import TaskQueueStats from '@/components/dashboard/TaskQueueStats';
import ChargePoolManager from '@/components/charge/ChargePoolManager';
import QRLogin from '@/components/auth/QRLogin';
import { useAuth } from '@/hooks/useAuth';
import { BarChart3 } from 'lucide-react';
import { APP_NAME_WITH_VERSION } from '@/lib/appVersion';

export default function DashboardPage() {
  const { user, loading, handleLoginSuccess, handleRefreshStatus, handleLogout } = useAuth();
  const [selectedVideoId, setSelectedVideoId] = useState<string | null>(null);

  const handleVideoSelect = (videoId: string) => {
    setSelectedVideoId(videoId);
  };

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
      <div className="space-y-6">
        <div className="rounded-lg bg-white p-6 shadow-md">
          <ChargePoolManager />
        </div>

        <div className="rounded-lg bg-white shadow-md">
          <div className="border-b border-gray-200 p-6">
            <div className="flex items-center space-x-3">
              <BarChart3 className="h-5 w-5 text-gray-600" />
              <h2 className="text-lg font-medium text-gray-900">任务队列</h2>
            </div>
          </div>

          <div className="p-6">
            <TaskQueueStats onVideoSelect={handleVideoSelect} />
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
