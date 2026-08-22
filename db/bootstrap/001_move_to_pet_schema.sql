-- =============================================================================
-- Bootstrap 001 — ย้ายตารางจาก public ไป schema pet (รันครั้งเดียว)
-- =============================================================================
-- ⚠️ อ่าน docs/REFACTOR_PLAN.md §10 (Backup & Data Verification Runbook) ให้จบก่อนรัน
--
-- ทำไมต้องทำก่อนใส่ Flyway:
--   ถ้าใส่ Flyway ที่ public ก่อนแล้วค่อยย้าย จะต้องย้าย flyway_schema_history
--   ตามไปด้วยและแก้ FLYWAY_TABLE กลางทาง — ยุ่งกว่าโดยไม่จำเป็น
--
-- ทำไมข้อมูลไม่หาย:
--   ALTER TABLE ... SET SCHEMA เป็น metadata-only operation
--   ไม่มีการ copy ข้อมูล ไม่มีการเขียนไฟล์ใหม่ เสร็จในเสี้ยววินาที
--   และ DDL ของ PostgreSQL เป็น transactional → สำเร็จทั้งหมดหรือ rollback ทั้งหมด
--
-- ⚠️ ต้องทำตามลำดับนี้เท่านั้น:
--   1. deploy app ที่ DSN มี search_path=pet,public ก่อน  ← ห้ามข้าม
--   2. backup + บันทึก fingerprint (db/verify/fingerprint.sql)
--   3. รันไฟล์นี้
--   4. เทียบ fingerprint หลังย้าย + รัน db/verify/post_move_checks.sql
--   5. flyway baseline แล้วค่อย migrate
--
-- ถ้าข้ามขั้นที่ 1 จะได้ relation "pets" does not exist ทั้งระบบทันที
-- =============================================================================

BEGIN;

SET lock_timeout = '5s';
SET statement_timeout = '5min';

CREATE SCHEMA IF NOT EXISTS pet;

DO $$
DECLARE
    t text;
    moved int := 0;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'pets', 'pet_permissions', 'pet_caregivers',
        'caregiver_permissions', 'litter_logs', 'water_logs'
    ]
    LOOP
        IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = t) THEN
            EXECUTE format('ALTER TABLE public.%I SET SCHEMA pet', t);
            moved := moved + 1;
            RAISE NOTICE 'ย้าย public.% → pet.%', t, t;
        ELSIF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'pet' AND tablename = t) THEN
            RAISE NOTICE 'ข้าม % (อยู่ใน pet อยู่แล้ว)', t;
        ELSE
            RAISE NOTICE 'ข้าม % (ไม่พบทั้งใน public และ pet)', t;
        END IF;
    END LOOP;
    RAISE NOTICE 'ย้ายทั้งหมด % ตาราง', moved;
END $$;

-- sequence ที่อาจค้างอยู่ที่ public (bigserial ของตารางที่ย้ายมา)
DO $$
DECLARE s record;
BEGIN
    FOR s IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_depend d ON d.objid = c.oid AND d.deptype = 'a'
        JOIN pg_class t ON t.oid = d.refobjid
        JOIN pg_namespace tn ON tn.oid = t.relnamespace
        WHERE c.relkind = 'S' AND n.nspname = 'public' AND tn.nspname = 'pet'
    LOOP
        EXECUTE format('ALTER SEQUENCE public.%I SET SCHEMA pet', s.relname);
        RAISE NOTICE 'ย้าย sequence public.% → pet.%', s.relname, s.relname;
    END LOOP;
END $$;

GRANT USAGE ON SCHEMA pet TO pet_app, pet_migrator;
GRANT ALL   ON SCHEMA pet TO pet_migrator;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA pet TO pet_app;
GRANT USAGE, SELECT                  ON ALL SEQUENCES IN SCHEMA pet TO pet_app;

ALTER DEFAULT PRIVILEGES FOR ROLE pet_migrator IN SCHEMA pet
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pet_app;
ALTER DEFAULT PRIVILEGES FOR ROLE pet_migrator IN SCHEMA pet
    GRANT USAGE, SELECT ON SEQUENCES TO pet_app;

COMMIT;
