// +build go1.9

// Copyright 2017 Tamás Gulácsi
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package ora

/*
#cgo CFLAGS: -Iodpi/src -Iodpi/include
#cgo LDFLAGS: -Lodpi/lib -lodpic -ldl

#include "dpiImpl.h"
*/
import "C"
import (
	"database/sql/driver"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"
	"unsafe"

	"github.com/pkg/errors"
)

const fetchRowCount = 1 //<< 7
const maxArraySize = 1 << 10

var _ = driver.Rows((*rows)(nil))
var _ = driver.RowsColumnTypeDatabaseTypeName((*rows)(nil))
var _ = driver.RowsColumnTypeLength((*rows)(nil))
var _ = driver.RowsColumnTypeNullable((*rows)(nil))
var _ = driver.RowsColumnTypePrecisionScale((*rows)(nil))
var _ = driver.RowsColumnTypeScanType((*rows)(nil))

type rows struct {
	*statement
	columns        []Column
	bufferRowIndex C.uint32_t
	fetched        C.uint32_t
	finished       bool
	vars           []*C.dpiVar
	data           [][]*C.dpiData
}

// Columns returns the names of the columns. The number of
// columns of the result is inferred from the length of the
// slice. If a particular column name isn't known, an empty
// string should be returned for that entry.
func (r *rows) Columns() []string {
	names := make([]string, len(r.columns))
	for i, col := range r.columns {
		names[i] = col.Name
	}
	return names
}

// Close closes the rows iterator.
func (r *rows) Close() error {
	for _, v := range r.vars {
		C.dpiVar_release(v)
	}
	if C.dpiStmt_release(r.statement.dpiStmt) == C.DPI_FAILURE {
		return r.getError()
	}
	return nil
}

// ColumnTypeLength return the length of the column type if the column is a variable length type.
// If the column is not a variable length type ok should return false.
// If length is not limited other than system limits, it should return math.MaxInt64.
// The following are examples of returned values for various types:
//
// TEXT          (math.MaxInt64, true)
// varchar(10)   (10, true)
// nvarchar(10)  (10, true)
// decimal       (0, false)
// int           (0, false)
// bytea(30)     (30, true)
func (r *rows) ColumnTypeLength(index int) (length int64, ok bool) {
	switch col := r.columns[index]; col.Type {
	case C.DPI_ORACLE_TYPE_VARCHAR, C.DPI_ORACLE_TYPE_NVARCHAR,
		C.DPI_ORACLE_TYPE_CHAR, C.DPI_ORACLE_TYPE_NCHAR,
		C.DPI_ORACLE_TYPE_LONG_VARCHAR,
		C.DPI_NATIVE_TYPE_BYTES:
		return int64(col.Size), true
	case C.DPI_ORACLE_TYPE_CLOB, C.DPI_ORACLE_TYPE_NCLOB,
		C.DPI_ORACLE_TYPE_BLOB,
		C.DPI_ORACLE_TYPE_BFILE,
		C.DPI_NATIVE_TYPE_LOB:
		return math.MaxInt64, true
	default:
		return 0, false
	}
}

