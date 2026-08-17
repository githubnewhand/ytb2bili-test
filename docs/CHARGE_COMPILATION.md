# 充电视频拼接工作流

## 操作顺序

1. 提交链接时选择“免费公开”“30元充电”或“50元充电”。未预选的旧任务会在下载完成后停在“待选择发布方式”。
2. 免费公开视频继续执行音频、字幕、翻译、封面和元数据处理。准备完成后写入 `ready_at`，默认在 60 分钟后创建持久化上传任务；详情页可手动立即入队。
3. 充电视频下载完成后只进入对应档位素材池，不会单独投稿。选择充电档位必须确认原创或已获得允许付费传播的完整授权。
4. 在仪表盘“充电视频拼接”中，从指定档位随机锁定 3 至 4 个可用素材。草稿阶段可重新随机、调整顺序或取消释放素材。
5. 确认草稿后，服务依次执行素材规范化、拼接、完整解码校验、组合封面、字幕、翻译和元数据处理。
6. 成片按草稿设置立即进入上传队列，或等待手动上传。只有B站投稿成功后，源素材才从“已锁定”变为“已使用”。

## 数据与状态

- `tb_saved_videos` 保存源视频和合成视频的统一状态、媒体路径、时长、就绪时间、上传策略和版权确认。
- `tb_charge_pool_items` 保存30元/50元素材池状态：`available`、`reserved`、`consumed`、`error`。
- `tb_compilation_batches` 保存草稿及拼接、处理、上传状态。
- `tb_compilation_items` 保存素材顺序和精确时间线。
- `tb_upload_jobs` 保存可恢复的上传任务、调度时间、尝试次数和租约。

批次状态：

`draft → queued → merging → processing → ready/upload_queued → uploading → uploaded`

失败状态分别为 `merge_failed`、`processing_failed`、`upload_failed`。上传失败重试会复用已处理成片，不重新拼接。服务在上传过程中重启时不会自动重复投稿，而会标记为 `submission_unknown`，要求先在B站创作中心核对。

## 拼接规范

- 每段先转成相同分辨率、帧率、H.264 `yuv420p` 视频和 AAC 48 kHz 双声道音频。
- 横竖屏按比例缩放并补黑边，不拉伸画面。
- 无音轨素材自动补静音轨。
- 规范化后使用 concat 拼接，完成后用 FFmpeg 全量解码验证，再原子替换最终文件。
- 清单中持久化源文件、规范化文件、每段时长及起止时间，并同步生成B站章节文本。

## 配置

`config.toml`：

```toml
[charge_compilation]
enabled = true
min_items = 3
max_items = 4
target_width = 1920
target_height = 1080
target_fps = 30
video_crf = 23
video_preset = "medium"
audio_bitrate = "192k"
default_preview_seconds = 180
free_auto_upload_delay_minutes = 60
charge_upload_policy = "immediate" # immediate / manual
```

充电投稿要求 `[BilibiliConfig] copyright = 1`。系统会在投稿前根据30元或50元解析当前账号对应的充电档位 ID，并在提交后核对稿件的充电专属状态。

## 管理 API

- `GET /api/v1/charge/pools/summary`
- `GET /api/v1/charge/pools?tier=30&state=available`
- `POST /api/v1/charge/batches/draft`
- `PUT /api/v1/charge/batches/:id/order`
- `POST /api/v1/charge/batches/:id/reroll`
- `POST /api/v1/charge/batches/:id/start`
- `POST /api/v1/charge/batches/:id/retry`
- `POST /api/v1/charge/batches/:id/cancel`
- `GET /api/v1/charge/batches`
- `GET /api/v1/charge/batches/:id`

素材声明和分流使用：

- `PUT /api/v1/videos/:id/publish-audience`
- `POST /api/v1/submit` 可直接带 `publish_audience`、`preview_seconds`、`rights_verified`

## 验证命令

```bash
go test ./...
go vet ./...
cd web && npm run build
```

媒体测试会在安装了 FFmpeg/FFprobe 的环境中生成一段横屏有声视频和一段竖屏无声视频，验证规范化、静音补轨、连续时间线、最终分辨率和完整解码。
