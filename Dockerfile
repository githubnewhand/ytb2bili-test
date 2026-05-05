# 多阶段构建 Dockerfile
# Stage 1: 构建Go后端
FROM golang:1.21-alpine AS backend-builder

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 复制 Go 模块文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建Go应用
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o ytb2bili-server .

# Stage 2: 运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    python3 \
    py3-pip \
    ffmpeg \
    && pip3 install --break-system-packages yt-dlp \
    && rm -rf /var/cache/apk/*

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非特权用户
RUN addgroup -g 1001 -S ytb2bili && \
    adduser -S ytb2bili -u 1001 -G ytb2bili

WORKDIR /app

# 复制构建的二进制文件
COPY --from=backend-builder /app/ytb2bili-server .
COPY --from=backend-builder /app/config.toml.example ./config.toml

# 创建必要的目录
RUN mkdir -p /data/ytb2bili /app/logs && \
    chown -R ytb2bili:ytb2bili /app /data/ytb2bili

# 切换到非特权用户
USER ytb2bili

# 暴露端口
EXPOSE 8096

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8096/health || exit 1

# 启动命令
CMD ["./ytb2bili-server"]