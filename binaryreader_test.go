package ion

import (
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"
)

func TestReadBadBVMs(t *testing.T) {
	t.Run("E00200E9", func(t *testing.T) {
		// Need a good first one or we'll get sent to the text reader.
		r := NewReaderBytes([]byte{0xE0, 0x01, 0x00, 0xEA, 0xE0, 0x02, 0x00, 0xE9})
		if r.Next() {
			t.Errorf("next returned true")
		}
		if r.Err() == nil {
			t.Fatal("err is nil")
		}
	})

	t.Run("E00200EA", func(t *testing.T) {
		r := NewReaderBytes([]byte{0xE0, 0x02, 0x00, 0xEA})
		if r.Next() {
			t.Errorf("next returned true")
		}
		if r.Err() == nil {
			t.Fatal("err is nil")
		}

		uve, ok := r.Err().(*UnsupportedVersionError)
		if !ok {
			t.Fatal("err is not an UnsupportedVersionError")
		}
		if uve.Major != 2 {
			t.Errorf("expected major=2, got %v", uve.Major)
		}
		if uve.Minor != 0 {
			t.Errorf("expected minor=0, got %v", uve.Minor)
		}
	})
}

func TestReadNullLST(t *testing.T) {
	ion := []byte{
		0xE0, 0x01, 0x00, 0xEA,
		0xE4, 0x82, 0x83, 0x87, 0xDF,
		0x71, 0x09,
	}
	r := NewReaderBytes(ion)
	_symbol(t, r, "$ion_shared_symbol_table")
	_eof(t, r)
}

func TestReadEmptyLST(t *testing.T) {
	ion := []byte{
		0xE0, 0x01, 0x00, 0xEA,
		0xE4, 0x82, 0x83, 0x87, 0xD0,
		0x71, 0x09,
	}
	r := NewReaderBytes(ion)
	_symbol(t, r, "$ion_shared_symbol_table")
	_eof(t, r)
}

func TestReadBadLST(t *testing.T) {
	ion := []byte{
		0xE0, 0x01, 0x00, 0xEA,
		0xE3, 0x81, 0x83, 0xD9,
		0x86, 0xB7, 0xD6, // imports:[{
		0x84, 0x81, 'a', // name: "a",
		0x85, 0x21, 0x01, // version: 1}]}
		0x0F, // null
	}
	r := NewReaderBytes(ion)
	if r.Next() {
		t.Fatal("next returned true")
	}
	if r.Err() == nil {
		t.Fatal("err is nil")
	}
}

func TestReadMultipleLSTs(t *testing.T) {
	r := readBinary([]byte{
		0x71, 0x0B, // $11
		0x71, 0x6F, // bar
		0xE3, 0x81, 0x83, 0xDF, // $ion_symbol_table::null.struct
		0xEE, 0x8F, 0x81, 0x83, 0xDD, // $ion_symbol_table::{
		0x86, 0x71, 0x03, // imports: $ion_symbol_table,
		0x87, 0xB8, // symbols:[
		0x83, 'f', 'o', 'o', // "foo"
		0x83, 'b', 'a', 'r', // "bar" ]}
		0x71, 0x0B, // bar
		0x71, 0x0C, // $12
		0x71, 0x6F, // $111
		0xEC, 0x81, 0x83, 0xD9, // $ion_symbol_table::{
		0x86, 0x71, 0x03, // imports: $ion_symbol_table
		0x87, 0xB4, // symbols:[
		0x83, 'b', 'a', 'z', // "baz" ]}
		0x71, 0x0B, // bar
		0x71, 0x0C, // baz
	})
	_symbol(t, r, "$11")
	_symbol(t, r, "bar")

	_symbol(t, r, "bar")
	_symbol(t, r, "$12")
	_symbol(t, r, "$111")

	_symbol(t, r, "bar")
	_symbol(t, r, "baz")
	_eof(t, r)
}

