-- =============================================================================
-- V7 — เพิ่ม permission MANAGE_WATER
-- =============================================================================
-- เดิมมี API บันทึกการดื่มน้ำ แต่ไม่มี permission คุมเลย
-- ตอนนี้ Phase 1.2 จะเริ่มบังคับสิทธิ์ จึงต้องมี permission ตัวนี้ก่อน
-- =============================================================================

INSERT INTO pet.pet_permissions (id, name, description, is_active) VALUES
    ('MANAGE_WATER', 'Record Water Intake', 'Can record water intake logs', true)
ON CONFLICT (id) DO NOTHING;

-- ❗ Backfill สำคัญมาก — ห้ามลืม
-- ก่อนหน้านี้ "ใครก็บันทึกน้ำได้" เพราะไม่มีการเช็คสิทธิ์เลย
-- ถ้าเริ่มบังคับ MANAGE_WATER โดยไม่ backfill caregiver ที่เคยบันทึกน้ำได้
-- จะบันทึกไม่ได้ทันทีหลัง deploy = พฤติกรรมเปลี่ยนสำหรับผู้ใช้จริง
--
-- ให้สิทธิ์กับ caregiver ที่มี MANAGE_TASKS อยู่แล้ว (งานประจำวันรวมการให้น้ำ)
INSERT INTO pet.caregiver_permissions (caregiver_model_id, permission_model_id)
SELECT cp.caregiver_model_id, 'MANAGE_WATER'
FROM pet.caregiver_permissions cp
WHERE cp.permission_model_id = 'MANAGE_TASKS'
ON CONFLICT DO NOTHING;

-- 🔸 ต้องตัดสินใจแยกต่างหาก: MANAGE_MEDICAL และ MANAGE_WEIGHT ยังไม่มี API รองรับเลย
--    จะทำ API ให้ หรือจะ deactivate ทิ้ง — อย่าปล่อยเป็น permission ลอยๆ
