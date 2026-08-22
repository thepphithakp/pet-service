-- =============================================================================
-- V5 — Constraint และ index ที่ AutoMigrate ทำให้ไม่ได้
-- =============================================================================
-- 🚫 ห้ามใส่ CHECK (type IN ('POOP','PEE')) ที่ litter_logs
--    litter type แก้ผ่าน backoffice UI ได้ ถ้าใส่ CHECK ไว้ admin เพิ่มชนิดใหม่
--    ผ่าน UI สำเร็จ แต่บันทึก log ด้วยชนิดนั้นไม่ได้ = ฟีเจอร์พังทันที
--    ใช้ FOREIGN KEY ไป mst_litter_types แบบ NOT VALID แทน → V8
-- =============================================================================

-- --- ทำความสะอาดก่อนใส่ constraint ---------------------------------------
-- โค้ดปัจจุบันไม่ validate amount เลย จึงอาจมี 0 หรือติดลบอยู่
-- เก็บเข้าตารางกักกันก่อน แล้วค่อยแก้เป็น 1 (ไม่ลบแถวทิ้ง — ข้อมูลต้องครบ)
INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data, reason)
SELECT 'litter_logs', to_jsonb(l), 'amount <= 0 ก่อนใส่ CHECK constraint'
FROM pet.litter_logs l WHERE l.amount IS NULL OR l.amount <= 0;

INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data, reason)
SELECT 'water_logs', to_jsonb(w), 'amount <= 0 ก่อนใส่ CHECK constraint'
FROM pet.water_logs w WHERE w.amount IS NULL OR w.amount <= 0;

UPDATE pet.litter_logs SET amount = 1 WHERE amount IS NULL OR amount <= 0;
UPDATE pet.water_logs  SET amount = 1 WHERE amount IS NULL OR amount <= 0;

ALTER TABLE pet.litter_logs ADD CONSTRAINT chk_litter_amount CHECK (amount > 0);
ALTER TABLE pet.water_logs  ADD CONSTRAINT chk_water_amount  CHECK (amount > 0);

-- --- caregiver: partial unique index -------------------------------------
-- ของเดิม unique (pet_id, user_id) ไม่กรอง deleted_at ทำให้แถวที่ soft delete แล้ว
-- ยังกินที่อยู่ → เชิญคนเดิมกลับมาไม่ได้ ต้องมี logic Restore มาช่วย
-- partial index แก้ที่ต้นเหตุ ทำให้ลบ CaregiverService.Add → Restore ทิ้งได้ (C-5)
DROP INDEX IF EXISTS pet.idx_pet_user;
CREATE UNIQUE INDEX IF NOT EXISTS idx_pet_user_active
    ON pet.pet_caregivers (pet_id, user_id) WHERE deleted_at IS NULL;

-- index สำหรับ authz query (Phase 1.1 FindAccess) — ยิงทุก request ที่มี :id
CREATE INDEX IF NOT EXISTS idx_pet_caregivers_user_active
    ON pet.pet_caregivers (user_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_pet_caregivers_pet_active
    ON pet.pet_caregivers (pet_id) WHERE deleted_at IS NULL;

-- --- pets ----------------------------------------------------------------
-- ชื่อสัตว์เลี้ยงว่างไม่ควรมี แต่ข้อมูลเดิมอาจมี → เติมค่าแทนก่อน
UPDATE pet.pets SET name = 'ไม่ระบุชื่อ' WHERE name IS NULL OR btrim(name) = '';
ALTER TABLE pet.pets ALTER COLUMN name SET NOT NULL;

-- index สำหรับ list ของ owner ที่ยังไม่ถูกลบ
CREATE INDEX IF NOT EXISTS idx_pets_owner_active
    ON pet.pets (owner_id) WHERE deleted_at IS NULL;
