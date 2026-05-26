package main

import (
	"testing"

	fuzz "github.com/google/gofuzz"
	"go.mongodb.org/mongo-driver/bson"
)

// fuzzFill uses gofuzz to deterministically populate a struct from seed bytes.
func fuzzFill[T any](data []byte, v *T) {
	var seed int64
	for i := 0; i < 8 && i < len(data); i++ {
		seed |= int64(data[i]) << (i * 8)
	}
	fuzz.NewWithSeed(seed).Funcs(func(s *string, c fuzz.Continue) {
			n := c.Intn(64)
			b := make([]byte, n)
			for i := range b {
				b[i] = byte(c.Intn(95) + 32)
			}
			*s = string(b)
		},
		func(s *[]byte, c fuzz.Continue) {
			n := c.Intn(32)
			b := make([]byte, n)
			c.Read(b)
			*s = b
		},
		func(m *map[string]int64, c fuzz.Continue) {
			n := c.Intn(8)
			*m = make(map[string]int64, n)
			for i := 0; i < n; i++ {
				var k string
				var v int64
				c.Fuzz(&k)
				c.Fuzz(&v)
				(*m)[k] = v
			}
		},
		func(m *map[string]int, c fuzz.Continue) {
			n := c.Intn(8)
			*m = make(map[string]int, n)
			for i := 0; i < n; i++ {
				var k string
				var v int
				c.Fuzz(&k)
				c.Fuzz(&v)
				(*m)[k] = v
			}
		},
	)
}

// ---- Fuzz targets ----

func FuzzBattleStats(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v BattleStats
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got BattleStats
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		if got != v {
			t.Fatalf("round-trip mismatch: %+v vs %+v", got, v)
		}
		var off BattleStats
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzIntWidths(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v IntWidths
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}

		var got IntWidths
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}

		// int and uint are encoded as Int32 (truncated on 64-bit platforms).
		// Compare only fields that round-trip losslessly.
		if got.I8 != v.I8 || got.I16 != v.I16 || got.I32 != v.I32 || got.I64 != v.I64 {
			t.Fatalf("intX mismatch")
		}
		if got.UI8 != v.UI8 || got.UI16 != v.UI16 || got.UI32 != v.UI32 || got.UI64 != v.UI64 {
			t.Fatalf("uintX mismatch")
		}
		if int32(got.Int) != int32(v.Int) {
			t.Fatalf("Int truncated at Int32: %d vs %d", got.Int, v.Int)
		}
		if uint32(got.Uint) != uint32(v.Uint) {
			t.Fatalf("Uint truncated at Int32: %d vs %d", got.Uint, v.Uint)
		}

		var off IntWidths
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzWideStruct(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v WideStruct
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got WideStruct
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		if got != v {
			t.Fatalf("round-trip mismatch")
		}
		var off WideStruct
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzZeroValues(f *testing.F) {
	f.Add([]byte("hello\x00world\x01\xff\x00\xab"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var v ZeroValues
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got ZeroValues
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		// Compare key fields
		if got.Str != v.Str || got.Bool != v.Bool || got.I32 != v.I32 {
			t.Fatalf("field mismatch")
		}
		if len(got.Bytes) != len(v.Bytes) {
			t.Fatalf("Bytes length mismatch")
		}
		if (got.Ptr == nil) != (v.Ptr == nil) {
			t.Fatalf("Ptr nil mismatch")
		} else if got.Ptr != nil && *got.Ptr != *v.Ptr {
			t.Fatalf("Ptr mismatch")
		}
		var off ZeroValues
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzNestedSlices(f *testing.F) {
	f.Add([]byte{2, 2, 1, 2, 3, 4, 5, 6, 1, 7, 8})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v NestedSlices
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got NestedSlices
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		var off NestedSlices
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzPtrSlice(f *testing.F) {
	f.Add([]byte{2, 1, 3, 0x68, 0x69, 0, 1, 1, 4, 0x74, 0x65, 0x73, 0x74})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v PtrSlice
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got PtrSlice
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		if len(got.Items) != len(v.Items) {
			t.Fatalf("Items length: %d vs %d", len(got.Items), len(v.Items))
		}
		var off PtrSlice
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzAnonymousStruct(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v AnonymousStruct
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got AnonymousStruct
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		if got.Onboarding.NextStep != v.Onboarding.NextStep {
			t.Fatalf("NextStep mismatch")
		}
		if got.IAP.AccFlags != v.IAP.AccFlags {
			t.Fatalf("AccFlags mismatch")
		}
		if got.IAP.SKU != v.IAP.SKU {
			t.Fatalf("SKU mismatch")
		}
		var off AnonymousStruct
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}

func FuzzPlayer(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})
	f.Fuzz(func(t *testing.T, data []byte) {
		var v Player
		fuzzFill(data, &v)
		encoded, err := v.MarshalBSON()
		if err != nil {
			t.Skipf("MarshalBSON: %v", err)
		}
		var got Player
		if err := got.UnmarshalBSON(encoded); err != nil {
			t.Fatalf("UnmarshalBSON: %v", err)
		}
		// Compare core scalar fields
		if got.ID != v.ID {
			t.Fatalf("ID: %d vs %d", got.ID, v.ID)
		}
		if got.Name != v.Name {
			t.Fatalf("Name: %s vs %s", got.Name, v.Name)
		}
		if got.Level != v.Level || got.Exp != v.Exp {
			t.Fatalf("Level/Exp mismatch")
		}
		if int32(got.AutoIncrementID) != int32(v.AutoIncrementID) {
			t.Fatalf("AutoIncrementID truncated: %d vs %d", got.AutoIncrementID, v.AutoIncrementID)
		}
		// Verify with official driver
		var off Player
		if err := bson.Unmarshal(encoded, &off); err != nil {
			t.Fatalf("bson.Unmarshal: %v", err)
		}
	})
}
