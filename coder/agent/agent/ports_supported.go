//go:build linux || (windows && amd64)

package agent

import (
	"github.com/cakturk/go-netstat/netstat"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"golang.org/x/xerrors"
)

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

		procName := ""
		if tab.Process != nil {
			procName = tab.Process.Name
		}
		ports[tab.LocalAddr.Port] = &agentsdk.ListeningPort{
			ProcessName: procName,
			Network:     agentsdk.ListeningPortNetworkTCP,
			Port:        tab.LocalAddr.Port,
		}
	}

	return ports, nil
}
