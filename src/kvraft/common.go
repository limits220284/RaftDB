package kvraft

import "time"

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
	ErrTimeout     = "ErrTimeout"
)

const (
	ChangeLeaderPeriods = 20 * time.Millisecond
	WaitCmdTimeOut      = 3000 * time.Millisecond
	CheckPeriods        = 20 * time.Millisecond //检查频率
)

type Err string

// Put or Append
type PutAppendArgs struct {
	ClientId int
	Seq      int
	Key      string
	Value    string
	Op       string // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key      string
	Seq      int
	ClientId int
	// You'll have to add definitions here.
}

type GetReply struct {
	Err   Err
	Value string
}
