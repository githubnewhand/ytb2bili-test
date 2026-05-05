# Bili-Up 浏览器插件集成功能

## 功能概述

我们为 Bili-Up Web 添加了完整的浏览器插件支持，用户可以通过安装插件来获取更好的使用体验。

## 新增功能

### 1. 插件下载导引
- **主页插件提示卡片**：在主页顶部显示醒目的插件安装提示
- **自动下载功能**：点击下载按钮自动获取最新版本插件
- **GitHub API 集成**：实时获取最新 release 版本

### 2. 专用插件页面 (`/extension`)
- **详细功能介绍**：展示插件的所有特性
- **分步安装教程**：图文并茂的安装指南
- **注意事项提醒**：提醒用户重要的安装步骤

### 3. 导航集成
- **侧边栏链接**：在所有页面的侧边栏添加"浏览器插件"入口
- **快捷访问**：主页功能卡片中包含插件页面入口

## 技术实现

### 自动下载逻辑
```javascript
const handleDownloadExtension = async () => {
  // 1. 调用 GitHub API 获取最新 release
  const response = await fetch('https://api.github.com/repos/difyz9/bili-up-extension/releases/latest');
  
  // 2. 查找 .zip 文件资源
  const zipAsset = release.assets.find(asset => asset.name.endsWith('.zip'));
  
  // 3. 创建下载链接并触发下载
  const link = document.createElement('a');
  link.href = zipAsset.browser_download_url;
  link.download = zipAsset.name;
  link.click();
};
```

### 用户界面
- 使用 Tailwind CSS 创建响应式设计
- Lucide React 图标库提供一致的视觉元素
- 渐变背景和卡片布局提升视觉效果

## 用户体验

### 插件功能说明
1. **自动获取视频信息** - 从 B 站页面提取标题、描述、封面
2. **快速导入视频** - 一键添加当前视频到上传队列
3. **批量操作** - 支持批量导入收藏夹或播放列表
4. **同步管理** - 与 Web 平台实时同步数据

### 安装流程
1. 下载插件压缩包
2. 解压到本地文件夹
3. 打开 Chrome 扩展管理页面
4. 启用开发者模式
5. 加载已解压的扩展程序

## 文件结构

```
src/
├── app/
│   ├── page.tsx              # 主页（添加插件导引）
│   └── extension/
│       └── page.tsx          # 专用插件页面
├── components/
│   └── layout/
│       └── AppLayout.tsx     # 布局组件（添加插件链接）
└── hooks/
    └── useAuth.ts            # 认证 Hook（复用）
```

## 注意事项

- 插件下载使用 GitHub API，需要网络连接
- 如果 API 调用失败，会回退到打开 GitHub releases 页面
- 插件安装需要用户手动操作，因为尚未上架应用商店
- 所有页面都支持插件功能访问，保持导航一致性

## 未来扩展

1. **Chrome Web Store 上架**：简化安装流程
2. **Firefox 插件支持**：扩展浏览器兼容性
3. **插件状态检测**：检测用户是否已安装插件
4. **插件通信**：Web 页面与插件的双向通信