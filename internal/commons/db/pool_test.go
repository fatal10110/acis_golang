package db

import "testing"

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
			name: "query params passed through",
			cfg:  Config{URL: "jdbc:mariadb://localhost/acis?useSSL=true", Login: "root", Password: ""},
			want: "root@tcp(localhost:3306)/acis?useSSL=true",
		},
		{
			name:    "missing scheme",
			cfg:     Config{URL: "jdbc:mysql://localhost/acis", Login: "root"},
			wantErr: true,
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
