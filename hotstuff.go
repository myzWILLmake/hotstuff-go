package hotstuff

import (
	"fmt"
	"sync"
	"time"
)

const ViewTimeOut = 10000
const noopTimeOut = 4000

type HotStuff struct {
	mu            *sync.Mutex
	servers       []peerWrapper
	clients       []peerWrapper
	request       *RequestArgs
	n             int
	f             int
	me            int
	viewId        int
	nodeMap       map[string]*LogNode
	lastNode      *LogNode
	genericQC     QC
	lockedQC      QC
	savedMsgs     map[int]*MsgArgs
	viewTimer     *TimerWithCancel
	noopTimer     *TimerWithCancel
	maliciousMode MaliciousBehaviorMode

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
	for id := range hs.servers {
		hs.sendMsg(id, rpcname, rpcargs)
	}
}

func (hs *HotStuff) sendMsg(id int, rpcname string, rpcacgs interface{}) {
	switch hs.maliciousMode {
	case NormalMode:
		hs.rawSendMsg(id, rpcname, rpcacgs)
	case CrashedLike:
		return
	case MaliciousMode:
		hs.sendMaliciousMsg(id, rpcname, rpcacgs)
	}
}

func (hs *HotStuff) rawSendMsg(id int, rpcname string, rpcacgs interface{}) {
	p := hs.servers[id]
	reply := &DefaultReply{}
	go p.Call("HotStuff."+rpcname, rpcacgs, reply)
}

func (hs *HotStuff) replyClient(clientId int, replyArgs *ReplyArgs) {
	client := hs.clients[clientId]
	defaultReply := &DefaultReply{}
	go client.Call("Client.Reply", replyArgs, defaultReply)
}

func (hs *HotStuff) debugPrint(msg string) {
	hs.debugCh <- msg
}

func (hs *HotStuff) processClientRequest(request *RequestArgs) {
	curProposal := hs.createLeaf(hs.genericQC.NodeId, request, hs.genericQC)
	genericMsg := &MsgArgs{}
	genericMsg.RepId = hs.me
	genericMsg.ViewId = hs.viewId
	genericMsg.Node = *curProposal
	hs.broadcast("Msg", genericMsg)
}

func (hs *HotStuff) createLeaf(parent string, request *RequestArgs, qc QC) *LogNode {
	if request == nil {
		return nil
	}

	parentNode, ok := hs.nodeMap[parent]
	if ok {
		tmpView := parentNode.ViewId + 1
		for tmpView < hs.viewId {
			dummyNode := &LogNode{}
			dummyNode.ViewId = tmpView
			dummyNode.Parent = parent
			dummyNode.Request = RequestArgs{}
			dummyNode.Request.Operation = "dummy"
			dummyNode.Id = getLogNodeId(dummyNode.ViewId, &dummyNode.Request)
			dummyNode.Justify = QC{}
			hs.nodeMap[dummyNode.Id] = dummyNode
			parent = dummyNode.Id
			tmpView++
		}
	}

	node := &LogNode{}
	node.ViewId = hs.viewId
	node.Parent = parent
	node.Request = *request
	node.Id = getLogNodeId(hs.viewId, request)
	node.Justify = qc

	hs.saveNode(node)
	msg := fmt.Sprintf("CreateLeaf: id[%s] parent[%s] view[%d] op[%s]\n", node.Id, node.Parent, node.ViewId, node.Request.Operation.(string))
	hs.debugPrint(msg)
	return node
}

func (hs *HotStuff) safeNode(n *LogNode, qc QC) bool {
	for n != nil {
		if n.Parent == hs.lockedQC.NodeId {
			return true
		}
		n = hs.nodeMap[n.Parent]
	}
	if qc.ViewId > hs.lockedQC.ViewId {
		return true
	}
	return false
}

func (hs *HotStuff) saveNode(n *LogNode) {
	hs.lastNode = n
	hs.nodeMap[n.Id] = n
}

