package db

import "github.com/fatal10110/acis_golang/internal/config"

// ConfigFromProperties reads the URL, Login and Password keys every server
// and tool shares, with a pool of maxConnections (zero keeps the default
// size).
func ConfigFromProperties(props *config.Properties, maxConnections int) Config {
	return Config{
		URL:            props.String("URL", "jdbc:mariadb://localhost/acis"),
		Login:          props.String("Login", "root"),
		Password:       props.String("Password", ""),
		MaxConnections: maxConnections,
	}
}