func TestReadBinaryLST(t *testing.T) {
	r := readBinary([]byte{0x0F})
	_next(t, r, NullType)

	lst := r.SymbolTable()
	if lst == nil {
		t.Fatal("symboltable is nil")
	}

	if lst.MaxID() != 111 {
		t.Errorf("expected maxid=111, got %v", lst.MaxID())
	}

	if _, ok := lst.FindByID(109); ok {
		t.Error("found a symbol for $109")
	}

	sym, ok := lst.FindByID(111)
	if !ok {
		t.Fatal("no symbol defined for $111")
	}
	if sym != "bar" {
		t.Errorf("expected $111=bar, got %v", sym)
	}

	id, ok := lst.FindByName("foo")
	if !ok {
		t.Fatal("no id defined for foo")
	}
	if id != 110 {
		t.Errorf("expected foo=$110, got $%v", id)
	}

	if _, ok := lst.FindByID(112); ok {
		t.Error("found a symbol for $112")
	}

	if _, ok := lst.FindByName("bogus"); ok {
		t.Error("found a symbol for bogus")
	}
}

func TestReadBinaryStructs(t *testing.T) {
	r := readBinary([]byte{
		0xDF,                   // null.struct
		0xD0,                   // {}
		0xEA, 0x81, 0xEE, 0xD7, // foo::{
		0x84, 0xE3, 0x81, 0xEF, 0xD0, // name:bar::{},
		0x88, 0x20, // max_id:0
		// }
	})

	_null(t, r, StructType)
	_struct(t, r, func(t *testing.T, r Reader) {
		_eof(t, r)
	})
	_structAF(t, r, "", []string{"foo"}, func(t *testing.T, r Reader) {
		_structAF(t, r, "name", []string{"bar"}, func(t *testing.T, r Reader) {
			_eof(t, r)
		})
		_intAF(t, r, "max_id", nil, 0)
	})
	_eof(t, r)
}

func TestReadBinarySexps(t *testing.T) {
	r := readBinary([]byte{
		0xCF,
		0xC3, 0xC1, 0xC0, 0xC0,
	})

	_null(t, r, SexpType)
	_sexp(t, r, func(t *testing.T, r Reader) {
		_sexp(t, r, func(t *testing.T, r Reader) {
			_sexp(t, r, func(t *testing.T, r Reader) {
				_eof(t, r)
			})
		})
		_sexp(t, r, func(t *testing.T, r Reader) {
			_eof(t, r)
		})
		_eof(t, r)
	})
	_eof(t, r)
}

func TestReadBinaryLists(t *testing.T) {
	r := readBinary([]byte{
		0xBF,
		0xB3, 0xB1, 0xB0, 0xB0,
	})

	_null(t, r, ListType)
	_list(t, r, func(t *testing.T, r Reader) {
		_list(t, r, func(t *testing.T, r Reader) {
			_list(t, r, func(t *testing.T, r Reader) {
				_eof(t, r)
			})
		})
		_list(t, r, func(t *testing.T, r Reader) {
			_eof(t, r)
		})
		_eof(t, r)
	})
	_eof(t, r)
}

func TestReadBinaryBlobs(t *testing.T) {
	r := readBinary([]byte{
		0xAF,
		0xA0,
		0xA1, 'a',
		0xAE, 0x96,
		'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd', ' ', 'b', 'u', 't',
		' ', 'l', 'o', 'n', 'g', 'e', 'r',
	})

	_null(t, r, BlobType)
	_blob(t, r, []byte(""))
	_blob(t, r, []byte("a"))
	_blob(t, r, []byte("hello world but longer"))
	_eof(t, r)
}

func TestReadBinaryClobs(t *testing.T) {
	r := readBinary([]byte{
		0x9F,
		0x90,      // {{}}
		0x91, 'a', // {{a}}
		0x9E, 0x96,
		'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd', ' ', 'b', 'u', 't',
		' ', 'l', 'o', 'n', 'g', 'e', 'r',
	})

	_null(t, r, ClobType)
	_clob(t, r, []byte(""))
	_clob(t, r, []byte("a"))
	_clob(t, r, []byte("hello world but longer"))
	_eof(t, r)
}