// ColumnTypeDatabaseTypeName returns the database system type name without the length.
// Type names should be uppercase.
// Examples of returned types: "VARCHAR", "NVARCHAR", "VARCHAR2", "CHAR", "TEXT", "DECIMAL", "SMALLINT", "INT", "BIGINT", "BOOL", "[]BIGINT", "JSONB", "XML", "TIMESTAMP".
func (r *rows) ColumnTypeDatabaseTypeName(index int) string {
	switch r.columns[index].Type {
	case C.DPI_ORACLE_TYPE_VARCHAR:
		return "VARCHAR2"
	case C.DPI_ORACLE_TYPE_NVARCHAR:
		return "NVARCHAR2"
	case C.DPI_ORACLE_TYPE_CHAR:
		return "CHAR"
	case C.DPI_ORACLE_TYPE_NCHAR:
		return "NCHAR"
	case C.DPI_ORACLE_TYPE_LONG_VARCHAR:
		return "LONG"
	case C.DPI_NATIVE_TYPE_BYTES, C.DPI_ORACLE_TYPE_RAW:
		return "RAW"
	case C.DPI_ORACLE_TYPE_ROWID, C.DPI_NATIVE_TYPE_ROWID:
		return "ROWID"
	case C.DPI_ORACLE_TYPE_LONG_RAW:
		return "LONG RAW"
	case C.DPI_ORACLE_TYPE_NUMBER:
		return "NUMBER"
	case C.DPI_ORACLE_TYPE_NATIVE_FLOAT, C.DPI_NATIVE_TYPE_FLOAT:
		return "FLOAT"
	case C.DPI_ORACLE_TYPE_NATIVE_DOUBLE, C.DPI_NATIVE_TYPE_DOUBLE:
		return "DOUBLE"
	case C.DPI_ORACLE_TYPE_NATIVE_INT, C.DPI_NATIVE_TYPE_INT64:
		return "BINARY_INTEGER"
	case C.DPI_ORACLE_TYPE_NATIVE_UINT, C.DPI_NATIVE_TYPE_UINT64:
		return "BINARY_INTEGER"
	case C.DPI_ORACLE_TYPE_TIMESTAMP, C.DPI_NATIVE_TYPE_TIMESTAMP:
		return "TIMESTAMP"
	case C.DPI_ORACLE_TYPE_TIMESTAMP_TZ:
		return "TIMESTAMP WITH TIMEZONE"
	case C.DPI_ORACLE_TYPE_TIMESTAMP_LTZ:
		return "TIMESTAMP WITH LOCAL TIMEZONE"
	case C.DPI_ORACLE_TYPE_DATE:
		return "DATE"
	case C.DPI_ORACLE_TYPE_INTERVAL_DS, C.DPI_NATIVE_TYPE_INTERVAL_DS:
		return "INTERVAL DAY TO SECOND"
	case C.DPI_ORACLE_TYPE_INTERVAL_YM, C.DPI_NATIVE_TYPE_INTERVAL_YM:
		return "INTERVAL YEAR TO MONTH"
	case C.DPI_ORACLE_TYPE_CLOB:
		return "CLOB"
	case C.DPI_ORACLE_TYPE_NCLOB:
		return "NCLOB"
	case C.DPI_ORACLE_TYPE_BLOB:
		return "BLOB"
	case C.DPI_ORACLE_TYPE_BFILE:
		return "BFILE"
	case C.DPI_ORACLE_TYPE_STMT, C.DPI_NATIVE_TYPE_STMT:
		return "SYS_REFCURSOR"
	case C.DPI_ORACLE_TYPE_BOOLEAN, C.DPI_NATIVE_TYPE_BOOLEAN:
		return "BOOLEAN"
	case C.DPI_ORACLE_TYPE_OBJECT:
		return "OBJECT"
	default:
		return fmt.Sprintf("OTHER[%d]", r.columns[index].Type)
	}
}

// ColumnTypeNullable. The nullable value should be true if it is known the column may be null, or false if the column is known to be not nullable. If the column nullability is unknown, ok should be false.

func (r *rows) ColumnTypeNullable(index int) (nullable, ok bool) {
	return r.columns[index].Nullable, true
}

// ColumnTypePrecisionScale returns the precision and scale for decimal types.
// If not applicable, ok should be false.
// The following are examples of returned values for various types:
//
// decimal(38, 4)    (38, 4, true)
// int               (0, 0, false)
// decimal           (math.MaxInt64, math.MaxInt64, true)
func (r *rows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	switch col := r.columns[index]; col.Type {
	case
		//C.DPI_ORACLE_TYPE_NATIVE_FLOAT, C.DPI_NATIVE_TYPE_FLOAT,
		//C.DPI_ORACLE_TYPE_NATIVE_DOUBLE, C.DPI_NATIVE_TYPE_DOUBLE,
		//C.DPI_ORACLE_TYPE_NATIVE_INT, C.DPI_NATIVE_TYPE_INT64,
		//C.DPI_ORACLE_TYPE_NATIVE_UINT, C.DPI_NATIVE_TYPE_UINT64,
		C.DPI_ORACLE_TYPE_NUMBER:
		return int64(col.Precision), int64(col.Scale), true
	default:
		return 0, 0, false
	}
}

