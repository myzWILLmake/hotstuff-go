package hotstuff

import (
	"fmt"
	"sync"
	"time"
)

const ViewTimeOut = 10000

type HotStuff struct {
	mu        *sync.Mutex
	servers   []peerWrapper
	clients   []peerWrapper
	request   *RequestArgs
	n         int
	f         int
	me        int
	viewId    int
	nodeMap   map[string]*LogNode
	genericQC QC
	lockedQC  QC
	savedMsgs map[int]*MsgArgs
	viewTimer *TimerWithCancel

	debugCh chan interface{}
}

func (hs *HotStuff) isLeader() bool {
	return hs.me == hs.viewId%hs.n
}

func (hs *HotStuff) isNextLeader() bool {
	return hs.me == (hs.viewId+1)%hs.n
}

func (hs *HotStuff) getLeader(viewId int) peerWrapper {
	id := viewId % hs.n
	return hs.servers[id]
}

func (hs *HotStuff) broadcast(rpcname string, rpcargs interface{}) {
	reply := &DefaultReply{}
	for _, peer := range hs.servers {
		p := peer
		go p.Call("HotStuff."+rpcname, rpcargs, reply)
	}
}

func (hs *HotStuff) replyClient(clientId int, replyArgs *ReplyArgs) {
	client := hs.clients[clientId]
	defaultReply := &DefaultReply{}
	go client.Call("Client.Reply", replyArgs, defaultReply)
}

func (hs *HotStuff) debugPrint(msg string) {
	hs.debugCh <- msg
}

func (hs *HotStuff) createLeaf(parent string, request *RequestArgs, qc QC) *LogNode {
	if request == nil {
		return nil
	}

	parentNode := hs.nodeMap[parent]
	parentView := parentNode.viewId
	for parentView < hs.viewId-1 {
		dummyNode := &LogNode{}
		dummyNode.viewId = parentView
		dummyNode.parent = parent
		dummyNode.request = RequestArgs{}
		dummyNode.request.Operation = "dummy"
		dummyNode.id = getLogNodeId(dummyNode.viewId, &dummyNode.request)
		dummyNode.justify = QC{}
		hs.nodeMap[dummyNode.id] = dummyNode
		parent = dummyNode.id
		parentView++
	}

	node := &LogNode{}
	node.viewId = hs.viewId
	node.parent = parent
	node.request = *request
	node.id = getLogNodeId(hs.viewId, request)
	node.justify = qc

	hs.nodeMap[node.id] = node
	return node
}

func (hs *HotStuff) safeNode(n *LogNode, qc QC) bool {
	if n.parent == hs.lockedQC.nodeId {
		return true
	}
	if qc.viewId > hs.lockedQC.viewId {
		return true
	}
	return false
}

func (hs *HotStuff) update(n *LogNode) {
	var prepare, precommit, commit, decide *LogNode
	var nodeId string
	prepare = n
	if prepare != nil {
		nodeId = prepare.justify.nodeId
		precommit = hs.nodeMap[nodeId]
	}
	if precommit != nil {
		nodeId = precommit.justify.nodeId
		commit = hs.nodeMap[nodeId]
	}
	if commit != nil {
		nodeId = commit.justify.nodeId
		decide = hs.nodeMap[nodeId]
	}

	if hs.safeNode(prepare, prepare.justify) {
		voteMsg := &MsgArgs{}
		voteMsg.RepId = hs.me
		voteMsg.ViewId = hs.viewId
		voteMsg.Node = *prepare
		voteMsg.ParSig = true
		replyArgs := &DefaultReply{}
		nextLeader := hs.getLeader(hs.viewId + 1)
		go nextLeader.Call("HotStuff.Msg", voteMsg, replyArgs)
	}

	if prepare.parent == precommit.id {
		hs.genericQC = prepare.justify
		if precommit.parent == commit.id {
			hs.lockedQC = precommit.justify
			if commit.parent == decide.id {
				//execute decide
				hs.debugPrint(fmt.Sprintf("Execute Request: id[%s], op[%s]", decide.id, decide.request.Operation.(string)))
				request := decide.request
				if request.Timestamp != 0 {
					reply := &ReplyArgs{}
					reply.ViewId = hs.viewId
					reply.Timestamp = request.Timestamp
					reply.ReplicaId = hs.me
					reply.Result = request.Operation
					hs.replyClient(request.ClientId, reply)
				}
			}
		}
	}
}

