package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func bytesEqual(got, want []byte) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestMarshal_FieldSkip(t *testing.T) {
	v := FieldSkip{Exported: "hello", Skipped: "x", unexported: "y", OmitMe: "z"}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestMarshal_InlineStruct(t *testing.T) {
	v := InlineStruct{
		OwnField: "own",
		Inline: InlineBase{
			BaseField1: "b1",
			BaseField2: 42,
		},
	}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestUnmarshal_InlineStruct(t *testing.T) {
	v := InlineStruct{
		OwnField: "own",
		Inline: InlineBase{
			BaseField1: "b1",
			BaseField2: 42,
		},
	}
	data, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	var got InlineStruct
	if err := got.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	var want InlineStruct
	if err := bson.Unmarshal(data, &want); err != nil {
		t.Fatalf("bson.Unmarshal() error: %v", err)
	}
	if got != want {
		t.Fatalf("inline struct mismatch: got %+v want %+v", got, want)
	}
}

func TestMarshal_EmbedOwner(t *testing.T) {
	v := EmbedOwner{
		EmbedRef: EmbedRef{Name: "embedded"},
		Extra:    "x",
	}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestUnmarshal_EmbedOwner(t *testing.T) {
	v := EmbedOwner{
		EmbedRef: EmbedRef{Name: "embedded"},
		Extra:    "x",
	}
	data, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	var got EmbedOwner
	if err := got.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	var want EmbedOwner
	if err := bson.Unmarshal(data, &want); err != nil {
		t.Fatalf("bson.Unmarshal() error: %v", err)
	}
	if got != want {
		t.Fatalf("embed owner mismatch: got %+v want %+v", got, want)
	}
}

func TestMarshal_PtrSlice(t *testing.T) {
	a, b := PtrItem{Val: "first"}, PtrItem{Val: "second"}
	v := PtrSlice{Items: []*PtrItem{&a, &b}}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestRoundTrip_PtrSlice(t *testing.T) {
	a, b := PtrItem{Val: "alpha"}, PtrItem{Val: "beta"}
	v := PtrSlice{Items: []*PtrItem{&a, &b}}
	data, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	var mid PtrSlice
	if err := mid.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	data2, err := mid.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	if !bytesEqual(data, data2) {
		t.Fatalf("round-trip mismatch: %d vs %d bytes", len(data), len(data2))
	}
}

func TestMarshal_AnonymousStruct(t *testing.T) {
	v := AnonymousStruct{
		Onboarding: struct {
			NextStep int32   `bson:"ns"`
			DoneList []int32 `bson:"dl"`
		}{NextStep: 3, DoneList: []int32{1, 2, 3}},
		IAP: struct {
			AccFlags int64  `bson:"f"`
			SKU      string `bson:"sku"`
		}{AccFlags: 100, SKU: "sku_001"},
	}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestRoundTrip_AnonymousStruct(t *testing.T) {
	v := AnonymousStruct{
		Onboarding: struct {
			NextStep int32   `bson:"ns"`
			DoneList []int32 `bson:"dl"`
		}{NextStep: 5, DoneList: []int32{10, 20}},
		IAP: struct {
			AccFlags int64  `bson:"f"`
			SKU      string `bson:"sku"`
		}{AccFlags: 200, SKU: "premium"},
	}
	data, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	var mid AnonymousStruct
	if err := mid.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	if mid.Onboarding.NextStep != v.Onboarding.NextStep {
		t.Errorf("Onboarding.NextStep: %d != %d", mid.Onboarding.NextStep, v.Onboarding.NextStep)
	}
	if mid.IAP.SKU != v.IAP.SKU {
		t.Errorf("IAP.SKU: %s != %s", mid.IAP.SKU, v.IAP.SKU)
	}
	data2, err := mid.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	if !bytesEqual(data, data2) {
		t.Fatalf("round-trip mismatch: %d vs %d bytes", len(data), len(data2))
	}
}

func TestMarshal_IntWidths(t *testing.T) {
	v := IntWidths{
		I8: -128, I16: 32767, I32: -999, I64: 1 << 50,
		UI8: 255, UI16: 65535, UI32: 1 << 30, UI64: 1<<63 - 1,
		Int: -42, Uint: 999,
	}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestRoundTrip_IntWidths(t *testing.T) {
	v := IntWidths{I8: -1, I16: -2, UI64: 1<<63 - 1, Uint: 0}
	data, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	var mid IntWidths
	if err := mid.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	data2, err := mid.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	if !bytesEqual(data, data2) {
		t.Fatalf("round-trip mismatch: %d vs %d bytes", len(data), len(data2))
	}
}

func TestMarshal_Player(t *testing.T) {
	hid := Hero{ID: 1, Name: "Aragorn", HP: 100, MP: 50}
	v := Player{
		ID:              1001,
		AccountID:       50001,
		Name:            "test_player",
		Level:           50,
		Exp:             99999,
		Head:            3,
		VipLevel:        2,
		Tag:             1,
		Rate:            100,
		Load:            true,
		HasRenamed:      false,
		AutoIncrementID: 42,
		Flag:            999,
		OfflineFight:    &hid,
		Heros:           []*Hero{&hid},
		Bag:             Bag{Gold: 1000, Items: []int32{1, 2, 3}},
		Statue:          StatueInfo{Count: 5, Active: true},
		Bids:            []string{"a", "b"},
	}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}

func TestUnmarshal_Player(t *testing.T) {
	hid := Hero{ID: 1, Name: "Aragorn", HP: 100, MP: 50}
	v := Player{
		ID:              2002,
		AccountID:       60002,
		Name:            "unmarshal_test",
		Level:           30,
		Exp:             5000,
		Head:            7,
		Tag:             99,
		Rate:            50,
		Load:            false,
		HasRenamed:      true,
		AutoIncrementID: 100,
		OfflineFight:    &hid,
		Heros:           []*Hero{&hid},
		Bag:             Bag{Gold: 500, Items: []int32{10, 20}},
		Statue:          StatueInfo{Count: 3, Active: false},
		Bids:            []string{"x"},
	}
	data, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	var got Player
	if err := got.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	var want Player
	if err := bson.Unmarshal(data, &want); err != nil {
		t.Fatalf("bson.Unmarshal() error: %v", err)
	}
	checks := []struct {
		name      string
		got, want interface{}
	}{
		{"ID", got.ID, want.ID},
		{"AccountID", got.AccountID, want.AccountID},
		{"Name", got.Name, want.Name},
		{"Level", got.Level, want.Level},
		{"Exp", got.Exp, want.Exp},
		{"Head", got.Head, want.Head},
		{"VipLevel", got.VipLevel, want.VipLevel},
		{"Tag", got.Tag, want.Tag},
		{"Rate", got.Rate, want.Rate},
		{"Load", got.Load, want.Load},
		{"HasRenamed", got.HasRenamed, want.HasRenamed},
		{"AutoIncrementID", got.AutoIncrementID, want.AutoIncrementID},
		{"Bag.Gold", got.Bag.Gold, want.Bag.Gold},
		{"Statue.Count", got.Statue.Count, want.Statue.Count},
		{"Statue.Active", got.Statue.Active, want.Statue.Active},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: %v != %v", c.name, c.got, c.want)
		}
	}
	if (got.OfflineFight == nil) != (want.OfflineFight == nil) {
		t.Errorf("OfflineFight nil mismatch")
	} else if got.OfflineFight != nil && *got.OfflineFight != *want.OfflineFight {
		t.Errorf("OfflineFight: %+v != %+v", *got.OfflineFight, *want.OfflineFight)
	}
}

func TestRoundTrip_Player(t *testing.T) {
	hid := Hero{ID: 2, Name: "Legolas", HP: 80, MP: 100}
	v := Player{
		ID:              3003,
		AccountID:       70003,
		Name:            "roundtrip",
		Level:           99,
		Exp:             1 << 30,
		Head:            1,
		VipLevel:        5,
		Tag:             7,
		Rate:            1,
		Load:            true,
		HasRenamed:      true,
		AutoIncrementID: 999,
		OfflineFight:    &hid,
		Heros:           []*Hero{&hid},
		Bag:             Bag{Gold: 99999, Items: []int32{}},
		Statue:          StatueInfo{Count: 1, Active: true},
		Bids:            []string{"item1", "item2"},
		ActiveContracts: []int32{101, 102, 103},
	}
	data, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	var mid Player
	if err := mid.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON() error: %v", err)
	}
	data2, err := mid.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	if !bytesEqual(data, data2) {
		t.Fatalf("round-trip mismatch: %d vs %d bytes", len(data), len(data2))
	}
}

func TestMarshal_WideStruct(t *testing.T) {
	v := WideStruct{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, I: 9, J: 10,
		K: 11, L: 12, M: 13, N: 14, O: 15, P: 16, Q: 17, R: 18, S: 19, T: 20,
		U: 21, V: 22, W: 23, X: 24, Y: 25, Z: 26}
	got, err := v.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON() error: %v", err)
	}
	want, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("bson.Marshal() error: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("mismatch:\ngot  %d: % x\nwant %d: % x", len(got), got, len(want), want)
	}
}