// ColumnTypeScanType returns the value type that can be used to scan types into.
// For example, the database column type "bigint" this should return "reflect.TypeOf(int64(0))".
func (r *rows) ColumnTypeScanType(index int) reflect.Type {
	switch col := r.columns[index]; col.Type {
	case C.DPI_NATIVE_TYPE_BYTES, C.DPI_ORACLE_TYPE_RAW,
		C.DPI_ORACLE_TYPE_ROWID, C.DPI_NATIVE_TYPE_ROWID,
		C.DPI_ORACLE_TYPE_LONG_RAW:
		return reflect.TypeOf([]byte(nil))
	case C.DPI_ORACLE_TYPE_NUMBER:
		switch col.DefaultNumType {
		case C.DPI_NATIVE_TYPE_INT64:
			return reflect.TypeOf(int64(0))
		case C.DPI_NATIVE_TYPE_UINT64:
			return reflect.TypeOf(uint64(0))
		case C.DPI_NATIVE_TYPE_FLOAT:
			return reflect.TypeOf(float32(0))
		case C.DPI_NATIVE_TYPE_DOUBLE:
			return reflect.TypeOf(float64(0))
		default:
			return reflect.TypeOf("")
		}
	case C.DPI_ORACLE_TYPE_NATIVE_FLOAT, C.DPI_NATIVE_TYPE_FLOAT:
		return reflect.TypeOf(float32(0))
	case C.DPI_ORACLE_TYPE_NATIVE_DOUBLE, C.DPI_NATIVE_TYPE_DOUBLE:
		return reflect.TypeOf(float64(0))
	case C.DPI_ORACLE_TYPE_NATIVE_INT, C.DPI_NATIVE_TYPE_INT64:
		return reflect.TypeOf(int64(0))
	case C.DPI_ORACLE_TYPE_NATIVE_UINT, C.DPI_NATIVE_TYPE_UINT64:
		return reflect.TypeOf(uint64(0))
	case C.DPI_ORACLE_TYPE_TIMESTAMP, C.DPI_NATIVE_TYPE_TIMESTAMP,
		C.DPI_ORACLE_TYPE_TIMESTAMP_TZ, C.DPI_ORACLE_TYPE_TIMESTAMP_LTZ,
		C.DPI_ORACLE_TYPE_DATE:
		return reflect.TypeOf(time.Time{})
	case C.DPI_ORACLE_TYPE_INTERVAL_DS, C.DPI_NATIVE_TYPE_INTERVAL_DS:
		return reflect.TypeOf(time.Duration(0))
	case C.DPI_ORACLE_TYPE_CLOB, C.DPI_ORACLE_TYPE_NCLOB:
		return reflect.TypeOf("")
	case C.DPI_ORACLE_TYPE_BLOB, C.DPI_ORACLE_TYPE_BFILE:
		return reflect.TypeOf([]byte(nil))
	case C.DPI_ORACLE_TYPE_STMT, C.DPI_NATIVE_TYPE_STMT:
		return reflect.TypeOf(&statement{})
	case C.DPI_ORACLE_TYPE_BOOLEAN, C.DPI_NATIVE_TYPE_BOOLEAN:
		return reflect.TypeOf(false)
	default:
		return reflect.TypeOf("")
	}
}

