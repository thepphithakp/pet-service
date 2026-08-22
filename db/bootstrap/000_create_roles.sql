-- =============================================================================
-- Bootstrap 000 — DB user แยกสิทธิ์ (รันครั้งเดียว ด้วย superuser)
-- =============================================================================
-- ไฟล์ในโฟลเดอร์ bootstrap/ ไม่อยู่ใน FLYWAY_LOCATIONS — เป็น runbook script
-- ที่รันด้วยมือครั้งเดียวตอนตั้งระบบ ไม่ใช่ migration
--
-- วิธีรัน:
--   psql "$SUPERUSER_DSN" -v migrator_pw="'...'" -v app_pw="'...'" -f 000_create_roles.sql
-- =============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pet_migrator') THEN
        EXECUTE format('CREATE ROLE pet_migrator LOGIN PASSWORD %L', :migrator_pw);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pet_app') THEN
        EXECUTE format('CREATE ROLE pet_app LOGIN PASSWORD %L', :app_pw);
    END IF;
END $$;

-- pet_migrator: มีสิทธิ์ DDL ใช้เฉพาะ Flyway Job
-- pet_app:      DML เท่านั้น ใช้ใน runtime
--
-- 🔐 ผลที่ได้: pet_app รัน CREATE/ALTER/DROP TABLE ไม่ได้
--    → AutoMigrate กลับมาเองไม่ได้อีกตลอดกาล
--    → แม้โดน SQL injection ก็ DROP TABLE ไม่ได้
