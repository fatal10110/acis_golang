package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Config is the MariaDB connection setup read from a server's Database
// section (the URL, Login, and Password keys).
type Config struct {
	URL      string
	Login    string
	Password string
}

// Default pool sizing. The shipped config files carry no pool-size keys, so
// this mirrors the MariaDB connector's own pool defaults: 8 connections,
// idle ones dropped after 10 minutes.
const (
	defaultMaxOpenConns = 8
	defaultMaxIdleTime  = 10 * time.Minute
)

// Open builds a MariaDB connection pool from cfg. It does not dial the
// database; connections are established lazily on first use, same as
// database/sql's usual behavior. *sql.DB is already a connection pool, so no
// extra pooling layer is built on top of it.
func Open(cfg Config) (*sql.DB, error) {
	dsn, err := dataSourceName(cfg)
	if err != nil {
		return nil, err
	}

	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}

	pool.SetMaxOpenConns(defaultMaxOpenConns)
	pool.SetMaxIdleConns(defaultMaxOpenConns)
	pool.SetConnMaxIdleTime(defaultMaxIdleTime)
	return pool, nil
}

// dataSourceName converts a MariaDB or MySQL JDBC URL, as shipped in
// server.properties/loginserver.properties, into a
// go-sql-driver/mysql DSN.
func dataSourceName(cfg Config) (string, error) {
	rest, ok := strings.CutPrefix(cfg.URL, "jdbc:mariadb://")
	if !ok {
		rest, ok = strings.CutPrefix(cfg.URL, "jdbc:mysql://")
		if !ok {
			return "", fmt.Errorf("parse database url %q: missing MariaDB or MySQL JDBC prefix", cfg.URL)
		}
	}

	u, err := url.Parse("//" + rest)
	if err != nil {
		return "", fmt.Errorf("parse database url %q: %w", cfg.URL, err)
	}

	host := u.Host
	if host == "" {
		return "", fmt.Errorf("parse database url %q: missing host", cfg.URL)
	}
	if !strings.Contains(host, ":") {
		host += ":3306"
	}

	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("parse database url %q: missing database name", cfg.URL)
	}

	driverCfg := mysql.NewConfig()
	driverCfg.User = cfg.Login
	driverCfg.Passwd = cfg.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = host
	driverCfg.DBName = name

	for key, values := range u.Query() {
		if err := applyConnectorOption(driverCfg, key, values[len(values)-1]); err != nil {
			return "", fmt.Errorf("parse database url %q: %w", cfg.URL, err)
		}
	}

	return driverCfg.FormatDSN(), nil
}

// applyConnectorOption translates one MariaDB Connector/J URL option
// (as documented in driver.properties of the bundled connector) into the
// equivalent go-sql-driver/mysql setting. Connector/J options with no safe
// go-sql-driver equivalent are rejected rather than silently dropped or
// forwarded as a MySQL session variable (see aCis_gameserver
// ConnectionPool.setUrl and PR #278).
func applyConnectorOption(driverCfg *mysql.Config, key, value string) error {
	switch key {
	case "timezone":
		if value == "disabled" {
			return nil
		}
		loc, err := time.LoadLocation(value)
		if err != nil {
			return fmt.Errorf("db url option \"timezone\" value %q is not a known IANA zone: %w", value, err)
		}
		driverCfg.Loc = loc
		return nil

	case "sslMode":
		switch value {
		case "disable":
			driverCfg.TLSConfig = "false"
		case "trust":
			driverCfg.TLSConfig = "skip-verify"
		case "verify-full":
			driverCfg.TLSConfig = "true"
		default:
			return fmt.Errorf("db url option \"sslMode\" value %q is not supported (supported: disable, trust, verify-full)", value)
		}
		return nil

	case "connectTimeout":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("db url option \"connectTimeout\" value %q is not an integer: %w", value, err)
		}
		driverCfg.Timeout = time.Duration(ms) * time.Millisecond
		return nil

	case "socketTimeout":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("db url option \"socketTimeout\" value %q is not an integer: %w", value, err)
		}
		driverCfg.ReadTimeout = time.Duration(ms) * time.Millisecond
		driverCfg.WriteTimeout = time.Duration(ms) * time.Millisecond
		return nil

	case "allowMultiQueries":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("db url option \"allowMultiQueries\" value %q is not a boolean: %w", value, err)
		}
		driverCfg.MultiStatements = b
		return nil

	default:
		return fmt.Errorf("db url option %q has no supported go-sql-driver/mysql equivalent; remove it or configure the driver directly", key)
	}
}
