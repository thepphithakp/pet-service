-- =============================================================================
-- V8 — ผูก transactional data เข้ากับ master data ด้วย FK แบบ NOT VALID
-- =============================================================================
-- ทำไมต้อง NOT VALID:
--   PostgreSQL รองรับ FK ที่ "บังคับกับแถวใหม่ แต่ไม่ตรวจแถวเดิม"
--   ทำให้เพิ่ม referential integrity ได้ทันทีโดยไม่ต้องแตะข้อมูลเดิมเลย
--   → app ทำงานได้เหมือนเดิม 100% และข้อมูลไม่หายแม้แต่แถวเดียว
--   เมื่อข้อมูลสะอาดแล้วค่อย ALTER TABLE ... VALIDATE CONSTRAINT ทีหลัง (แยก PR)
--
-- ทำไมไม่ใช้ CHECK:
--   master data พวกนี้แก้ผ่าน backoffice UI ได้ ถ้าใช้ CHECK แล้ว admin
--   เพิ่มค่าใหม่ผ่าน UI จะบันทึกข้อมูลด้วยค่านั้นไม่ได้
--   FK ให้ความปลอดภัยพอกัน แต่ขยายค่าใหม่ได้โดยไม่ต้อง migration
--
-- FK ยังให้สิ่งที่ CHECK ให้ไม่ได้: กัน hard delete master data ที่มีคนใช้อยู่
-- =============================================================================

-- --- ขั้นที่ 1: ดูดค่าที่มีอยู่จริงในข้อมูลเข้ามาเป็น master ก่อน ----------
-- ทำแบบนี้เพื่อไม่ต้องเดาว่า prod เก็บค่าอะไรไว้บ้าง (โค้ดเดิมไม่ validate เลย)
-- ค่าที่ดูดเข้ามาจะถูก mark is_active = false → ไม่โผล่ใน dropdown ของแอป
-- แต่ข้อมูลเดิมยังแสดงผลได้ปกติ และ FK ไม่พัง
INSERT INTO pet.mst_litter_types (code, name_en, sort_order, is_active)
SELECT DISTINCT l.type, l.type, 900, false
FROM pet.litter_logs l
WHERE l.type IS NOT NULL AND l.type <> ''
ON CONFLICT (code) DO NOTHING;

INSERT INTO pet.mst_genders (code, name_en, sort_order, is_active)
SELECT DISTINCT p.gender, p.gender, 900, false
FROM pet.pets p
WHERE p.gender IS NOT NULL AND p.gender <> ''
ON CONFLICT (code) DO NOTHING;

INSERT INTO pet.mst_species (code, name_en, sort_order, is_active)
SELECT DISTINCT p.species, p.species, 900, false
FROM pet.pets p
WHERE p.species IS NOT NULL AND p.species <> ''
ON CONFLICT (code) DO NOTHING;

-- --- ขั้นที่ 2: ค่าว่างและ NULL ------------------------------------------
-- FK ยอมให้ NULL ผ่าน แต่ไม่ยอมให้สตริงว่างที่ไม่มีใน master
-- แปลงสตริงว่างเป็น NULL (ไม่กระทบ client เพราะ JSON คืน null แทน "" ซึ่งทั้งคู่ falsy)
UPDATE pet.pets        SET gender  = NULL WHERE gender  = '';
UPDATE pet.pets        SET species = NULL WHERE species = '';
UPDATE pet.litter_logs SET type    = NULL WHERE type    = '';

-- --- ขั้นที่ 3: ใส่ FK แบบ NOT VALID -------------------------------------
ALTER TABLE pet.litter_logs
    ADD CONSTRAINT fk_litter_logs_type FOREIGN KEY (type)
    REFERENCES pet.mst_litter_types (code) NOT VALID;

ALTER TABLE pet.pets
    ADD CONSTRAINT fk_pets_gender FOREIGN KEY (gender)
    REFERENCES pet.mst_genders (code) NOT VALID;

ALTER TABLE pet.pets
    ADD CONSTRAINT fk_pets_species FOREIGN KEY (species)
    REFERENCES pet.mst_species (code) NOT VALID;

-- 🔸 pets.breed และ pets.blood_type ยังไม่ใส่ FK ในรอบนี้ (ระดับ L1: validate ที่ชั้น app)
--    เพราะ breed เก็บเป็น display string เช่น 'Scottish Fold (หูพับ)'
--    ส่วน mst_cat_breeds.code คือ 'SCOTTISH_FOLD' → ผูกตรงๆ ไม่ได้
--    ต้องทำ column migration 3 เฟส (breed_code + backfill + dual-write) ใน Phase 5
--
-- 🔸 เมื่อข้อมูลสะอาดครบแล้ว ให้รันแยก PR (ล็อกเบา ไม่บล็อก read/write):
--      ALTER TABLE pet.litter_logs VALIDATE CONSTRAINT fk_litter_logs_type;
--      ALTER TABLE pet.pets        VALIDATE CONSTRAINT fk_pets_gender;
--      ALTER TABLE pet.pets        VALIDATE CONSTRAINT fk_pets_species;