func TestReadBinaryStrings(t *testing.T) {
	r := readBinary([]byte{
		0x8F,
		0x80,      // ""
		0x81, 'a', // "a"
		0x8E, 0x96,
		'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd', ' ', 'b', 'u', 't',
		' ', 'l', 'o', 'n', 'g', 'e', 'r',
	})

	_null(t, r, StringType)
	_string(t, r, "")
	_string(t, r, "a")
	_string(t, r, "hello world but longer")
	_eof(t, r)
}

func TestReadBinarySymbols(t *testing.T) {
	r := readBinary([]byte{
		0x7F,
		0x70,       // $0
		0x71, 0x01, // $ion
		0x71, 0x0A, // $10
		0x71, 0x6E, // foo
		0xE4, 0x81, 0xEE, 0x71, 0x6F, // foo::bar
		0x78, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // ${maxint64}
	})

	_null(t, r, SymbolType)
	_symbol(t, r, "$0")
	_symbol(t, r, "$ion")
	_symbol(t, r, "$10")
	_symbol(t, r, "foo")
	_symbolAF(t, r, "", []string{"foo"}, "bar")
	_symbol(t, r, fmt.Sprintf("$%v", uint64(math.MaxUint64)))
	_eof(t, r)
}

func TestReadBinaryTimestamps(t *testing.T) {
	r := readBinary([]byte{
		0x6F,
		0x62, 0x80, 0x81, // 0001T
		0x63, 0x80, 0x81, 0x81, // 0001-01T
		0x64, 0x80, 0x81, 0x81, 0x81, // 0001-01-01T
		0x66, 0x80, 0x81, 0x81, 0x81, 0x80, 0x80, // 0001-01-01T00:00Z
		0x67, 0x80, 0x81, 0x81, 0x81, 0x80, 0x80, 0x80, // 0001-01-01T00:00:00Z
		0x6E, 0x8E, // 0x0E-bit timestamp
		0x04, 0xD8, // offset: +600 minutes (+10:00)
		0x0F, 0xE3, // year:   2019
		0x88,                   // month:  8
		0x84,                   // day:    4
		0x88,                   // hour:   8 utc (18 local)
		0x8F,                   // minute: 15
		0xAB,                   // second: 43
		0xC9,                   // exp:    -9
		0x33, 0x77, 0xDF, 0x70, // nsec:   863494000
	})

	_null(t, r, TimestampType)

	for i := 0; i < 5; i++ {
		_timestamp(t, r, time.Time{})
	}

	nowish, _ := time.Parse(time.RFC3339Nano, "2019-08-04T18:15:43.863494+10:00")
	_timestamp(t, r, nowish)
	_eof(t, r)
}

func TestReadBinaryDecimals(t *testing.T) {
	r := readBinary([]byte{
		0x50,       // 0.
		0x5F,       // null.decimal
		0x51, 0xC3, // 0.000, aka 0 x 10^-3
		0x53, 0xC3, 0x03, 0xE8, // 1.000, aka 1000 x 10^-3
		0x53, 0xC3, 0x83, 0xE8, // -1.000, aka -1000 x 10^-3
		0x53, 0x00, 0xE4, 0x01, // 1d100, aka 1 * 10^100
		0x53, 0x00, 0xE4, 0x81, // -1d100, aka -1 * 10^100
	})

	_decimal(t, r, MustParseDecimal("0."))
	_null(t, r, DecimalType)
	_decimal(t, r, MustParseDecimal("0.000"))
	_decimal(t, r, MustParseDecimal("1.000"))
	_decimal(t, r, MustParseDecimal("-1.000"))
	_decimal(t, r, MustParseDecimal("1d100"))
	_decimal(t, r, MustParseDecimal("-1d100"))
	_eof(t, r)
}

