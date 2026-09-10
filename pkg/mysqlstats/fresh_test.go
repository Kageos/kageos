package mysqlstats

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"io"
	"strings"
	"testing"
)

type testDriver struct {
	failSet     bool
	connections []*testConn
}

func (d *testDriver) Open(string) (driver.Conn, error) {
	c := &testConn{owner: d}
	d.connections = append(d.connections, c)
	return c, nil
}

type testConn struct {
	owner         *testDriver
	fresh, closed bool
}

func (c *testConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }
func (c *testConn) Close() error                        { c.closed = true; return nil }
func (c *testConn) Begin() (driver.Tx, error)           { return nil, errors.New("unexpected transaction") }
func (c *testConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.HasPrefix(q, "SET SESSION") {
		if c.owner.failSet {
			return nil, errors.New("set failed")
		}
		c.fresh = true
	}
	return driver.RowsAffected(0), nil
}
func (c *testConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	v := int64(0)
	if c.fresh {
		v = 1
	}
	return &testRows{value: v}, nil
}

type testRows struct {
	value int64
	done  bool
}

func (r *testRows) Columns() []string { return []string{"fresh"} }
func (r *testRows) Close() error      { return nil }
func (r *testRows) Next(out []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	out[0] = r.value
	return nil
}
func TestFreshPinsSessionAndDoesNotLeakSettings(t *testing.T) {
	for _, scenario := range []string{"success", "read_failure", "set_failure"} {
		t.Run(scenario, func(t *testing.T) {
			d := &testDriver{failSet: scenario == "set_failure"}
			sql.Register(t.Name(), d)
			pool, err := sql.Open(t.Name(), "")
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			pool.SetMaxOpenConns(1)
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			err = Fresh(context.Background(), db, func(session *gorm.DB) error {
				called = true
				var fresh int
				if err := session.Raw("SELECT fresh").Scan(&fresh).Error; err != nil {
					return err
				}
				if fresh != 1 {
					t.Fatal("read did not use the uncached session")
				}
				if scenario == "read_failure" {
					return errors.New("read failed")
				}
				return nil
			})
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result: %v", err)
			}
			if called == (scenario == "set_failure") {
				t.Fatal("read must not run after setting failure")
			}
			if len(d.connections) != 1 || !d.connections[0].closed {
				t.Fatal("dedicated connection was not discarded")
			}
			var fresh int
			if err := pool.QueryRow("SELECT fresh").Scan(&fresh); err != nil {
				t.Fatal(err)
			}
			if fresh != 0 {
				t.Fatal("session settings leaked to a later borrower")
			}
		})
	}
}
