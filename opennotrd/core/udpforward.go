package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ICKelin/opennotr/pkg/logs"
	"github.com/hashicorp/yamux"
)

type UDPForward struct {
	sessMgr *SessionManager
}

func NewUDPForward() *UDPForward {
	return &UDPForward{
		sessMgr: GetSessionManager(),
	}
}

func (f *UDPForward) ListenAndServe(listenAddr string) error {
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		logs.Error("resolve udp fail: %v", err)
		return err
	}

	lconn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return err
	}

	// set socket with ip transparent option
	file, err := lconn.File()
	if err != nil {
		return err
	}
	defer file.Close()

	err = syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if err != nil {
		return err
	}

	// set socket with recv origin dst option
	err = syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
	if err != nil {
		return err
	}

	// create raw socket fd
	// we use rawsocket to send udp packet back to client.
	rawfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil || rawfd < 0 {
		logs.Error("call socket fail: %v", err)
		return err
	}
	defer syscall.Close(rawfd)

	err = syscall.SetsockoptInt(rawfd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
	if err != nil {
		return err
	}

	streams := sync.Map{}
	defer func() {
		streams.Range(func(k, v interface{}) bool {
			v.(*yamux.Stream).Close()
			return true
		})
	}()

	buf := make([]byte, 64*1024)
	oob := make([]byte, 1024)
	for {
		nr, oobn, _, raddr, err := lconn.ReadMsgUDP(buf, oob)
		if err != nil {
			logs.Error("read from udp fail: %v", err)
			break
		}

		origindst, err := getOriginDst(oob[:oobn])
		if err != nil {
			logs.Error("%v", err)
			continue
		}

		dip, dport, _ := net.SplitHostPort(origindst.String())
		sip, sport, _ := net.SplitHostPort(raddr.String())

		key := fmt.Sprintf("%s:%s:%s:%s", sip, sport, dip, dport)
		val, ok := streams.Load(key)
		if !ok {
			sess := f.sessMgr.GetSession(dip)
			if sess == nil {
				logs.Error("no route to host: %s", dip)
				continue
			}

			stream, err := sess.conn.OpenStream()
			if err != nil {
				logs.Error("open stream fail: %v", err)
				continue
			}
			streams.Store(key, stream)

			bytes := encodeProxyProtocol("udp", sip, sport, "127.0.0.1", dport)
			stream.SetWriteDeadline(time.Now().Add(time.Second * 10))
			_, err = stream.Write(bytes)
			stream.SetWriteDeadline(time.Time{})
			if err != nil {
				logs.Error("stream write fail: %v", err)
				continue
			}
			go f.forwardUDP(stream, rawfd, origindst, raddr)
		}

		val, ok = streams.Load(key)
		if !ok {
			logs.Error("get stream for %s fail", key)
			continue
		}

		stream := val.(*yamux.Stream)
		bytes := encode(buf[:nr])
		stream.SetWriteDeadline(time.Now().Add(time.Second * 10))
		_, err = stream.Write(bytes)
		stream.SetWriteDeadline(time.Time{})
		if err != nil {
			logs.Error("stream write fail: %v", err)
		}
	}
	return nil
}

func (f *UDPForward) forwardUDP(stream *yamux.Stream, tofd int, fromaddr, toaddr *net.UDPAddr) {
	hdr := make([]byte, 2)
	for {
		_, err := io.ReadFull(stream, hdr)
		if err != nil {
			logs.Error("read stream fail %v", err)
			break
		}

		nlen := binary.BigEndian.Uint16(hdr)
		buf := make([]byte, nlen)
		_, err = io.ReadFull(stream, buf)
		if err != nil {
			logs.Error("read stream body fail: %v", err)
			break
		}

		err = sendUDPViaRaw(tofd, fromaddr, toaddr, buf)
		if err != nil {
			logs.Error("send via raw socket fail: %v", err)
		}
	}
}

func getOriginDst(hdr []byte) (*net.UDPAddr, error) {
	msgs, err := syscall.ParseSocketControlMessage(hdr)
	if err != nil {
		return nil, err
	}

	var origindst *net.UDPAddr
	for _, msg := range msgs {
		if msg.Header.Level == syscall.SOL_IP &&
			msg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			originDstRaw := &syscall.RawSockaddrInet4{}
			err := binary.Read(bytes.NewReader(msg.Data), binary.LittleEndian, originDstRaw)
			if err != nil {
				logs.Error("read origin dst fail: %v", err)
				continue
			}

			// only support for ipv4
			if originDstRaw.Family == syscall.AF_INET {
				pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(originDstRaw))
				p := (*[2]byte)(unsafe.Pointer(&pp.Port))
				origindst = &net.UDPAddr{
					IP:   net.IPv4(pp.Addr[0], pp.Addr[1], pp.Addr[2], pp.Addr[3]),
					Port: int(p[0])<<8 + int(p[1]),
				}
			}
		}
	}

	if origindst == nil {
		return nil, fmt.Errorf("get origin dst fail")
	}

	return origindst, nil
}