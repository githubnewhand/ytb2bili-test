"use client";

import { useState } from 'react';
import AppLayout from '@/components/layout/AppLayout';
import QRLogin from '@/components/auth/QRLogin';
import { useAuth } from '@/hooks/useAuth';
import { 
  Download, 
  ExternalLink, 
  Chrome, 
  CheckCircle, 
  AlertCircle,
  FileText,
  Settings,
  Puzzle
} from 'lucide-react';
import { APP_NAME_WITH_VERSION } from '@/lib/appVersion';

export default function ExtensionPage() {
  const { user, loading, handleLoginSuccess, handleRefreshStatus, handleLogout } = useAuth();
  const [isDownloading, setIsDownloading] = useState(false);

  const handleDownloadExtension = async () => {
    setIsDownloading(true);
    try {
      const response = await fetch('https://api.github.com/repos/difyz9/bili-up-extension/releases/latest');
      const release = await response.json();
      
      if (release.assets && release.assets.length > 0) {
        const zipAsset = release.assets.find((asset: any) => asset.name.endsWith('.zip'));
        if (zipAsset) {
          const link = document.createElement('a');
          link.href = zipAsset.browser_download_url;
          link.download = zipAsset.name;
          document.body.appendChild(link);
          link.click();
          document.body.removeChild(link);
        } else {
          window.open('https://github.com/difyz9/bili-up-extension/releases/latest', '_blank');
        }
      } else {
        window.open('https://github.com/difyz9/bili-up-extension/releases/latest', '_blank');
      }
    } catch (error) {
      console.error('下载失败:', error);
      window.open('https://github.com/difyz9/bili-up-extension/releases/latest', '_blank');
    } finally {
      setIsDownloading(false);
    }
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
        {/* 插件介绍 */}
        <div className="bg-gradient-to-r from-purple-50 to-pink-50 rounded-lg border border-purple-200 p-6">
          <div className="flex items-start space-x-4">
            <div className="flex-shrink-0">
              <Puzzle className="w-8 h-8 text-purple-600" />
            </div>
            <div className="flex-1">
              <h2 className="text-xl font-semibold text-gray-900 mb-2">
                Bili-Up 浏览器插件
              </h2>
              <p className="text-gray-600 mb-4">
                通过安装我们的浏览器插件，您可以更方便地使用 Bili-Up 平台的各项功能。
              </p>
              <div className="flex flex-col sm:flex-row gap-3">
                <button
                  onClick={handleDownloadExtension}
                  disabled={isDownloading}
                  className="flex items-center justify-center px-6 py-2 bg-purple-600 text-white rounded-md hover:bg-purple-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {isDownloading ? (
                    <>
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2"></div>
                      下载中...
                    </>
                  ) : (
                    <>
                      <Download className="w-4 h-4 mr-2" />
                      下载最新版本
                    </>
                  )}
                </button>
                <a
                  href="https://github.com/difyz9/bili-up-extension"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-center px-6 py-2 border border-purple-300 text-purple-700 rounded-md hover:bg-purple-50 transition-colors"
                >
                  <ExternalLink className="w-4 h-4 mr-2" />
                  GitHub 项目
                </a>
              </div>
            </div>
          </div>
        </div>

        {/* 功能特性 */}
        <div className="bg-white rounded-lg shadow-md">
          <div className="p-6 border-b border-gray-200">
            <h3 className="text-lg font-medium text-gray-900">插件功能</h3>
          </div>
          <div className="p-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="flex items-start space-x-3">
                <CheckCircle className="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-medium text-gray-900">自动获取视频信息</h4>
                  <p className="text-sm text-gray-600">在 B 站视频页面自动提取标题、描述、封面等信息</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-3">
                <CheckCircle className="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-medium text-gray-900">快速导入视频</h4>
                  <p className="text-sm text-gray-600">一键将当前浏览的视频添加到上传队列</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-3">
                <CheckCircle className="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-medium text-gray-900">批量操作</h4>
                  <p className="text-sm text-gray-600">支持批量导入收藏夹或播放列表中的视频</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-3">
                <CheckCircle className="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-medium text-gray-900">同步管理</h4>
                  <p className="text-sm text-gray-600">与 Bili-Up Web 平台实时同步数据</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* 安装教程 */}
        <div className="bg-white rounded-lg shadow-md">
          <div className="p-6 border-b border-gray-200">
            <h3 className="text-lg font-medium text-gray-900">安装教程</h3>
          </div>
          <div className="p-6">
            <div className="space-y-4">
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-sm font-medium">
                  1
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-900 mb-1">下载插件文件</h4>
                  <p className="text-sm text-gray-600">点击上方的&ldquo;下载最新版本&rdquo;按钮，下载插件压缩包到本地</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-sm font-medium">
                  2
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-900 mb-1">解压文件</h4>
                  <p className="text-sm text-gray-600">将下载的 zip 文件解压到一个文件夹中</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-sm font-medium">
                  3
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-900 mb-1">打开扩展管理页面</h4>
                  <p className="text-sm text-gray-600">在 Chrome 浏览器中访问 <code className="bg-gray-100 px-1 rounded">chrome://extensions/</code></p>
                </div>
              </div>
              
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-sm font-medium">
                  4
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-900 mb-1">启用开发者模式</h4>
                  <p className="text-sm text-gray-600">在扩展页面右上角打开&ldquo;开发者模式&rdquo;开关</p>
                </div>
              </div>
              
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-sm font-medium">
                  5
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-900 mb-1">加载插件</h4>
                  <p className="text-sm text-gray-600">点击&ldquo;加载已解压的扩展程序&rdquo;，选择刚才解压的文件夹</p>
                </div>
              </div>
            </div>

            <div className="mt-6 p-4 bg-amber-50 border border-amber-200 rounded-lg">
              <div className="flex items-start space-x-2">
                <AlertCircle className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-medium text-amber-800">注意事项</h4>
                  <p className="text-sm text-amber-700 mt-1">
                    由于插件还未上架应用商店，需要手动安装。如果遇到问题，请查看 GitHub 项目页面的详细说明。
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
