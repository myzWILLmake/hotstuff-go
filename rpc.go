package hotstuff

import "fmt"

func (hs *HotStuff) Request(args *RequestArgs, reply *DefaultReply) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	// TODO: only one client request should be serviced
	msg := fmt.Sprintf("Recieve Request: id[%d] op[%s] time[%d]\n", args.ClientId, args.Operation.(string), args.Timestamp)
	hs.debugPrint(msg)
	if hs.isLeader() {
		hs.processClientRequest(args)
		if hs.noopTimer != nil {
			hs.noopTimer.Cancel()
			hs.noopTimer = nil
		}
	}

	return nil
}

func (hs *HotStuff) Msg(args *MsgArgs, reply *DefaultReply) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if !args.ParSig {
		// From Leader
		msg := fmt.Sprintf("\033[1;36mReceive Msg From Leader:\033[0m rid[%d] viewId[%d] nodeId[%s]\n", args.RepId, args.ViewId, args.Node.Id)
		hs.debugPrint(msg)
		if args.ViewId%hs.n != args.RepId {
			reply.Err = fmt.Sprintf("Generic msg from invalid leader[%d].\n", args.RepId)
			return nil
		}

		if args.ViewId > hs.viewId {
			// TODO: check threshold signature
			hs.newView(args.ViewId)
		}

		if args.ViewId != hs.viewId {
			reply.Err = fmt.Sprintf("Generic msg from invalid viewId[%d].\n", args.ViewId)
			return nil
		}

		hs.update(&args.Node)
	} else {
		// To Leader
		msg := fmt.Sprintf("\033[1;36mReceive Msg to Leader:\033[0m rid[%d] viewId[%d] nodeId[%s] qcId[%s]\n", args.RepId, args.ViewId, args.Node.Id, args.QC.NodeId)
		hs.debugPrint(msg)
		// if args.ViewId != hs.viewId {
		// 	reply.Err = fmt.Sprintf("Vote msg from invalid viewId[%d].\n", args.ViewId)
		// 	return nil
		// }

		if hs.isNextLeader() {
			hs.savedMsgs[args.RepId] = args
			hs.processSavedMsgs()
		}
	}

	return nil
}

func (c *Client) Reply(args *ReplyArgs, reply *DefaultReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debugPrint(fmt.Sprintf("Received Reply[%d, %s, %d] from ReplicaId[%d]\n", args.Timestamp, args.Result, args.ViewId, args.ReplicaId))
	c.saveReply(args)
	c.processReplies(args.Timestamp)
	return nil
}
