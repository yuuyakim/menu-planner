# 注意: Windows の GNU Make 3.81 は UTF-8 の echo が文字化けし、複雑なシェル構文も壊れる。
# そのためレシピ内は ASCII のみ・1コマンド単位に保つこと（日本語はコメントに書く）。

.DEFAULT_GOAL := help
.PHONY: help up down logs dev test test-backend test-frontend lint build health clean migrate migrate-down migrate-version seed deps

help: ## このヘルプを表示する
	@echo "Usage: make <target>"
	@echo ""
	@echo "  up             start containers"
	@echo "  down           stop containers"
	@echo "  logs           follow logs"
	@echo "  dev            start containers and follow logs"
	@echo "  health         check /health endpoint"
	@echo "  migrate        apply DB migrations"
	@echo "  migrate-down   roll back one migration"
	@echo "  migrate-version show current migration version"
	@echo "  seed           load menu master data"
	@echo "  test           run all tests"
	@echo "  test-backend   run Go tests"
	@echo "  test-frontend  run frontend checks"
	@echo "  lint           run linters"
	@echo "  build          build production images"
	@echo "  deps           reinstall frontend deps in the container"
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

# マイグレーションは backend コンテナ内から実行する。ホストに golang-migrate を
# 入れる必要がなく、DBのホスト名(db)とDATABASE_URLをそのまま使える。
migrate: ## マイグレーションを適用する
	docker compose run --rm backend go run ./cmd/migrate up

migrate-down: ## マイグレーションを1つ戻す
	docker compose run --rm backend go run ./cmd/migrate down

migrate-version: ## 現在のマイグレーションバージョンを表示する
	docker compose run --rm backend go run ./cmd/migrate version

seed: ## 献立マスタを投入する
	docker compose run --rm backend go run ./cmd/seed

# ローカルでは -race を使わない（cgo=gcc が必要なため）。CI の Linux 上では有効化している。
test-backend: ## Goのテストを実行する
	cd backend && go test ./... -cover

test-frontend: ## フロントエンドの型チェック・Lint・テストを実行する
	cd frontend && npx tsc --noEmit
	cd frontend && npm run lint
	cd frontend && npm test

test: test-backend test-frontend ## 全テストを実行する

# golangci-lint v2 が必要:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
lint: ## Lintを実行する
	cd backend && go vet ./...
	cd backend && golangci-lint run
	cd frontend && npm run lint

# :latest は compose がビルドする開発用イメージのタグと衝突するため :prod を使う
build: ## 本番イメージをビルドする
	docker build -t menu-planner-backend:prod --target prod ./backend
	docker build -t menu-planner-frontend:prod --target prod ./frontend

# frontend の node_modules は匿名ボリュームなので、package.json を変えても
# 再ビルドだけでは反映されない。-V で匿名ボリュームごと作り直す。
deps: ## 依存を追加したあとコンテナの node_modules を入れ直す
	docker compose build frontend
	docker compose up -d -V frontend

clean: ## コンテナとボリュームを削除する
	docker compose down -v
