# 注意: Windows の GNU Make 3.81 は UTF-8 の echo が文字化けし、複雑なシェル構文も壊れる。
# そのためレシピ内は ASCII のみ・1コマンド単位に保つこと（日本語はコメントに書く）。

.DEFAULT_GOAL := help
.PHONY: help up down logs dev test test-backend test-frontend lint build health clean

help: ## このヘルプを表示する
	@echo "Usage: make <target>"
	@echo ""
	@echo "  up             start containers"
	@echo "  down           stop containers"
	@echo "  logs           follow logs"
	@echo "  dev            start containers and follow logs"
	@echo "  health         check /health endpoint"
	@echo "  test           run all tests"
	@echo "  test-backend   run Go tests"
	@echo "  test-frontend  run frontend checks"
	@echo "  lint           run linters"
	@echo "  build          build production images"
	@echo "  clean          remove containers and volumes"

up: ## コンテナを起動する
	docker compose up -d

down: ## コンテナを停止する
	docker compose down

logs: ## ログを追跡する
	docker compose logs -f

dev: up logs ## 起動してログを追跡する

health: ## /health の疎通を確認する
	curl -fsS http://localhost:8080/health

# ローカルでは -race を使わない（cgo=gcc が必要なため）。CI の Linux 上では有効化している。
test-backend: ## Goのテストを実行する
	cd backend && go test ./... -cover

test-frontend: ## フロントエンドの型チェックとLintを実行する
	cd frontend && npx tsc --noEmit
	cd frontend && npm run lint

test: test-backend test-frontend ## 全テストを実行する

lint: ## Lintを実行する
	cd backend && go vet ./...
	cd frontend && npm run lint

# :latest は compose がビルドする開発用イメージのタグと衝突するため :prod を使う
build: ## 本番イメージをビルドする
	docker build -t menu-planner-backend:prod --target prod ./backend
	docker build -t menu-planner-frontend:prod --target prod ./frontend

clean: ## コンテナとボリュームを削除する
	docker compose down -v