func (hs *HotStuff) processSavedMsgs() {
	if len(hs.savedMsgs) > hs.n-hs.f {
		checkVoteMap := make(map[string]int)
		// try to find genericQC
		for _, msg := range hs.savedMsgs {
			if msg.Node.id != "" {
				node := msg.Node
				checkVoteMap[node.id]++
				if checkVoteMap[node.id] > hs.f {
					newQc := QC{}
					newQc.viewId = node.viewId
					newQc.nodeId = node.id
					hs.genericQC = newQc
					return
				}
			}
		}
	}
}

func (hs *HotStuff) newView(viewId int) {
	if hs.viewId >= viewId {
		return
	}

	msg := fmt.Sprintf("HotStuff: Change to NewView[%d]\n", viewId)
	hs.debugPrint(msg)
	if hs.viewTimer != nil {
		hs.viewTimer.Cancel()
		hs.viewTimer = nil
	}

	hs.viewId = viewId
	if hs.isLeader() {
		var highNode *LogNode
		for _, msg := range hs.savedMsgs {
			if msg.Qc.nodeId != "" {
				node, ok := hs.nodeMap[msg.Qc.nodeId]
				if ok {
					if highNode == nil || node.viewId > highNode.viewId {
						highNode = node
					}
				}
			}
		}

		if highNode != nil {
			highQC := highNode.justify
			if highQC.viewId > hs.genericQC.viewId {
				hs.genericQC = highQC
			}
		}

	}

	hs.savedMsgs = make(map[int]*MsgArgs)

	hs.viewTimer = NewTimerWithCancel(time.Duration(ViewTimeOut * time.Millisecond))
	hs.viewTimer.SetTimeout(func() {
		hs.mu.Lock()
		defer hs.mu.Unlock()

		nextLeader := hs.getLeader(hs.viewId + 1)
		newViewMsg := &MsgArgs{}
		newViewMsg.ViewId = hs.viewId
		newViewMsg.RepId = hs.me
		newViewMsg.Qc = hs.genericQC
		newViewMsg.ParSig = true
		go nextLeader.Call("HotStuff.Msg", newViewMsg, &DefaultReply{})
		hs.newView(hs.viewId + 1)
	})
	hs.viewTimer.Start()
}

func (hs *HotStuff) getServerInfo() map[string]interface{} {
	info := make(map[string]interface{})
	info["id"] = hs.me
	info["viewId"] = hs.viewId
	info["n"] = hs.n
	info["f"] = hs.f
	info["genericQCId"] = hs.genericQC.nodeId
	info["genericQCView"] = hs.genericQC.viewId
	info["lockedQCId"] = hs.lockedQC.nodeId
	info["lockedQCView"] = hs.lockedQC.viewId
	return info
}

func MakeHotStuff(id int, serverPeers, clientPeers []peerWrapper, debugCh chan interface{}) *HotStuff {
	hs := &HotStuff{}
	hs.mu = &sync.Mutex{}
	hs.me = id
	hs.servers = serverPeers
	hs.clients = clientPeers
	hs.viewId = 0
	hs.nodeMap = make(map[string]*LogNode)
	hs.n = len(hs.servers)
	hs.f = (hs.n - 1) / 3
	hs.savedMsgs = make(map[int]*MsgArgs)
	hs.newView(1)

	hs.debugCh = debugCh
	return hs
}
