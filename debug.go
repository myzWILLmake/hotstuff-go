package hotstuff

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

type IDebugServer interface {
	handleConnArgs(net.Conn, []string)
}

type DebugServerBase struct {
	addr     string
	tcpl     net.Listener
	clients  map[string]net.Conn
	notifyCh chan interface{}
}

func (ds *DebugServerBase) getNotifyMsg() {
	for true {
		msg := <-ds.notifyCh
		for _, conn := range ds.clients {
			conn.Write([]byte(msg.(string)))
		}
	}
}

func (ds *DebugServerBase) readConnData(ids IDebugServer, conn net.Conn) {
	buf := make([]byte, 4096)
	for {
		cnt, err := conn.Read(buf)
		if err != nil {
			return
		}
		msg := string(buf[:cnt])
		args := strings.Fields(msg)
		if len(args) > 0 {
			ids.handleConnArgs(conn, args)
		}
	}
}

func (ds *DebugServerBase) run(ids IDebugServer, wg *sync.WaitGroup) {
	go ds.getNotifyMsg()
	for true {
		tcpConn, err := ds.tcpl.Accept()
		if err != nil {
			break
		}
		ds.clients[tcpConn.RemoteAddr().String()] = tcpConn
		go ds.readConnData(ids, tcpConn)
	}
	wg.Done()
}

type HotStuffDebugServer struct {
	DebugServerBase
	hotStuffServer *HotStuff
}

func (hds *HotStuffDebugServer) handlePrint(conn net.Conn) {
	info := hds.hotStuffServer.getServerInfo()
	msg := fmt.Sprintf(`
HotStuff Server State:
id:             %d
n:              %d
viewId          %d
gQC             %d
	%s
lQC             %d
    %s
`, info["id"].(int), info["n"].(int), info["viewId"].(int),
		info["genericQCView"].(int), info["genericQCId"].(string),
		info["lockedQCView"].(int), info["lockedQCId"].(string))
	conn.Write([]byte(msg))
}

func (hds *HotStuffDebugServer) handleMaliciousBehavior(conn net.Conn, args []string) {
	if len(args) < 2 {
		conn.Write([]byte("Arguments not enough\n"))
		return
	}

	mbmode, err := strconv.Atoi(args[1])
	if err != nil {
		conn.Write([]byte("Invalid malicious mode\n"))
		return
	}

	err = hds.hotStuffServer.setMaliciousMode(mbmode)
	if err != nil {
		conn.Write([]byte(err.Error() + "\n"))
		return
	}

	conn.Write([]byte(fmt.Sprintf("malicious behavior set. mode[%d]\n", mbmode)))
}

func (hds *HotStuffDebugServer) handleNodes(conn net.Conn) {
	msg := hds.hotStuffServer.getRecentNodes()
	conn.Write([]byte(msg))
}

func (hds *HotStuffDebugServer) handleConnArgs(conn net.Conn, args []string) {
	switch args[0] {
	case "mb":
		hds.handleMaliciousBehavior(conn, args)
	case "kill":
		conn.Write([]byte("Kill Server...\n"))
		conn.Close()
		hds.tcpl.Close()
	case "print":
		hds.handlePrint(conn)
	case "nodes":
		hds.handleNodes(conn)
	case "quit":
		conn.Write([]byte("Bye!\n"))
	case "echo":
		msg := strings.Join(args[1:], " ")
		conn.Write([]byte(msg))
	}
}

func MakeHotStuffDebugServer(addr string, ch chan interface{}, hotStuff *HotStuff, wg *sync.WaitGroup) *HotStuffDebugServer {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("debug server listen error:", addr, err)
	}

	hds := &HotStuffDebugServer{}
	hds.tcpl = tcpListener
	hds.addr = addr
	hds.clients = make(map[string]net.Conn)
	hds.notifyCh = ch
	hds.hotStuffServer = hotStuff
	wg.Add(1)
	go hds.run(hds, wg)
	return hds
}

type ClientDebugServer struct {
	DebugServerBase
	clientServer *Client
}

func (cds *ClientDebugServer) handleRequest(conn net.Conn, args []string) {
	request := strings.Join(args[1:], " ")
	cds.clientServer.newRequest(request)
	reply := fmt.Sprintf("Request[%s] sent.\n", request)
	conn.Write([]byte(reply))
}

func (cds *ClientDebugServer) handleConnArgs(conn net.Conn, args []string) {
	switch args[0] {
	case "req":
		cds.handleRequest(conn, args)
	case "kill":
		conn.Write([]byte("Kill Server...\n"))
		conn.Close()
		cds.tcpl.Close()
	case "print":
		// handlePrint()
	case "quit":
		conn.Write([]byte("Bye!\n"))
		conn.Close()
	case "echo":
		msg := strings.Join(args[1:], " ")
		conn.Write([]byte(msg))
	}
}

func MakeClientDebugServer(addr string, ch chan interface{}, client *Client, wg *sync.WaitGroup) *ClientDebugServer {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("clientSrv listen error:", addr, err)
	}
	cds := &ClientDebugServer{}
	cds.tcpl = tcpListener
	cds.clients = make(map[string]net.Conn)
	cds.addr = addr
	cds.notifyCh = ch
	cds.clientServer = client
	wg.Add(1)
	go cds.run(cds, wg)
	return cds
}
