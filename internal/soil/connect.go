package soil

import (
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

// Connect dials NATS with the NimsForest connection conventions:
// a connection name, endless reconnects, and credentials from the
// NATS_CREDENTIALS environment variable when it is set. Without
// credentials the connection is anonymous, which is correct on local
// forests that run without auth.
func Connect(url, name string) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name(name),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Printf("soil: nats disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("soil: nats reconnected to %s", nc.ConnectedUrl())
		}),
	}
	if creds := os.Getenv("NATS_CREDENTIALS"); creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	return nats.Connect(url, opts...)
}
