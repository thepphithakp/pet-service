-- =============================================================================
-- V4 — FK จาก log ไป pets + soft delete ให้ตรงกันทั้งระบบ
-- =============================================================================
-- แก้ C-9 (delete semantics ไม่ตรงกัน) และ C-12 (ไม่มี FK ทำให้เกิด orphan)
-- litter_logs / water_logs เดิมเป็น hard delete ส่วน pets / pet_caregivers เป็น soft delete
-- =============================================================================

-- ตารางกักกัน: ข้อมูลที่ต้องเอาออกเพื่อให้ใส่ FK ได้ จะถูกเก็บไว้ที่นี่ ไม่ลบทิ้งเฉยๆ
-- ถ้าหลัง migrate ตารางนี้ว่าง แปลว่าไม่มีอะไรหาย → drop ทิ้งได้
-- ถ้าไม่ว่าง ต้องเอาไปคุยกับทีมก่อนตัดสินใจ
CREATE TABLE IF NOT EXISTS pet.orphaned_logs_quarantine (
    id             bigserial PRIMARY KEY,
    source_table   text        NOT NULL,
    row_data       jsonb       NOT NULL,
    reason         text        NOT NULL,
    quarantined_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data, reason)
SELECT 'litter_logs', to_jsonb(l), 'pet_id ไม่มีอยู่ในตาราง pets'
FROM pet.litter_logs l
WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = l.pet_id);

INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data, reason)
SELECT 'water_logs', to_jsonb(w), 'pet_id ไม่มีอยู่ในตาราง pets'
FROM pet.water_logs w
WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = w.pet_id);

DELETE FROM pet.litter_logs l WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = l.pet_id);
DELETE FROM pet.water_logs  w WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = w.pet_id);

-- soft delete ให้เหมือน pets / pet_caregivers
ALTER TABLE pet.litter_logs ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE pet.water_logs  ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_litter_logs_deleted_at ON pet.litter_logs (deleted_at);
CREATE INDEX IF NOT EXISTS idx_water_logs_deleted_at  ON pet.water_logs (deleted_at);

-- updated_by ที่ขาดอยู่ (pets/pet_caregivers มี แต่ log ไม่มี)
ALTER TABLE pet.litter_logs ADD COLUMN IF NOT EXISTS updated_by text;
ALTER TABLE pet.water_logs  ADD COLUMN IF NOT EXISTS updated_by text;

-- ⚠️ ON DELETE CASCADE ทำงานเฉพาะตอน hard delete เท่านั้น
--    pets ใช้ soft delete อยู่ → การซ่อน log ตาม pet ที่ถูกลบ ต้องทำที่ชั้น query
--    (query log ผ่าน pet เสมอ ห้าม query log ตรงๆ ข้าม pet)
ALTER TABLE pet.litter_logs
    ADD CONSTRAINT fk_litter_logs_pet FOREIGN KEY (pet_id)
    REFERENCES pet.pets (id) ON DELETE CASCADE;

ALTER TABLE pet.water_logs
    ADD CONSTRAINT fk_water_logs_pet FOREIGN KEY (pet_id)
    REFERENCES pet.pets (id) ON DELETE CASCADE;
