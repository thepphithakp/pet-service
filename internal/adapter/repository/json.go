package repository

import "encoding/json"

// รวม json helper ไว้ที่เดียว เผื่อวันหนึ่งเปลี่ยนไปใช้ library อื่น
func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
