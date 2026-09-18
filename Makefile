.PHONY: build-backend build-frontend build run-dev run-backend stats tidy install sync-dist pm2-start pm2-reload pm2-stop pm2-logs deploy

# ==== Build Backend ====
build-backend:
	mkdir -p bin .data/logs
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
sync-dist:
	mkdir -p web
	rm -rf web/*
	cp -r frontend/dist/* web/

stats:
	cd backend && go mod tidy

# ==== PM2 (deployment tanpa Docker) ====
pm2-start:
	pm2 start ecosystem.config.cjs

pm2-reload:
	pm2 reload ecosystem.config.cjs --update-env

pm2-stop:
	pm2 stop fiber-monitor

pm2-logs:
	pm2 logs fiber-monitor

# Build ulang + deploy lewat PM2
deploy: build sync-dist
	pm2 startOrReload ecosystem.config.cjs --update-env
	pm2 save

# ==== Install ke /opt/fiber-monitor (Debian, PM2) ====
install: build sync-dist
	sudo mkdir -p /opt/fiber-monitor/bin /opt/fiber-monitor/.data/logs
	sudo cp bin/fiber-monitor-server /opt/fiber-monitor/bin/
	sudo rm -rf /opt/fiber-monitor/web
	sudo cp -r web /opt/fiber-monitor/web
	sudo cp ecosystem.config.cjs /opt/fiber-monitor/
	@if [ ! -f /opt/fiber-monitor/.env ]; then sudo cp .env.example /opt/fiber-monitor/.env && sudo chmod 600 /opt/fiber-monitor/.env; fi
	@echo "Selesai. Langkah berikutnya:"
	@echo "  cd /opt/fiber-monitor"
	@echo "  nano .env          # isi DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY"
	@echo "  pm2 start ecosystem.config.cjs && pm2 save"