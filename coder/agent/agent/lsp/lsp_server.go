package lsp

import (
	"cdr.dev/slog"
	"context"
	"github.com/gage-technologies/gigo-lib/db/models"
	"time"
)

type LspServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	stdout chan string
	stderr chan string
	done   chan bool
	logger slog.Logger

	Language models.ProgrammingLanguage
}

func NewLspServer(ctx context.Context, lang models.ProgrammingLanguage, logger slog.Logger) *LspServer {
	ctx, cancel := context.WithCancel(ctx)

	s := &LspServer{
		ctx:      ctx,
		cancel:   cancel,
		stdout:   make(chan string),
		stderr:   make(chan string),
		done:     make(chan bool),
		logger:   logger,
		Language: lang,
	}

	// launch the server
	go s.run()

	return s
}

func (l *LspServer) run() {
	// launch routine to forward lsp logs to output log
	go func() {
		for {
			select {
			case <-l.ctx.Done():
				break
			case s := <-l.stdout:
				l.logger.Info(l.ctx, "lsp stdout", slog.F("output", s))
			case e := <-l.stderr:
				l.logger.Error(l.ctx, "lsp stderr", slog.F("output", e))
			}
		}
	}()

	for {
		select {
		case <-l.ctx.Done():
			// mark the LSP server as done
			go func() {
				l.done <- true
			}()
			return
		default:
		}

		// launch the language server for the language
		res, err := launchLsp(l.Language, l.ctx, l.stdout, l.stderr)
		if err != nil {
			l.logger.Error(l.ctx, "error launching lsp", slog.Error(err))
			// sleep 1 second before continuing
			time.Sleep(time.Second)
			continue
		}

		// log the command exit
		l.logger.Info(l.ctx, "lsp exited", slog.F("code", res.ExitCode), slog.F("runtime", res.Cost.Milliseconds()))

		// sleep 1 second before continuing
		time.Sleep(time.Second)
	}
}

func (l *LspServer) Close() {
	l.cancel()
	<-l.done
}
