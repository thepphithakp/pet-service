package repository

import (
	"context"

	"gorm.io/gorm"
)

// txKey เป็น type ส่วนตัวเพื่อไม่ให้ package อื่นเขียนทับค่าใน context ได้
type txKey struct{}

// WithTx ผูก transaction เข้ากับ context
//
// ใช้โดย TxManager เท่านั้น — repository ไม่ต้องรู้ว่าใครเป็นคนเริ่ม transaction
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// dbFrom คืน connection ที่ควรใช้กับ query นี้
//
// ถ้า context มี transaction อยู่ (เพราะถูกเรียกจากใน TxManager.Within)
// ให้ใช้ transaction นั้น ไม่งั้นใช้ connection ปกติ
//
// การส่งผ่าน context แทนการเปลี่ยน signature ของทุก method
// ทำให้ repository ที่ไม่เกี่ยวกับ transaction ไม่ต้องแก้อะไรเลย
// และไม่มีทางลืมส่ง tx ต่อ เพราะ ctx ถูกส่งอยู่แล้วทุกชั้น
func dbFrom(ctx context.Context, base *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return base.WithContext(ctx)
}

// GORMTxManager รัน function ใน transaction เดียว
type GORMTxManager struct {
	db *gorm.DB
}

func NewGORMTxManager(db *gorm.DB) *GORMTxManager {
	return &GORMTxManager{db: db}
}

// Within รัน fn ใน transaction — commit เมื่อ fn คืน nil, rollback เมื่อคืน error
//
// ถ้า context มี transaction อยู่แล้วจะใช้ตัวเดิมต่อ ไม่เปิดซ้อน
// (GORM รองรับ nested ด้วย savepoint แต่ที่นี่ไม่ต้องการความซับซ้อนนั้น
//
//	และการใช้ transaction เดิมต่อคือพฤติกรรมที่ผู้เรียกคาดหวัง)
func (m *GORMTxManager) Within(ctx context.Context, fn func(context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
