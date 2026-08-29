package db

import (
	"testing"
)

// ---- from pool_test.go ----
func TestDataSourceName(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    string
		wantErr bool
	}{
		{
			name: "shipped default",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis",
		},
		{
			name: "with password",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis", Login: "root", Password: "secret"},
			want: "root:secret@tcp(localhost:3306)/acis",
		},
		{
			name: "explicit port kept as-is",
			cfg:  Config{URL: "jdbc:mariadb://db.internal:3307/acis", Login: "acis", Password: ""},
			want: "acis@tcp(db.internal:3307)/acis",
		},
		{
			name:    "unsupported connector option is rejected, not forwarded as a server variable",
			cfg:     Config{URL: "jdbc:mariadb://localhost/acis?serverTimezone=UTC", Login: "root", Password: ""},
			wantErr: true,
		},
		{
			name: "timezone option sets client/server session location",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?timezone=America/New_York", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?loc=America%2FNew_York",
		},
		{
			name: "timezone disabled is a no-op",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?timezone=disabled", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis",
		},
		{
			name:    "timezone with an unknown zone is rejected",
			cfg:     Config{URL: "jdbc:mariadb://localhost/acis?timezone=Nowhere/Place", Login: "root", Password: ""},
			wantErr: true,
		},
		{
			name: "sslMode disable maps to tls=false",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?sslMode=disable", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?tls=false",
		},
		{
			name: "sslMode trust maps to tls=skip-verify",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?sslMode=trust", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?tls=skip-verify",
		},
		{
			name: "sslMode verify-full maps to tls=true",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?sslMode=verify-full", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?tls=true",
		},
		{
			name:    "sslMode verify-ca has no go-sql-driver equivalent and is rejected",
			cfg:     Config{URL: "jdbc:mariadb://localhost/acis?sslMode=verify-ca", Login: "root", Password: ""},
			wantErr: true,
		},
		{
			name: "connectTimeout maps to dial timeout",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?connectTimeout=5000", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?timeout=5s",
		},
		{
			name: "socketTimeout maps to read and write timeout",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?socketTimeout=2000", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?readTimeout=2s&writeTimeout=2s",
		},
		{
			name: "allowMultiQueries maps to multiStatements",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?allowMultiQueries=true", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?multiStatements=true",
		},
		{
			name:    "unknown connector option is rejected",
			cfg:     Config{URL: "jdbc:mariadb://localhost/acis?poolName=acis-pool", Login: "root", Password: ""},
			wantErr: true,
		},
		{
			name: "mysql scheme",
			cfg:  Config{URL: "jdbc:mysql://localhost/acis", Login: "root"},
			want: "root@tcp(localhost:3306)/acis",
		},
		{
			name:    "missing host",
			cfg:     Config{URL: "jdbc:mariadb:///acis", Login: "root"},
			wantErr: true,
		},
		{
			name:    "missing database name",
			cfg:     Config{URL: "jdbc:mariadb://localhost", Login: "root"},
			wantErr: true,
		},
		{
			name:    "missing database name with trailing slash",
			cfg:     Config{URL: "jdbc:mariadb://localhost/", Login: "root"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dataSourceName(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("dataSourceName(%+v) = %q, want error", tt.cfg, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("dataSourceName(%+v) unexpected error: %v", tt.cfg, err)
			}
			if got != tt.want {
				t.Errorf("dataSourceName(%+v) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestOpenConfiguresPoolWithoutDialing(t *testing.T) {
	pool, err := Open(Config{URL: "jdbc:mariadb://localhost/acis", Login: "root"})
	if err != nil {
		t.Fatalf("Open() unexpected error: %v", err)
	}
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxOpenConnections != defaultMaxOpenConns {
		t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, defaultMaxOpenConns)
	}
	if stats.OpenConnections != 0 {
		t.Errorf("OpenConnections = %d, want 0 (Open must not dial)", stats.OpenConnections)
	}
}

func TestOpenRejectsMalformedURL(t *testing.T) {
	if _, err := Open(Config{URL: "not-a-jdbc-url"}); err == nil {
		t.Fatal("Open() with malformed url: want error, got nil")
	}
}
