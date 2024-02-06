//go:build linux || (windows && amd64)

package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"cdr.dev/slog"
	"git.mills.io/prologic/go-netstat"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"golang.org/x/xerrors"
)

func isSSL(port uint16) bool {
	dialer := &net.Dialer{
		Timeout: time.Second * 3,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", port), &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func isHTTP(port uint16, useTls bool) bool {
	client := &http.Client{
		Timeout: time.Second * 3,
	}
	protocol := "http"
	if useTls {
		protocol = "https"
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	resp, err := client.Get(fmt.Sprintf("%s://localhost:%d", protocol, port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func getListeningPorts(ctx context.Context, logger slog.Logger) (ActivePortState, error) {
	defer func() {
		r := recover()
		if r != nil {
			logger.Error(ctx, "recovered panic while scanning for listening ports", slog.F("panic", r))
		}
	}()

	// logger.Debug(ctx, "checking for listening ports")
	tabs, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
		return s.State == netstat.Listen
	})
	if err != nil {
		return nil, xerrors.Errorf("scan listening ports: %w", err)
	}

	// logger.Debug(ctx, "listening ports detected", slog.F("ports", tabs))

	ports := make(ActivePortState)
	for _, tab := range tabs {
		if tab.LocalAddr == nil || tab.LocalAddr.Port < agentsdk.MinimumListeningPort {
			// logger.Debug(ctx, "ignoring port because it was too low", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))
			continue
		}

		// Don't include ports that we've already seen. This can happen on
		// Windows, and maybe on Linux if you're using a shared listener socket.
		if _, ok := ports[tab.LocalAddr.Port]; ok {
			// logger.Debug(ctx, "ignoring duplicate port", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))
			continue
		}

		// skip blacklisted ports
		if _, ok := agentsdk.IgnoredListeningPorts[tab.LocalAddr.Port]; ok {
			// logger.Debug(ctx, "ignoring blacklisted port", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))
			continue
		}

		// logger.Debug(ctx, "processing new port", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))

		// logger.Debug(ctx, "checking ssl", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))
		// check if the port is ssl
		isSsl := isSSL(tab.LocalAddr.Port)

		// logger.Debug(ctx, "checking http", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))
		// check if the port is http
		isHttp := isHTTP(tab.LocalAddr.Port, isSsl)

		// logger.Debug(ctx, "adding new port", slog.F("port", tab.LocalAddr.Port), slog.F("address", tab.LocalAddr.IP), slog.F("process", tab.Process))

		procName := ""
		if tab.Process != nil {
			procName = tab.Process.Name
		}
		ports[tab.LocalAddr.Port] = &agentsdk.ListeningPort{
			ProcessName: procName,
			Network:     agentsdk.ListeningPortNetworkTCP,
			Port:        tab.LocalAddr.Port,
			HTTP:        isHttp,
			SSL:         isSsl,
		}
	}

	return ports, nil
}
