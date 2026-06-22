package batchflow_test

import (
	"testing"
	"time"

	"github.com/rushairer/batchflow/v2"
)

func TestRequest_Setters_Getters_Validate(t *testing.T) {
	s := batchflow.NewSQLSchema("t", batchflow.ConflictIgnoreOperationConfig, "i32", "i64", "f32", "f64", "s", "b", "ts", "bin")
	r := batchflow.NewRequest(s).
		SetInt32("i32", 1).
		SetInt64("i64", 2).
		SetFloat32("f32", 3.5).
		SetFloat64("f64", 4.5).
		SetString("s", "x").
		SetBool("b", true).
		SetTime("ts", time.Unix(0, 0)).
		SetBytes("bin", []byte{1, 2, 3})
	// 还测试通用 Set/SetNull 不破坏 Columns()
	r.Set("custom", 123).SetNull("custom")

	cols := r.Columns()
	if len(cols) < 9 {
		t.Fatalf("Columns len=%d, want >=9", len(cols))
	}

	if v, err := r.GetInt32("i32"); err != nil || v != 1 {
		t.Fatalf("GetInt32=%v,%v", v, err)
	}
	if v, err := r.GetInt64("i64"); err != nil || v != 2 {
		t.Fatalf("GetInt64=%v,%v", v, err)
	}
	if v, err := r.GetFloat64("f64"); err != nil || v != 4.5 {
		t.Fatalf("GetFloat64=%v,%v", v, err)
	}
	if v, err := r.GetBool("b"); err != nil || v != true {
		t.Fatalf("GetBool=%v,%v", v, err)
	}
	if v, err := r.GetTime("ts"); err != nil || v.IsZero() {
		t.Fatalf("GetTime zero=%v, err=%v", v.IsZero(), err)
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate err=%v", err)
	}

	// 未设置的 schema 列应该在 GetX 时报错
	if _, err := r.GetInt32("not_exists"); err == nil {
		t.Fatalf("expect error for missing column")
	}
}

func TestRequest_ExtendedIntegerSetters(t *testing.T) {
	s := batchflow.NewSQLSchema(
		"t",
		batchflow.ConflictIgnoreOperationConfig,
		"i",
		"i8",
		"i16",
		"u",
		"u8",
		"u16",
		"u32",
		"u64",
	)

	r := batchflow.NewRequest(s).
		SetInt("i", 1).
		SetInt8("i8", 2).
		SetInt16("i16", 3).
		SetUint("u", 4).
		SetUint8("u8", 5).
		SetUint16("u16", 6).
		SetUint32("u32", 7).
		SetUint64("u64", 8)

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate err=%v", err)
	}

	cols := r.Columns()
	cases := []struct {
		name string
		want any
	}{
		{name: "i", want: int(1)},
		{name: "i8", want: int8(2)},
		{name: "i16", want: int16(3)},
		{name: "u", want: uint(4)},
		{name: "u8", want: uint8(5)},
		{name: "u16", want: uint16(6)},
		{name: "u32", want: uint32(7)},
		{name: "u64", want: uint64(8)},
	}

	for _, tc := range cases {
		if got := cols[tc.name]; got != tc.want {
			t.Fatalf("column %s=%T(%v), want %T(%v)", tc.name, got, got, tc.want, tc.want)
		}
	}
}

func TestRequestColumnsReturnsCopy(t *testing.T) {
	s := batchflow.NewSchema("t", "id", "name")
	r := batchflow.NewRequest(s).SetInt("id", 1).SetString("name", "alice")

	cols := r.Columns()
	cols["name"] = "mutated"
	delete(cols, "id")

	got := r.Columns()
	if got["name"] != "alice" {
		t.Fatalf("Columns exposed mutable state: name=%v", got["name"])
	}
	if got["id"] != 1 {
		t.Fatalf("Columns exposed mutable state: id=%v", got["id"])
	}
}
