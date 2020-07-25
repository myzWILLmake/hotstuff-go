package hotstuff

import (
	"crypto/sha1"
	"time"
)

type TimerWithCancel struct {
	d time.Duration
	t *time.Timer
	c chan interface{}
	f func()
}

func NewTimerWithCancel(d time.Duration) *TimerWithCancel {
	t := &TimerWithCancel{}
	t.d = d
	t.c = make(chan interface{})
	return t
}

func (t *TimerWithCancel) Start() {
	t.t = time.NewTimer(t.d)
	go func() {
		select {
		case <-t.t.C:
			t.f()
		case <-t.c:
		}
	}()
}

func (t *TimerWithCancel) SetTimeout(f func()) {
	t.f = f
}

func (t *TimerWithCancel) Cancel() {
	t.c <- nil
}

type LogNode struct {
	id      string
	parent  string
	viewId  int
	request RequestArgs
	justify QC
}

func getLogNodeId(viewId int, request *RequestArgs) string {
	s := string(viewId) + " " + request.Operation.(string)
	h := sha1.New()
	h.Write([]byte(s))
	bs := h.Sum(nil)
	return string(bs)[:8]
}

type QC struct {
	viewId int
	nodeId string
	//TODO: threshold signatures
}

type DefaultReply struct {
	Err string
}

type RequestArgs struct {
	Operation interface{}
	Timestamp int64
	ClientId  int
}

type ReplyArgs struct {
	ViewId    int
	Timestamp int64
	ReplicaId int
	Result    interface{}
}

type MsgArgs struct {
	RepId  int
	ViewId int
	Node   LogNode
	Qc     QC
	// just a mark for partial signature
	// TODO: implement partial signature
	ParSig bool
}
