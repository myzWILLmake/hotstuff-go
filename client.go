package hotstuff

import (
	"fmt"
	"sync"
	"time"
)

// save operation as string
type Client struct {
	mu       *sync.Mutex
	me       int
	n        int
	f        int
	peers    []peerWrapper
	requests map[int64]string
	replies  map[int64]map[int]string

	debugCh chan interface{}
}

func (c *Client) broadcast(rpcname string, rpcargs interface{}) {
	reply := &DefaultReply{}
	for _, peer := range c.peers {
		p := peer
		go p.Call("HotStuff."+rpcname, rpcargs, reply)
	}
}

func (c *Client) newRequest(command string) {
	requestArgs := &RequestArgs{}
	requestArgs.ClientId = c.me
	requestArgs.Operation = command
	requestArgs.Timestamp = time.Now().Unix()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.replies[requestArgs.Timestamp] = make(map[int]string)
	c.requests[requestArgs.Timestamp] = command
	c.broadcast("Request", requestArgs)
}

func (c *Client) saveReply(replyArgs *ReplyArgs) {
	timestamp := replyArgs.Timestamp
	if c.replies[timestamp] == nil {
		return
	}

	c.replies[timestamp][replyArgs.ReplicaId] = replyArgs.Result.(string)
}

func (c *Client) processReplies(timestamp int64) {
	replies := c.replies[timestamp]
	if replies == nil || len(replies) <= c.f {
		return
	}

	resultMap := make(map[string]int)
	maxCnt := 0
	maxResult := ""
	for _, result := range replies {
		resultMap[result]++
		if resultMap[result] > maxCnt {
			maxCnt = resultMap[result]
			maxResult = result
		}
	}

	if maxCnt > c.f {
		// accept Reply
		c.acceptReply(timestamp, maxResult)
	}
}

func (c *Client) acceptReply(timestamp int64, result string) {
	if c.requests[timestamp] == "" {
		return
	}

	// output the result
	command := c.requests[timestamp]
	msg := fmt.Sprintf("Client [%d]: Command[%s] got Result[%s]\n", c.me, command, result)
	c.debugPrint(msg)

	delete(c.requests, timestamp)
	delete(c.replies, timestamp)
}

func (c *Client) debugPrint(msg string) {
	c.debugCh <- msg
}

func MakeClient(id int, peers []peerWrapper, ch chan interface{}) *Client {
	c := &Client{}
	c.mu = &sync.Mutex{}
	c.me = id
	c.peers = peers
	c.requests = make(map[int64]string)
	c.replies = make(map[int64]map[int]string)
	c.debugCh = ch
	c.n = len(c.peers)
	c.f = (c.n - 1) / 3

	return c
}
