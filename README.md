# fastbson — Zero-Reflection BSON Code Generator

`fastbson` is a Go code generator that produces **zero-reflection** `MarshalBSON()` and `UnmarshalBSON()` methods for your structs. It reads Go source files, finds structs annotated with `//go:fastbson`, and generates type-specific BSON encoding/decoding code using `go.mongodb.org/mongo-driver/x/bsonx/bsoncore`.

## Why?

The official `go.mongodb.org/mongo-driver/bson` package uses **reflection** at runtime to marshal/unmarshal every field. For hot-path game servers, high-throughput APIs, or any latency-sensitive application, this overhead adds up.

`fastbson` generates **concrete field-level code** at build time — no reflection, no interface dispatch, no type-casting at runtime.

## Installation

```bash
# Install from source
go install github.com/xsean2020/fastbson@latest

# Or build locally
git clone https://github.com/xsean2020/fastbson.git
cd fastbson
go build -o fastbson main.go
```

## Quick Start

### 1. Annotate Your Structs

Add the `//go:fastbson` directive above any struct you want to generate code for:

```go
package main

//go:fastbson
type Player struct {
    ID    int64  `bson:"_id"`
    Name  string `bson:"name"`
    Level int32  `bson:"lv"`
    Items []int32 `bson:"items,omitempty"`
}
```

### 2. Generate Code

```bash
# Generate for a single file
./fastbson player.go

# Or for an entire directory (scans all .go files)
./fastbson .
```

This generates `player_bson.go` with:
- `func (z *Player) MarshalBSON() ([]byte, error)`
- `func (z *Player) UnmarshalBSON(b []byte) error`

### 3. Use the Generated Methods

```go
package main

import (
    "fmt"
    "log"
)

func main() {
    // Create a player
    p := &Player{
        ID:    1001,
        Name:  "Hero",
        Level: 50,
        Items: []int32{1, 2, 3},
    }

    // Marshal to BSON
    data, err := p.MarshalBSON()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Marshaled %d bytes\n", len(data))

    // Unmarshal from BSON
    var p2 Player
    if err := p2.UnmarshalBSON(data); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Player: %+v\n", p2)
}
```

## Detailed Usage

### Basic Types

```go
//go:fastbson
type BasicTypes struct {
    // Numeric types
    Int     int     `bson:"int"`
    Int8    int8    `bson:"int8"`
    Int16   int16   `bson:"int16"`
    Int32   int32   `bson:"int32"`
    Int64   int64   `bson:"int64"`
    Uint    uint    `bson:"uint"`
    Uint8   uint8   `bson:"uint8"`
    Uint16  uint16  `bson:"uint16"`
    Uint32  uint32  `bson:"uint32"`
    Uint64  uint64  `bson:"uint64"`
    Float32 float32 `bson:"float32"`
    Float64 float64 `bson:"float64"`

    // String and Boolean
    Name    string `bson:"name"`
    Active  bool   `bson:"active"`

    // Binary data
    Data    []byte `bson:"data,omitempty"`
}
```

### Time and ObjectID

```go
import (
    "time"
    "go.mongodb.org/mongo-driver/bson/primitive"
)

//go:fastbson
type Timestamped struct {
    ID        primitive.ObjectID `bson:"_id"`
    CreatedAt time.Time          `bson:"created_at"`
    UpdatedAt primitive.DateTime `bson:"updated_at"`
}
```

### Nested Structs

```go
//go:fastbson
type Address struct {
    Street string `bson:"street"`
    City   string `bson:"city"`
    Zip    string `bson:"zip"`
}

//go:fastbson
type User struct {
    ID      int64   `bson:"_id"`
    Name    string  `bson:"name"`
    Address Address `bson:"address"` // Nested struct
}
```

### Pointers

```go
//go:fastbson
type WithPointers struct {
    Name    *string `bson:"name,omitempty"`
    Age     *int32  `bson:"age,omitempty"`
    Profile *User   `bson:"profile,omitempty"` // Pointer to another fastbson struct
}
```

### Slices and Arrays

