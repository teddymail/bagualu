package persistence

import (
	"database/sql"
	"encoding/json"
	"time"
)

const timeLayout = time.RFC3339

func encodeTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func decodeTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func encodeOptTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	v := encodeTime(*t)
	return v
}

func decodeOptTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := decodeTime(ns.String)
	return &t
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	if b == nil {
		return "{}"
	}
	return string(b)
}

func unmarshalJSON(s string, v interface{}) {
	_ = json.Unmarshal([]byte(s), v)
}

func nowUTC() time.Time { return time.Now().UTC() }
