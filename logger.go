package pack

import (
	"github.com/fatih/color"
	"io"
	"log"
)

type Logger struct {
	debug  bool
	prefix string
	out    *log.Logger
	err    *log.Logger
}

func NewLogger(stdout, stderr io.Writer, debug, timestamps bool) *Logger {
	flags := 0
	prefix := ""

	if timestamps {
		flags = log.LstdFlags
		prefix = color.HiCyanString("| ")
	}

	return &Logger{
		debug:  debug,
		prefix: prefix,
		out:    log.New(stdout, "", flags),
		err:    log.New(stderr, "", flags),
	}
}

func (l *Logger) printf(log *log.Logger, newline bool, format string, a ...interface{}) {
	ending := ""
	if newline {
		ending = "\n"
	}
	log.Printf(l.prefix+format+ending, a...)
}

func (l *Logger) Info(format string, a ...interface{}) {
	l.printf(l.out, true, format, a...)
}

func (l *Logger) Error(format string, a ...interface{}) {
	errorColor := color.New(color.FgRed, color.Bold).SprintFunc()
	l.printf(l.err, true, errorColor("ERROR: ")+format, a...)
}

func (l *Logger) Debug(format string, a ...interface{}) {
	if l.debug {
		l.printf(l.out, true, format, a...)
	}
}

func (l *Logger) Tip(format string, a ...interface{}) {
	tipColor := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	l.printf(l.out, true, tipColor("Tip: ")+format, a...)
}