```go
//go:fastbson
type Collections struct {
    Tags    []string     `bson:"tags"`
    Scores  []int32      `bson:"scores"`
    Users   []User       `bson:"users"`      // Slice of structs
    Matrix  [][]int32    `bson:"matrix"`     // Nested slices
    Ptrs    []*User      `bson:"ptrs"`       // Slice of pointers
}
```

### Maps

```go
//go:fastbson
type WithMaps struct {
    Metadata map[string]string      `bson:"metadata"`
    Counters map[string]int64       `bson:"counters"`
    Nested   map[string]User        `bson:"nested"`   // Map of structs
}
```

### Anonymous/Inline Structs

```go
//go:fastbson
type GameData struct {
    Player struct {
        ID   int32  `bson:"id"`
        Name string `bson:"name"`
    } `bson:"player"`

    Stats struct {
        Wins   int32 `bson:"wins"`
        Losses int32 `bson:"losses"`
    } `bson:"stats"`
}
```

### Embedded Structs with Inline

```go
//go:fastbson
type Base struct {
    ID   int64  `bson:"_id"`
    Name string `bson:"name"`
}

//go:fastbson
type Extended struct {
    Base  `bson:",inline"` // Flatten Base fields into Extended
    Extra string `bson:"extra"`
}
```

### Primitive Types

```go
import "go.mongodb.org/mongo-driver/bson/primitive"

//go:fastbson
type WithPrimitives struct {
    Binary     primitive.Binary     `bson:"binary"`
    Regex      primitive.Regex      `bson:"regex"`
    Timestamp  primitive.Timestamp  `bson:"timestamp"`
    Decimal    primitive.Decimal128 `bson:"decimal"`
    JavaScript primitive.JavaScript `bson:"javascript"`
    Symbol     primitive.Symbol     `bson:"symbol"`
    Null       primitive.Null       `bson:"null"`
    MinKey     primitive.MinKey     `bson:"min_key"`
    MaxKey     primitive.MaxKey     `bson:"max_key"`
    D          primitive.D          `bson:"d"` // Dynamic document
    A          primitive.A          `bson:"a"` // Dynamic array
    M          primitive.M          `bson:"m"` // Map[string]interface{}
}
```

## Tag Options

### Basic Tags

```go
type Example struct {
    Field1 string `bson:"field1"`           // Custom key name
    Field2 string `bson:"-"`               // Skip this field
    Field3 string `bson:"field3,omitempty"` // Skip if zero/nil
    Field4 int64  `bson:"field4,minsize"`  // Encode as Int32 if fits
    Field5 Base   `bson:",inline"`         // Flatten embedded struct
}
```

### OmitEmpty Behavior

The `omitempty` option skips fields with zero values:

```go
//go:fastbson
type OmitExample struct {
    Name  string  `bson:"name,omitempty"`   // Skips empty string
    Age   int32   `bson:"age,omitempty"`    // Skips 0
    Score float64 `bson:"score,omitempty"`  // Skips 0.0
    Items []int32 `bson:"items,omitempty"`  // Skips nil slices
    Data  *User   `bson:"data,omitempty"`   // Skips nil pointers
}
```

### MinSize Behavior

The `minsize` option encodes `int64` values as `int32` when they fit:

```go
//go:fastbson
type MinSizeExample struct {
    Count int64 `bson:"count,minsize"` // Encoded as Int32 if < 2^31
}
```

## Cross-File References

`fastbson` supports struct references across files. If `StructA` references `StructB` (with `//go:fastbson`), ensure both files are in the same directory:

```go
// hero.go
//go:fastbson
type Hero struct {
    ID   int32  `bson:"id"`
    Name string `bson:"name"`
}

// player.go
//go:fastbson
type Player struct {
    ID    int64  `bson:"_id"`
    Hero  Hero   `bson:"hero"` // References Hero from hero.go
}
```

```bash
# Generate code for all files in the directory
./fastbson .
```

## Performance

Benchmarks on Apple M1 (Go 1.25), comparing generated code vs `go.mongodb.org/mongo-driver/bson`:

### Marshal

