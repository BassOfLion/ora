//-----------------------------------------------------------------------------
// Copyright (c) 2016, 2017 Oracle and/or its affiliates.  All rights reserved.
// This program is free software: you can modify it and/or redistribute it
// under the terms of:
//
// (i)  the Universal Permissive License v 1.0 or at your option, any
//      later version (http://oss.oracle.com/licenses/upl); and/or
//
// (ii) the Apache License v 2.0. (http://www.apache.org/licenses/LICENSE-2.0)
//-----------------------------------------------------------------------------

//-----------------------------------------------------------------------------
// TestInsert.c
//   Tests simple insert of numbers and strings.
//-----------------------------------------------------------------------------

#include "Test.h"
#define DELETE_TEXT         "delete from TestTempTable"
#define INSERT_TEXT         "insert into TestTempTable values (:1, :2)"

//-----------------------------------------------------------------------------
// main()
//-----------------------------------------------------------------------------
int main(int argc, char **argv)
{
    dpiData intColValue, stringColValue;
    uint32_t numQueryColumns;
    uint64_t rowCount;
    dpiStmt *stmt;
    dpiConn *conn;

    // connect to database and create statement
    conn = GetConnection(0, NULL);
    if (!conn)
        return -1;
    if (dpiConn_prepareStmt(conn, 0, DELETE_TEXT, strlen(DELETE_TEXT), NULL, 0,
            &stmt) < 0)
        return ShowError();

    // perform delete
    if (dpiStmt_execute(stmt, 0, &numQueryColumns) < 0)
        return ShowError();
    if (dpiStmt_getRowCount(stmt, &rowCount) < 0)
        return ShowError();
    printf("%" PRIu64 " rows deleted.\n", rowCount);
    dpiStmt_release(stmt);

    // prepare insert statement for execution
    if (dpiConn_prepareStmt(conn, 0, INSERT_TEXT, strlen(INSERT_TEXT), NULL, 0,
            &stmt) < 0)
        return ShowError();
    intColValue.isNull = 0;
    stringColValue.isNull = 0;

    // create first row
    intColValue.value.asInt64 = 1;
    if (dpiStmt_bindValueByPos(stmt, 1, DPI_NATIVE_TYPE_INT64,
            &intColValue) < 0)
        return ShowError();
    stringColValue.value.asBytes.ptr = "TEST 1";
    stringColValue.value.asBytes.length = strlen("TEST 1");
    if (dpiStmt_bindValueByPos(stmt, 2, DPI_NATIVE_TYPE_BYTES,
            &stringColValue) < 0)
        return ShowError();
    if (dpiStmt_execute(stmt, 0, &numQueryColumns) < 0)
        return ShowError();
    if (dpiStmt_getRowCount(stmt, &rowCount) < 0)
        return ShowError();
    printf("%" PRIu64 " rows inserted.\n", rowCount);

    // create second row
    intColValue.value.asInt64 = 2;
    if (dpiStmt_bindValueByPos(stmt, 1, DPI_NATIVE_TYPE_INT64,
            &intColValue) < 0)
        return ShowError();
    stringColValue.value.asBytes.ptr = "TEST #2";
    stringColValue.value.asBytes.length = strlen("TEST #2");
    if (dpiStmt_bindValueByPos(stmt, 2, DPI_NATIVE_TYPE_BYTES,
            &stringColValue) < 0)
        return ShowError();
    if (dpiStmt_execute(stmt, 0, &numQueryColumns) < 0)
        return ShowError();
    if (dpiStmt_getRowCount(stmt, &rowCount) < 0)
        return ShowError();
    printf("%" PRIu64 " rows inserted.\n", rowCount);

    // commit changes
    if (dpiConn_commit(conn) < 0)
        return ShowError();

    // clean up
    dpiStmt_release(stmt);
    dpiConn_release(conn);

    printf("Done.\n");
    return 0;
}

