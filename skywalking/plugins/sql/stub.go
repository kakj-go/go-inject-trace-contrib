//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects the template files (sql.go for database/sql, mysql.go for
// github.com/go-sql-driver/mysql, pgxstdlib.go for
// github.com/jackc/pgx/v5/stdlib), which are parsed as injection templates
// and checked against the real target packages instead.
package sql
