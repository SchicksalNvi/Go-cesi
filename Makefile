APP_NAME   := superview
VERSION    ?= $(shell date +%Y%m%d)
GOOS       ?= linux
GOARCH     ?= amd64
BUILD_DIR  := build
RELEASE_DIR:= release
FRONTEND   := web/react-app

.PHONY: all frontend backend release clean help

all: backend frontend

## 前端构建
frontend:
	@echo "==> Building frontend..."
	cd $(FRONTEND) && NODE_OPTIONS=--max-old-space-size=768 npm ci && npm run build

## 后端构建
backend:
	@echo "==> Building backend ($(GOOS)/$(GOARCH))..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	GOGC=20 GOMEMLIMIT=1200MiB \
		go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) cmd/main.go
	@cp -f $(BUILD_DIR)/$(APP_NAME) ./$(APP_NAME)

## 打包发布（前端需提前构建: make frontend）
release: backend
	@echo "==> Packaging release $(VERSION)..."
	$(eval PKG := $(APP_NAME)-$(VERSION)-$(GOOS)-$(GOARCH))
	@rm -rf $(RELEASE_DIR)/$(PKG)
	@mkdir -p $(RELEASE_DIR)/$(PKG)/config \
		$(RELEASE_DIR)/$(PKG)/data \
		$(RELEASE_DIR)/$(PKG)/logs \
		$(RELEASE_DIR)/$(PKG)/pids \
		$(RELEASE_DIR)/$(PKG)/web/react-app
	cp $(BUILD_DIR)/$(APP_NAME) $(RELEASE_DIR)/$(PKG)/
	cp -r $(FRONTEND)/dist $(RELEASE_DIR)/$(PKG)/web/react-app/dist
	cp config/config.toml.example   $(RELEASE_DIR)/$(PKG)/config/config.toml.example
	cp config/nodelist.toml.example $(RELEASE_DIR)/$(PKG)/config/nodelist.toml.example
	cp config/.env.example          $(RELEASE_DIR)/$(PKG)/config/.env.example
	cp superview.sh         $(RELEASE_DIR)/$(PKG)/
	cp README.md            $(RELEASE_DIR)/$(PKG)/ 2>/dev/null || true
	cd $(RELEASE_DIR) && tar -czf $(PKG).tar.gz $(PKG)
	@rm -rf $(RELEASE_DIR)/$(PKG)
	@echo "==> Created: $(RELEASE_DIR)/$(PKG).tar.gz"
	@ls -lh $(RELEASE_DIR)/$(PKG).tar.gz

clean:
	rm -rf $(BUILD_DIR) $(RELEASE_DIR)

help:
	@echo "用法:"
	@echo "  make                    构建前后端"
	@echo "  make frontend           仅构建前端"
	@echo "  make backend            仅构建后端"
	@echo "  make release            打包发布 (tar.gz)"
	@echo "  make clean              清理构建产物"
	@echo ""
	@echo "变量:"
	@echo "  VERSION=v1.0.0          版本号 (默认: 日期)"
	@echo "  GOOS=linux              目标系统"
	@echo "  GOARCH=amd64            目标架构"
	@echo ""
	@echo "示例:"
	@echo "  make release VERSION=v1.0.0"
	@echo "  make release GOOS=linux GOARCH=arm64 VERSION=v1.0.0"
	@echo ""
	@echo "部署:"
	@echo "  tar -xzf superview-v1.0.0-linux-amd64.tar.gz"
	@echo "  cd superview-v1.0.0-linux-amd64"
	@echo "  cp config/*.example config/  (去掉 .example 后缀并修改)"
	@echo "  ./superview.sh start"
