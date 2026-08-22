-- =============================================================================
-- Fingerprint — พิสูจน์ว่าข้อมูลครบและไม่เปลี่ยน
-- =============================================================================
-- รันก่อนและหลังทุกครั้งที่แตะข้อมูล แล้ว diff ผลลัพธ์กัน
--
--   psql "$DSN" -f db/verify/fingerprint.sql > before.txt
--   ... ทำ migration ...
--   psql "$DSN" -f db/verify/fingerprint.sql > after.txt
--   diff before.txt after.txt
--
-- การตีความ:
--   ทุกค่าตรงกัน                → ข้อมูลเหมือนเดิมเป๊ะ ✅
--   n + id_hash ตรง row_hash ต่าง → จำนวนและตัวตนครบ แต่ค่าในบางแถวเปลี่ยน
--                                  ⚠️ ถูกต้องเฉพาะเมื่อเป็น migration ที่ตั้งใจแปลงข้อมูล
--                                     ต้องอธิบายได้ว่าแถวไหนเปลี่ยนเพราะอะไร
--   n ตรง แต่ id_hash ต่าง        → จำนวนเท่ากันแต่คนละแถว 🚨 หยุดทันที rollback
--   n ต่าง                       → ข้อมูลหายหรือเกิน 🚨 หยุดทันที rollback
--
-- สำหรับ ALTER TABLE ... SET SCHEMA ทั้งสามค่าต้องตรง 100%
-- เพราะไม่มีการแตะข้อมูลเลย ถ้าไม่ตรงแปลว่ามีคนเขียนเข้ามาระหว่างนั้น
-- หรือย้ายตารางไม่ครบ
-- =============================================================================

\pset format unaligned
\pset fieldsep |
\pset tuples_only off

SELECT 'pets' AS tbl, count(*) AS n,
       md5(coalesce(string_agg(id::text, ',' ORDER BY id), '')) AS id_hash,
       md5(coalesce(string_agg(md5(p.*::text), ',' ORDER BY p.id), '')) AS row_hash
FROM pets p
UNION ALL
SELECT 'pet_caregivers', count(*),
       md5(coalesce(string_agg(id::text, ',' ORDER BY id), '')),
       md5(coalesce(string_agg(md5(c.*::text), ',' ORDER BY c.id), ''))
FROM pet_caregivers c
UNION ALL
SELECT 'litter_logs', count(*),
       md5(coalesce(string_agg(id::text, ',' ORDER BY id), '')),
       md5(coalesce(string_agg(md5(l.*::text), ',' ORDER BY l.id), ''))
FROM litter_logs l
UNION ALL
SELECT 'water_logs', count(*),
       md5(coalesce(string_agg(id::text, ',' ORDER BY id), '')),
       md5(coalesce(string_agg(md5(w.*::text), ',' ORDER BY w.id), ''))
FROM water_logs w
UNION ALL
SELECT 'pet_permissions', count(*),
       md5(coalesce(string_agg(id::text, ',' ORDER BY id), '')),
       md5(coalesce(string_agg(md5(pp.*::text), ',' ORDER BY pp.id), ''))
FROM pet_permissions pp
ORDER BY tbl;

-- join table ไม่มี PK เดี่ยว จึง hash จากคู่คีย์
SELECT 'caregiver_permissions' AS tbl, count(*) AS n,
       md5(coalesce(string_agg(caregiver_model_id::text || ':' || permission_model_id,
                    ',' ORDER BY caregiver_model_id, permission_model_id), '')) AS id_hash,
       NULL AS row_hash
FROM caregiver_permissions;
