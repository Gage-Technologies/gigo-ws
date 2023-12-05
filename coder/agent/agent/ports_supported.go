//go:build linux || (windows && amd64)

package agent

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"git.mills.io/prologic/go-netstat"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"golang.org/x/xerrors"
)

func isSSL(port uint16) bool {
	conn, err := tls.Dial("tcp", fmt.Sprintf("localhost:%d", port), &tls.Config{
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

func getListeningPorts() (ActivePortState, error) {
	tabs, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
		return s.State == netstat.Listen
	})
	if err != nil {
		return nil, xerrors.Errorf("scan listening ports: %w", err)
	}

	ports := make(ActivePortState)
	for _, tab := range tabs {
		if tab.LocalAddr == nil || tab.LocalAddr.Port < agentsdk.MinimumListeningPort {
			continue
		}

		// Don't include ports that we've already seen. This can happen on
		// Windows, and maybe on Linux if you're using a shared listener socket.
		if _, ok := ports[tab.LocalAddr.Port]; ok {
			continue
		}

		// skip blacklisted ports
		if _, ok := agentsdk.IgnoredListeningPorts[tab.LocalAddr.Port]; ok {
			continue
		}

		// check if the port is ssl
		isSsl := isSSL(tab.LocalAddr.Port)

		// check if the port is http
		isHttp := isHTTP(tab.LocalAddr.Port, isSsl)

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
