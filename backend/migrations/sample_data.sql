-- ============================================================
--  Monitoring - Sample Data (opsional, untuk testing)
--  Jalankan: psql $DATABASE_URL -f backend/migrations/sample_data.sql
-- ============================================================

-- 3 VPN contoh (password diisi nilai dummy terenkripsi)
INSERT INTO vpn_connections (name, vpn_type, server_address, username, password_encrypted,
                             local_ip, interface_name, is_active, status)
VALUES
  ('VPN-01', 'L2TP', '10.10.10.1', 'vpn01', 'enc:placeholder', '10.10.10.2', 'l2tp', true, 'CONNECTED'),
  ('VPN-02', 'SSTP', 'vpn.example.com', 'vpn02', 'enc:placeholder', '10.10.11.2', 'sstp', true, 'CONNECTED'),
  ('VPN-03', 'L2TP', '10.10.12.1', 'vpn03', 'enc:placeholder', '10.10.12.2', 'l2tp', false, 'DISABLED')
ON CONFLICT (name) DO NOTHING;

-- Beberapa pelanggan contoh di sekitar Jawa (Jawa Tengah)
INSERT INTO customers (customer_code, customer_name, ip_address, latitude, longitude,
                       vpn_id, monitoring_enabled, ping_interval, timeout_ms, retry_count,
                       status, latency_ms, location, last_check)
SELECT v.code, v.name, v.ip, v.lat, v.lng, vp.id, true, 10, 2000, 2,
       v.status::customer_status, v.latency, ST_SetSRID(ST_MakePoint(v.lng, v.lat), 4326), now()
FROM (VALUES
  ('JMK-0001', 'Budi Santoso',  '103.143.196.10', -7.5231, 110.8123, 'VPN-01', 'ONLINE', 8::int),
  ('JMK-0002', 'Andi Wijaya',   '103.143.196.11', -7.7600, 110.3700, 'VPN-01', 'ONLINE', 12::int),
  ('JMK-0003', 'Sari Dewi',     '103.143.196.12', -6.9175, 107.6191, 'VPN-02', 'OFFLINE', NULL::int),
  ('JMK-0004', 'Rina Marlina',  '103.143.196.13', -6.2000, 106.8166, 'VPN-02', 'ONLINE', 25::int),
  ('JMK-0005', 'Joko Susilo',   '103.143.196.14', -7.2504, 112.7508, 'VPN-01', 'WARNING', 60::int),
  ('JMK-0006', 'Dewi Lestari',  '103.143.196.15', -6.9667, 110.4167, 'VPN-03', 'WARNING', 45::int),
  ('JMK-0007', 'Agus Salim',    '103.143.196.16', -6.9000, 112.0500, 'VPN-02', 'ONLINE', 14::int),
  ('JMK-0008', 'Fitri Handayani','103.143.196.17', -7.0051, 110.4381, 'VPN-01', 'OFFLINE', NULL::int),
  ('JMK-0009', 'Hendra Gunawan','103.143.196.18', -7.1150, 109.9000, 'VPN-01', 'ONLINE', 9::int),
  ('JMK-0010', 'Lina Kusuma',   '103.143.196.19', -7.9666, 112.6333, 'VPN-02', 'ONLINE', 18::int)
) AS v(code, name, ip, lat, lng, vpn, status, latency)
JOIN vpn_connections vp ON vp.name = v.vpn;