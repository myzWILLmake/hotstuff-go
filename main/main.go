package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/myzWILLmake/hotstuff-go"
	"github.com/spf13/viper"
)

type NodeInfo struct {
	Id      int    `json:"id"`
	Address string `json:"address"`
	Debug   string `json:"debug"`
}

type X struct {
	Servers []NodeInfo `json:"servers"`
	Clients []NodeInfo `json:"clients"`
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Invalid augments")
		return
	}

	nodeType := os.Args[1]
	if nodeType != "client" && nodeType != "server" {
		log.Fatal("Invalid node type")
		return
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatal("Invalid id")
		return
	}
	viper.SetConfigName("config.json")
	viper.AddConfigPath(".")
	viper.SetConfigType("json")
	err = viper.ReadInConfig()
	if err != nil {
		fmt.Printf("config file error: %s\n", err)
		os.Exit(1)
	}
	var x X
	viper.Unmarshal(&x)

	serverAddrs := make([]string, len(x.Servers))
	for _, node := range x.Servers {
		serverAddrs[node.Id] = node.Address
	}
	clientAddrs := make([]string, len(x.Clients))
	for _, node := range x.Clients {
		clientAddrs[node.Id] = node.Address
	}

	if nodeType == "server" {
		debugAddr := x.Servers[id].Debug
		wg := &sync.WaitGroup{}
		hotstuff.RunHotStuffServer(id, serverAddrs, clientAddrs, true, debugAddr, wg)
		wg.Wait()
	} else if nodeType == "client" {
		clientAddr := x.Clients[id].Address
		debugAddr := x.Clients[id].Debug
		wg := &sync.WaitGroup{}
		hotstuff.RunClient(id, clientAddr, serverAddrs, true, debugAddr, wg)
		wg.Wait()
	}

}
