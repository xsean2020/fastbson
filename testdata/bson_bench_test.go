package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func makePlayer() Player {
	hid := Hero{ID: 1, Name: "Aragorn", HP: 100, MP: 50}
	return Player{
		ID:        1001,
		AccountID: 50001,
		Name:      "test_player",
		Level:     50,
		Exp:       99999,
		Head:      3,
		VipLevel:  2,
		Tag:       1,
		Rate:      100,
		Load:      true,
		HasRenamed: false,
		AutoIncrementID: 42,
		OfflineFight: &hid,
		Heros:       []*Hero{&hid},
		Bag:         Bag{Gold: 1000, Items: []int32{1, 2, 3}},
		Statue:      StatueInfo{Count: 5, Active: true},
		Bids:        []string{"a", "b"},
	}
}

func makeWide() WideStruct {
	return WideStruct{
		A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26,
	}
}

// ---- Marshal benchmarks ----

func BenchmarkMarshal_Generated_Player(b *testing.B) {
	v := makePlayer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_Official_Player(b *testing.B) {
	v := makePlayer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkMarshal_Generated_Wide(b *testing.B) {
	v := makeWide()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_Official_Wide(b *testing.B) {
	v := makeWide()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkMarshal_Generated_IntWidths(b *testing.B) {
	v := IntWidths{I8: -128, I16: 32767, I32: -999, I64: 1 << 50,
		UI8: 255, UI16: 65535, UI32: 1 << 30, UI64: 1<<63 - 1,
		Int: -42, Uint: 999,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_Official_IntWidths(b *testing.B) {
	v := IntWidths{I8: -128, I16: 32767, I32: -999, I64: 1 << 50,
		UI8: 255, UI16: 65535, UI32: 1 << 30, UI64: 1<<63 - 1,
		Int: -42, Uint: 999,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

// ---- Unmarshal benchmarks ----

func BenchmarkUnmarshal_Generated_Player(b *testing.B) {
	v := makePlayer()
	data, _ := bson.Marshal(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got Player
		_ = got.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_Official_Player(b *testing.B) {
	v := makePlayer()
	data, _ := bson.Marshal(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got Player
		_ = bson.Unmarshal(data, &got)
	}
}

func BenchmarkUnmarshal_Generated_Wide(b *testing.B) {
	v := makeWide()
	data, _ := bson.Marshal(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got WideStruct
		_ = got.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_Official_Wide(b *testing.B) {
	v := makeWide()
	data, _ := bson.Marshal(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got WideStruct
		_ = bson.Unmarshal(data, &got)
	}
}

// ---- Round-trip benchmark ----

func BenchmarkRoundTrip_Generated_Player(b *testing.B) {
	v := makePlayer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := v.MarshalBSON()
		var got Player
		_ = got.UnmarshalBSON(data)
	}
}

func BenchmarkRoundTrip_Official_Player(b *testing.B) {
	v := makePlayer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := bson.Marshal(v)
		var got Player
		_ = bson.Unmarshal(data, &got)
	}
}
