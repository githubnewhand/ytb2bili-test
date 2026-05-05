# Bili-Up-Web Frontend Management Dashboard

一个基于 Next.js + TypeScript + Tailwind CSS 的前端管理界面，用于管理 bili-up-backend 系统。

## 🚀 功能特性

### 📱 核心功能
- **Bilibili 二维码登录**: 扫码登录 Bilibili 账号
- **视频管理**: 浏览、查看和管理上传的视频
- **字幕管理**: 查看和编辑视频字幕
- **上传验证**: 上传前的验证和预览
- **状态追踪**: 实时监控上传任务状态

### 🎨 界面特性
- **响应式设计**: 支持桌面和移动端
- **现代化 UI**: 使用 Tailwind CSS 构建美观界面
- **实时状态**: 显示登录状态、任务进度等
- **错误处理**: 完善的错误提示和处理

## 🛠️ 技术栈

- **框架**: Next.js 15 (App Router)
- **语言**: TypeScript
- **样式**: Tailwind CSS
- **状态管理**: Zustand
- **API 管理**: React Query (TanStack Query)
- **HTTP 客户端**: Axios
- **图标**: Lucide React

## 📁 项目结构

```
bili-up-web/
├── src/
│   ├── app/                 # Next.js App Router 页面
│   │   ├── layout.tsx       # 全局布局
│   │   ├── page.tsx         # 首页
│   │   └── globals.css      # 全局样式
│   ├── components/          # React 组件
│   │   ├── auth/            # 认证组件
│   │   │   └── QRLogin.tsx  # 二维码登录
│   │   ├── video/           # 视频管理组件
│   │   │   ├── VideoList.tsx
│   │   │   └── VideoItem.tsx
│   │   └── ui/              # UI 组件
│   └── lib/                 # 工具库
│       ├── api.ts           # API 服务
│       ├── store.ts         # 状态管理
│       ├── types.ts         # 类型定义
│       └── utils.ts         # 工具函数
├── public/                  # 静态资源
├── .next/                   # Next.js 构建输出
└── 配置文件...
```

## 🚀 快速开始

### 环境要求

- Node.js 18+ 
- npm 或 yarn
- bili-up-backend 后端服务运行在 localhost:8096

### 安装依赖

```bash
cd bili-up-web
npm install
```

### 开发环境

```bash
# 启动开发服务器
npm run dev

# 访问: http://localhost:3000
```

### 生产构建

```bash
# 构建生产版本
npm run build

# 启动生产服务器
npm start
```

## 🔧 配置说明

### 环境变量

创建 `.env.local` 文件：

```env
# 后端 API 地址
BACKEND_URL=http://localhost:8096

# 其他配置...
```

### API 代理

前端通过 Next.js 的 rewrites 功能代理后端 API：

```javascript
// next.config.js
async rewrites() {
  return [
    {
      source: '/api/:path*',
      destination: 'http://localhost:8096/api/:path*',
    },
  ]
}
```

## 📋 API 接口

### 认证相关

| 接口 | 方法 | 说明 |
|-----|------|------|
| `/api/v1/auth/qrcode` | GET | 获取登录二维码 |
| `/api/v1/auth/status` | GET | 检查登录状态 |

### 视频管理

| 接口 | 方法 | 说明 |
|-----|------|------|
| `/api/v1/videos` | GET | 获取视频列表 |
| `/api/v1/videos/:id` | GET | 获取视频详情 |
| `/api/v1/submit` | POST | 提交新视频 |

### 字幕管理

| 接口 | 方法 | 说明 |
|-----|------|------|
| `/api/v1/subtitles/:id` | GET | 获取字幕内容 |

## 🎯 使用流程

### 1. 登录 Bilibili

1. 访问首页
2. 点击"登录 Bilibili"
3. 扫描显示的二维码
4. 等待登录成功

### 2. 提交视频

1. 在视频管理页面点击"添加视频"
2. 输入 YouTube/其他平台的视频 URL
3. 添加字幕内容（可选）
4. 点击提交

### 3. 监控进度

1. 提交后视频进入处理队列
2. 状态变化：待处理 → 处理中 → 完成
3. 可以查看详细的处理日志

### 4. 查看结果

1. 处理完成后查看上传结果
2. 获取 Bilibili 视频链接
3. 查看生成的中文字幕

## 🎨 界面预览

### 登录页面
- 简洁的二维码登录界面
- 实时显示登录状态
- 自动刷新二维码

### 视频管理
- 视频列表展示
- 状态标识（待处理/处理中/完成/失败）
- 快速操作按钮

### 字幕查看
- 双语字幕对照
- 时间轴显示
- 可编辑字幕内容

## 🔄 状态管理

使用 Zustand 管理全局状态：

```typescript
interface AppState {
  // 用户状态
  user: User | null
  isLoggedIn: boolean
  
  // 视频状态
  videos: Video[]
  currentVideo: Video | null
  
  // UI 状态
  loading: boolean
  error: string | null
}
```

## 🚨 错误处理

### 网络错误
- 自动重试机制
- 错误提示用户
- 优雅降级

### 认证错误
- 自动跳转登录
- 刷新 token
- 清理过期状态

## 📱 响应式设计

### 断点设置
- `sm`: 640px+ (手机横屏)
- `md`: 768px+ (平板)
- `lg`: 1024px+ (桌面)
- `xl`: 1280px+ (大屏)

### 移动端优化
- 触摸友好的按钮大小
- 简化的导航菜单
- 优化的表单布局

## 🧪 开发指南

### 添加新页面

1. 在 `src/app/` 下创建目录
2. 添加 `page.tsx` 文件
3. 使用 TypeScript 和 Tailwind CSS

### 创建新组件

1. 在适当的 `src/components/` 子目录下创建
2. 使用 TypeScript 定义 props
3. 遵循组件命名约定

### API 集成

1. 在 `src/lib/api.ts` 中定义 API 函数
2. 使用 React Query 进行状态管理
3. 处理加载和错误状态

## 🔧 故障排查

### 常见问题

**1. 无法连接后端**
```bash
Error: connect ECONNREFUSED 127.0.0.1:8096
```
解决方案：确保 bili-up-backend 服务运行在 8096 端口

**2. 构建失败**
```bash
Module not found: Can't resolve 'autoprefixer'
```
解决方案：安装缺失的依赖
```bash
npm install autoprefixer
```

**3. 类型错误**
```bash
Property 'xxx' does not exist on type
```
解决方案：检查 `src/lib/types.ts` 中的类型定义

### 调试技巧

1. **开发者工具**: 使用浏览器开发者工具查看网络请求
2. **Next.js 日志**: 查看终端输出的详细日志
3. **React 调试**: 使用 React DevTools 调试组件状态

## 🤝 贡献指南

### 代码规范

- 使用 TypeScript 严格模式
- 遵循 ESLint 规则
- 使用 Prettier 格式化代码
- 编写有意义的提交信息

### 开发流程

1. Fork 项目
2. 创建功能分支
3. 编写代码和测试
4. 提交 Pull Request

## 📄 许可证

MIT License

## 🆘 支持

如果遇到问题：

1. 查看本文档的故障排查部分
2. 检查 Issues 中是否有类似问题
3. 创建新的 Issue 描述问题

---

**现在你可以通过 http://localhost:3000 访问前端管理界面！** 🎉

记得先启动后端服务 (`bili-up-backend`) 才能正常使用所有功能。