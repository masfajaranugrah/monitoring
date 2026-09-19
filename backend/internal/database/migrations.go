package database

import (
	"context"
	"log"
)

var schema = `
CREATE EXTENSION IF NOT EXISTS postgis;

DO $$ BEGIN
  CREATE TYPE customer_status AS ENUM ('ONLINE', 'OFFLINE', 'WARNING');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE vpn_status AS ENUM ('CONNECTED', 'DISCONNECTED', 'ERROR', 'DISABLED');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE vpn_type AS ENUM ('L2TP', 'SSTP', 'PPTP', 'OPENVPN', 'WIREGUARD');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Extend enum in case DB already existed without PPTP
DO $$ BEGIN
  ALTER TYPE vpn_type ADD VALUE IF NOT EXISTS 'PPTP';
EXCEPTION WHEN duplicate_object THEN NULL; WHEN undefined_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE role_type AS ENUM ('ADMIN', 'OPERATOR');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  full_name VARCHAR(150) NOT NULL DEFAULT '',
  role role_type NOT NULL DEFAULT 'OPERATOR',
  is_active BOOLEAN NOT NULL DEFAULT true,
  last_login TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vpn_connections (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(50) NOT NULL UNIQUE,
  vpn_type vpn_type NOT NULL DEFAULT 'L2TP',
  server_address VARCHAR(255) NOT NULL,
  username VARCHAR(100) NOT NULL,
  password_encrypted TEXT NOT NULL,
  local_ip VARCHAR(45),
  interface_name VARCHAR(100) DEFAULT 'l2tp',
  is_active BOOLEAN NOT NULL DEFAULT true,
  status vpn_status NOT NULL DEFAULT 'DISCONNECTED',
  latency_ms INTEGER,
  last_connected_at TIMESTAMPTZ,
  customer_count INTEGER NOT NULL DEFAULT 0,
  description TEXT DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customers (
  id BIGSERIAL PRIMARY KEY,
  customer_code VARCHAR(50) NOT NULL UNIQUE,
  customer_name VARCHAR(200) NOT NULL,
  ip_address VARCHAR(45) NOT NULL,
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  location GEOMETRY(Point, 4326),
  vpn_id BIGINT REFERENCES vpn_connections(id) ON DELETE SET NULL,
  description TEXT DEFAULT '',
  monitoring_enabled BOOLEAN NOT NULL DEFAULT true,
  ping_interval INTEGER NOT NULL DEFAULT 10,
  timeout_ms INTEGER NOT NULL DEFAULT 2000,
  retry_count INTEGER NOT NULL DEFAULT 2,
  status customer_status NOT NULL DEFAULT 'OFFLINE',
  latency_ms INTEGER,
  last_check TIMESTAMPTZ,
  last_online TIMESTAMPTZ,
  last_offline TIMESTAMPTZ,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  total_checks BIGINT NOT NULL DEFAULT 0,
  uptime_percentage NUMERIC(5,2) NOT NULL DEFAULT 100.00,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_customers_vpn_id ON customers(vpn_id);
CREATE INDEX IF NOT EXISTS idx_customers_status ON customers(status);
CREATE INDEX IF NOT EXISTS idx_customers_location ON customers USING GIST(location);
CREATE INDEX IF NOT EXISTS idx_customers_code ON customers(customer_code);

CREATE TABLE IF NOT EXISTS ping_results (
  id BIGSERIAL PRIMARY KEY,
  customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  vpn_id BIGINT REFERENCES vpn_connections(id) ON DELETE SET NULL,
  status customer_status NOT NULL,
  latency_ms INTEGER,
  pinged_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ping_results_customer_time ON ping_results(customer_id, pinged_at DESC);
CREATE INDEX IF NOT EXISTS idx_ping_results_time ON ping_results(pinged_at DESC);

CREATE TABLE IF NOT EXISTS customer_status_logs (
  id BIGSERIAL PRIMARY KEY,
  customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  old_status customer_status,
  new_status customer_status NOT NULL,
  latency_ms INTEGER,
  reason VARCHAR(255) DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_status_logs_customer ON customer_status_logs(customer_id, created_at DESC);

CREATE TABLE IF NOT EXISTS alerts (
  id BIGSERIAL PRIMARY KEY,
  customer_id BIGINT REFERENCES customers(id) ON DELETE SET NULL,
  alert_type VARCHAR(50) NOT NULL,
  severity VARCHAR(20) NOT NULL DEFAULT 'WARNING',
  title VARCHAR(200) NOT NULL,
  message TEXT NOT NULL,
  is_read BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_read ON alerts(is_read);

CREATE TABLE IF NOT EXISTS map_features (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL,
  feature_type VARCHAR(20) NOT NULL DEFAULT 'point',
  icon VARCHAR(30) NOT NULL DEFAULT 'dot',
  color VARCHAR(20) NOT NULL DEFAULT '#3b82f6',
  description TEXT DEFAULT '',
  geometry JSONB NOT NULL,
  properties JSONB DEFAULT '{}'::jsonb,
  source VARCHAR(10) NOT NULL DEFAULT 'manual',
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_map_features_type ON map_features(feature_type);
CREATE INDEX IF NOT EXISTS idx_map_features_created ON map_features(created_at DESC);
`

func RunMigrations(ctx context.Context) error {
	log.Println("Running database migrations...")
	_, err := Pool.Exec(ctx, schema)
	if err != nil {
		return err
	}
	log.Println("Database migrations completed")
	return nil
}