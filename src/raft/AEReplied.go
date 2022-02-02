package raft

import "log"

type AEReplied struct {
	server            int
	count             int
	success           bool
	conflictPrevTerm  int
	conflictPrevIndex int
	log               *LogStateMachine
}

func (trans *AEReplied) getName() string {
	return "AEReplied"
}

func (trans *AEReplied) isRW() bool {
	return true
}

func (rf *Raft) makeAEReplied(server int, count int, success bool, conflictPrevTerm int, conflictPrevIndex int) *AEReplied {
	return &AEReplied{
		server:            server,
		count:             count,
		success:           success,
		conflictPrevTerm:  conflictPrevTerm,
		conflictPrevIndex: conflictPrevIndex,
		log:               rf.log,
	}
}

func (trans *AEReplied) doSuccess() {
	// If successful: update nextIndex and matchIndex for follower
	trans.log.raft.machine.rwmu.RLock()
	trans.log.raft.print("increment %d follower nextIndex by %d", trans.server, trans.count)
	trans.log.raft.machine.rwmu.RUnlock()
	trans.log.nextIndex[trans.server] += trans.count
	trans.log.matchIndex[trans.server] = trans.log.nextIndex[trans.server] - 1
	trans.log.tryCommit()
}

func (trans *AEReplied) doFailed() {
	// If AppendEntries fails because of log inconsistency:
	// decrement nextIndex and retry
	//trans.log.nextIndex[trans.server]--

	// optimized method
	conflictPrevIndex := trans.log.backTrackLogTerm(trans.conflictPrevTerm)
	conflictReplyIndex := trans.conflictPrevIndex
	if conflictReplyIndex < conflictPrevIndex {
		trans.log.nextIndex[trans.server] = conflictReplyIndex + 1
	} else {
		trans.log.nextIndex[trans.server] = conflictPrevIndex + 1
	}
	trans.log.raft.machine.rwmu.RLock()
	trans.log.raft.print("log rejected by %d, try again on nextIndex %d next cycle", trans.server, trans.log.nextIndex[trans.server])
	trans.log.raft.machine.rwmu.RUnlock()

}

func (trans *AEReplied) transfer(source SMState) SMState {
	if source != logNormalState {
		log.Fatalln("log not at normal state")
	}
	if trans.success {
		trans.doSuccess()
	} else {
		trans.doFailed()
	}
	trans.log.raft.machine.rwmu.RLock()
	trans.log.raft.print("nextIndex %v", trans.log.nextIndex)
	trans.log.raft.machine.rwmu.RUnlock()
	return notTransferred
}

func (sm *LogStateMachine) tryCommit() {
	Ntemp := sm.commitIndex + 1
	if Ntemp > sm.lastLogIndex() {
		return
	}

	sm.raft.machine.rwmu.RLock()
	defer sm.raft.machine.rwmu.RUnlock()

	oldCommit := sm.commitIndex

	for {
		agreeCount := 0
		for i := 0; i < sm.raft.peerCount(); i++ {
			if i == sm.raft.me {
				continue
			}
			if sm.matchIndex[i] >= Ntemp && sm.getEntry(Ntemp).Term == sm.raft.machine.currentTerm {
				agreeCount++
			}
		}
		if agreeCount+1 > sm.raft.peerCount()/2 {
			sm.commitIndex = Ntemp
		}
		Ntemp++
		if Ntemp > sm.lastLogIndex() {
			break
		}
	}
	if sm.commitIndex > oldCommit {
		sm.raft.print("commitIndex updated")
		//sm.issueTransfer(sm.raft.makeApplyNew())
		sm.tryApply()
	}
}
