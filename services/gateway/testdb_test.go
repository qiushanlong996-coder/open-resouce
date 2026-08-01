package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 集成测试的数据库入口。
//
// 集成测试会真实写入并删除数据。此前测试直接连生产库，导致生产库里累积了
// 25 个测试账号和 2 个假项目，后者还带 published 状态出现在公开目录里。
// 根子上不该让测试碰生产数据，因此这里做两件事：
//   1. 统一环境变量，只认 MYSQL_TEST_DATABASE_URL（旧名 TEST_MYSQL_DSN 仍兼容）。
//   2. 强制库名以 _test 结尾，把「误指生产库」变成不可能，而不是靠人记得别写错。

const testDatabaseSuffix = "_test"

// requireTestDatabase 返回集成测试专用连接；未配置时跳过，指向非测试库时直接失败。
func requireTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := testDatabaseDSN()
	if dsn == "" {
		t.Skip("MYSQL_TEST_DATABASE_URL is not configured")
	}
	name, err := testDatabaseName(dsn)
	if err != nil {
		t.Fatalf("解析测试数据库 DSN 失败：%v", err)
	}
	// 这道校验是防线本身，不能因为“本地图方便”而放宽。
	if !strings.HasSuffix(name, testDatabaseSuffix) {
		t.Fatalf("拒绝在非测试库上运行集成测试：库名 %q 必须以 %q 结尾。"+
			"集成测试会真实增删数据，指向生产库会污染线上内容。", name, testDatabaseSuffix)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := openMySQLDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	// 关闭必须通过 t.Cleanup 注册：defer 早于 t.Cleanup 执行，
	// 会让后面注册的清理语句在连接关闭后静默失败。
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return database
}

// testDatabaseDSN 读取测试库 DSN。TEST_MYSQL_DSN 是旧名，保留兼容。
func testDatabaseDSN() string {
	if dsn := strings.TrimSpace(os.Getenv("MYSQL_TEST_DATABASE_URL")); dsn != "" {
		return dsn
	}
	return strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
}

// testDatabaseName 从 DSN 中取出库名。
// 本项目的 DATABASE_URL 是 `mysql://user:pass@host:port/db` 这种 URL 形式，
// 而 mysql.ParseDSN 只认 go-sql-driver 原生格式，所以先用项目自己的
// mysqlDSN 做一次归一化，两种写法都能正确取到库名。
func testDatabaseName(dsn string) (string, error) {
	normalized, err := mysqlDSN(dsn)
	if err != nil {
		// 不是 URL 形式时当作原生 DSN 直接解析。
		normalized = dsn
	}
	config, err := mysql.ParseDSN(normalized)
	if err != nil {
		return "", err
	}
	return config.DBName, nil
}

// TestTestDatabaseGuardRejectsProductionName 这道校验是防止污染生产库的唯一防线，
// 必须有测试守住它：库名不以 _test 结尾时一律视为非测试库。
func TestTestDatabaseGuardRejectsProductionName(t *testing.T) {
	for name, testCase := range map[string]struct {
		dsn      string
		database string
		allowed  bool
	}{
		// 两种 DSN 写法都要能正确取到库名。
		"URL 形式生产库": {"mysql://u:p@host:3306/open_resouce", "open_resouce", false},
		"URL 形式测试库": {"mysql://u:p@host:3306/open_resouce_test", "open_resouce_test", true},
		"原生格式生产库":   {"root:pw@tcp(host:3306)/open_resouce?parseTime=true", "open_resouce", false},
		"原生格式测试库":   {"root:pw@tcp(host:3306)/open_resouce_test?parseTime=true", "open_resouce_test", true},
		"仅前缀相似":     {"mysql://u:p@host:3306/test_open_resouce", "test_open_resouce", false},
		"库名含 test":  {"mysql://u:p@host:3306/my_testing_db", "my_testing_db", false},
		"另一个测试库":    {"mysql://u:p@host:3306/gateway_test", "gateway_test", true},
	} {
		parsed, err := testDatabaseName(testCase.dsn)
		if err != nil {
			t.Fatalf("%s: parse DSN: %v", name, err)
		}
		if parsed != testCase.database {
			t.Fatalf("%s: database name = %q, want %q", name, parsed, testCase.database)
		}
		allowed := strings.HasSuffix(parsed, testDatabaseSuffix)
		if allowed != testCase.allowed {
			t.Fatalf("%s: allowed = %v, want %v (库名 %q)", name, allowed, testCase.allowed, parsed)
		}
	}
}

// TestTestDatabaseDSNPrefersNewName 统一环境变量时保留旧名兼容。
func TestTestDatabaseDSNPrefersNewName(t *testing.T) {
	t.Setenv("MYSQL_TEST_DATABASE_URL", "new")
	t.Setenv("TEST_MYSQL_DSN", "old")
	if dsn := testDatabaseDSN(); dsn != "new" {
		t.Fatalf("dsn = %q, want new", dsn)
	}
	t.Setenv("MYSQL_TEST_DATABASE_URL", "")
	if dsn := testDatabaseDSN(); dsn != "old" {
		t.Fatalf("fallback dsn = %q, want old", dsn)
	}
}
