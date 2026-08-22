# Runbook — Rotate คีย์ที่ใช้เซ็น JWT

> 🔴 **ทำไมต้องทำ:** `vertex-auth-service/keys/private.pem` ถูก commit เข้า git
> ตั้งแต่ commit แรก และเป็นคีย์ที่คู่กับ public key ที่ pet-service ใช้ verify
> (ยืนยันแล้วด้วยการเทียบ modulus)
>
> ใครที่เข้าถึง repo ได้จึงปลอม token เป็นใครก็ได้ รวมถึง `SUPER_ADMIN`
> **ตราบใดที่ยังไม่ rotate งาน RBAC ทั้งหมดไม่มีความหมาย**
>
> การลบไฟล์ออกจาก git หยุดได้แค่ commit ใหม่ — คีย์ตัวเดิมยังอยู่ใน history
> และในเครื่องของทุกคนที่เคย clone ต้องถือว่ารั่วถาวร

---

## 1. หลักการ — ทำไมถึงไม่มี downtime

ปัญหาของการเปลี่ยนคีย์คือ **token ที่ผู้ใช้ถืออยู่มีอายุ 72 ชั่วโมง**
ถ้าเปลี่ยนคีย์ทันที token ทุกใบที่ยังไม่หมดอายุจะใช้ไม่ได้พร้อมกันทั้งระบบ

ทางแก้คือให้ผู้ตรวจ **ยอมรับคีย์สองใบพร้อมกัน** ในช่วงเปลี่ยนผ่าน:

```
                    ┌──────────── ยอมรับทั้งสองใบ ────────────┐
เซ็นด้วยคีย์เก่า ────┤                                          ├──── เซ็นด้วยคีย์ใหม่
                    │  token เก่ายังตรวจผ่าน จนหมดอายุเอง       │
                    └──────────── อย่างน้อย 72 ชม. ────────────┘
```

**`kid` ทำให้เลือกคีย์ได้ถูกใบ**
ทั้งสอง service คำนวณ `kid` จาก SHA-256 thumbprint ของตัว public key เอง
จึงได้ค่าตรงกันเสมอโดยไม่ต้องตกลงชื่อกันล่วงหน้าและตั้งค่าผิดไม่ได้

- token ใหม่มี `kid` → ผู้ตรวจเลือกใบที่ตรงทันที
- token เก่าไม่มี `kid` → ลองทีละใบ (`jwt.VerificationKeySet`)

**รูปแบบการใส่หลายคีย์**: PEM ระบุขอบเขตของตัวเองอยู่แล้ว จึงต่อกันหลายบล็อก
ใน environment variable ตัวเดียวได้เลย ไม่ต้องมีตัวคั่นพิเศษ

---

## 2. เตรียมคีย์ใหม่

```bash
# สร้างคู่คีย์ใหม่ — เก็บไว้นอก repo เท่านั้น
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out new-private.pem
openssl rsa -pubout -in new-private.pem -out new-public.pem
chmod 600 new-private.pem
```

🚫 **ห้าม `git add` ไฟล์เหล่านี้เด็ดขาด** — มี CI job คอยตรวจอยู่แล้ว
แต่ CI จับได้หลังจากคุณ commit ไปแล้ว จึงต้องระวังตั้งแต่ต้น

ดึง public key เก่าที่ใช้อยู่ตอนนี้:
```bash
kubectl get secret jwt-keys -n vertex -o jsonpath='{.data.public\.pem}' | base64 -d > old-public.pem
# หรือถ้ายังอ่านจากไฟล์ใน image อยู่
curl -s https://<auth-host>/api/v1/auth/public-key > old-public.pem
```

---

## 3. ขั้นตอน — ห้ามสลับลำดับ

### เฟส 0 · ตรวจสถานะปัจจุบัน

```bash
curl -s https://<auth-host>/api/v1/auth/key-info | jq
# { "signingKeyId": "...", "acceptedKeyIds": ["..."], "tokenTtl": "72h0m0s" }
```
บันทึก `signingKeyId` ไว้ — คือคีย์เก่าที่กำลังจะเลิกใช้

---

### เฟส 1 · ให้ **ผู้ตรวจ** ยอมรับทั้งสองใบก่อน ⬅️ สำคัญที่สุด

```bash
kubectl create secret generic jwt-public-keys -n vertex \
  --from-file=keys=<(cat old-public.pem new-public.pem) \
  --dry-run=client -o yaml | kubectl apply -f -
```

ตั้งให้ pet-service อ่านจาก secret นี้ผ่าน `JWT_PUBLIC_KEYS` แล้ว deploy

**ตอนนี้ยังไม่มีอะไรเปลี่ยน** — auth ยังเซ็นด้วยคีย์เก่า pet แค่ยอมรับใบใหม่เผื่อไว้

```
[ ] pet-service ทุก pod ขึ้นครบและ /health เขียว
[ ] ดู log ต้องเห็น "ยอมรับ public key kid=..." สองบรรทัด
[ ] ยิง API ด้วย token ที่มีอยู่ → ต้องยังได้ 200        ← GATE
```

> ⚠️ **ห้ามข้ามเฟสนี้** ถ้าเปลี่ยนคีย์ที่ auth ก่อนที่ pet จะรู้จักใบใหม่
> ทุก request จะได้ 401 พร้อมกันทั้งระบบ

---

### เฟส 2 · เปลี่ยนคีย์ที่ใช้ **เซ็น**

```bash
kubectl create secret generic jwt-signing-key -n vertex \
  --from-file=private=new-private.pem \
  --dry-run=client -o yaml | kubectl apply -f -
```