func TestReadBinaryFloats(t *testing.T) {
	r := readBinary([]byte{
		0x40,                         // 0
		0x4F,                         // null.float
		0x44, 0x7F, 0x7F, 0xFF, 0xFF, // MaxFloat32
		0x44, 0xFF, 0x7F, 0xFF, 0xFF, // -MaxFloat32
		0x48, 0x7F, 0xEF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // MaxFloat64
		0x48, 0xFF, 0xEF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // -MaxFloat64
		0x48, 0x7F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // +inf
		0x48, 0xFF, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // -inf
		0x48, 0x7F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // NaN
	})

	_float(t, r, 0)
	_null(t, r, FloatType)
	_float(t, r, math.MaxFloat32)
	_float(t, r, -math.MaxFloat32)
	_float(t, r, math.MaxFloat64)
	_float(t, r, -math.MaxFloat64)
	_float(t, r, math.Inf(1))
	_float(t, r, math.Inf(-1))
	_float(t, r, math.NaN())
	_eof(t, r)
}

func TestReadBinaryInts(t *testing.T) {
	r := readBinary([]byte{
		0x20,       // 0
		0x2F,       // null.int
		0x21, 0x01, // 1
		0x31, 0x01, // -1
		0x28, 0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // 0x7FFFFFFFFFFFFFFF
		0x38, 0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // -0x7FFFFFFFFFFFFFFF
		0x28, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 0x8000000000000000
		0x38, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 0x8000000000000000
	})

	_int(t, r, 0)
	_null(t, r, IntType)
	_int(t, r, 1)
	_int(t, r, -1)
	_int64(t, r, math.MaxInt64)
	_int64(t, r, -math.MaxInt64)

	_uint(t, r, math.MaxInt64+1)

	i := new(big.Int).SetUint64(math.MaxInt64 + 1)
	_bigInt(t, r, new(big.Int).Neg(i))

	_eof(t, r)
}

func TestReadBinaryBools(t *testing.T) {
	r := readBinary([]byte{
		0x10, // false
		0x11, // true
		0x1F, // null.bool
	})

	_bool(t, r, false)
	_bool(t, r, true)
	_null(t, r, BoolType)
	_eof(t, r)
}

func TestReadBinaryNulls(t *testing.T) {
	r := readBinary([]byte{
		0x00,       // 1-byte NOP
		0x0F,       // null
		0x01, 0xFF, // 2-byte NOP
		0xE3, 0x81, 0x81, 0x0F, // $ion::null
		0x0E, 0x8F, // 16-byte NOP
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xE4, 0x82, 0xEE, 0xEF, 0x0F, // foo::bar::null
	})

	_null(t, r, NullType)
	_nullAF(t, r, NullType, "", []string{"$ion"})
	_nullAF(t, r, NullType, "", []string{"foo", "bar"})
	_eof(t, r)
}

func TestReadEmptyBinary(t *testing.T) {
	r := NewReaderBytes([]byte{0xE0, 0x01, 0x00, 0xEA})
	_eof(t, r)
	_eof(t, r)
}

func readBinary(ion []byte) Reader {
	prefix := []byte{
		0xE0, 0x01, 0x00, 0xEA, // $ion_1_0
		0xEE, 0x9F, 0x81, 0x83, 0xDE, 0x9B, // $ion_symbol_table::{
		0x86, 0xBE, 0x8E, // imports:[
		0xDD,                                // {
		0x84, 0x85, 'b', 'o', 'g', 'u', 's', // name: "bogus"
		0x85, 0x21, 0x2A, // version: 42
		0x88, 0x21, 0x64, // max_id: 100
		// }]
		0x87, 0xB8, // symbols: [
		0x83, 'f', 'o', 'o', // "foo"
		0x83, 'b', 'a', 'r', // "bar"
		// ]
		// }
	}
	return NewReaderBytes(append(prefix, ion...))
}
