-- =============================================================================
-- V6 — ตาราง role → capability ของ pet-service
-- =============================================================================
-- RBAC แบ่งเป็น 2 ชั้น (docs/REFACTOR_PLAN.md §Phase 1A.1):
--   ชั้นที่ 1 Global RBAC  — auth-service ใส่ roles มาใน JWT (SUPER_ADMIN / PET_ADMIN / USER)
--   ชั้นที่ 2 Resource ACL — pet_caregivers + caregiver_permissions (สิทธิ์ต่อสัตว์เลี้ยงหนึ่งตัว)
--
-- JWT พกแค่ "roles" ไม่พก permission list
-- แต่ละ service map role → capability ในตารางของตัวเอง
-- → token เล็ก, service ไม่ผูกกัน, เพิ่ม capability ใหม่ได้โดยไม่ต้องแตะ auth-service
--
-- ⚠️ ตารางนี้เป็น "ชั้น A (code-owned)" — ไม่เปิดให้ backoffice แก้
--    เพราะ capability ผูกกับโค้ดที่บังคับใช้จริง เพิ่มผ่าน UI แล้วจะไม่มีผลอะไร
--    จึง seed ด้วย R__0005_role_capabilities.sql ได้อย่างถูกต้อง
-- =============================================================================

CREATE TABLE IF NOT EXISTS pet.role_capabilities (
    role_code  varchar(50)  NOT NULL,
    capability varchar(100) NOT NULL,
    CONSTRAINT role_capabilities_pkey PRIMARY KEY (role_code, capability)
);

CREATE INDEX IF NOT EXISTS idx_role_capabilities_role ON pet.role_capabilities (role_code);
