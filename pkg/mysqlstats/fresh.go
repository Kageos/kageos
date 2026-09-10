// Package mysqlstats isolates uncached capacity reads from application sessions.
package mysqlstats

import (
	"context"
	"database/sql/driver"
	"fmt"

	"gorm.io/gorm"
)

// Fresh reads engine statistics on one pinned connection. The connection is
// discarded afterward so its session settings cannot leak into the shared pool.
// This bypasses INFORMATION_SCHEMA caching; engine allocation is still estimated.
func Fresh(ctx context.Context, db *gorm.DB, read func(*gorm.DB) error) error {
	if db.Dialector.Name() != "mysql" {
		return read(db.WithContext(ctx))
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer conn.Raw(func(any) error { return driver.ErrBadConn })
	session := db.Session(&gorm.Session{NewDB: true, Context: ctx})
	session.Statement.ConnPool = conn
	if err := session.Exec("SET SESSION information_schema_stats_expiry = 0").Error; err != nil {
		return fmt.Errorf("disable capacity statistics cache: %w", err)
	}
	return read(session)
}
