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
#cgo CFLAGS: -I./odpi/src -I./odpi/include
#cgo LDFLAGS: -Lodpi/lib -lodpic -ldl

#include "dpiImpl.h"

const int sizeof_dpiData = sizeof(void);
*/
import "C"
import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/pkg/errors"
)

// Option for NamedArgs
type Option uint8

// PlSQLArrays is to signal that the slices given in arguments of Exec to
// be left as is - the default is to treat them as arguments for ExecMany.
const PlSQLArrays = Option(1)

var _ = driver.Stmt((*statement)(nil))
var _ = driver.StmtQueryContext((*statement)(nil))
var _ = driver.StmtExecContext((*statement)(nil))
var _ = driver.NamedValueChecker((*statement)(nil))

const sizeofDpiData = C.sizeof_dpiData

type statement struct {
	sync.Mutex
	*conn
	dpiStmt     *C.dpiStmt
	query       string
	data        [][]C.dpiData
	vars        []*C.dpiVar
	PlSQLArrays bool
	arrLen      int
}

// Close closes the statement.
//
// As of Go 1.1, a Stmt will not be closed if it's in use
// by any queries.
func (st *statement) Close() error {
	st.Lock()
	defer st.Unlock()
	for _, v := range st.vars {
		C.dpiVar_release(v)
	}
	if C.dpiStmt_release(st.dpiStmt) == C.DPI_FAILURE {
		return st.getError()
	}
	st.data = nil
	st.vars = nil
	st.dpiStmt = nil
	return nil
}

// NumInput returns the number of placeholder parameters.
//
// If NumInput returns >= 0, the sql package will sanity check
// argument counts from callers and return errors to the caller
// before the statement's Exec or Query methods are called.
//
// NumInput may also return -1, if the driver doesn't know
// its number of placeholders. In that case, the sql package
// will not sanity check Exec or Query argument counts.
func (st *statement) NumInput() int {
	var colCount C.uint32_t
	if C.dpiStmt_execute(st.dpiStmt, C.DPI_MODE_EXEC_PARSE_ONLY, &colCount) == C.DPI_FAILURE {
		return -1
	}
	var cnt C.uint32_t
	if C.dpiStmt_getBindCount(st.dpiStmt, &cnt) == C.DPI_FAILURE {
		return -1
	}
	//fmt.Printf("%p.NumInput=%d\n", st, cnt)
	return int(cnt)
}

// Exec executes a query that doesn't return rows, such
// as an INSERT or UPDATE.
//
// Deprecated: Drivers should implement StmtExecContext instead (or additionally).
func (st *statement) Exec(args []driver.Value) (driver.Result, error) {
	nargs := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		nargs[i].Ordinal = i + 1
		nargs[i].Value = arg
	}
	return st.ExecContext(context.Background(), nargs)
}

// Query executes a query that may return rows, such as a
// SELECT.
//
// Deprecated: Drivers should implement StmtQueryContext instead (or additionally).
func (st *statement) Query(args []driver.Value) (driver.Rows, error) {
	nargs := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		nargs[i].Ordinal = i + 1
		nargs[i].Value = arg
	}
	return st.QueryContext(context.Background(), nargs)
}

// ExecContext executes a query that doesn't return rows, such as an INSERT or UPDATE.
//
// ExecContext must honor the context timeout and return when it is canceled.
func (st *statement) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	st.Lock()
	defer st.Unlock()

	// bind variables
	if err := st.bindVars(args); err != nil {
		return nil, err
	}

	// execute
	done := make(chan struct{}, 1)
	go func() {
		select {
		case <-ctx.Done():
			_ = st.Break()
		case <-done:
			return
		}
	}()

	mode := C.dpiExecMode(C.DPI_MODE_EXEC_DEFAULT)
	if !st.inTransaction {
		mode |= C.DPI_MODE_EXEC_COMMIT_ON_SUCCESS
	}
	var res C.int
	if !st.PlSQLArrays && st.arrLen > 0 {
		res = C.dpiStmt_executeMany(st.dpiStmt, mode, C.uint32_t(st.arrLen))
	} else {
		var colCount C.uint32_t
		res = C.dpiStmt_execute(st.dpiStmt, mode, &colCount)
	}
	done <- struct{}{}
	if res == C.DPI_FAILURE {
		return nil, errors.Wrapf(st.getError(), "dpiStmt_execute(mode=%d arrLen=%d)", mode, st.arrLen)
	}
	var count C.uint64_t
	if C.dpiStmt_getRowCount(st.dpiStmt, &count) == C.DPI_FAILURE {
		return nil, nil
	}
	return driver.RowsAffected(count), nil
}

