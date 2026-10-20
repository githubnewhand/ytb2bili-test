# 本机 Ollama 字幕翻译与元数据生成

此项目可用 Ollama 的 Qwen3 模型翻译字幕。本机可分别选择让“翻译字幕”和正式“生成元数据”步骤使用本机推理；视频下载、字幕获取仍按 `ProxyConfig` 使用网络代理。

## 本机设置

当前这台电脑的 Ollama 运行文件和模型保存在 Windows 目录 `C:\Users\WC\AppData\Local\OllamaWSLValidation`。这样模型和测试视频不会增加 WSL 的虚拟磁盘占用。

本机已注册 `ollama-local.service` 用户服务，WSL 用户会话启动时自动启动。可以在 WSL 中检查或手动启动：

```bash
systemctl --user status ollama-local.service
systemctl --user start ollama-local.service
```

也可在项目目录运行 `./scripts/start-ollama-local.sh` 以前台方式临时启动。不要同时启动两个实例，它们使用相同的 11434 端口。

在另一个 WSL 终端检查模型：

```bash
OLLAMA_HOST=127.0.0.1:11434 /mnt/c/Users/WC/AppData/Local/OllamaWSLValidation/runtime/bin/ollama list
```

`config.toml` 设置：

```toml
[SubtitleTranslationConfig]
provider = "ollama"
ollama_url = "http://127.0.0.1:11434"
ollama_model = "qwen3:4b-instruct"
timeout = 180
```

正式元数据步骤也可单独选择本地模型：

```toml
[MetadataGenerationConfig]
provider = "ollama"
ollama_url = "http://127.0.0.1:11434"
ollama_model = "qwen3:4b-instruct"
timeout = 180
```

`provider = "auto"` 保持原有的 Gemini 优先、DeepSeek 回退方式。选择 `ollama` 后，正式元数据步骤只调用本机模型；本机模型失败会使该步骤失败，不会静默转去收费 API。正式任务链会照常保存 `meta.json` 并更新数据库记录。元数据仍应在发布前人工核对，尤其是自动字幕可能误识别的细节。

本机 API 强制使用回环地址并绕过 `ProxyConfig`。启动脚本仅在拉取模型时给 Ollama 设置 `HTTPS_PROXY`，代理地址从项目配置读取。若要恢复原翻译服务，将 `provider` 改为 `deepseek` 并重新启动项目进程。

## 单独运行本地字幕及元数据步骤

下载视频及原语言字幕后，可运行：

```bash
/usr/local/go/bin/go run ./cmd/local-subtitle-run \
  -video-id 1s3KQg0lEqk \
  -date 2026-10-19 \
  -source-srt media/2026-10-19/1s3KQg0lEqk/1s3KQg0lEqk.en.srt \
  -metadata
```

该命令调用项目的音频提取、字幕翻译和正式元数据处理器，生成同目录的 `1s3KQg0lEqk.wav`、`1s3KQg0lEqk.srt`、`zh.srt` 和 `meta.json`。`-metadata` 在此本地命令中只写元数据文件，不写入数据库或创建上传任务。日常任务链在项目服务重启后读取上述 Ollama 配置，并照常把正式元数据写入数据库。
