package raft

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

type Timer struct {
	elapsedTime time.Duration
	lastTime    time.Time
}

func (t *Timer) String() string {
	return fmt.Sprintf("{elapsedTime:%v}", t.elapsedTime)
}

func (t *Timer) elapsed() {
	t.elapsedTime += time.Now().Sub(t.lastTime)
	t.lastTime = time.Now()
}

func (t *Timer) isTimeOut(timeOut time.Duration) bool {
	return t.elapsedTime > timeOut
}

func (t *Timer) reset() {
	t.elapsedTime = 0
	t.lastTime = time.Now()
}

// Debugging
const debug = false
const info = false
const trace = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if debug {
		log.Printf(format, a...)
	}
	return
}

type logTopic string

const (
	dClient  logTopic = "CLNT"
	dCommit  logTopic = "CMIT"
	dDrop    logTopic = "DROP"
	dError   logTopic = "ERRO"
	dInfo    logTopic = "INFO"
	dLeader  logTopic = "LEAD"
	dLog     logTopic = "LOG1"
	dLog2    logTopic = "LOG2"
	dPersist logTopic = "PERS"
	dSnap    logTopic = "SNAP"
	dTerm    logTopic = "TERM"
	dTest    logTopic = "TEST"
	dTimer   logTopic = "TIMR"
	dTrace   logTopic = "TRCE"
	dVote    logTopic = "VOTE"
	dWarn    logTopic = "WARN"
)

var enableTopic = []logTopic{dLeader, dTest, dSnap, dPersist, dTrace}

// Retrieve the verbosity level from an environment variable
func getVerbosity() int {
	v := os.Getenv("VERBOSE")
	level := 0
	if v != "" {
		var err error
		level, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("Invalid verbosity %v", v)
		}
	}
	return level
}

var debugStart time.Time
var debugVerbosity int

func init() {
	debugVerbosity = getVerbosity()
	debugStart = time.Now()

	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
}

func Debug(topic logTopic, format string, a ...interface{}) {
	if debug {
		time := time.Since(debugStart).Microseconds()
		time /= 100
		prefix := fmt.Sprintf("%06d %v ", time, string(topic))
		format = prefix + format
		log.Printf(format, a...)
	}
}

func Info(topic logTopic, format string, a ...interface{}) {
	if info {
		time := time.Since(debugStart).Microseconds()
		time /= 100
		prefix := fmt.Sprintf("%06d %v ", time, string(topic))
		format = prefix + format
		log.Printf(format, a...)
	}
}

func Trace(topic logTopic, format string, a ...interface{}) {
	if trace {
		time := time.Since(debugStart).Microseconds()
		time /= 100
		prefix := fmt.Sprintf("%06d %v ", time, string(topic))
		format = prefix + format
		log.Printf(format, a...)
	}
}
