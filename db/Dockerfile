# =============================================================================
# Migration image — build และ push พร้อม app image ด้วย tag เดียวกัน (sha-<commit>)
# =============================================================================
# ทำไมต้อง build image แทนการ mount ConfigMap:
#   ConfigMap จำกัด 1MB และ helm .Files อ่านนอก chart directory ไม่ได้
#   image ทำให้ schema version ผูกกับ app version เสมอ
#
# ⚠️ pin version ไว้ ห้ามใช้ latest — Flyway major upgrade เปลี่ยนพฤติกรรม checksum ได้
FROM flyway/flyway:11-alpine

COPY migration  /flyway/sql/migration
COPY codeowned  /flyway/sql/codeowned

# 🚫 ไม่ COPY db/seed เข้ามา — ข้อมูลตัวอย่างต้องไม่มีทางหลุดไป production
