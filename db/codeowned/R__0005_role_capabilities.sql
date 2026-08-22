-- =============================================================================
-- Repeatable — role → capability ของ pet-service
-- =============================================================================
-- ✅ ใช้ R__ ได้อย่างถูกต้อง เพราะตารางนี้เป็น "ชั้น A (code-owned)"
--    capability ผูกกับโค้ดที่บังคับใช้จริง จึงต้องมาคู่กับ code change เสมอ
--    และไม่เปิดให้ backoffice แก้ → ไม่มีอะไรให้เขียนทับ
--
-- แก้ไฟล์นี้แล้วรัน flyway migrate ซ้ำได้เลย Flyway จะ re-apply ให้เมื่อ checksum เปลี่ยน
-- =============================================================================

INSERT INTO pet.role_capabilities (role_code, capability) VALUES
    ('SUPER_ADMIN', 'pet:read:any'),
    ('SUPER_ADMIN', 'pet:write:any'),
    ('SUPER_ADMIN', 'pet:delete:any'),
    ('SUPER_ADMIN', 'caregiver:manage:any'),
    ('SUPER_ADMIN', 'log:read:any'),
    ('SUPER_ADMIN', 'log:write:any'),
    ('SUPER_ADMIN', 'masterdata:write'),
    ('PET_ADMIN',   'pet:read:any'),
    ('PET_ADMIN',   'pet:write:any'),
    ('PET_ADMIN',   'log:read:any'),
    ('PET_ADMIN',   'masterdata:write')
ON CONFLICT (role_code, capability) DO NOTHING;

-- ลบ capability ที่ถูกถอดออกจากไฟล์นี้ ให้ตารางตรงกับ git เสมอ
DELETE FROM pet.role_capabilities
WHERE (role_code, capability) NOT IN (
    ('SUPER_ADMIN', 'pet:read:any'),
    ('SUPER_ADMIN', 'pet:write:any'),
    ('SUPER_ADMIN', 'pet:delete:any'),
    ('SUPER_ADMIN', 'caregiver:manage:any'),
    ('SUPER_ADMIN', 'log:read:any'),
    ('SUPER_ADMIN', 'log:write:any'),
    ('SUPER_ADMIN', 'masterdata:write'),
    ('PET_ADMIN',   'pet:read:any'),
    ('PET_ADMIN',   'pet:write:any'),
    ('PET_ADMIN',   'log:read:any'),
    ('PET_ADMIN',   'masterdata:write')
);