// QueryContext executes a query that may return rows, such as a SELECT.
//
// QueryContext must honor the context timeout and return when it is canceled.
func (st *statement) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	st.Lock()
	defer st.Unlock()

	//fmt.Printf("QueryContext(%+v)\n", args)
	// bind variables
	if err := st.bindVars(args); err != nil {
		return nil, err
	}

	// execute
	done := make(chan struct{}, 1)
	go func() {
		select {
		case <-ctx.Done():
			_ = st.Break()
		case <-done:
			return
		}
	}()
	var colCount C.uint32_t
	res := C.dpiStmt_execute(st.dpiStmt, C.DPI_MODE_EXEC_DEFAULT, &colCount)
	done <- struct{}{}
	if res == C.DPI_FAILURE {
		return nil, errors.Wrapf(st.getError(), "dpiStmt_execute")
	}
	return st.openRows(int(colCount))
}

// bindVars binds the given args into new variables.
//
// FIXME(tgulacsi): handle sql.Out params and arrays as ExecuteMany OR PL/SQL arrays.
func (st *statement) bindVars(args []driver.NamedValue) error {
	var named bool
	if cap(st.vars) < len(args) {
		st.vars = make([]*C.dpiVar, len(args))
	} else {
		st.vars = st.vars[:len(args)]
	}
	if cap(st.data) < len(args) {
		st.data = make([][]C.dpiData, len(args))
	} else {
		st.data = st.data[:len(args)]
	}

	rArgs := make([]reflect.Value, len(args))
	minArrLen, maxArrLen := -1, -1
	for i, a := range args {
		rArgs[i] = reflect.ValueOf(a.Value)
		if _, isByteSlice := a.Value.([]byte); isByteSlice {
			continue
		}
		if rArgs[i].Kind() == reflect.Slice {
			n := rArgs[i].Len()
			if minArrLen == -1 || n < minArrLen {
				minArrLen = n
			}
			if maxArrLen == -1 || n > maxArrLen {
				maxArrLen = n
			}
		}
	}
	if maxArrLen > maxArraySize {
		return errors.Errorf("slice is bigger (%d) than the maximum (%d)", maxArrLen, maxArraySize)
	}
	if !st.PlSQLArrays && minArrLen != -1 && minArrLen != maxArrLen {
		return errors.Errorf("PlSQLArrays is not set, but has different lengthed slices (min=%d < %d=max)", minArrLen, maxArrLen)
	}

	st.arrLen = minArrLen
	doExecMany := !st.PlSQLArrays && st.arrLen > 0
	dataSliceLen := 1
	if doExecMany {
		dataSliceLen = st.arrLen
	}

	//fmt.Printf("bindVars %d\n", len(args))
	for i, a := range args {
		if !named {
			named = a.Name != ""
		}
		value := a.Value
		if out, ok := value.(sql.Out); ok {
			value = out.Dest
			if rv := reflect.ValueOf(value); rv.Kind() == reflect.Ptr {
				value = rv.Elem().Interface()
			}
		}

		var set dataSetter
		var typ C.dpiOracleTypeNum
		var natTyp C.dpiNativeTypeNum
		var bufSize int
		switch v := value.(type) {
		case Lob, []Lob:
			typ, natTyp = C.DPI_ORACLE_TYPE_BLOB, C.DPI_NATIVE_TYPE_LOB
			var isClob bool
			switch v := v.(type) {
			case Lob:
				isClob = v.IsClob
			case []Lob:
				isClob = len(v) > 0 && v[0].IsClob
			}
			if isClob {
				typ = C.DPI_ORACLE_TYPE_CLOB
			}
			set = st.dataSetLOB

		case int, []int:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_INT64
			set = dataSetNumber
		case int32, []int32:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_INT64
			set = dataSetNumber
		case int64, []int64:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_INT64
			set = dataSetNumber
		case uint, []uint:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_UINT64
			set = dataSetNumber
		case uint64, []uint64:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_UINT64
			set = dataSetNumber
		case float32, []float32:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_FLOAT
			set = dataSetNumber
		case float64, []float64:
			typ, natTyp = C.DPI_ORACLE_TYPE_NUMBER, C.DPI_NATIVE_TYPE_DOUBLE
			set = dataSetNumber
		case bool, []bool:
			typ, natTyp = C.DPI_ORACLE_TYPE_BOOLEAN, C.DPI_NATIVE_TYPE_BOOLEAN
			set = func(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error {
				b := C.int(0)
				if v.(bool) {
					b = 1
				}
				C.dpiData_setBool(data, b)
				return nil
			}

		case []byte, [][]byte:
			typ, natTyp = C.DPI_ORACLE_TYPE_RAW, C.DPI_NATIVE_TYPE_BYTES
			switch v := v.(type) {
			case []byte:
				bufSize = len(v)
			case [][]byte:
				for _, b := range v {
					if n := len(b); n > bufSize {
						bufSize = n
					}
				}
			}
			set = dataSetBytes

		case string, []string:
			typ, natTyp = C.DPI_ORACLE_TYPE_VARCHAR, C.DPI_NATIVE_TYPE_BYTES
			switch v := v.(type) {
			case string:
				bufSize = 4 * len(v)
			case []string:
				for _, s := range v {
					if n := 4 * len(s); n > bufSize {
						bufSize = n
					}
				}
			}
			set = dataSetBytes

		case time.Time, []time.Time:
			typ, natTyp = C.DPI_ORACLE_TYPE_TIMESTAMP_TZ, C.DPI_NATIVE_TYPE_TIMESTAMP
			set = func(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error {
				t := v.(time.Time)
				_, z := t.Zone()
				C.dpiData_setTimestamp(data,
					C.int16_t(t.Year()), C.uint8_t(t.Month()), C.uint8_t(t.Day()),
					C.uint8_t(t.Hour()), C.uint8_t(t.Minute()), C.uint8_t(t.Second()), C.uint32_t(t.Nanosecond()),
					C.int8_t(z/3600), C.int8_t((z%3600)/60),
				)
				return nil
			}

		default:
			return errors.Errorf("%d. arg: unknown type %T", i+1, value)
		}

		var err error
		if st.vars[i], st.data[i], err = st.newVar(
			st.PlSQLArrays, typ, natTyp, dataSliceLen, bufSize,
		); err != nil {
			return errors.WithMessage(err, fmt.Sprintf("%d", i))
		}

		dv, data := st.vars[i], st.data[i]
		if !doExecMany {
			if err := set(dv, 0, &data[0], a.Value); err != nil {
				return errors.Wrapf(err, "set(data[%d][%d], %#v (%T))", i, 0, a.Value, a.Value)
			}
		} else {
			//fmt.Println("n:", len(st.data[i]))
			for j := 0; j < dataSliceLen; j++ {
				//fmt.Printf("d[%d]=%p\n", j, st.data[i][j])
				if err := set(dv, j, &data[j], rArgs[i].Index(j).Interface()); err != nil {
					v := rArgs[i].Index(j).Interface()
					return errors.Wrapf(err, "set(data[%d][%d], %#v (%T))", i, j, v, v)
				}
			}
		}
		//fmt.Printf("data[%d]: %#v\n", i, st.data[i])
	}

	if !named {
		for i, v := range st.vars {
			//fmt.Printf("bindByPos(%d, %p, %p)\n", i+1, v, st.data[i])
			if C.dpiStmt_bindByPos(st.dpiStmt, C.uint32_t(i+1), v) == C.DPI_FAILURE {
				return st.getError()
			}
		}
	} else {
		for i, a := range args {
			name := a.Name
			if name == "" {
				name = strconv.Itoa(a.Ordinal)
			}
			//fmt.Printf("bindByName(%q)\n", name)
			cName := C.CString(name)
			res := C.dpiStmt_bindByName(st.dpiStmt, cName, C.uint32_t(len(name)), st.vars[i])
			C.free(unsafe.Pointer(cName))
			if res == C.DPI_FAILURE {
				return st.getError()
			}
		}
	}

	return nil
}

type dataSetter func(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error

func dataSetNumber(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error {
	switch x := v.(type) {
	case int:
		C.dpiData_setInt64(data, C.int64_t(x))
	case int32:
		C.dpiData_setInt64(data, C.int64_t(x))
	case int64:
		C.dpiData_setInt64(data, C.int64_t(x))

	case uint:
		C.dpiData_setUint64(data, C.uint64_t(x))
	case uint32:
		C.dpiData_setUint64(data, C.uint64_t(x))
	case uint64:
		C.dpiData_setUint64(data, C.uint64_t(x))

	case float32:
		C.dpiData_setFloat(data, C.float(x))
	case float64:
		C.dpiData_setDouble(data, C.double(x))

	default:
		return errors.Errorf("unknown number [%T] %#v", v, v)
	}

	//fmt.Printf("setInt64(%#v, %#v)\n", data, C.int64_t(int64(v.(int))))
	return nil
}
func dataSetBytes(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error {
	switch x := v.(type) {
	case []byte:
		C.dpiVar_setFromBytes(dv, C.uint32_t(pos), (*C.char)(unsafe.Pointer(&x[0])), C.uint32_t(len(x)))
	case string:
		b := []byte(x)
		C.dpiVar_setFromBytes(dv, C.uint32_t(pos), (*C.char)(unsafe.Pointer(&b[0])), C.uint32_t(len(x)))
	default:
		return errors.Errorf("awaited []byte/string, got %T (%#v)", v, v)
	}
	return nil
}

func (c *conn) dataSetLOB(dv *C.dpiVar, pos int, data *C.dpiData, v interface{}) error {
	L := v.(Lob)
	if v == nil || L.Reader == nil {
		data.isNull = 1
		return nil
	}
	typ := C.dpiOracleTypeNum(C.DPI_ORACLE_TYPE_BLOB)
	if L.IsClob {
		typ = C.DPI_ORACLE_TYPE_CLOB
	}
	var lob *C.dpiLob
	if C.dpiConn_newTempLob(c.dpiConn, typ, &lob) == C.DPI_FAILURE {
		return errors.Wrapf(c.getError(), "newTempLob(typ=%d)", typ)
	}
	var chunkSize C.uint32_t
	_ = C.dpiLob_getChunkSize(lob, &chunkSize)
	if chunkSize == 0 {
		chunkSize = 1 << 20
	}
	lw := &dpiLobWriter{dpiLob: lob, conn: c}
	_, err := io.CopyBuffer(lw, L, make([]byte, int(chunkSize)))
	//fmt.Printf("%p written %d with chunkSize=%d\n", lob, n, chunkSize)
	if closeErr := lw.Close(); closeErr != nil {
		if err == nil {
			err = closeErr
		}
		//fmt.Printf("close %p: %+v\n", lob, closeErr)
	}
	C.dpiVar_setFromLob(dv, C.uint32_t(pos), lob)
	return err
}

// CheckNamedValue is called before passing arguments to the driver
// and is called in place of any ColumnConverter. CheckNamedValue must do type
// validation and conversion as appropriate for the driver.
//
// If CheckNamedValue returns ErrRemoveArgument, the NamedValue will not be included
// in the final query arguments.
// This may be used to pass special options to the query itself.
//
// If ErrSkip is returned the column converter error checking path is used
// for the argument.
// Drivers may wish to return ErrSkip after they have exhausted their own special cases.
func (st *statement) CheckNamedValue(nv *driver.NamedValue) error {
	if nv == nil {
		return nil
	}
	if nv.Value == PlSQLArrays {
		st.PlSQLArrays = true
		return driver.ErrRemoveArgument
	}
	return nil
}

func (st *statement) openRows(colCount int) (*rows, error) {
	C.dpiStmt_setFetchArraySize(st.dpiStmt, fetchRowCount)

	r := rows{
		statement: st,
		columns:   make([]Column, colCount),
		vars:      make([]*C.dpiVar, colCount),
		data:      make([][]C.dpiData, colCount),
	}
	var info C.dpiQueryInfo
	for i := 0; i < colCount; i++ {
		if C.dpiStmt_getQueryInfo(st.dpiStmt, C.uint32_t(i+1), &info) == C.DPI_FAILURE {
			return nil, st.getError()
		}
		bufSize := int(info.clientSizeInBytes)
		//fmt.Println(typ, numTyp, info.precision, info.scale, info.clientSizeInBytes)
		switch info.defaultNativeTypeNum {
		case C.DPI_ORACLE_TYPE_NUMBER:
			info.defaultNativeTypeNum = C.DPI_NATIVE_TYPE_BYTES
		case C.DPI_ORACLE_TYPE_DATE:
			info.defaultNativeTypeNum = C.DPI_NATIVE_TYPE_TIMESTAMP
		}
		r.columns[i] = Column{
			Name:       C.GoStringN(info.name, C.int(info.nameLength)),
			OracleType: info.oracleTypeNum,
			NativeType: info.defaultNativeTypeNum,
			Size:       info.clientSizeInBytes,
			Precision:  info.precision,
			Scale:      info.scale,
			Nullable:   info.nullOk == 1,
			ObjectType: info.objectType,
		}
		switch info.oracleTypeNum {
		case C.DPI_ORACLE_TYPE_VARCHAR, C.DPI_ORACLE_TYPE_NVARCHAR, C.DPI_ORACLE_TYPE_CHAR, C.DPI_ORACLE_TYPE_NCHAR:
			bufSize *= 4
		}
		var err error
		//fmt.Printf("%d. %+v\n", i, r.columns[i])
		if r.vars[i], r.data[i], err = st.newVar(
			false, info.oracleTypeNum, info.defaultNativeTypeNum, fetchRowCount, bufSize,
		); err != nil {
			return nil, err
		}

		if C.dpiStmt_define(st.dpiStmt, C.uint32_t(i+1), r.vars[i]) == C.DPI_FAILURE {
			return nil, st.getError()
		}
	}
	if C.dpiStmt_addRef(st.dpiStmt) == C.DPI_FAILURE {
		return &r, st.getError()
	}
	return &r, nil
}

// Column holds the info from a column.
type Column struct {
	Name       string
	OracleType C.dpiOracleTypeNum
	NativeType C.dpiNativeTypeNum
	Size       C.uint32_t
	Precision  C.int16_t
	Scale      C.int8_t
	Nullable   bool
	ObjectType *C.dpiObjectType
}
