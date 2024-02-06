package server

import (
	"cdr.dev/slog"
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"gigo-ws/coder/agent/agent/lsp"
	utils2 "gigo-ws/coder/agent/agent/server/utils"
	"github.com/go-chi/chi"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-playground/validator/v10"
	"github.com/sourcegraph/conc"
)

type CtxKey string

const (
	CtxKeyIPAddress CtxKey = "ip-address"

	DefaultErrorMessage = "internal server error"
)

type HttpApiParams struct {
	// NodeID
	//
	//  The id of the node that is running the api.
	NodeID int64

	// Snowflake
	//
	//  The snowflake node to use for generating ids.
	Snowflake *snowflake.Node

	// Port
	//
	//  The port to listen on.
	Port uint16

	// Host
	//
	//  The host to listen on.
	Host string

	// Logger
	//
	//  The logger to use for logging http requests and core function calls.
	Logger slog.Logger

	// Secret
	//
	//  Workspace secret that should be used to authenticate requests.
	Secret string
}

// HttpApi
//
//	The main http api server for the application.
type HttpApi struct {
	HttpApiParams
	wg                *conc.WaitGroup
	listener          net.Listener
	router            *chi.Mux
	validator         *validator.Validate
	activeConnections *atomic.Int64
	server            *http.Server
	lsp               *atomic.Pointer[lsp.LspServer]
}

// NewHttpApi
//
//	Creates a new http api server.
func NewHttpApi(params HttpApiParams) (*HttpApi, error) {
	// create a new listener
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", params.Host, params.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %v", err)
	}

	// create a new router
	router := chi.NewRouter()

	// create a new external API
	externalAPI := &HttpApi{
		HttpApiParams:     params,
		wg:                conc.NewWaitGroup(),
		listener:          listener,
		router:            router,
		validator:         validator.New(),
		activeConnections: &atomic.Int64{},
		lsp:               &atomic.Pointer[lsp.LspServer]{},
	}

	// link global middleware
	externalAPI.router.Use(
		// panic catcher
		middleware.Recoverer,
		// configure CORS handler
		cors.Handler(cors.Options{
			// TODO: tighten this up for production
			// AllowedOrigins: "*",
			AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
			AllowedMethods:   []string{"GET"},
			AllowedHeaders:   []string{"*"},
			ExposedHeaders:   []string{"Content-Disposition"},
			AllowCredentials: true,
			MaxAge:           300, // Maximum value not ignored by any of major browsers
		}),
		// init middleware
		externalAPI.initRequest,
	)

	// link api to router
	externalAPI.linkApi()

	// start a goroutine to log active connections
	go func() {
		for {
			// params.Logger.Info("active connections", zap.Int64("count", externalAPI.activeConnections.Load()))
			time.Sleep(5 * time.Second)
		}
	}()

	return externalAPI, nil
}

