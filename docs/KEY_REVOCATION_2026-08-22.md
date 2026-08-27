# บันทึกการเพิกถอนคีย์ + ข้อเสนอเรื่องที่เก็บ

วันที่ 2026-08-22

---

## 1. สรุปเหตุการณ์

| | |
|---|---|
| **คีย์ที่ถูกเพิกถอน** | `kid=y3tVWjNy4M2fhFAKE7a56-mfxH44k1OMKVAzPZzqnt8` (RSA 2048) |
| **สาเหตุ** | `vertex-auth-service/keys/private.pem` ถูก commit เข้า git ตั้งแต่ commit แรก (`727c75a`) |
| **ผลกระทบ** | ใครที่เข้าถึง repo ได้ ปลอม token เป็นผู้ใช้คนใดก็ได้ รวมถึง `SUPER_ADMIN` |
| **ขอบเขต** | คีย์เดียวกันถูกใช้ทั้ง `vertex-auth-service` (เซ็น) และ `vertex-pet-service` (ตรวจ) — ยืนยันด้วยการเทียบ kid แล้วว่าตรงกัน |
| **คีย์ใหม่** | `kid=0m7kfRkabxoJv7vlsdTAPLXNRtAC2JWdHfy7cMh3eDc` (RSA 4096) |
| **ที่เก็บชั่วคราว** | `~/vertex-keys/jwt-signing-20260822.key` (โหมด 600, นอก repo ทุกตัว) |

⚠️ **คีย์เก่ายังอยู่ใน git history ตลอดไป** การ `git rm` หยุดได้แค่ commit ใหม่
ทุกคนที่เคย clone ยังมีสำเนาอยู่ในเครื่อง — ต้องถือว่ารั่วถาวร

---

## 2. เพิกถอนทันที หรือค่อยๆ เปลี่ยน — ต้องเลือก

`docs/KEY_ROTATION.md` อธิบายการ rotate แบบ **ไม่มี downtime** ซึ่งเก็บคีย์เก่าไว้ 72 ชั่วโมง
แต่กรณีนี้ต่างออกไป เพราะคีย์เก่า**ถูกเปิดเผยแล้ว** ไม่ใช่แค่ "เก่า"

| | เพิกถอนทันที | rotate แบบไม่มี downtime |
|---|---|---|
| ช่วงที่ยังปลอม token ได้ | 0 | **อีก 72 ชั่วโมง** |
| ผลต่อผู้ใช้ | ทุกคน login ใหม่ 1 ครั้ง | ไม่รู้สึกอะไรเลย |
| ความซับซ้อน | ขั้นตอนเดียว | 5 เฟส |

### ✅ แนะนำ: เพิกถอนทันที

การเก็บคีย์เก่าไว้อีก 72 ชั่วโมง = เปิดให้ปลอม token ระดับ `SUPER_ADMIN` ต่ออีก 3 วัน
ทั้งที่รู้อยู่แล้วว่าคีย์รั่ว — ราคาของการให้ผู้ใช้ login ใหม่ครั้งเดียวถูกกว่ามาก

> 🔸 ถ้ามีเหตุผลทางธุรกิจที่รับ downtime ของ session ไม่ได้จริงๆ
> ให้ใช้ `KEY_ROTATION.md` แทน **แต่ต้องยอมรับความเสี่ยง 72 ชั่วโมงอย่างรู้ตัว**

---

## 3. ขั้นตอนเพิกถอนทันที

### 3.1 สร้าง Secret

```bash
KEYDIR=~/vertex-keys
NEW=$KEYDIR/jwt-signing-20260822

kubectl create secret generic jwt-signing-key -n vertex \
  --from-file=private=$NEW.key \
  --dry-run=client -o yaml | kubectl apply -f -

# 🔑 ใส่ "เฉพาะคีย์ใหม่" — ไม่ใส่คีย์เก่า นี่คือจุดที่ทำให้เป็นการเพิกถอน
kubectl create secret generic jwt-public-keys -n vertex \
  --from-file=keys=$NEW.pub \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 3.2 Deploy — auth ก่อน แล้วตามด้วย pet

```bash
helm upgrade auth-service ./helm/auth-service -n vertex \
  --reuse-values \
  --set jwt.signingKeySecret=jwt-signing-key \
  --set jwt.publicKeysSecret=jwt-public-keys

helm upgrade pet-service ./helm/pet-service -n vertex \
  --reuse-values \
  --set jwt.publicKeysSecret=jwt-public-keys
