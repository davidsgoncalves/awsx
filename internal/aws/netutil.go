package aws

import "net"

// FreeLocalPort asks the OS for a free TCP port on the loopback interface and
// returns it. There is a small window between closing the listener and the
// caller using the port, but it is adequate for launching a local tunnel.
func FreeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
