-- =============================================================================
-- ข้อมูลตัวอย่างสำหรับ local dev เท่านั้น
-- =============================================================================
-- 🚫 โฟลเดอร์ db/seed ต้องไม่อยู่ใน FLYWAY_LOCATIONS ของ staging/production
--    ดู db/Dockerfile — image ที่ deploy จริง COPY แค่ migration/ กับ codeowned/
-- =============================================================================

INSERT INTO pet.pets (
    id, owner_id, owner_username, name, species, breed, gender,
    birth_date, is_spayed_neutered, created_at, updated_at
) VALUES
    ('aaaaaaaa-0000-4000-8000-000000000001',
     '11111111-1111-1111-1111-111111111111', 'devuser',
     'มะลิ', 'CAT', 'Scottish Fold (หูพับ)', 'Female',
     '2021-03-15', true, now(), now()),
    ('aaaaaaaa-0000-4000-8000-000000000002',
     '11111111-1111-1111-1111-111111111111', 'devuser',
     'ส้ม', 'CAT', 'Persian', 'Male',
     '2022-07-01', false, now(), now())
ON CONFLICT (id) DO NOTHING;

INSERT INTO pet.litter_logs (id, pet_id, date, type, amount, created_at, updated_at, is_active) VALUES
    ('bbbbbbbb-0000-4000-8000-000000000001',
     'aaaaaaaa-0000-4000-8000-000000000001', now() - interval '1 day', 'Poop', 1, now(), now(), true),
    ('bbbbbbbb-0000-4000-8000-000000000002',
     'aaaaaaaa-0000-4000-8000-000000000001', now() - interval '2 hours', 'Pee', 2, now(), now(), true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO pet.water_logs (id, pet_id, date, amount, created_at, updated_at, is_active) VALUES
    ('cccccccc-0000-4000-8000-000000000001',
     'aaaaaaaa-0000-4000-8000-000000000001', now() - interval '3 hours', 45, now(), now(), true)
ON CONFLICT (id) DO NOTHING;