// Start
//
//	Starts active listening of the external api server on the
//	configured address. The listening is bound to the passed
//	context.
func (a *HttpApi) Start(ctx context.Context) error {
	// create http server to serve the external api
	server := http.Server{
		ErrorLog: log.New(io.Discard, "", 0),
		Handler:  a.router,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	a.server = &server

	// launch server on the external api listener
	return server.Serve(a.listener)
}

func (a *HttpApi) Shutdown(ctx context.Context) error {
	err := a.server.Shutdown(ctx)
	if l := a.lsp.Load(); l != nil {
		l.Close()
	}
	return err
}

func (a *HttpApi) linkApi() {
	// create a new router to link the api
	router := chi.NewRouter()

	// handle all options calls
	router.Method("OPTIONS", "/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// basic ping api
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	// basic health api
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	// link pprof profile endpoints
	router.Route("/debug/pprof", func(r chi.Router) {
		r.HandleFunc("/*", pprof.Index)
		r.HandleFunc("/cmdline", pprof.Cmdline)
		r.HandleFunc("/profile", pprof.Profile)
		r.HandleFunc("/symbol", pprof.Symbol)
		r.HandleFunc("/trace", pprof.Trace)
		r.HandleFunc("/vars", expVars)

		r.Handle("/goroutine", pprof.Handler("goroutine"))
		r.Handle("/threadcreate", pprof.Handler("threadcreate"))
		r.Handle("/mutex", pprof.Handler("mutex"))
		r.Handle("/heap", pprof.Handler("heap"))
		r.Handle("/block", pprof.Handler("block"))
		r.Handle("/allocs", pprof.Handler("allocs"))
	})

	// base external api path
	router.Route("/api/v1", func(r chi.Router) {
		// create router bound to authenticated users
		authRouter := r.With(a.authenticateSession)
		authRouter.Get("/ws", a.MasterWebSocket)
	})

	// mount the host router to the main router
	a.router.Mount("/", router)
}

// initRequest
//
//	Middleware to initialize a http request.
//	This should be the first middleware called on the system (excluding a logger).
func (a *HttpApi) initRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// a.Logger.Debug("calling initRequest", zap.String("path", r.URL.Path), zap.String("method", r.Method), zap.String("ip", utils.GetRemoteAddr(r)))
		ctx = context.WithValue(ctx, CtxKeyIPAddress, utils2.GetRemoteAddr(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleError
//
//	Uniform handler for logging errors and writing a response message.
func (a *HttpApi) handleError(w http.ResponseWriter, r *http.Request,
	status int, message string, err error) {
	a.Logger.Error(
		r.Context(),
		"api call failed",
		slog.Error(err),
		slog.F("path", r.URL.Path),
		slog.F("reqId", middleware.GetReqID(r.Context())),
	)
	w.WriteHeader(status)
	w.Header().Add("Content-Type", "application/json")
	_, err = w.Write([]byte(`{"message":"` + message + `"}`))
	if err != nil {
		a.Logger.Error(
			r.Context(),
			"failed to write error response",
			slog.Error(err),
			slog.F("reqId", middleware.GetReqID(r.Context())),
		)
		return
	}
}

// handeJsonResponse
//
//	Uniform handler for JSON responses.
func (a *HttpApi) handeJsonResponse(w http.ResponseWriter, r *http.Request,
	status int, response any) {
	// create variable to hold the response buffer
	var buf []byte

	// handle string response by wrapping in json
	if s, ok := response.(string); ok {
		buf = []byte(`{"message":"` + s + `"}`)
	} else {
		// martial json response
		var err error
		buf, err = json.Marshal(response)
		if err != nil {
			a.handleError(
				w, r,
				http.StatusInternalServerError,
				DefaultErrorMessage,
				fmt.Errorf("failed to marshal json response: %v", err),
			)
			return
		}
	}

	// write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(buf)
	if err != nil {
		a.Logger.Error(
			r.Context(),
			"failed to write json response: %v",
			slog.Error(err),
			slog.F("reqId", middleware.GetReqID(r.Context())),
		)
		return
	}
}

// validateRequest
//
//	Loads a json request from the request body and validates it's schema.
func (a *HttpApi) validateRequest(w http.ResponseWriter, r *http.Request, buf io.Reader, value interface{}) bool {
	// attempt to decode the request body
	err := json.NewDecoder(buf).Decode(value)
	if err != nil {
		a.handleError(
			w, r,
			http.StatusInternalServerError,
			DefaultErrorMessage,
			fmt.Errorf("failed to decode request body: %v", err),
		)
		return false
	}

	// validate the schema
	err = a.validator.Struct(value)

	// handle known validation errors
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		message := ""
		for _, validationError := range validationErrors {
			if len(message) > 0 {
				message += ", "
			}
			message += fmt.Sprintf("Invalid field %q: value `%s` failed validation %s", validationError.Field(), validationError.Value(), validationError.Tag())
		}
		a.handleError(
			w, r,
			http.StatusBadRequest,
			message,
			fmt.Errorf("validation failed: %v", err),
		)
		return false
	}

	// handle unexpected validation errors
	if err != nil {
		a.handleError(
			w, r,
			http.StatusInternalServerError,
			DefaultErrorMessage,
			fmt.Errorf("validation failed: %v", err),
		)
		return false
	}

	return true
}

// authenticateSession
//
//	Authenticate a session using the auth token in the bearer. If
//	there is no auth token found or the token is invalid a 403 response is
//	written but no error is logged.
func (a *HttpApi) authenticateSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// attempt to load an auth token from the bearer header
		authToken := ""
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			authToken = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// if no auth token was found then return a 403
		if len(authToken) == 0 {
			a.Logger.Debug(
				r.Context(),
				"no auth token found",
				slog.F("path", r.URL.Path),
				slog.F("ip", r.Context().Value(CtxKeyIPAddress)),
			)
			a.handeJsonResponse(
				w, r,
				http.StatusForbidden,
				"forbidden",
			)
			return
		}

		// validate auth token
		if strings.ToLower(strings.TrimSpace(authToken)) != a.Secret {
			a.Logger.Debug(
				r.Context(),
				"invalid token",
				slog.F("path", r.URL.Path),
				slog.F("ip", r.Context().Value(CtxKeyIPAddress)),
				slog.F("token", authToken),
			)
			a.handeJsonResponse(
				w, r,
				http.StatusForbidden,
				"forbidden",
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// /////// Copied from: https://github.com/go-chi/chi/blob/51068a747f1a32dbce24c499561a0f5ecb7f7158/middleware/profiler.go

// Replicated from expvar.go as not public.
func expVars(w http.ResponseWriter, r *http.Request) {
	first := true
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, "{\n")
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			_, _ = fmt.Fprintf(w, ",\n")
		}
		first = false
		_, _ = fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	_, _ = fmt.Fprintf(w, "\n}\n")
}
