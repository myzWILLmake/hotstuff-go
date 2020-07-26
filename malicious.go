package hotstuff

import "errors"

func (hs *HotStuff) setMaliciousMode(maliciousMode int) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if maliciousMode < 0 || maliciousMode > MaliciousMode {
		return errors.New("Invalid malicious mode")
	}

	hs.maliciousMode = MaliciousBehaviorMode(maliciousMode)
	return nil
}

func (hs *HotStuff) sendMaliciousMsg(id int, rpcname string, rpcacgs interface{}) {
	args := rpcacgs.(*MsgArgs)
	if !args.ParSig {
		// From Leader
		hs.rawSendMsg(id, rpcname, args)
	} else {
		// To Leader
		// Generic message
		if args.Node.Id != "" {
			fakeReq := &RequestArgs{}
			fakeReq.ClientId = 1
			fakeReq.Operation = "fakeop"
			newId := getLogNodeId(hs.viewId, fakeReq)
			newIdwithColor := "\033[0;31m" + newId + "\033[0m"
			args.Node.Id = newIdwithColor
			args.Node.Request = *fakeReq
			hs.rawSendMsg(id, rpcname, args)
		} else {
			hs.rawSendMsg(id, rpcname, args)
		}
	}
}
