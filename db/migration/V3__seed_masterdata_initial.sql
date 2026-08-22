-- =============================================================================
-- V3 — Seed master data ครั้งแรก (รันครั้งเดียวแล้วจบ)
-- =============================================================================
-- 🚫 ห้ามย้ายไฟล์นี้ไปเป็น R__ เด็ดขาด
--    master data เหล่านี้แก้ผ่าน backoffice UI ได้ ถ้าใช้ R__ ทุกครั้งที่ deploy
--    จะเขียนทับสิ่งที่ admin แก้ไว้ (docs/REFACTOR_PLAN.md §Phase 3.1)
--    หลังจากนี้ database เป็น source of truth ไม่ใช่ git
--
-- ค่าที่ seed ต้องตรงกับที่ internal/application/litter_service.go เคย hardcode ไว้
-- ทุกตัวอักษร เพื่อให้ API v1 คืนค่าเหมือนเดิม (มี golden test เฝ้าอยู่)
-- =============================================================================

INSERT INTO pet.mst_species (code, name_en, name_th, sort_order, is_active) VALUES
    ('CAT', 'Cat', 'แมว', 10, true),
    ('DOG', 'Dog', 'สุนัข', 20, true)
ON CONFLICT (code) DO NOTHING;

-- legacy_label คือสตริงที่ GET /api/v1/master-data/cat-breeds เคยคืน — ห้ามเปลี่ยน
INSERT INTO pet.mst_cat_breeds (code, species_code, name_en, name_th, legacy_label, sort_order, is_active) VALUES
    ('SCOTTISH_FOLD',      'CAT', 'Scottish Fold',      'สก็อตติชโฟลด์',    'Scottish Fold (หูพับ)',            10,  true),
    ('SCOTTISH_STRAIGHT',  'CAT', 'Scottish Straight',  'สก็อตติชสเตรท',    'Scottish Straight (หูตั้ง)',        20,  true),
    ('BRITISH_SHORTHAIR',  'CAT', 'British Shorthair',  'บริติชชอร์ตแฮร์',  'British Shorthair',                30,  true),
    ('PERSIAN',            'CAT', 'Persian',            'เปอร์เซีย',        'Persian',                          40,  true),
    ('MAINE_COON',         'CAT', 'Maine Coon',         'เมนคูน',           'Maine Coon',                       50,  true),
    ('SIAMESE',            'CAT', 'Siamese',            'วิเชียรมาศ',       'Siamese (วิเชียรมาศ)',              60,  true),
    ('KHAO_MANEE',         'CAT', 'Khao Manee',         'ขาวมณี',           'Khao Manee (ขาวมณี)',               70,  true),
    ('SPHYNX',             'CAT', 'Sphynx',             'สฟิงซ์',           'Sphynx',                           80,  true),
    ('BENGAL',             'CAT', 'Bengal',             'เบงกอล',           'Bengal',                           90,  true),
    ('RAGDOLL',            'CAT', 'Ragdoll',            'แร็กดอลล์',        'Ragdoll',                          100, true),
    ('AMERICAN_SHORTHAIR', 'CAT', 'American Shorthair', 'อเมริกันชอร์ตแฮร์','American Shorthair',               110, true),
    ('EXOTIC_SHORTHAIR',   'CAT', 'Exotic Shorthair',   'เอ็กโซติกชอร์ตแฮร์','Exotic Shorthair',                120, true),
    ('MUNCHKIN',           'CAT', 'Munchkin',           'มันช์กิน',         'Munchkin (ขาสั้น)',                 130, true),
    ('MIXED',              'CAT', 'Mixed / Other',      'พันธุ์ผสม/อื่นๆ',  'Mixed / Other (พันธุ์ผสม/อื่นๆ)',   999, true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO pet.mst_blood_types (code, name_en, name_th, legacy_label, sort_order, is_active) VALUES
    ('UNKNOWN', 'Unknown', 'ไม่ทราบ', 'Unknown', 10, true),
    ('A',       'A',       'A',       'A',       20, true),
    ('B',       'B',       'B',       'B',       30, true),
    ('AB',      'AB',      'AB',      'AB',      40, true)
ON CONFLICT (code) DO NOTHING;

-- ⚠️ code ของ litter type ต้องตรงกับ "ค่าที่เก็บอยู่จริงใน litter_logs.type"
--    ไม่ใช่ค่าที่ดูสวย เพราะค่านี้ถูกส่งกลับให้ client ทาง API ตรงๆ
--    internal/domain/litter_log.go เขียนไว้ว่าค่าคือ "Poop" หรือ "Pee" (ตัวพิมพ์ผสม)
--    การ normalize เป็นตัวพิมพ์ใหญ่เป็น breaking change ที่ต้องประสานกับ client ก่อน
--    → แยกเป็นงานต่างหาก ไม่ทำในนี้
--
--    V8 จะดูดค่าอื่นที่พบในข้อมูลจริงเข้ามาเป็น inactive ให้อัตโนมัติ
--    จึงไม่ต้องเดาว่า prod มีค่าอะไรบ้าง
INSERT INTO pet.mst_litter_types (code, name_en, name_th, sort_order, is_active) VALUES
    ('Poop', 'Poop', 'อุจจาระ', 10, true),
    ('Pee',  'Pee',  'ปัสสาวะ', 20, true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO pet.mst_genders (code, name_en, name_th, sort_order, is_active) VALUES
    ('Male',    'Male',    'เพศผู้',   10, true),
    ('Female',  'Female',  'เพศเมีย',  20, true),
    ('Unknown', 'Unknown', 'ไม่ทราบ',  30, true)
ON CONFLICT (code) DO NOTHING;
