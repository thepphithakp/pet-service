-- =============================================================================
-- ตรวจหลังย้าย schema — ทุกข้อต้องผ่านก่อนไปขั้นถัดไป
-- =============================================================================

\echo '--- 1. ตารางทั้งหมดต้องอยู่ใน schema pet ไม่มีค้างที่ public ---'
SELECT schemaname, tablename
FROM pg_tables
WHERE tablename IN ('pets','pet_caregivers','pet_permissions',
                    'caregiver_permissions','litter_logs','water_logs')
ORDER BY schemaname, tablename;
-- คาดหวัง: schemaname = 'pet' ทุกแถว

\echo '--- 2. ไม่มี sequence ค้างที่ public ---'
SELECT sequencename, schemaname FROM pg_sequences WHERE schemaname = 'public';
-- คาดหวัง: 0 แถว

\echo '--- 3. FK ทั้งหมดชี้ไปที่ schema pet ---'
SELECT conname,
       conrelid::regclass  AS from_table,
       confrelid::regclass AS to_table,
       convalidated        AS validated
FROM pg_constraint
WHERE contype = 'f' AND connamespace = 'pet'::regnamespace
ORDER BY conname;

\echo '--- 4. ข้อมูลที่ถูกกักกันระหว่าง migration (ต้องว่าง) ---'
SELECT source_table, reason, count(*)
FROM pet.orphaned_logs_quarantine
GROUP BY source_table, reason;
-- ถ้าไม่ว่าง ต้องเอาไปคุยกับทีมก่อนไปต่อ

\echo '--- 5. ค่าจริงของ column ที่จะผูกกับ master data ---'
SELECT 'litter.type' AS col, coalesce(type, '(null)') AS val, count(*)
FROM pet.litter_logs GROUP BY 2
UNION ALL SELECT 'pets.gender',  coalesce(gender, '(null)'),  count(*) FROM pet.pets GROUP BY 2
UNION ALL SELECT 'pets.species', coalesce(species, '(null)'), count(*) FROM pet.pets GROUP BY 2
ORDER BY 1, 3 DESC;
-- ทุกค่าที่ไม่ใช่ null ต้องมีอยู่ใน mst_* ที่สอดคล้องกัน (V8 ดูดเข้ามาให้อัตโนมัติแล้ว)

\echo '--- 6. Flyway history ---'
SELECT installed_rank, version, description, type, success
FROM pet.flyway_schema_history ORDER BY installed_rank;

\echo '--- 7. pet_app ต้องไม่มีสิทธิ์ DDL ---'
SELECT has_schema_privilege('pet_app', 'pet', 'CREATE') AS can_create_in_pet;
-- คาดหวัง: false
