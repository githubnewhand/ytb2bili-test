import { useCallback, useEffect, useState } from 'react';
import { getApiUrl } from '@/lib/apiBase';

interface UserInfo {
  id: string;
  name: string;
  mid: string;
  avatar?: string;
}

interface AuthStatusResponse {
  code: number;
  is_logged_in: boolean;
  user?: UserInfo;
}

export function useAuth() {
  const [user, setUser] = useState<UserInfo | null>(null);
  const [loading, setLoading] = useState(true);

  const checkAuthStatus = useCallback(async () => {
    try {
      const response = await fetch(getApiUrl('/auth/status'), {
        cache: 'no-store',
      });
      const data = (await response.json()) as AuthStatusResponse;

      if (response.ok && data.code === 0 && data.is_logged_in && data.user) {
        setUser(data.user);
      } else {
        setUser(null);
      }
    } catch (error) {
      console.error('检查登录状态失败:', error);
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void checkAuthStatus();
  }, [checkAuthStatus]);

  const handleLoginSuccess = (userData: UserInfo) => {
    setUser(userData);
  };

  const handleRefreshStatus = async () => {
    await checkAuthStatus();
  };

  const handleLogout = async () => {
    try {
      await fetch(getApiUrl('/auth/logout'), { method: 'POST' });
      setUser(null);
    } catch (error) {
      console.error('退出失败:', error);
    }
  };

  return {
    user,
    loading,
    handleLoginSuccess,
    handleRefreshStatus,
    handleLogout,
  };
}