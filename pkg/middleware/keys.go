package middleware

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// KeyID คำนวณ kid จากตัว public key เอง (SHA-256 thumbprint ของ DER)
//
// การคำนวณจากตัวคีย์ทำให้ผู้เซ็นและผู้ตรวจได้ค่าเดียวกันเสมอ
// โดยไม่ต้องตกลงชื่อ kid กันล่วงหน้าและไม่มีทางตั้งค่าไม่ตรงกัน
func KeyID(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// keySet เก็บ public key ทุกใบที่ยอมรับได้ พร้อม index ตาม kid
type keySet struct {
	byKID map[string]*rsa.PublicKey
	all   []jwt.VerificationKey
}

func newKeySet(keys []*rsa.PublicKey) *keySet {
	ks := &keySet{byKID: make(map[string]*rsa.PublicKey, len(keys))}
	for _, k := range keys {
		if k == nil {
			continue
		}
		ks.byKID[KeyID(k)] = k
		ks.all = append(ks.all, k)
	}
	return ks
}

// keyfunc เลือก key ที่จะใช้ตรวจลายเซ็น
//
// รองรับการ rotate key แบบไม่มี downtime:
//   - token ใหม่มี kid → เลือกใบที่ตรงได้ทันที
//   - token เก่าไม่มี kid → คืนทั้งชุดให้ไลบรารีลองทีละใบ
//
// ระหว่างช่วง rotate จะมี key สองใบพร้อมกัน (ใบเก่าไว้ตรวจ token ที่ยังไม่หมดอายุ)
// เอาใบเก่าออกได้เมื่อผ่านไปนานกว่าอายุ token ที่ยาวที่สุด
func (ks *keySet) keyfunc(t *jwt.Token) (interface{}, error) {
	if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
	}
	if len(ks.all) == 0 {
		return nil, fmt.Errorf("ไม่ได้ตั้งค่า public key ไว้เลย")
	}

	if kid, ok := t.Header["kid"].(string); ok && kid != "" {
		if key, found := ks.byKID[kid]; found {
			return key, nil
		}
		// kid ไม่ตรงกับใบไหนเลย — อาจเป็น key ที่ยังไม่ได้ deploy มาที่นี่
		return nil, fmt.Errorf("ไม่รู้จัก kid %q — ตรวจว่า JWT_PUBLIC_KEYS มีคีย์ใบนี้แล้วหรือยัง", kid)
	}

	return jwt.VerificationKeySet{Keys: ks.all}, nil
}
