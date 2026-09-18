.PHONY: build-backend build-frontend build run-dev run-backend stats tidy install sync-dist pm2-start pm2-reload pm2-stop pm2-logs deploy

# ==== Build Backend ====
build-backend:
	mkdir -p bin .data/logs
	cd backend && go build -o ../bin/monitoring-server ./cmd/server

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
	pm2 stop monitoring

pm2-logs:
	pm2 logs monitoring

# Build ulang + deploy lewat PM2
deploy: build sync-dist
	pm2 startOrReload ecosystem.config.cjs --update-env
	pm2 save

# ==== Install ke /opt/monitoring (Debian, PM2) ====
install: build sync-dist
	sudo mkdir -p /opt/monitoring/bin /opt/monitoring/.data/logs
	sudo cp bin/monitoring-server /opt/monitoring/bin/
	sudo rm -rf /opt/monitoring/web
	sudo cp -r web /opt/monitoring/web
	sudo cp ecosystem.config.cjs /opt/monitoring/
	@if [ ! -f /opt/monitoring/.env ]; then sudo cp .env.example /opt/monitoring/.env && sudo chmod 600 /opt/monitoring/.env; fi
	@echo "Selesai. Langkah berikutnya:"
	@echo "  cd /opt/monitoring"
	@echo "  nano .env          # isi DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY"
	@echo "  pm2 start ecosystem.config.cjs && pm2 save"