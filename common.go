package hotstuff

import (
	"strconv"
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
	Id      string
	Parent  string
	ViewId  int
	Request RequestArgs
	Justify QC
}

func getLogNodeId(viewId int, request *RequestArgs) string {
	s := strconv.Itoa(viewId) + "_" + request.Operation.(string)
	// h := sha1.New()
	// h.Write([]byte(s))
	// bs := h.Sum(nil)
	// ns := fmt.Sprintf("%x", bs)
	// return ns[:8]
	return s
}

type QC struct {
	ViewId int
	NodeId string
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
	QC     QC
	// just a mark for partial signature
	// TODO: implement partial signature
	ParSig bool
}

type MaliciousBehaviorMode int

const (
	NormalMode = iota
	CrashedLike
	MaliciousMode
	PartialMaliciousMode
)
