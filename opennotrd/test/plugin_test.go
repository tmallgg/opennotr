package plugin

import (
	"net"
	"testing"
	"time"

	"github.com/ICKelin/opennotr/opennotrd/plugin"
	_ "github.com/ICKelin/opennotr/opennotrd/plugin/tcpproxy"
)

var listener net.Listener

func init() {

}

func runTCPServer(bufsize int) net.Listener {
	lis, err := net.Listen("tcp", "127.0.0.1:2345")
	if err != nil {
		panic(err)
	}

	go func() {
		defer lis.Close()
		for {
			conn, err := lis.Accept()
			if err != nil {
				break
			}

			go onconn(conn, bufsize)
		}
	}()
	return lis
}

func onconn(conn net.Conn, bufsize int) {
	defer conn.Close()
	buf := make([]byte, bufsize)
	nr, _ := conn.Read(buf)
	conn.Write(buf[:nr])
}

func runEcho(t *testing.T, bufsize, numconn int) {
	item := &plugin.PluginMeta{
		Protocol:      "tcp",
		From:          "127.0.0.1:1234",
		To:            "127.0.0.1:2345",
		RecycleSignal: make(chan struct{}),
	}
	err := plugin.DefaultPluginManager().AddProxy(item)
	if err != nil {
		t.Error(err)
		return
	}
	defer plugin.DefaultPluginManager().DelProxy(item)

	lis := runTCPServer(bufsize)

	// client
	for i := 0; i < numconn; i++ {
		go func() {
			buf := make([]byte, bufsize)
			conn, err := net.Dial("tcp", "127.0.0.1:1234")
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			for {
				conn.Write(buf)
				conn.Read(buf)
			}
		}()
	}
	tick := time.NewTicker(time.Second * 60)
	<-tick.C
	lis.Close()
	time.Sleep(time.Second)
}

func TestTCPEcho128B(t *testing.T) {
	numconn := 128
	bufsize := 1024
	runEcho(t, bufsize, numconn)
}
