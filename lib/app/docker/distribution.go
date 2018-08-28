package docker

import (
	"context"
	"io/ioutil"
	"log/syslog"
	"net"
	"net/http"
	"os"

	"github.com/docker/distribution/configuration"
	registrycontext "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/handlers"
	"github.com/docker/distribution/registry/listener"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	"github.com/docker/distribution/version"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	sysloghook "github.com/sirupsen/logrus/hooks/syslog"
)

// NewRegistry creates a new registry instance from the specified configuration.
func NewRegistry(config *configuration.Configuration) (*Registry, error) {
	ctx, cancel := defaultContext()
	app := handlers.NewApp(ctx, config)
	app.RegisterHealthChecks()
	handler := alive("/", app)

	server := &http.Server{
		Handler: handler,
	}

	return &Registry{
		app:    app,
		config: config,
		server: server,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Starts starts the registry server and returns when the server
// has actually started listening.
func (r *Registry) Start() error {
	initC := make(chan error, 1)
	go r.listenAndServe(initC)
	return trace.Wrap(<-initC)
}

// listenAndServe runs the registry's HTTP server.
func (r *Registry) listenAndServe(initC chan error) error {
	config := r.config

	listener, err := listener.NewListener(config.HTTP.Net, config.HTTP.Addr)
	if err != nil {
		initC <- err
		close(initC)
		return trace.Wrap(err)
	}

	r.addr = listener.Addr()
	registrycontext.GetLogger(r.app).Infof("listening on %v", r.addr)
	close(initC)

	go func() {
		<-r.ctx.Done()
		listener.Close()
	}()

	return r.server.Serve(listener)
}

// Addr returns the address this registry listens on.
func (r *Registry) Addr() string {
	return r.addr.String()
}

// Close shuts down the registry.
func (r *Registry) Close() error {
	r.cancel()
	return nil
}

// A Registry represents a complete instance of the registry.
type Registry struct {
	config *configuration.Configuration
	app    *handlers.App
	server *http.Server
	ctx    context.Context
	cancel context.CancelFunc
	addr   net.Addr
}

// alive simply wraps the handler with a route that always returns an http 200
// response when the path is matched. If the path is not matched, the request
// is passed to the provided handler. There is no guarantee of anything but
// that the server is up. Wrap with other handlers (such as health.Handler)
// for greater affect.
func alive(path string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// BasicConfiguration creates a configuration object for running
// a local registry server on the specified address addr and using rootdir
// as a root directory for a filesystem driver
func BasicConfiguration(addr, rootdir string) *configuration.Configuration {
	config := &configuration.Configuration{
		Version: "0.1",
		Storage: configuration.Storage{
			"cache":      configuration.Parameters{"blobdescriptor": "inmemory"},
			"filesystem": configuration.Parameters{"rootdirectory": rootdir},
		},
	}
	config.HTTP.Addr = addr
	config.HTTP.Headers = http.Header{
		"X-Content-Type-Options": []string{"nosniff"},
	}
	return config
}

func defaultContext() (context.Context, context.CancelFunc) {
	ctx := registrycontext.WithVersion(context.Background(), version.Version)
	ctx = registrycontext.WithLogger(ctx, newLogger())
	return context.WithCancel(ctx)
}

func newLogger() registrycontext.Logger {
	logger := log.New()
	logger.SetLevel(log.WarnLevel)
	logger.SetHooks(make(log.LevelHooks))
	hook, err := sysloghook.NewSyslogHook("", "", syslog.LOG_WARNING, "")
	if err != nil {
		logger.Out = os.Stderr
	} else {
		logger.AddHook(hook)
		logger.Out = ioutil.Discard
	}
	// distribution expects an instance of log.Entry
	return logger.WithField("source", "local-docker-registry")
}
