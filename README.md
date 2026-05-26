# fastbson — Zero-Reflection BSON Code Generator

`fastbson` is a Go code generator that produces **zero-reflection** `MarshalBSON()` and `UnmarshalBSON()` methods for your structs. It reads Go source files, finds structs annotated with `//go:fastbson`, and generates type-specific BSON encoding/decoding code using `go.mongodb.org/mongo-driver/x/bsonx/bsoncore`.

## Why?

The official `go.mongodb.org/mongo-driver/bson` package uses **reflection** at runtime to marshal/unmarshal every field. For hot-path game servers, high-throughput APIs, or any latency-sensitive application, this overhead adds up.

`fastbson` generates **concrete field-level code** at build time — no reflection, no interface dispatch, no type-casting at runtime.

## Quick Start

```go
//go:fastbson
type Player struct {
    ID    int64  `bson:"_id"`
    Name  string `bson:"name"`
    Level int32  `bson:"lv"`
    Items []int32 `bson:"items"`
}
```

```bash
go run github.com/xsean2020/fastbson@latest player.go
```

This generates `player_bson.go` with `func (z *Player) MarshalBSON() ([]byte, error)` and `func (z *Player) UnmarshalBSON(b []byte) error`.

## Usage

```bash
# Install
go build -o fastbson main.go

# Generate BSON code for a single file
./fastbson types.go

# Or for an entire directory (scans all .go files)
./fastbson .

# Generated files: types_bson.go (per input file)
```

Add the `//go:fastbson` directive above any struct you want to generate code for:

```go
//go:fastbson
type MyStruct struct {
    Field1 string  `bson:"field1"`
    Field2 int32   `bson:"field2,omitempty"`
}
```

## Supported Types

| Go Type | BSON Type | Marshal | Unmarshal |
|---------|-----------|---------|-----------|
| `float64` | Double | ✓ | ✓ |
| `float32` | Double (cast) | ✓ | ✓ |
| `string` | String | ✓ | ✓ |
| `bool` | Boolean | ✓ | ✓ |
| `int32` | Int32 | ✓ | ✓ |
| `int64` | Int64 | ✓ | ✓ |
| `int`, `int8`, `int16` | Int32 | ✓ | ✓ |
| `uint`, `uint32` | Int64 | ✓ | ✓ |
| `uint16` | Int32 | ✓ | ✓ |
| `uint64` | Int64 | ✓ | ✓ |
| `uint8`, `byte` | Int32 | ✓ | ✓ |
| `time.Time` | DateTime (via `UnixMilli()`) | ✓ | ✓ |
| `primitive.ObjectID` | ObjectID | ✓ | ✓ |
| `primitive.DateTime` | DateTime | ✓ | ✓ |
| `primitive.Binary` | Binary | ✓ | ✓ |
| `primitive.Regex` | Regex | ✓ | ✓ |
| `primitive.Timestamp` | Timestamp | ✓ | ✓ |
| `primitive.Decimal128` | Decimal128 | ✓ | ✓ |
| `primitive.JavaScript` | JavaScript | ✓ | ✓ |
| `primitive.Symbol` | Symbol | ✓ | ✓ |
| `primitive.Null` | Null | ✓ | ✓ |
| `primitive.Undefined` | Undefined | ✓ | ✓ |
| `primitive.MinKey` | MinKey | ✓ | ✓ |
| `primitive.MaxKey` | MaxKey | ✓ | ✓ |
| `primitive.D` / `primitive.M` | Document | ✓ | ✓ |
| `primitive.A` | Array | ✓ | ✓ |
| `[]byte` | Binary (subtype 0) | ✓ | ✓ |
| `[]T` | Array | ✓ | ✓ |
| `map[string]T` | Document | ✓ | ✓ |
| `*T` | Null / Document | ✓ | ✓ |
| `struct{...}` (named, with `//go:fastbson`) | Document | ✓ | ✓ |
| `struct{...}` (anonymous inline) | Document | ✓ | ✓ |
| `[][]T` | Nested Array | ✓ | ✓ |
| `[]*T` | Array of documents | ✓ | ✓ |

## Tag Support

Supports standard `bson` struct tags:

- **`bson:"name"`** — custom field key
- **`bson:"-"`** — skip field
- **`bson:",omitempty"`** — skip zero/nil values
- **`bson:",minsize"`** — encode int64 as Int32 when value fits
- **`bson:",inline"`** — flatten embedded struct (relayed to official driver)

## Performance

Benchmarks on Apple M1 (Go 1.25), comparing generated code vs `go.mongodb.org/mongo-driver/bson`:

### Marshal

| Struct | Size | Generated | Official | Speedup |
|--------|------|-----------|----------|---------|
| BattleStats (3 int32) | 37 B | **105 ns/op** | 489 ns/op | **4.7×** |
| IntWidths (10 ints) | 148 B | **230 ns/op** | 674 ns/op | **2.9×** |
| WideStruct (26 int32) | 324 B | **416 ns/op** | 1361 ns/op | **3.3×** |
| Player (complex, 17+ fields) | ~500 B | **1001 ns/op** | 2651 ns/op | **2.6×** |

### Unmarshal

| Struct | Generated | Official | Speedup |
|--------|-----------|----------|---------|
| BattleStats (3 int32) | **67 ns/op** | 187 ns/op | **2.8×** |
| WideStruct (26 int32) | **470 ns/op** | 603 ns/op | **1.3×** |
| Player (complex, 17+ fields) | **1513 ns/op** | 2013 ns/op | **1.3×** |
| PlayerHeroRefs (sub-docs) | **1186 ns/op** | 1394 ns/op | **1.2×** |

### Memory (Player benchmarks)

| Operation | Generated | Official | Improvement |
|-----------|-----------|----------|-------------|
| Marshal | 1448 B/op, 18 allocs | 800 B/op, 2 allocs | — (sub-buffers) |
| Unmarshal | **752 B/op, 20 allocs** | 1712 B/op, 24 allocs | **56% less memory** |
| Round-trip | **2136 B/op, 37 allocs** | 2484 B/op, 26 allocs | **14% less memory** |

> **Note:** Simple flat structs (e.g., WideStruct, BattleStats) achieve **zero allocations** during unmarshal thanks to direct byte-level element iteration without intermediate `[]Element` slice allocation or per-element `Key()` string allocation.

### Running Benchmarks

```bash
cd testdata
go test -bench=. -benchmem -count=1
```

## How It Works

1. **Parsing**: Reads Go source, finds `//go:fastbson` structs
2. **Classification**: Categorizes each field into ~35 BSON types
3. **Code Generation**: Emits type-specific `MarshalBSON()` / `UnmarshalBSON()` methods

The generated code uses **`bsoncore.AppendXxxElement`** for all supported types — no `reflect` package calls at runtime. Unknown types fall back to `bson.Marshal`.

## Comparison with Official Driver

| Aspect | `go.mongodb.org/mongo-driver/bson` | `fastbson` |
|--------|--------------------------------------|------------|
| Runtime reflection | Yes — encodes/decodes via `reflect` | **None** — concrete code per field |
| Type discovery | Runtime struct inspection | **Build time** — AST parsing |
| Code generation | No | Yes — adds `_bson.go` files |
| BSON type coverage | Complete | ~95% (all common types) |
| Maintenance | Driver updates | Must re-generate on struct changes |
| Marshal speed | Baseline | **2–3× faster** |
| Unmarshal memory | Baseline | **15–28% less** |

## License

MIT
