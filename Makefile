.PHONY: build-backend build-frontend build run-dev run-backend stats tidy serve systemd install

# ==== Build Backend ====
build-backend:
	cd backend && go build -o ../bin/fiber-monitor-server ./cmd/server

# ==== Build Frontend ====
build-frontend:
	cd frontend && npm ci && npm run build

# ==== Build Keduanya ====
build: build-backend build-frontend
	@echo "Build selesai. Backend di bin/, frontend di frontend/dist/"

# ==== Jalankan Development ====
run-backend:
	cd backend && go run ./cmd/server

run-dev:
	cd frontend && npm run dev

# ==== Sync frontend build ke folder web (untuk serve langsung oleh Go) ====
sync-dist: build-frontend
	mkdir -p web
	cp -r frontend/dist/* web/

stats:
	cd backend && go mod tidy

# ==== Install ke /opt/fiber-monitor (Debian) ====
install: build sync-dist
	sudo mkdir -p /opt/fiber-monitor
	sudo cp bin/fiber-monitor-server /opt/fiber-monitor/
	@if [ ! -f /opt/fiber-monitor/web/index.html ]; then sudo mkdir -p /opt/fiber-monitor/web && sudo cp -r web/* /opt/fiber-monitor/web/; fi
	@echo "Instal sistemd:"
	@echo "  sudo cp deployment/fiber-monitor.service /etc/systemd/system/"
	@echo "  sudo systemctl daemon-reload && sudo systemctl enable --now fiber-monitor"

# ==== Docker ====
docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f backend