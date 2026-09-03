package mysql

import (
	"errors"

	mysqldrv "github.com/go-sql-driver/mysql"
)

// mysqlDupKey is MySQL's error number for a duplicate key violation
// (ER_DUP_ENTRY).
const mysqlDupKey uint16 = 1062

// isDuplicateErr reports whether err is a MySQL duplicate-key violation
// (unique/primary index collision).
func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	var me *mysqldrv.MySQLError
	if !errors.As(err, &me) {
		return false
	}
	return me.Number == mysqlDupKey
}
