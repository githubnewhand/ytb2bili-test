'use client';

import { useState } from 'react';
import { Upload, Link, FileText, AlertCircle, CheckCircle } from 'lucide-react';

interface VideoSubmissionFormProps {
  onSubmit?: (data: any) => void;
}

export default function VideoSubmissionForm({ onSubmit }: VideoSubmissionFormProps) {
  const [formData, setFormData] = useState({
    url: '',
    title: '',
    subtitles: [] as any[],
  });
  const [subtitleText, setSubtitleText] = useState('');
  const [loading, setLoading] = useState(false);
  const [validation, setValidation] = useState<any>(null);

  const handleUrlChange = (e: any) => {
    const url = e.target.value;
    setFormData(prev => ({ ...prev, url }));
    
    // 简单的URL验证
    if (url && (url.includes('youtube.com') || url.includes('bilibili.com'))) {
      setValidation({ is_valid: true, errors: [], warnings: [] });
    } else if (url) {
      setValidation({ 
        is_valid: false, 
        errors: ['不支持的视频URL格式'], 
        warnings: [] 
      });
    } else {
      setValidation(null);
    }
  };

  const parseSubtitles = (text: string) => {
    const lines = text.split('\n').filter(line => line.trim());
    const subtitles = [];
    let currentTime = 0;

    for (const line of lines) {
      if (line.trim()) {
        subtitles.push({
          text: line.trim(),
          duration: 3.0, // 默认3秒
          offset: currentTime,
          lang: 'en'
        });
        currentTime += 3.0;
      }
    }

    return subtitles;
  };

  const handleSubtitleChange = (e: any) => {
    const text = e.target.value;
    setSubtitleText(text);
    
    if (text.trim()) {
      const parsedSubtitles = parseSubtitles(text);
      setFormData(prev => ({ ...prev, subtitles: parsedSubtitles }));
    } else {
      setFormData(prev => ({ ...prev, subtitles: [] }));
    }
  };

  const handleSubmit = async (e: any) => {
    e.preventDefault();
    
    if (!formData.url || !formData.title) {
      alert('请填写完整信息');
      return;
    }

    setLoading(true);
    
    try {
      const response = await fetch('/api/v1/submit', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData),
      });

      const result = await response.json();
      
      if (result.code === 200) {
        alert('视频提交成功！');
        if (onSubmit) {
          onSubmit(result.data);
        }
        // 重置表单
        setFormData({ url: '', title: '', subtitles: [] });
        setSubtitleText('');
        setValidation(null);
      } else {
        alert(`提交失败: ${result.message}`);
      }
    } catch (error) {
      console.error('提交失败:', error);
      alert('提交失败，请重试');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto bg-white rounded-lg shadow-md p-6">
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900 mb-2">
          <Upload className="inline-block w-6 h-6 mr-2" />
          提交新视频
        </h2>
        <p className="text-gray-600">
          支持 YouTube 和 Bilibili 视频链接
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* 视频URL */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            <Link className="inline-block w-4 h-4 mr-1" />
            视频链接
          </label>
          <input
            type="url"
            value={formData.url}
            onChange={handleUrlChange}
            placeholder="https://www.youtube.com/watch?v=..."
            className="input-field"
            required
          />
          
          {/* URL验证状态 */}
          {validation && (
            <div className="mt-2">
              {validation.is_valid ? (
                <div className="flex items-center text-sm text-green-600">
                  <CheckCircle className="w-4 h-4 mr-1" />
                  URL格式正确
                </div>
              ) : (
                <div className="text-sm text-red-600">
                  {validation.errors.map((error: string, index: number) => (
                    <div key={index} className="flex items-center">
                      <AlertCircle className="w-4 h-4 mr-1" />
                      {error}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* 视频标题 */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            视频标题
          </label>
          <input
            type="text"
            value={formData.title}
            onChange={(e) => setFormData(prev => ({ ...prev, title: e.target.value }))}
            placeholder="输入视频标题"
            className="input-field"
            required
          />
        </div>

        {/* 字幕输入 */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            <FileText className="inline-block w-4 h-4 mr-1" />
            字幕内容 (可选)
          </label>
          <textarea
            value={subtitleText}
            onChange={handleSubtitleChange}
            placeholder="每行一句字幕，系统会自动设置时间轴"
            rows={8}
            className="input-field resize-none"
          />
          
          {formData.subtitles.length > 0 && (
            <div className="mt-2 text-sm text-gray-600">
              已解析 {formData.subtitles.length} 条字幕
            </div>
          )}
        </div>

        {/* 提交按钮 */}
        <div className="flex justify-end space-x-4">
          <button
            type="button"
            onClick={() => {
              setFormData({ url: '', title: '', subtitles: [] });
              setSubtitleText('');
              setValidation(null);
            }}
            className="btn-secondary"
          >
            重置
          </button>
          
          <button
            type="submit"
            disabled={loading || !validation?.is_valid}
            className="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? (
              <>
                <div className="inline-block w-4 h-4 mr-2 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                提交中...
              </>
            ) : (
              <>
                <Upload className="inline-block w-4 h-4 mr-2" />
                提交视频
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}