package config

import "github.com/fatal10110/acis_golang/internal/commons/db"

// Database reads the URL, Login and Password keys every server and tool
// shares, with a pool of maxConnections (zero keeps the default size).
func Database(props *Properties, maxConnections int) db.Config {
	return db.Config{
		URL:            props.String("URL", "jdbc:mariadb://localhost/acis"),
		Login:          props.String("Login", "root"),
		Password:       props.String("Password", ""),
		MaxConnections: maxConnections,
	}
}
