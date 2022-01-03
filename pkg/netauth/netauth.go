package netauth

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/netauth/netauth/internal/startup"
	"github.com/netauth/netauth/pkg/token"
	"github.com/netauth/netauth/pkg/netauth/cache"

	// The default token service is the jwt implementation, and
	// since its internal, the client needs to import it on behalf
	// of consumers.
	_ "github.com/netauth/netauth/pkg/token/jwt"

	// Since most applicatios don't need persistent token caching
	// the default is to use an in-memory store.  This is imported
	// here to make thee interface cleaner in the general case.
	_ "github.com/netauth/netauth/pkg/netauth/cache/memory"

	rpc "github.com/netauth/protocol/v2"
)

func init() {
	viper.SetDefault("core.port", 1729)
	viper.SetDefault("tls.certificate", "keys/tls.pem")
	viper.SetDefault("token.cache", "memory")
	viper.SetDefault("token.backend", "jwt-rsa")
}

// NewWithLog uses the specified logger to contruct a NetAuth client.
// Note that the log handler cannot be changed after setup, so the
// handler that is provided should have the correct name and be
// parented to the correct point on the log tree.
func NewWithLog(l hclog.Logger) (*Client, error) {
	if viper.GetString("core.conf") == "" {
		viper.Set("core.conf", filepath.Dir(viper.ConfigFileUsed()))
		l.Debug("Config relative load path set", "path", viper.GetString("core.conf"))
	}

	conn, err := connect(false)
	if err != nil {
		return nil, err
	}

	cache, err := cache.NewTokenCache(viper.GetString("token.cache"))
	if err != nil {
		return nil, err
	}

	token.SetParentLogger(l)

	// Logging and config are available, run deferred startup
	// hooks.
	startup.DoCallbacks()

	ts, err := token.New(viper.GetString("token.backend"))
	if err != nil {
		l.Warn("Token service initialization error", "error", err)
	}

	hn, err := os.Hostname()
	if err != nil {
		viper.SetDefault("client.ID", "BOGUS_CLIENT")
	} else {
		viper.SetDefault("client.ID", hn)
	}

	return &Client{
		TokenCache: cache,
		Service:    ts,
		rpc:        rpc.NewNetAuth2Client(conn),
		log:        l,
		clientName: viper.GetString("client.ID"),
	}, nil
}

// New returns a client initialized, connected, and ready to use.
func New() (*Client, error) {
	return NewWithLog(hclog.L().Named("cli"))
}

func connect(writable bool) (*grpc.ClientConn, error) {
	addr := viper.GetString("core.server")

	// This has to happen here since it needs to happen after
	// everything else is already parsed.
	if viper.GetString("core.master") == "" {
		viper.Set("core.master", viper.GetString("core.server"))
	}

	if writable {
		addr = viper.GetString("core.master")
	}

	var opts []grpc.DialOption
	if viper.GetBool("tls.pwn_me") {
		opts = []grpc.DialOption{grpc.WithInsecure()}
	} else {
		// If this is a relative path its relative to the home
		// directory.
		certPath := viper.GetString("tls.certificate")
		if !filepath.IsAbs(certPath) {
			certPath = filepath.Join(viper.GetString("core.conf"), certPath)
		}

		creds, err := credentials.NewClientTLSFromFile(certPath, "")
		if err != nil {
			return nil, err
		}
		opts = []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	}
	return grpc.Dial(
		fmt.Sprintf("%s:%d", addr, viper.GetInt("core.port")),
		opts...,
	)
}

// SetServiceName sets the self identified service this client serves.
// This should be set prior to making any calls to the server.
func (c *Client) SetServiceName(s string) {
	c.serviceName = s
}

func (c *Client) makeWritable() error {
	// If the master server is the one that we would already be
	// connected to, then just return.  Also return if we are
	// already not readonly.
	if viper.GetString("core.server") == viper.GetString("core.master") || c.writeable {
		return nil
	}

	conn, err := connect(true)
	if err != nil {
		return err
	}
	c.rpc = rpc.NewNetAuth2Client(conn)
	c.writeable = true
	return nil
}
