import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { User, Video } from '@/types';

interface AuthState {
  user: User | null;
  isLoggedIn: boolean;
  setUser: (user: User | null) => void;
  logout: () => void;
}

interface VideoState {
  videos: Video[];
  currentVideo: Video | null;
  setVideos: (videos: Video[]) => void;
  addVideo: (video: Video) => void;
  updateVideo: (id: number, updates: Partial<Video>) => void;
  setCurrentVideo: (video: Video | null) => void;
  removeVideo: (id: number) => void;
}

// 认证状态管理
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      isLoggedIn: false,
      setUser: (user) => set({ user, isLoggedIn: !!user }),
      logout: () => set({ user: null, isLoggedIn: false }),
    }),
    {
      name: 'auth-storage',
    }
  )
);

// 视频状态管理
export const useVideoStore = create<VideoState>((set) => ({
  videos: [],
  currentVideo: null,
  setVideos: (videos) => set({ videos }),
  addVideo: (video) => set((state) => ({ 
    videos: [video, ...state.videos] 
  })),
  updateVideo: (id, updates) => set((state) => ({
    videos: state.videos.map(video => 
      video.id === id ? { ...video, ...updates } : video
    ),
    currentVideo: state.currentVideo?.id === id 
      ? { ...state.currentVideo, ...updates } 
      : state.currentVideo
  })),
  setCurrentVideo: (video) => set({ currentVideo: video }),
  removeVideo: (id) => set((state) => ({
    videos: state.videos.filter(video => video.id !== id),
    currentVideo: state.currentVideo?.id === id ? null : state.currentVideo
  })),
}));