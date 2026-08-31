// Package jsonstrict contains reusable checks that encoding/json does not
// provide by default for signed or network protocol payloads.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RejectDuplicateKeys rejects duplicate object keys at every nesting level.
// This keeps signed and protocol JSON deterministic instead of accepting the
// last duplicate value silently.
func RejectDuplicateKeys(payload []byte) error {
	type frame struct {
		kind         byte
		keys         map[string]struct{}
		expectingKey bool
	}
	frames := make([]frame, 0, 2)
	decoder := json.NewDecoder(bytes.NewReader(payload))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if len(frames) != 0 {
				return errors.New("unterminated JSON container")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("scan JSON: %w", err)
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				frames = append(frames, frame{kind: '{', keys: make(map[string]struct{}), expectingKey: true})
			case '[':
				frames = append(frames, frame{kind: '['})
			case '}', ']':
				if len(frames) == 0 {
					return errors.New("unexpected JSON container close")
				}
				top := frames[len(frames)-1]
				if (delimiter == '}' && top.kind != '{') || (delimiter == ']' && top.kind != '[') {
					return errors.New("mismatched JSON container")
				}
				if top.kind == '{' && !top.expectingKey {
					return errors.New("JSON object is missing a value")
				}
				frames = frames[:len(frames)-1]
				if len(frames) > 0 && frames[len(frames)-1].kind == '{' {
					frames[len(frames)-1].expectingKey = true
				}
			}
			continue
		}
		if len(frames) == 0 {
			continue
		}
		top := &frames[len(frames)-1]
		if top.kind != '{' {
			continue
		}
		if top.expectingKey {
			key, ok := token.(string)
			if !ok {
				return errors.New("JSON object key must be a string")
			}
			if _, exists := top.keys[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			top.keys[key] = struct{}{}
			top.expectingKey = false
		} else {
			top.expectingKey = true
		}
	}
}
