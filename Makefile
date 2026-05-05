.PHONY: all build build-web build-api clean run test help install-deps

# 默认目标
all: build

# 项目路径
ROOT_DIR := $(shell pwd)
WEB_DIR := $(ROOT_DIR)/web
OUT_DIR := $(WEB_DIR)/output
TARGET_DIR := $(ROOT_DIR)/internal/web/bili-up-web
BINARY_NAME := bili-up-api-server

# Go 构建变量
GO := $(or $(shell command -v go 2>/dev/null),$(wildcard /usr/local/go/bin/go),go)
GOCMD := $(GO)
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# 构建标志
BUILD_FLAGS := -v
LDFLAGS := -s -w

# 帮助信息
help:
	@echo "Bili-Up API Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - 构建前端并打包到 Go 二进制"
	@echo "  make build-web      - 仅构建前端静态文件"
	@echo "  make build-api      - 仅构建 Go 后端（需要已有前端文件）"
	@echo "  make clean          - 清理构建产物"
	@echo "  make run            - 构建并运行服务器"
	@echo "  make test           - 运行测试"
	@echo "  make install-deps   - 安装依赖"
	@echo "  make help           - 显示此帮助信息"
	@echo ""
	@echo "Environment Variables:"
	@echo "  BACKEND_URL         - 前端构建时使用的后端 URL（默认: http://localhost:8096）"
	@echo ""

# 完整构建流程（前端 + 后端）
build: build-web build-api
	@echo "✅ 构建完成！"
	@echo "📦 二进制文件: $(BINARY_NAME)"
	@echo "🚀 运行: ./$(BINARY_NAME)"

