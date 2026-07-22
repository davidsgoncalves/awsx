package aws

import (
	"fmt"
	"net"
	"testing"
)

func TestFreeLocalPort(t *testing.T) {
	p, err := FreeLocalPort()
	if err != nil {
		t.Fatal(err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("port out of range: %d", p)
	}
	// The port should be bindable right after being reported free.
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		t.Fatalf("reported port %d not usable: %v", p, err)
	}
	_ = l.Close()
}