// Next is called to populate the next row of data into
// the provided slice. The provided slice will be the same
// size as the Columns() are wide.
//
// Next should return io.EOF when there are no more rows.
func (r *rows) Next(dest []driver.Value) error {
	if r.finished {
		return io.EOF
	}
	if r.fetched == 0 {
		var moreRows C.int
		if C.dpiStmt_fetchRows(r.dpiStmt, fetchRowCount, &r.bufferRowIndex, &r.fetched, &moreRows) == C.DPI_FAILURE {
			return r.getError()
		}
		if r.fetched == 0 {
			r.finished = moreRows == 0
			return io.EOF
		}
		//fmt.Printf("data=%#v\n", r.data)
	}
	//fmt.Printf("data=%#v\n", r.data)

	//fmt.Printf("bri=%d fetched=%d\n", r.bufferRowIndex, r.fetched)
	//fmt.Printf("data=%#v\n", r.data[0][r.bufferRowIndex])
	//fmt.Printf("VC=%d\n", C.DPI_ORACLE_TYPE_VARCHAR)
	for i, col := range r.columns {
		typ := col.Type
		d := r.data[i][r.bufferRowIndex]
		//fmt.Printf("data=%#v typ=%d\n", d, typ)
		if d.isNull == 1 {
			dest[i] = nil
			continue
		}

		switch typ {
		case C.DPI_ORACLE_TYPE_VARCHAR, C.DPI_ORACLE_TYPE_NVARCHAR,
			C.DPI_ORACLE_TYPE_CHAR, C.DPI_ORACLE_TYPE_NCHAR,
			C.DPI_ORACLE_TYPE_LONG_VARCHAR,
			C.DPI_NATIVE_TYPE_BYTES:
			//fmt.Printf("CHAR\n")
			b := C.dpiData_getBytes(d)
			if b.ptr == nil {
				dest[i] = ""
				continue
			}
			dest[i] = C.GoStringN(b.ptr, C.int(b.length))

		case C.DPI_ORACLE_TYPE_NUMBER:
			switch col.DefaultNumType {
			case C.DPI_NATIVE_TYPE_INT64:
				dest[i] = C.dpiData_getInt64(d)
			case C.DPI_NATIVE_TYPE_UINT64:
				dest[i] = C.dpiData_getUint64(d)
			case C.DPI_NATIVE_TYPE_FLOAT:
				dest[i] = C.dpiData_getFloat(d)
			case C.DPI_NATIVE_TYPE_DOUBLE:
				dest[i] = C.dpiData_getDouble(d)
			default:
				b := C.dpiData_getBytes(d)
				//fmt.Printf("b=%p[%d] t=%d i=%d\n", b.ptr, b.length, col.DefaultNumType, C.dpiData_getInt64(d))
				dest[i] = C.GoStringN(b.ptr, C.int(b.length))
			}

		case C.DPI_ORACLE_TYPE_ROWID, C.DPI_NATIVE_TYPE_ROWID,
			C.DPI_ORACLE_TYPE_RAW, C.DPI_ORACLE_TYPE_LONG_RAW:
			fmt.Printf("RAW\n")
			b := C.dpiData_getBytes(d)
			dest[i] = C.GoBytes(unsafe.Pointer(b.ptr), C.int(b.length))
		case C.DPI_ORACLE_TYPE_NATIVE_FLOAT, C.DPI_NATIVE_TYPE_FLOAT:
			fmt.Printf("FLOAT\n")
			dest[i] = float32(C.dpiData_getFloat(d))
		case C.DPI_ORACLE_TYPE_NATIVE_DOUBLE, C.DPI_NATIVE_TYPE_DOUBLE:
			fmt.Printf("DOUBLE\n")
			dest[i] = float64(C.dpiData_getDouble(d))
		case C.DPI_ORACLE_TYPE_NATIVE_INT, C.DPI_NATIVE_TYPE_INT64:
			fmt.Printf("INT\n")
			dest[i] = int64(C.dpiData_getInt64(d))
		case C.DPI_ORACLE_TYPE_NATIVE_UINT, C.DPI_NATIVE_TYPE_UINT64:
			fmt.Printf("UINT\n")
			dest[i] = uint64(C.dpiData_getUint64(d))
		case C.DPI_ORACLE_TYPE_TIMESTAMP,
			C.DPI_ORACLE_TYPE_TIMESTAMP_TZ, C.DPI_ORACLE_TYPE_TIMESTAMP_LTZ,
			C.DPI_NATIVE_TYPE_TIMESTAMP,
			C.DPI_ORACLE_TYPE_DATE:
			//fmt.Printf("TS\n")
			ts := C.dpiData_getTimestamp(d)
			tz := time.Local
			if col.Type != C.DPI_ORACLE_TYPE_TIMESTAMP && col.Type != C.DPI_ORACLE_TYPE_DATE {
				tz = time.FixedZone(
					fmt.Sprintf("%02d:%02d", ts.tzHourOffset, ts.tzMinuteOffset),
					int(ts.tzHourOffset)*3600+int(ts.tzMinuteOffset)*60,
				)
			}
			dest[i] = time.Date(int(ts.year), time.Month(ts.month), int(ts.day), int(ts.hour), int(ts.minute), int(ts.second), int(ts.fsecond), tz)
		case C.DPI_ORACLE_TYPE_INTERVAL_DS, C.DPI_NATIVE_TYPE_INTERVAL_DS:
			fmt.Printf("INTERVAL_DS\n")
			ds := C.dpiData_getIntervalDS(d)
			dest[i] = time.Duration(ds.days)*24*time.Hour +
				time.Duration(ds.hours)*time.Hour +
				time.Duration(ds.minutes)*time.Minute +
				time.Duration(ds.seconds)*time.Second +
				time.Duration(ds.fseconds)
		case C.DPI_ORACLE_TYPE_INTERVAL_YM, C.DPI_NATIVE_TYPE_INTERVAL_YM:
			fmt.Printf("FLOAT\n")
			ym := C.dpiData_getIntervalYM(d)
			dest[i] = fmt.Sprintf("%dy%dm", ym.years, ym.months)
		case C.DPI_ORACLE_TYPE_CLOB, C.DPI_ORACLE_TYPE_NCLOB,
			C.DPI_ORACLE_TYPE_BLOB,
			C.DPI_ORACLE_TYPE_BFILE,
			C.DPI_NATIVE_TYPE_LOB:
			fmt.Printf("INTERVAL_YM\n")
			dest[i] = &Lob{
				Reader: &dpiLobReader{dpiLob: C.dpiData_getLOB(d)},
				IsClob: typ == C.DPI_ORACLE_TYPE_CLOB || typ == C.DPI_ORACLE_TYPE_NCLOB,
			}
		case C.DPI_ORACLE_TYPE_STMT, C.DPI_NATIVE_TYPE_STMT:
			fmt.Printf("STMT\n")
			st := &statement{dpiStmt: C.dpiData_getStmt(d)}
			var colCount C.uint32_t
			if C.dpiStmt_getNumQueryColumns(st.dpiStmt, &colCount) == C.DPI_FAILURE {
				return r.getError()
			}
			r2, err := st.openRows(int(colCount))
			if err != nil {
				return err
			}
			dest[i] = r2
		case C.DPI_ORACLE_TYPE_BOOLEAN, C.DPI_NATIVE_TYPE_BOOLEAN:
			fmt.Printf("BOOL\n")
			dest[i] = C.dpiData_getBool(d) == 1
			//case C.DPI_ORACLE_TYPE_OBJECT: //Default type used for named type columns in the database. Data is transferred to/from Oracle in Oracle's internal format.
		default:
			fmt.Printf("OTHER(%d)\n", typ)
			return errors.Errorf("unsupported column type %d", typ)
		}

		//fmt.Printf("dest[%d]=%#v\n", i, dest[i])
	}
	r.bufferRowIndex++
	r.fetched--

	return nil
}

// Lob is for reading/writing a LOB.
type Lob struct {
	io.Reader
	IsClob bool
}

var _ = io.Reader((*dpiLobReader)(nil))

type dpiLobReader struct {
	*conn
	dpiLob *C.dpiLob
	offset C.uint64_t
}

func (dlr *dpiLobReader) Read(p []byte) (int, error) {
	n := C.uint64_t(len(p))
	if C.dpiLob_readBytes(dlr.dpiLob, dlr.offset, n, (*C.char)(unsafe.Pointer(&p[0])), &n) == C.DPI_FAILURE {
		return 0, dlr.getError()
	}
	dlr.offset += n
	return int(n), nil
}
