-- =============================================================================
-- Rollback ของ 001_move_to_pet_schema.sql
-- =============================================================================
-- ใช้เมื่อต้องถอยกลับหลังจาก COMMIT ไปแล้ว
-- app ที่มี search_path=pet,public จะยังทำงานได้ทั้งก่อนและหลังรันไฟล์นี้
-- =============================================================================

BEGIN;
SET lock_timeout = '5s';

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'pets', 'pet_permissions', 'pet_caregivers',
        'caregiver_permissions', 'litter_logs', 'water_logs'
    ]
    LOOP
        IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'pet' AND tablename = t) THEN
            EXECUTE format('ALTER TABLE pet.%I SET SCHEMA public', t);
        END IF;
    END LOOP;
END $$;

COMMIT;
