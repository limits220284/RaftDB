package raft

import (
	"fmt"
	"time"
)

type AppendEntriesArgs struct {
	Term         int         // leader's Term
	LeaderId     int         // so Follower can redirct clients
	PrevLogIndex int         // index of log entry immediately preceding new ones
	PrevLogTerm  int         // term of PrevLogIndex
	Entries      []*LogEntry // log entries to store (empty for heartBeat may send more than one for efficiency)
	LeaderCommit int         // leader's commitIndex
}

type AppendEntriesReply struct {
	Term          int
	Success       bool
	ConflictIndex int // Success == false a log that may occure confict
}

type InstallSnapshotArgs struct {
	Term              int // leader's Term
	LeaderId          int // so Follower can redirct clients
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
	Done              bool
}

func (args *InstallSnapshotArgs) String() string {
	return fmt.Sprintf("{Term:%v, LeaderId:%v, LastIncludedIndex:%v,LastIncludedTerm:%v}",
		args.Term, args.LeaderId, args.LastIncludedIndex, args.LastIncludedTerm)
}

func (rf *Raft) heartBeate(server int) {
	rf.mu.Lock()
	if rf.role == ILeader {
		args := rf.newAppendEntriesArgs(server)
		go rf.sendAppendEntries(server, args)
	}
	rf.mu.Unlock()
}

func (rf *Raft) heartBeater(server int) {
	heartBeateTimer := time.NewTimer(HeartBeateTimeout)
	defer heartBeateTimer.Stop()
	for rf.killed() == false {
		select {
		case <-rf.notifyStopCh:
			return
		case <-heartBeateTimer.C:
			rf.notify(rf.notifyHeartBeateCh[server])
		case <-rf.notifyHeartBeateCh[server]:
			rf.heartBeate(server)
			heartBeateTimer.Reset(HeartBeateTimeout)
		}
	}
}

func (rf *Raft) initHeartBeater() {
	for server, _ := range rf.peers {
		if server == rf.me {
			continue
		}
		go rf.heartBeater(server)
	}
}

func (rf *Raft) toLeader() {
	rf.role = ILeader
	for i := 0; i < len(rf.nextIndex); i++ {
		rf.nextIndex[i] = rf.log.last().Index + 1
		rf.matchIndex[i] = 0
	}
	// send initial empty AppendEntries RPC
	for server, _ := range rf.peers {
		if server == rf.me {
			continue
		}
		rf.notify(rf.notifyHeartBeateCh[server])
	}

}

func (rf *Raft) newAppendEntriesArgs(server int) (args *AppendEntriesArgs) {
	var prevLogIndex, prevLogTerm int
	var entries []*LogEntry
	if rf.nextIndex[server] <= rf.log.LastIncludedIndex {
		prevLogIndex = rf.log.LastIncludedIndex
		prevLogTerm = rf.log.LastIncludedTerm
		entries = nil
	} else {
		prevLogIndex = rf.nextIndex[server] - 1
		prevLogTerm = rf.log.get(prevLogIndex).Term
		entries = rf.log.after(prevLogIndex)
	}
	args = &AppendEntriesArgs{
		Term:         rf.currentTerm,
		LeaderId:     rf.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: rf.commitIndex,
	}
	return args
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs) (ok bool, reply *AppendEntriesReply) {
	reply = &AppendEntriesReply{}
	Trace(dTrace, "[%v]sendAppendEntries[%v] %+v %+v %+v", rf.me, server, args)
	ok = rf.peers[server].Call("Raft.AppendEntries", args, reply)
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// deal the rpc response
	if ok {
		// public check
		if reply.Term > rf.currentTerm {
			rf.currentTerm = reply.Term
			rf.role = IFollower
			rf.votedFor = -1
			rf.persist()
			return
		}

		// overdate message || self status have changed
		if rf.currentTerm != args.Term || rf.role != ILeader {
			return
		}

		// begin to deal the response
		// 1. will receive many response from rpc, repeat and out of order
		// This means that the state of rf.nextIndex and rf.matchIndex may have been changed. There are several ways to handle this:
		// Idempotence: The modification of rf.nextIndex and rf.matchIndex takes values from the original request parameters.
		// Choose the optimal value: First, check if the data has been modified by other reply routines, and if so, compare and choose the optimal value.
		// Currently, the first method is adopted as it is simple and easy to understand.
		if reply.Success {
			rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
			rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
			rf.notify(rf.notifyCommitCh)
		} else {
			rf.nextIndex[server] = reply.ConflictIndex + 1
			// retry
			rf.notify(rf.notifyLogReplicateCh[server])
			// next check to retry
			// retryArgs := rf.newAppendEntriesArgs(server)
			// go rf.sendAppendEntries(server, retryArgs)
		}
	}
	return ok, reply
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer Trace(dTrace, "[%v]AppendEntries[%v] %+v %+v %+v", rf.me, args.LeaderId, args, reply, rf)

	// 公共检查
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.role = IFollower
		rf.votedFor = args.LeaderId
		rf.persist()
	}

	if args.Term < rf.currentTerm {
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	}

	rf.electionTimer.reset()
	rf.role = IFollower
	rf.votedFor = args.LeaderId
	rf.persist()

	prevLog := rf.log.get(args.PrevLogIndex)

	// success == false
	if prevLog == nil {
		reply.ConflictIndex = rf.log.last().Index
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	}
	// success == false
	if prevLog.Term != args.PrevLogTerm {
		i := args.PrevLogIndex
		// 跳过一个 term,因为当前PrevLogTerm的所有log都是冲突的
		for i = args.PrevLogIndex; i > rf.log.LastIncludedIndex; i-- {
			if rf.log.get(i).Term != prevLog.Term {
				break
			}
		}
		reply.ConflictIndex = i
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	}

	//Debug(dInfo, "[%v] AppendEntries %+v %+v %+v", rf.me, prevLog, args)

	// success == true
	rf.log.rewrite(args.PrevLogIndex+1, args.Entries)
	rf.persist()
	if args.LeaderCommit > rf.commitIndex {
		rf.commitIndex = min(args.LeaderCommit, rf.log.last().Index)
		//通知commit
		rf.notify(rf.notifyCommitCh)
	}

	reply.Success = true
	reply.Term = rf.currentTerm

	return
}