| Struct | Size | Generated | Official | Speedup |
|--------|------|-----------|----------|---------|
| BattleStats (3 int32) | 37 B | **88 ns/op** | 237 ns/op | **2.7×** |
| IntWidths (10 ints) | 148 B | **230 ns/op** | 674 ns/op | **2.9×** |
| WideStruct (26 int32) | 324 B | **315 ns/op** | 1170 ns/op | **3.7×** |
| Player (complex, 17+ fields) | ~500 B | **680 ns/op** | 2100 ns/op | **3.1×** |

### Unmarshal

| Struct | Generated | Official | Speedup |
|--------|-----------|----------|---------|
| BattleStats (3 int32) | **55 ns/op** | 160 ns/op | **2.9×** |
| WideStruct (26 int32) | **455 ns/op** | 590 ns/op | **1.3×** |
| Player (complex, 17+ fields) | **1565 ns/op** | 1740 ns/op | **1.1×** |
| PlayerHeroRefs (sub-docs) | **1010 ns/op** | 1170 ns/op | **1.2×** |

### Memory

| Operation | Generated | Official | Improvement |
|-----------|-----------|----------|-------------|
| Marshal | 1384 B/op, 17 allocs | 768 B/op, 2 allocs | — (sub-buffers) |
| Unmarshal | **1064 B/op, 28 allocs** | 2056 B/op, 32 allocs | **48% less memory** |
| Round-trip | **2448 B/op, 45 allocs** | 2824 B/op, 34 allocs | **13% less memory** |

> **Note:** Simple flat structs (e.g., WideStruct, BattleStats) achieve **zero allocations** during unmarshal thanks to direct byte-level element iteration. For larger/complex structs, the speedup is smaller (1.1×-1.3×) but still consistently faster than the official driver.

### Running Benchmarks

```bash
cd testdata
go test -bench=. -benchmem -count=1
```

## How It Works

1. **Parsing**: Reads Go source files, finds `//go:fastbson` structs
2. **Classification**: Categorizes each field into ~35 BSON types
3. **Code Generation**: Emits type-specific `MarshalBSON()` / `UnmarshalBSON()` methods

The generated code uses **`bsoncore.AppendXxxElement`** for all supported types — no `reflect` package calls at runtime. Unknown types fall back to `bson.Marshal`.

### Key Optimizations

- **Zero-allocation key dispatch**: Uses `unsafe.String` for O(1) string comparison without allocation
- **Direct byte-level parsing**: Iterates BSON elements without intermediate slice allocation
- **Pre-allocated slices**: Arrays and maps are pre-allocated with known capacity
- **Stack-allocated buffers**: Uses fixed-size arrays for temporary operations

## Comparison with Official Driver

| Aspect | `go.mongodb.org/mongo-driver/bson` | `fastbson` |
|--------|--------------------------------------|------------|
| Runtime reflection | Yes — encodes/decodes via `reflect` | **None** — concrete code per field |
| Type discovery | Runtime struct inspection | **Build time** — AST parsing |
| Code generation | No | Yes — adds `_bson.go` files |
| BSON type coverage | Complete | ~95% (all common types) |
| Maintenance | Driver updates | Must re-generate on struct changes |
| Marshal speed | Baseline | **2–3× faster** |
| Unmarshal memory | Baseline | **15–50% less** |

## Project Structure

```
fastbson/
├── main.go              # Code generator source
├── go.mod               # Go module definition
├── README.md            # This file
├── testdata/            # Test files and benchmarks
│   ├── types.go         # Test struct definitions
│   ├── types_bson.go    # Generated code (do not edit)
│   ├── bson_test.go     # Unit tests
│   ├── bson_bench_test.go # Benchmarks
│   └── bson_fuzz_test.go  # Fuzz tests
└── fastbson             # Compiled binary (gitignored)
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License. See [LICENSE](LICENSE) for details.

## Acknowledgments

- [go.mongodb.org/mongo-driver](https://github.com/mongodb/mongo-go-driver) — Official MongoDB Go driver
- [bsoncore](https://pkg.go.dev/go.mongodb.org/mongo-driver/x/bsonx/bsoncore) — Low-level BSON operations
