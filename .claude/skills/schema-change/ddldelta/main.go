/**
*
* Licensed to the Apache Software Foundation (ASF) under one
* or more contributor license agreements. See the NOTICE file
* distributed with this work for additional information
* regarding copyright ownership. The ASF licenses this file
* to you under the Apache License, Version 2.0 (the
* "License"); you may not use this file except in compliance
* with the License. You may obtain a copy of the License at
*
* http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing,
* software distributed under the License is distributed on an
* "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
* KIND, either express or implied. See the License for the
* specific language governing permissions and limitations
* under the License.
 */

// Command ddldelta prints the DDL AutoMigrate would run on top of every committed
// migration — i.e. the body of the next migration file.
//
// It applies internal/db/migrations to a throwaway schema, runs AutoMigrate over the
// current entities with SQL logging on, prints the schema-changing statements, and
// drops the schema. It lives under .claude/ so ./... never builds it.
//
//	AIRAVATA_TEST_POSTGRES_DSN='postgres://airavata:123456@localhost:15432/airavata?sslmode=disable' \
//	  go run ./.claude/skills/schema-change/ddldelta
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/apache/airavata/internal/db"
)

var ddl = regexp.MustCompile(`(?i)^\s*(CREATE|ALTER|DROP|COMMENT)\s`)

// capture records every schema-changing statement GORM executes.
type capture struct {
	logger.Interface
	stmts []string
}

func (c *capture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if err == nil && ddl.MatchString(sql) {
		c.stmts = append(c.stmts, sql+";")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ddldelta:", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("AIRAVATA_TEST_POSTGRES_DSN")
	if dsn == "" {
		return fmt.Errorf("set AIRAVATA_TEST_POSTGRES_DSN (see CLAUDE.md)")
	}
	schema := fmt.Sprintf("ddldelta_%d", os.Getpid())
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	admin, err := db.Open(db.DefaultConfig(dsn), silent)
	if err != nil {
		return err
	}
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		return err
	}
	defer admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")

	// search_path goes in the DSN, not a SET, so it holds on every pooled connection.
	scoped, err := withSearchPath(dsn, schema)
	if err != nil {
		return err
	}
	gdb, err := db.Open(db.DefaultConfig(scoped), silent)
	if err != nil {
		return err
	}
	if _, err := db.NewMigrator(gdb, db.Migrations()).Up(context.Background()); err != nil {
		return fmt.Errorf("apply committed migrations: %w", err)
	}

	c := &capture{Interface: logger.Default.LogMode(logger.Silent)}
	if err := db.AutoMigrate(gdb.Session(&gorm.Session{Logger: c})); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	if len(c.stmts) == 0 {
		fmt.Fprintln(os.Stderr, "no schema changes: committed migrations already match the entities")
		return nil
	}
	fmt.Println(strings.Join(c.stmts, "\n\n"))
	return nil
}

func withSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DSN: %w", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
