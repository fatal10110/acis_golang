package db

import (
	"database/sql"
	"fmt"
	"net/url"
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

// dataSourceName converts a "jdbc:mariadb://host[:port]/db[?params]" URL, as
// shipped in server.properties/loginserver.properties, into a
// go-sql-driver/mysql DSN.
func dataSourceName(cfg Config) (string, error) {
	const prefix = "jdbc:mariadb://"
	rest, ok := strings.CutPrefix(cfg.URL, prefix)
	if !ok {
		return "", fmt.Errorf("parse database url %q: missing %s prefix", cfg.URL, prefix)
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
	if query := u.Query(); len(query) > 0 {
		driverCfg.Params = make(map[string]string, len(query))
		for key, values := range query {
			driverCfg.Params[key] = values[0]
		}
	}
	return driverCfg.FormatDSN(), nil
}
