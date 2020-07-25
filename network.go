package hotstuff

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

type peerWrapper struct {
	client  *rpc.Client
	address string
}

func (c *peerWrapper) Call(serviceMethod string, args interface{}, reply interface{}) error {
	var err error
	if c.client == nil {
		err = errors.New("")
	} else {
		err = c.client.Call(serviceMethod, args, reply)
	}
	if err != nil {
		var errdial error
		c.client, errdial = rpc.DialHTTP("tcp", c.address)
		if errdial != nil {
			return errdial
		}
		err = c.client.Call(serviceMethod, args, reply)
	}
	return err
}

func createPeers(addresses []string) []peerWrapper {
	peers := make([]peerWrapper, len(addresses))
	for i := 0; i < len(addresses); i++ {
		peers[i].client = nil
		peers[i].address = addresses[i]
	}

	return peers
}

func RunHotStuffServer(id int, serverAddrs, clientAddrs []string, debug bool, debugAddr string, wg *sync.WaitGroup) *HotStuff {
	debugCh := make(chan interface{}, 1024)
	servers := createPeers(serverAddrs)
	clients := createPeers(clientAddrs)
	hotStuff := MakeHotStuff(id, servers, clients, debugCh)

	if debug {
		MakeHotStuffDebugServer(debugAddr, debugCh, hotStuff, wg)
	}

	rpc.Register(hotStuff)
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", serverAddrs[id])
	if err != nil {
		log.Fatal("listen error:", err)
		return nil
	}

	go http.Serve(l, nil)
	return hotStuff
}

func RunClient(id int, clientAddr string, hotStuffAddrs []string, debug bool, debugAddr string, wg *sync.WaitGroup) *Client {
	debugCh := make(chan interface{}, 1024)
	peers := createPeers(hotStuffAddrs)
	client := MakeClient(id, peers, debugCh)

	if debug {
		MakeClientDebugServer(debugAddr, debugCh, client, wg)
	}

	rpc.Register(client)
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", clientAddr)
	if err != nil {
		log.Fatal("listen error:", err)
		return nil
	}

	go http.Serve(l, nil)
	return client
}