func (hs *HotStuff) update(n *LogNode) {
	var prepare, precommit, commit, decide *LogNode
	var nodeId string
	prepare = n
	if prepare != nil {
		nodeId = prepare.Justify.NodeId
		precommit = hs.nodeMap[nodeId]
	}
	if precommit != nil {
		nodeId = precommit.Justify.NodeId
		commit = hs.nodeMap[nodeId]
	}
	if commit != nil {
		nodeId = commit.Justify.NodeId
		decide = hs.nodeMap[nodeId]
	}

	if hs.safeNode(prepare, prepare.Justify) {
		// node saved
		msg := fmt.Sprintf("LogNode saved: id[%s] qcId[%s] qcview[%d] \n", n.Id, n.Justify.NodeId, n.Justify.ViewId)
		hs.debugPrint(msg)

		hs.saveNode(n)
		voteMsg := &MsgArgs{}
		voteMsg.RepId = hs.me
		voteMsg.ViewId = hs.viewId
		voteMsg.Node = *prepare
		voteMsg.ParSig = true
		nextLeaderId := (hs.viewId + 1) % hs.n
		hs.sendMsg(nextLeaderId, "Msg", voteMsg)
	}

	if prepare != nil && precommit != nil && prepare.Parent == precommit.Id {
		hs.genericQC = prepare.Justify
		if precommit != nil && commit != nil && precommit.Parent == commit.Id {
			hs.lockedQC = precommit.Justify
			if commit != nil && decide != nil && commit.Parent == decide.Id {
				//execute decide
				hs.debugPrint(fmt.Sprintf("Execute Request: id[%s], op[%s]\n", decide.Id, decide.Request.Operation.(string)))
				request := decide.Request
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
	if len(hs.savedMsgs) >= hs.n-hs.f {
		checkVoteMap := make(map[string]int)
		// try to find genericQC
		for _, msg := range hs.savedMsgs {
			if msg.Node.Id != "" {
				node := msg.Node
				checkVoteMap[node.Id]++
				if checkVoteMap[node.Id] > hs.f {
					newQc := QC{}
					newQc.ViewId = node.ViewId
					newQc.NodeId = node.Id
					hs.genericQC = newQc
					hs.newView(hs.viewId + 1)
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

	msg := fmt.Sprintf("=== HotStuff: Change to NewView[%d] ===\n", viewId)
	hs.debugPrint(msg)
	if hs.viewTimer != nil {
		hs.viewTimer.Cancel()
		hs.viewTimer = nil
	}

	if hs.noopTimer != nil {
		hs.noopTimer.Cancel()
		hs.noopTimer = nil
	}

	hs.viewId = viewId
	if hs.isLeader() {
		var highNode *LogNode
		for _, msg := range hs.savedMsgs {
			if msg.QC.NodeId != "" {
				node, ok := hs.nodeMap[msg.QC.NodeId]
				if ok {
					if highNode == nil || node.ViewId > highNode.ViewId {
						highNode = node
					}
				}
			}
		}

		if highNode != nil {
			highQC := highNode.Justify
			if highQC.ViewId > hs.genericQC.ViewId {
				hs.genericQC = highQC
			}
		}

		hs.noopTimer = NewTimerWithCancel(time.Duration(noopTimeOut * time.Millisecond))
		hs.noopTimer.SetTimeout(func() {
			hs.mu.Lock()
			defer hs.mu.Unlock()
			hs.noopTimer = nil

			noopRequset := &RequestArgs{}
			noopRequset.Operation = "noop"
			hs.processClientRequest(noopRequset)
		})
		hs.noopTimer.Start()
	}

	hs.savedMsgs = make(map[int]*MsgArgs)

	hs.viewTimer = NewTimerWithCancel(time.Duration(ViewTimeOut * time.Millisecond))
	hs.viewTimer.SetTimeout(func() {
		hs.mu.Lock()
		defer hs.mu.Unlock()
		hs.viewTimer = nil
		msg := fmt.Sprintf("NewView timeout: rep[%d] oldview[%d]\n", hs.me, hs.viewId)
		hs.debugPrint(msg)

		newViewMsg := &MsgArgs{}
		newViewMsg.ViewId = hs.viewId
		newViewMsg.RepId = hs.me
		newViewMsg.QC = hs.genericQC
		newViewMsg.ParSig = true
		nextLeaderId := (hs.viewId + 1) % hs.n
		hs.sendMsg(nextLeaderId, "Msg", newViewMsg)
		hs.newView(hs.viewId + 1)
	})
	hs.viewTimer.Start()
}

func (hs *HotStuff) getServerInfo() map[string]interface{} {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	info := make(map[string]interface{})
	info["id"] = hs.me
	info["viewId"] = hs.viewId
	info["n"] = hs.n
	info["f"] = hs.f
	info["genericQCId"] = hs.genericQC.NodeId
	info["genericQCView"] = hs.genericQC.ViewId
	info["lockedQCId"] = hs.lockedQC.NodeId
	info["lockedQCView"] = hs.lockedQC.ViewId
	return info
}

func (hs *HotStuff) getRecentNodes() string {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	node := hs.lastNode
	msg := "Recent Node: \n"
	for i := 0; i < 5; i++ {
		if node == nil {
			return msg
		}

		msg += fmt.Sprintf("    nodeId[%s] view[%d] parent[%s] qc[%s]\n", node.Id, node.ViewId, node.Parent, node.Justify.NodeId)
		pId := node.Parent
		node = hs.nodeMap[pId]
	}

	return msg
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
	hs.maliciousMode = NormalMode
	go hs.newView(1)

	hs.debugCh = debugCh
	return hs
}