# 构建前端静态文件
build-web:
	@echo "📦 开始构建前端..."
	@if [ ! -d "$(WEB_DIR)" ]; then \
		echo "❌ 错误: 找不到 bili-up-web 目录"; \
		echo "   预期路径: $(WEB_DIR)"; \
		exit 1; \
	fi
	@echo "📂 前端目录: $(WEB_DIR)"
	
	@# 安装依赖
	@echo "📥 安装前端依赖..."
	@cd $(WEB_DIR) && \
	if [ -f package-lock.json ]; then \
		npm ci --silent; \
	else \
		npm install --silent; \
	fi
	
	@# 构建前端
	@echo "🔨 构建 Next.js 应用..."
	@cd $(WEB_DIR) && \
	export BACKEND_URL=$${BACKEND_URL:-http://localhost:8096} && \
	npm run build
	
	@# Next.js 15+ 使用 output: 'export' 配置后，build 命令会自动导出到 out 目录
	@# 检查导出结果
	@if [ ! -d "$(OUT_DIR)" ]; then \
		echo "❌ 导出失败: 找不到输出目录 $(OUT_DIR)"; \
		echo "   请确认 next.config.js 中已配置 output: 'export' 和 distDir: 'out'"; \
		exit 1; \
	fi
	
	@# 复制到目标目录
	@echo "📋 复制静态文件到 Go 项目..."
	@rm -rf $(TARGET_DIR)
	@mkdir -p $(TARGET_DIR)
	@cp -a $(OUT_DIR)/. $(TARGET_DIR)/
	
	@# 复制 public 资源（如果存在）
	@if [ -d "$(WEB_DIR)/public" ]; then \
		echo "📋 复制 public 资源..."; \
		cp -a $(WEB_DIR)/public/. $(TARGET_DIR)/ 2>/dev/null || true; \
	fi
	
	@# 复制 _next/static（如果需要）
	@if [ -d "$(WEB_DIR)/.next/static" ]; then \
		echo "📋 复制 _next/static..."; \
		mkdir -p $(TARGET_DIR)/_next; \
		cp -a $(WEB_DIR)/.next/static $(TARGET_DIR)/_next/ 2>/dev/null || true; \
	fi
	
	@echo "✅ 前端构建完成"
	@echo "📂 静态文件位置: $(TARGET_DIR)"

# 构建 Go 后端
build-api:
	@echo "🔨 开始构建 Go 后端..."
	@# 检查静态文件是否存在
	@if [ ! -d "$(TARGET_DIR)" ] || [ -z "$$(ls -A $(TARGET_DIR) 2>/dev/null)" ]; then \
		echo "⚠️  警告: 静态文件目录不存在或为空"; \
		echo "   先运行 'make build-web' 构建前端"; \
		echo "   或继续构建（将不包含前端页面）"; \
		read -p "   是否继续? [y/N] " -n 1 -r; \
		echo; \
		if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
			exit 1; \
		fi; \
	fi
	
	@# 整理依赖
	@echo "📥 整理 Go 依赖..."
	@$(GOMOD) tidy
	
	@# 构建二进制
	@echo "🔧 编译 Go 程序..."
	@$(GOBUILD) $(BUILD_FLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	
	@echo "✅ Go 后端构建完成"
	@ls -lh $(BINARY_NAME)

# 清理构建产物
clean:
	@echo "🧹 清理构建产物..."
	@rm -f $(BINARY_NAME)
	@rm -rf $(TARGET_DIR)
	@rm -rf $(OUT_DIR)
	@if [ -d "$(WEB_DIR)/.next" ]; then \
		rm -rf $(WEB_DIR)/.next; \
	fi
	@if [ -d "$(WEB_DIR)/out" ]; then \
		rm -rf $(WEB_DIR)/out; \
	fi
	@$(GOCLEAN)
	@echo "✅ 清理完成"

# 安装依赖
install-deps:
	@echo "📥 安装依赖..."
	@# Go 依赖
	@echo "📦 安装 Go 依赖..."
	@$(GOMOD) download
	@$(GOMOD) tidy
	
	@# 前端依赖
	@if [ -d "$(WEB_DIR)" ]; then \
		echo "📦 安装前端依赖..."; \
		cd $(WEB_DIR) && npm install; \
	else \
		echo "⚠️  找不到 bili-up-web 目录，跳过前端依赖安装"; \
	fi
	@echo "✅ 依赖安装完成"

# 运行测试
test:
	@echo "🧪 运行测试..."
	@$(GOTEST) -v ./...

# 构建并运行
run: build
	@echo "🚀 启动服务器..."
	@./$(BINARY_NAME)

# 仅构建 Go（快速构建，不包含前端更新）
quick-build:
	@echo "⚡ 快速构建（仅 Go）..."
	@$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) .
	@echo "✅ 快速构建完成"

# 开发模式（监视文件变化，需要安装 air 或类似工具）
dev:
	@echo "🔥 开发模式..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "❌ 请先安装 air: go install github.com/cosmtrek/air@latest"; \
		echo "或直接运行: make run"; \
	fi

# 生产构建（优化大小）
build-prod: BUILD_FLAGS += -trimpath
build-prod: LDFLAGS += -X main.Version=$(shell git describe --tags --always --dirty) -X main.BuildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
build-prod: build
	@echo "🎉 生产构建完成"
	@echo "📊 二进制文件大小:"
	@ls -lh $(BINARY_NAME)

# 检查代码质量
lint:
	@echo "🔍 代码检查..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "⚠️  golangci-lint 未安装，跳过检查"; \
		echo "   安装: brew install golangci-lint (macOS)"; \
		echo "   或访问: https://golangci-lint.run/usage/install/"; \
	fi

# 格式化代码
fmt:
	@echo "🎨 格式化代码..."
	@$(GOCMD) fmt ./...
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && npm run lint --fix 2>/dev/null || true; \
	fi
	@echo "✅ 代码格式化完成"

# 显示项目信息
info:
	@echo "📋 项目信息"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "根目录:         $(ROOT_DIR)"
	@echo "前端目录:       $(WEB_DIR)"
	@echo "静态文件目录:   $(TARGET_DIR)"
	@echo "二进制文件:     $(BINARY_NAME)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Go 版本:        $$($(GOCMD) version)"
	@if command -v node > /dev/null; then \
		echo "Node 版本:      $$(node --version)"; \
	fi
	@if command -v npm > /dev/null; then \
		echo "npm 版本:       $$(npm --version)"; \
	fi
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
