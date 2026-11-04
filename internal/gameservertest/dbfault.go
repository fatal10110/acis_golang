package gameservertest

import "github.com/fatal10110/acis_golang/internal/dbtest"

// WithDBFault routes every store of the server through a handle fault takes
// away while armed, so a suite can put the server on the path a lost
// database takes without stopping the shared instance. Seeds, Server.DB and
// the login acceptor keep the unfaulted handle, so the suite still writes
// and asserts rows while the server's database is down.
func WithDBFault(fault *dbtest.Fault) Option {
	return func(o *options) { o.dbFault = fault }
}