```

> ลำดับไม่สำคัญมากในกรณีนี้ เพราะ token เก่าใช้ไม่ได้อยู่แล้วทั้งสองทาง
> แต่ deploy auth ก่อนทำให้ผู้ใช้ที่ login ใหม่ได้ token ที่ใช้งานได้ทันที

### 3.3 ยืนยัน

```
[ ] curl /api/v1/auth/key-info
      signingKeyId   = 0m7kfRkabxoJv7vlsdTAPLXNRtAC2JWdHfy7cMh3eDc
      acceptedKeyIds = [0m7kfRkab...]   ← ต้องมีค่าเดียว
[ ] ยิง API ด้วย token เก่า → 401                                    ← ยืนยันว่าเพิกถอนสำเร็จ
[ ] login ใหม่ → token header มี kid=0m7kfRkab...
      echo "$TOKEN" | cut -d. -f1 | base64 -d | jq
[ ] ยิง API ด้วย token ใหม่ → 200
[ ] เข้า backoffice ด้วยบัญชี SUPER_ADMIN ได้                        ← GATE
```

### 3.4 เก็บกวาด

```
[ ] เอา COPY keys ./keys ออกจาก Dockerfile ทั้งสอง service
[ ] ลบ keys/*.pem ออกจาก working tree ทั้งสอง repo
[ ] แจ้งทุกคนที่เคย clone ว่าคีย์เก่าถูกเพิกถอนแล้ว
[ ] เปิด secret scanning ที่ GitHub (Settings → Code security and analysis)
[ ] พิจารณาลบคีย์ออกจาก git history ด้วย git-filter-repo
    ⚠️ เขียน history ใหม่ ทุกคนต้อง re-clone — วางแผนกับทีมก่อน
       ประโยชน์จำกัด เพราะคีย์ถูกเพิกถอนไปแล้ว แต่ช่วยลดโอกาสสับสนในอนาคต
```

---

## 4. ข้อเสนอเรื่องที่เก็บคีย์

ตอนนี้คีย์ใหม่อยู่ที่ `~/vertex-keys/` บนเครื่องคุณ ซึ่ง**ยังไม่ใช่ที่เก็บถาวรที่ดี**
เพราะไม่มี backup ไม่มี audit และถ้าเครื่องหายก็หายไปด้วย

### เปรียบเทียบทางเลือก

| วิธี | ข้อดี | ข้อเสีย | เหมาะกับ |
|---|---|---|---|
| **1. k8s Secret ล้วน** (ตอนนี้) | ทำได้ทันที ไม่ต้องติดตั้งอะไร | ไม่มี version, ไม่มี audit, base64 ไม่ใช่การเข้ารหัส, ใครมีสิทธิ์อ่าน secret ก็อ่านได้ | ชั่วคราวเท่านั้น |
| **2. Sealed Secrets** ⬅️ **แนะนำ** | ไฟล์ที่เข้ารหัสแล้ว commit ลง git ได้อย่างปลอดภัย, มี version, GitOps ครบวงจร, ไม่ต้องพึ่งบริการภายนอก, ติดตั้งบนคลัสเตอร์ได้ใน 5 นาที | ต้อง backup คีย์ของ controller เอง, ถอดรหัสได้เฉพาะในคลัสเตอร์นั้น | โปรเจกต์ขนาดนี้ |
| **3. External Secrets Operator + Vault/1Password/Cloud SM** | audit log ครบ, หมุนคีย์อัตโนมัติได้, แชร์ข้ามคลัสเตอร์ได้ | ต้องดูแลระบบเพิ่มอีกตัว หรือจ่ายค่าบริการ | เมื่อมีหลายคลัสเตอร์หรือมี compliance |
| **4. KMS ที่เซ็นให้** (GCP/AWS KMS asymmetric) | **คีย์ไม่เคยออกจาก HSM เลย** รั่วแบบนี้อีกไม่ได้, audit ทุกครั้งที่เซ็น | ต้องแก้โค้ดให้เรียก KMS API แทนการเซ็นเอง, latency เพิ่ม, ค่าใช้จ่ายต่อครั้ง | เมื่อมีข้อมูลผู้ใช้จริงจำนวนมาก |

### แนะนำสำหรับตอนนี้: **Sealed Secrets**

เหตุผล: ปัญหาที่เพิ่งเกิดคือ "ของลับหลุดเข้า git" — Sealed Secrets แก้ที่ต้นเหตุโดยตรง
เพราะทำให้ของลับ **commit ลง git ได้อย่างปลอดภัย** แทนที่จะต้องหลบเลี่ยง git
ซึ่งเป็นสิ่งที่คนทำแล้วพลาดซ้ำได้เสมอ และเข้ากับคลัสเตอร์ที่ใช้อยู่โดยไม่ต้องพึ่งบริการภายนอก

```bash
# ติดตั้ง controller
helm repo add sealed-secrets https://bitnami-labs.github.io/sealed-secrets
helm install sealed-secrets sealed-secrets/sealed-secrets -n kube-system

# แปลง Secret เป็น SealedSecret ที่ commit ได้
kubectl create secret generic jwt-signing-key -n vertex \
  --from-file=private=~/vertex-keys/jwt-signing-20260822.key \
  --dry-run=client -o yaml \
  | kubeseal --format yaml > deployment/sealed/jwt-signing-key.yaml

git add deployment/sealed/jwt-signing-key.yaml   # ✅ ปลอดภัย ถอดรหัสได้เฉพาะ controller
```

⚠️ **สิ่งที่ต้อง backup คือคีย์ของ controller เอง** ไม่ใช่ SealedSecret
ถ้าคลัสเตอร์พังและไม่มีคีย์นี้ จะถอดรหัสไฟล์ที่ commit ไว้ไม่ได้เลย:
```bash
kubectl get secret -n kube-system -l sealedsecrets.bitnami.com/sealed-secrets-key \
  -o yaml > sealed-secrets-master.key   # 🔒 เก็บนอกคลัสเตอร์ ในที่ที่ปลอดภัย
```

### กติกาที่ควรตั้งไว้ ไม่ว่าจะเลือกวิธีไหน

1. 🚫 **ไฟล์ `.pem`, `.key` ห้ามอยู่ใน git โดยไม่เข้ารหัส** — มี CI job คุมทั้งสอง repo แล้ว
2. 🚫 **ห้าม `COPY keys` ลง image** — ทำให้ rotate ต้อง rebuild และคีย์ติดไปกับทุก layer
3. ✅ **คีย์มาจาก env/Secret เท่านั้น** — โค้ดรองรับแล้วผ่าน `JWT_PRIVATE_KEY` / `JWT_PUBLIC_KEYS`
4. ✅ **หมุนคีย์ตามกำหนด** ทุก 6–12 เดือน ตาม `KEY_ROTATION.md` (แบบไม่มี downtime)
5. ✅ **เปิด secret scanning ที่ GitHub** — จับได้ตั้งแต่ตอน push ไม่ต้องรอให้มีคนสังเกต

### สิ่งที่จะทำให้การหมุนคีย์ครั้งหน้าถูกลงมาก

ตอนนี้ token มีอายุ **72 ชั่วโมง** และไม่มี refresh token
ทำให้ทุกการ rotate ต้องเลือกระหว่าง "รอ 3 วัน" กับ "บังคับ login ใหม่"

ถ้าลดเหลือ 15–60 นาที + เพิ่ม refresh token:
- ช่วงที่ต้องยอมรับคีย์สองใบเหลือแค่ไม่กี่นาที
- token ที่หลุดมีอายุสั้นลงมาก
- เพิกถอน session รายคนได้ (ตอนนี้ทำไม่ได้เลย)

**เป็นงานที่คุ้มที่สุดถัดจากนี้** แต่ต้องแก้ฝั่ง client ด้วย จึงควรวางแผนแยก

---

## 5. สถานะโค้ด

| ความสามารถ | สถานะ |
|---|---|
| อ่านคีย์จาก env แทนไฟล์ใน image | ✅ ทั้งสอง service |
| ยอมรับ public key หลายใบ | ✅ ทั้งสอง service |
| `kid` ใน token header | ✅ |
| endpoint ตรวจสถานะคีย์ | ✅ `GET /api/v1/auth/key-info` |
| CI กันคีย์หลุดเข้า repo | ✅ ทั้งสอง repo |
| คีย์ใหม่ 4096 bit ผ่านการทดสอบเซ็น/ตรวจแล้ว | ✅ |
| **เอา `COPY keys` ออกจาก Dockerfile** | ❌ ยังไม่ทำ — รอให้ Secret พร้อมก่อน |
| **Sealed Secrets** | ❌ ยังไม่ติดตั้ง |
| **ลดอายุ token + refresh token** | ❌ งานแยก |
