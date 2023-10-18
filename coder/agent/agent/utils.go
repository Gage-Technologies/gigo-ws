package agent

import (
	"context"
	"fmt"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"golang.org/x/xerrors"
	"io"
	"os"
	"os/user"
	"sync"
	"tailscale.com/types/netlogtype"
)

func convertAgentStats(counts map[netlogtype.Connection]netlogtype.Counts) *agentsdk.AgentStats {
	stats := &agentsdk.AgentStats{
		ConnsByProto: map[string]int64{},
		NumConns:     int64(len(counts)),
	}

	for conn, count := range counts {
		stats.ConnsByProto[conn.Proto.String()]++
		stats.RxPackets += int64(count.RxPackets)
		stats.RxBytes += int64(count.RxBytes)
		stats.TxPackets += int64(count.TxPackets)
		stats.TxBytes += int64(count.TxBytes)
	}

	return stats
}

// Bicopy copies all of the data between the two connections and will close them
// after one or both of them are done writing. If the context is canceled, both
// of the connections will be closed.
func Bicopy(ctx context.Context, c1, c2 io.ReadWriteCloser) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		_ = c1.Close()
		_ = c2.Close()
	}()

	var wg sync.WaitGroup
	copyFunc := func(dst io.WriteCloser, src io.Reader) {
		defer func() {
			wg.Done()
			// If one side of the copy fails, ensure the other one exits as
			// well.
			cancel()
		}()
		_, _ = io.Copy(dst, src)
	}

	wg.Add(2)
	go copyFunc(c1, c2)
	go copyFunc(c2, c1)

	// Convert waitgroup to a channel so we can also wait on the context.
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// isQuietLogin checks if the SSH server should perform a quiet login or not.
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L816
func isQuietLogin(rawCommand string) bool {
	// We are always quiet unless this is a login shell.
	if len(rawCommand) != 0 {
		return true
	}

	// TODO: make sure this okay
	// we always want the plug - no silence in our house

	// Best effort, if we can't get the home directory,
	// we can't lookup .hushlogin.
	// homedir, err := userHomeDir()
	// if err != nil {
	// 	return false
	// }

	// _, err = os.Stat(filepath.Join(homedir, ".hushlogin"))
	// return err == nil

	return false
}

// showPlug
//
//	Like showMOTD but it's only a plug
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L784
func showPlug(dest io.Writer) error {
	// Carriage return ensures each line starts
	// at the beginning of the terminal.
	_, err := fmt.Fprint(dest, "Welcome to GIGO.\r\n")
	if err != nil {
		return xerrors.Errorf("write MOTD: %w", err)
	}

	return nil
}

// userHomeDir returns the home directory of the current user, giving
// priority to the $HOME environment variable.
func userHomeDir() (string, error) {
	// First we check the environment.
	homedir, err := os.UserHomeDir()
	if err == nil {
		return homedir, nil
	}

	// As a fallback, we try the user information.
	u, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("current user: %w", err)
	}
	return u.HomeDir, nil
}
