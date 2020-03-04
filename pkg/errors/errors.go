package errors

import (
	"fmt"
	"strings"
)

type kvErr struct {
	cause         error
	keysAndValues []interface{}
	message       string
}

func New(cause error, message string, keysAndValues ...interface{}) *kvErr {
	return &kvErr{
		cause:         cause,
		message:       message,
		keysAndValues: keysAndValues,
	}
}

func (e *kvErr) Error() string {
	var kvs []string
	// copy-and-paste logic from zap logger
	for i := 0; i < len(e.keysAndValues); {
		if i == len(e.keysAndValues)-1 {
			// odd number of elements; ignore this one
			break
		}

		// Consume this value and the next, treating them as a key-value pair
		key, val := e.keysAndValues[i], e.keysAndValues[i+1]
		kvs = append(kvs, fmt.Sprintf("%v=%v", key, val))
		i += 2
	}
	var b strings.Builder
	b.WriteString(e.message)
	if len(kvs) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(kvs, " "))
	}
	msg := b.String()
	if e.cause != nil {
		return fmt.Sprintf("%s: %s", msg, e.cause.Error())
	}
	return msg
}