type InstallSnapshotReply struct {
	Term int
}

func (rf *Raft) LogReplicate(server int) {
	rf.mu.Lock()
	if rf.role == ILeader {
		if rf.nextIndex[server] <= rf.log.LastIncludedIndex {
			// install snapshot
			args := rf.newInstallSnapshotArgs()
			go rf.sendInstallSnapshot(server, args)
		} else if rf.log.last().Index >= rf.nextIndex[server] {
			// AppendEntries
			args := rf.newAppendEntriesArgs(server)
			go rf.sendAppendEntries(server, args)
		}
	}
	rf.mu.Unlock()
}

func (rf *Raft) LogReplicator(server int) {
	checkTimer := time.NewTimer(CheckPeriods)
	defer checkTimer.Stop()
	for rf.killed() == false {
		select {
		case <-rf.notifyStopCh:
			return
		case <-checkTimer.C:
			rf.notify(rf.notifyLogReplicateCh[server])
		case <-rf.notifyLogReplicateCh[server]:
			rf.LogReplicate(server)
			checkTimer.Reset(CheckPeriods)
		}
	}
}

func (rf *Raft) initLogReplicator() {
	for server, _ := range rf.peers {
		if server == rf.me {
			continue
		}
		go rf.LogReplicator(server)
	}
}

func (rf *Raft) commit() {
	rf.mu.Lock()

	if rf.role == ILeader {
		hasCommit := false
		for N := rf.commitIndex + 1; N <= rf.log.last().Index; N++ {
			majority := len(rf.peers)/2 + 1
			counter := 1
			for i, _ := range rf.peers {
				if i == rf.me {
					continue
				}
				if rf.matchIndex[i] >= N {
					counter++
				}
			}

			if counter >= majority && rf.log.get(N).Term == rf.currentTerm {
				rf.commitIndex = N
				hasCommit = true
				// Debug(dCommit, "[%v]commitIndex %+v", rf.me, rf)
			}
		}
		// 通知apply
		if hasCommit {
			rf.notify(rf.notifyApplyCh)
		}
	}
	rf.mu.Unlock()

}

func (rf *Raft) committer() {
	checkTimer := time.NewTimer(CheckPeriods)
	defer checkTimer.Stop()
	for rf.killed() == false {
		select {
		case <-rf.notifyStopCh:
			return
		case <-checkTimer.C:
			rf.notify(rf.notifyCommitCh)
		case <-rf.notifyCommitCh:
			rf.commit()
			checkTimer.Reset(CheckPeriods)
		}
	}
}
