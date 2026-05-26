package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// ---- BattleStats (simple, 3 int32 fields) ----

func BenchmarkMarshal_BattleStats_Generated(b *testing.B) {
	v := BattleStats{Wins: 100, Losses: 50, Rating: 1500}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_BattleStats_Official(b *testing.B) {
	v := BattleStats{Wins: 100, Losses: 50, Rating: 1500}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkUnmarshal_BattleStats_Generated(b *testing.B) {
	v := BattleStats{Wins: 100, Losses: 50, Rating: 1500}
	data, _ := v.MarshalBSON()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst BattleStats
		_ = dst.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_BattleStats_Official(b *testing.B) {
	v := BattleStats{Wins: 100, Losses: 50, Rating: 1500}
	data, _ := bson.Marshal(v)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst BattleStats
		_ = bson.Unmarshal(data, &dst)
	}
}

// ---- Player with untagged Hero references ----

func BenchmarkMarshal_PlayerHeroRefs_Generated(b *testing.B) {
	h := Hero{ID: 42, Name: "Aragorn", HP: 200, MP: 80}
	v := Player{ID: 1, Name: "p", OfflineFight: &h, Heros: []*Hero{&h}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_PlayerHeroRefs_Official(b *testing.B) {
	h := Hero{ID: 42, Name: "Aragorn", HP: 200, MP: 80}
	v := Player{ID: 1, Name: "p", OfflineFight: &h, Heros: []*Hero{&h}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkUnmarshal_PlayerHeroRefs_Generated(b *testing.B) {
	h := Hero{ID: 42, Name: "Aragorn", HP: 200, MP: 80}
	v := Player{ID: 1, Name: "p", OfflineFight: &h, Heros: []*Hero{&h}}
	data, _ := v.MarshalBSON()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst Player
		_ = dst.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_PlayerHeroRefs_Official(b *testing.B) {
	h := Hero{ID: 42, Name: "Aragorn", HP: 200, MP: 80}
	v := Player{ID: 1, Name: "p", OfflineFight: &h, Heros: []*Hero{&h}}
	data, _ := bson.Marshal(v)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst Player
		_ = bson.Unmarshal(data, &dst)
	}
}

// ---- WideStruct (26 int32 fields) ----

func BenchmarkMarshal_WideStruct_Generated(b *testing.B) {
	v := WideStruct{
		A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_WideStruct_Official(b *testing.B) {
	v := WideStruct{
		A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkUnmarshal_WideStruct_Generated(b *testing.B) {
	v := WideStruct{
		A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26,
	}
	data, _ := v.MarshalBSON()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst WideStruct
		_ = dst.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_WideStruct_Official(b *testing.B) {
	v := WideStruct{
		A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26,
	}
	data, _ := bson.Marshal(v)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst WideStruct
		_ = bson.Unmarshal(data, &dst)
	}
}

// ---- Player (complex: nested structs, slices, pointers, omitempty) ----

func newTestPlayer() Player {
	hid := Hero{ID: 1, Name: "Aragorn", HP: 100, MP: 50}
	return Player{
		ID:              1001,
		AccountID:       50001,
		Name:            "bench_player",
		Level:           50,
		Exp:             99999,
		Head:            3,
		VipLevel:        2,
		Tag:             1,
		Rate:            100,
		Load:            true,
		HasRenamed:      false,
		AutoIncrementID: 42,
		OfflineFight:    &hid,
		Heros:           []*Hero{&hid},
		Bag:             Bag{Gold: 1000, Items: []int32{1, 2, 3}},
		Statue:          StatueInfo{Count: 5, Active: true},
		Bids:            []string{"bid_a", "bid_b", "bid_c"},
		ActiveContracts: []int32{101, 102, 103},
	}
}

func BenchmarkMarshal_Player_Generated(b *testing.B) {
	v := newTestPlayer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.MarshalBSON()
	}
}

func BenchmarkMarshal_Player_Official(b *testing.B) {
	v := newTestPlayer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bson.Marshal(v)
	}
}

func BenchmarkUnmarshal_Player_Generated(b *testing.B) {
	v := newTestPlayer()
	data, _ := v.MarshalBSON()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst Player
		_ = dst.UnmarshalBSON(data)
	}
}

func BenchmarkUnmarshal_Player_Official(b *testing.B) {
	v := newTestPlayer()
	data, _ := bson.Marshal(v)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst Player
		_ = bson.Unmarshal(data, &dst)
	}
}