ตั้งที่ auth-service:
- `JWT_PRIVATE_KEY` ← คีย์ใหม่
- `JWT_PUBLIC_KEYS` ← **ทั้งสองใบ** (auth ก็ verify token เองที่ `/me` และ `RequireAuth`)

```
[ ] curl /api/v1/auth/key-info → signingKeyId เปลี่ยนเป็นของใบใหม่
[ ] acceptedKeyIds มีสองค่า
[ ] login ใหม่ 1 ครั้ง → decode token ดูว่า header มี kid ของใบใหม่
       echo "$TOKEN" | cut -d. -f1 | base64 -d | jq
[ ] ยิง pet API ด้วย token ใหม่ → 200                    ← GATE
[ ] ยิง pet API ด้วย token เก่าที่ยังไม่หมดอายุ → 200      ← GATE
```

---

### เฟส 3 · รอให้ token เก่าหมดอายุ

**รออย่างน้อย 72 ชั่วโมง** (เท่า `tokenTtl` จาก `/key-info`)

ระหว่างนี้ทั้งสองคีย์ใช้งานได้ปกติ ไม่ต้องทำอะไร

> 🔸 ถ้าลด TTL ลงเหลือ 15–60 นาที + เพิ่ม refresh token (งานแยกที่แนะนำไว้)
> ช่วงรอตรงนี้จะเหลือแค่ไม่กี่นาที

---

### เฟส 4 · ถอดคีย์เก่าออก

```bash
kubectl create secret generic jwt-public-keys -n vertex \
  --from-file=keys=new-public.pem \
  --dry-run=client -o yaml | kubectl apply -f -
```
แล้ว rollout ทั้ง auth-service และ pet-service

```
[ ] /key-info → acceptedKeyIds เหลือค่าเดียว
[ ] token ที่เซ็นด้วยคีย์เก่า → 401
[ ] token ปัจจุบัน → 200
[ ] ลบไฟล์คีย์เก่าออกจากทุกที่ที่เก็บไว้
```

---

### เฟส 5 · เก็บกวาด

```
[ ] เอา COPY keys ./keys ออกจาก Dockerfile ทั้งสอง service
[ ] ลบ keys/*.pem ออกจาก working tree
[ ] แจ้งทุกคนที่เคย clone repo ว่าคีย์เก่ารั่วและถูกเพิกถอนแล้ว
[ ] พิจารณาลบคีย์ออกจาก git history ด้วย git-filter-repo
    ⚠️ เป็นการเขียน history ใหม่ ทุกคนต้อง re-clone — วางแผนกับทีมก่อน
[ ] ตั้ง secret scanning ที่ GitHub (Settings → Code security)
```

---

## 4. ถ้าต้องถอย

| อยู่เฟสไหน | ทำอย่างไร |
|---|---|
| เฟส 1 | เอา `JWT_PUBLIC_KEYS` ออก กลับไปใช้ค่าเดิม — ไม่มีผลกระทบ |
| เฟส 2 | ตั้ง `JWT_PRIVATE_KEY` กลับเป็นคีย์เก่า **โดยที่ `JWT_PUBLIC_KEYS` ยังมีสองใบ** → token ที่ออกไปแล้วด้วยคีย์ใหม่ยังใช้ได้ |
| เฟส 4 | ใส่คีย์เก่ากลับเข้า `JWT_PUBLIC_KEYS` |

จุดที่ย้อนยากที่สุดคือเฟส 4 เพราะถ้าถอดคีย์เก่าเร็วเกินไป
token ที่ยังไม่หมดอายุจะใช้ไม่ได้ **จึงต้องรอให้ครบ 72 ชั่วโมงจริงๆ**

---

## 5. ตรวจสอบด้วยตัวเองได้ตลอด

```bash
# ดูว่าแต่ละ pod เซ็นด้วยคีย์ใบไหนและยอมรับใบไหนบ้าง
curl -s https://<auth-host>/api/v1/auth/key-info | jq

# ดู kid ใน token ที่ถืออยู่
echo "$TOKEN" | cut -d. -f1 | base64 -d 2>/dev/null | jq

# คำนวณ kid จากไฟล์ public key เพื่อเทียบ
openssl rsa -pubin -in new-public.pem -outform DER 2>/dev/null \
  | openssl dgst -sha256 -binary | basenc --base64url | tr -d '='
```

---

## 6. สิ่งที่โค้ดรองรับแล้ว

| ความสามารถ | ที่ไหน |
|---|---|
| อ่าน private key จาก env แทนไฟล์ใน image | `vertex-auth-service/config.go` `initRSAKeys()` |
| ใส่ `kid` ใน token header | `vertex-auth-service/main.go` `generateToken()` |
| `kid` คำนวณจากตัวคีย์ (ไม่ต้องตั้งค่าให้ตรงกัน) | `keys.go` / `pkg/middleware/keys.go` |
| ยอมรับ public key หลายใบ | ทั้งสอง service ผ่าน `JWT_PUBLIC_KEYS` |
| ตรวจ token เก่าที่ไม่มี `kid` | `jwt.VerificationKeySet` |
| endpoint ตรวจสถานะคีย์ | `GET /api/v1/auth/key-info` |
| CI กันไม่ให้ private key หลุดเข้า repo อีก | workflow ทั้งสอง repo |

**Test ที่ครอบเรื่องนี้**
- `pkg/middleware/auth_test.go` — `TestKeyRotation`, `TestKeyID_Routing`
- `vertex-auth-service/keys_test.go` — `TestVerificationKeyfunc_Rotation`

ครอบทั้งเคส token เก่าไม่มี `kid`, `kid` ไม่รู้จัก,
และเคสที่ `kid` ชี้คีย์หนึ่งแต่เซ็นด้วยอีกคีย์ (ต้องถูกปฏิเสธ)
