package main

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// ---- Edge case: basic field skipping ----

//go:fastbson
type FieldSkip struct {
	Exported   string `bson:"exported"`
	unexported string `bson:"unexported"`  // should be skipped
	Skipped    string `bson:"-"`           // should be skipped
	OmitMe     string `bson:"-,omitempty"` // should be skipped
}

// ---- Edge case: bson:",inline" embedded struct ----

//go:fastbson
type InlineBase struct {
	BaseField1 string `bson:"bf1"`
	BaseField2 int    `bson:"bf2"`
}

//go:fastbson
type InlineStruct struct {
	OwnField string     `bson:"own"`
	Inline   InlineBase `bson:",inline"`
}

// ---- Edge case: anonymous embedded struct ----

//go:fastbson
type EmbedRef struct {
	Name string `bson:"name"`
}

//go:fastbson
type EmbedOwner struct {
	EmbedRef `bson:",inline"`
	Extra    string `bson:"extra"`
}

// ---- Edge case: nested slices ----

//go:fastbson
type NestedSlices struct {
	Matrix [][]int32  `bson:"matrix"`
	Jagged [][]string `bson:"jagged"`
}

// ---- Edge case: slices of pointers ----

//go:fastbson
type PtrItem struct {
	Val string `bson:"val"`
}

//go:fastbson
type PtrSlice struct {
	Items []*PtrItem `bson:"items"`
}

// ---- Edge case: anonymous struct fields ----

//go:fastbson
type AnonymousStruct struct {
	Onboarding struct {
		NextStep int32   `bson:"ns"`
		DoneList []int32 `bson:"dl"`
	} `bson:"onboarding"`

	IAP struct {
		AccFlags int64  `bson:"f"`
		SKU      string `bson:"sku"`
	} `bson:"iap"`
}

// ---- Edge case: various integer widths ----

//go:fastbson
type IntWidths struct {
	I8   int8   `bson:"i8"`
	I16  int16  `bson:"i16"`
	I32  int32  `bson:"i32"`
	I64  int64  `bson:"i64"`
	UI8  uint8  `bson:"ui8"`
	UI16 uint16 `bson:"ui16"`
	UI32 uint32 `bson:"ui32"`
	UI64 uint64 `bson:"ui64"`
	Int  int    `bson:"int"`
	Uint uint   `bson:"uint"`
}

// ---- Edge case: zero/nil/empty values ----

//go:fastbson
type ZeroValues struct {
	Str   string         `bson:"str"`
	Bool  bool           `bson:"bool"`
	I32   int32          `bson:"i32"`
	F64   float64        `bson:"f64"`
	Bytes []byte         `bson:"bytes"`
	Slice []string       `bson:"slice"`
	Map   map[string]int `bson:"map"`
	Ptr   *string        `bson:"ptr"`
	Time  time.Time      `bson:"time"`

	OptStr  string  `bson:"opt_str,omitempty"`
	OptI32  int32   `bson:"opt_i32,omitempty"`
	OptBool bool    `bson:"opt_bool,omitempty"`
	OptPtr  *string `bson:"opt_ptr,omitempty"`
}

// ---- Edge case: realistic gamedev struct snippet ----

//go:fastbson
type BattleStats struct {
	Wins   int32 `bson:"wins"`
	Losses int32 `bson:"losses"`
	Rating int32 `bson:"rating"`
}

type Hero struct {
	ID   int32  `bson:"id"`
	Name string `bson:"name"`
	HP   int32  `bson:"hp"`
	MP   int32  `bson:"mp"`
}

//go:fastbson
type Bag struct {
	Gold  int64   `bson:"gold"`
	Items []int32 `bson:"items"`
}

//go:fastbson
type Player struct {
	ID         int64  `bson:"_id"`
	AccountID  int64  `bson:"account_id"`
	Name       string `bson:"name"`
	Level      int32  `bson:"lv"`
	Exp        int32  `bson:"exp"`
	Head       int32  `bson:"head"`
	VipLevel   int32  `bson:"vip"`
	Tag        int32  `bson:"tag"`
	Rate       int8   `bson:"rate"`
	Load       bool   `bson:"load"`
	HasRenamed bool   `bson:"renamed"`

	// bson:"-" fields — should be skipped
	Flag int      `bson:"-"`
	Dels []bson.M `bson:"-"`

	// unexported — should be skipped
	dirty bool
	l     int

	// pointer to //go:fastbson struct
	OfflineFight *Hero `bson:"offline_fight,omitempty"`

	// slices of pointers to //go:fastbson structs
	Heros []*Hero `bson:"heros,omitempty"`

	// nested struct
	Bag Bag `bson:"bag"`

	// nested slice of struct pointers
	SalePackages []*struct {
		ID   int32  `bson:"id"`
		Name string `bson:"name"`
	} `bson:"sale_pkgs,omitempty"`

	// nested slices
	ClientFilter [][]int32          `bson:"cdf,omitempty"`
	PBCounters   []map[string]int64 `bson:"pbcs,omitempty"`

	// slice of maps
	BossDropSeqs []map[string]int64 `bson:"bds"`

	// string slices
	Bids    []string `bson:"bids"`
	SubChan []string `bson:"sub_chan,omitempty"`

	// int32 slices
	ActiveContracts []int32 `bson:"contracts,omitempty"`

	// various inline/alias int types
	AutoIncrementID int `bson:"nid"`

	Statue StatueInfo `bson:"statues"`
}

//go:fastbson
type StatueInfo struct {
	Count  int32 `bson:"c"`
	Active bool  `bson:"active"`
}

// ---- large document to test buffer allocation ----

//go:fastbson
type WideStruct struct {
	A int32 `bson:"a"`
	B int32 `bson:"b"`
	C int32 `bson:"c"`
	D int32 `bson:"d"`
	E int32 `bson:"e"`
	F int32 `bson:"f"`
	G int32 `bson:"g"`
	H int32 `bson:"h"`
	I int32 `bson:"i"`
	J int32 `bson:"j"`
	K int32 `bson:"k"`
	L int32 `bson:"l"`
	M int32 `bson:"m"`
	N int32 `bson:"n"`
	O int32 `bson:"o"`
	P int32 `bson:"p"`
	Q int32 `bson:"q"`
	R int32 `bson:"r"`
	S int32 `bson:"s"`
	T int32 `bson:"t"`
	U int32 `bson:"u"`
	V int32 `bson:"v"`
	W int32 `bson:"w"`
	X int32 `bson:"x"`
	Y int32 `bson:"y"`
	Z int32 `bson:"z"`
}
