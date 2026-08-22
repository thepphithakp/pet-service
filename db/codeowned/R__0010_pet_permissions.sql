-- =============================================================================
-- Repeatable — master permission ของ caregiver
-- =============================================================================
-- ✅ ใช้ R__ ได้ เพราะเป็น "ชั้น A (code-owned)"
--    การเพิ่ม permission ใหม่ต้องมีโค้ดที่บังคับใช้มันด้วยเสมอ
--    เพิ่มผ่าน UI แล้วจะเป็น permission ที่ไม่ทำอะไรเลย → ไม่เปิดให้แก้ผ่าน backoffice
--
-- ต่างจาก mst_cat_breeds / mst_blood_types / mst_litter_types / mst_genders
-- ที่แก้ผ่าน UI ได้ และต้อง seed ด้วย V__ ครั้งเดียวเท่านั้น
-- =============================================================================

INSERT INTO pet.pet_permissions (id, name, description, is_active) VALUES
    ('EDIT_PROFILE',   'Edit Profile',           'Can edit pet''s basic profile details',      true),
    ('MANAGE_MEDICAL', 'Manage Medical Records', 'Can view and add medical records/vaccines',  true),
    ('MANAGE_WEIGHT',  'Update Weight Log',      'Can add weight records',                     true),
    ('MANAGE_TASKS',   'Manage Daily Tasks',     'Can view and tick off daily tasks',          true),
    ('MANAGE_LITTER',  'Record Litter Box',      'Can record poop and pee events',             true),
    ('MANAGE_WATER',   'Record Water Intake',    'Can record water intake logs',               true)
ON CONFLICT (id) DO UPDATE SET
    name        = EXCLUDED.name,
    description = EXCLUDED.description,
    is_active   = EXCLUDED.is_active;

-- permission ที่ถูกถอดออกจากไฟล์นี้ → ปิดการใช้งาน ไม่ DELETE
-- เพราะ caregiver_permissions อ้างอยู่ (FK) และ log เดิมต้องยังอ่านได้
UPDATE pet.pet_permissions SET is_active = false
WHERE is_active = true
  AND id NOT IN ('EDIT_PROFILE','MANAGE_MEDICAL','MANAGE_WEIGHT',
                 'MANAGE_TASKS','MANAGE_LITTER','MANAGE_WATER');
