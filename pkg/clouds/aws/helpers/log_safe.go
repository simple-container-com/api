// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// logSafeReplacer strips CR/LF from a string. CodeQL's `go/log-injection`
// query recognises strings.Replacer / ReplaceAll on \n and \r as a
// sanitization sink, so wrapping a logged value through this drops the
// taint trace from the lambda event payload to the log call.
var logSafeReplacer = strings.NewReplacer("\n", " ", "\r", " ")

// sanitizeForLog serialises an arbitrary AWS lambda event payload into a
// single-line, log-injection-safe string. json.Marshal already escapes
// embedded control characters within string fields (\n → \\n inside
// quoted JSON); the residual Replacer call defends against pathological
// cases (e.g. encoding.TextMarshaler implementations returning raw
// control bytes) and is what CodeQL pattern-matches as a sanitizer.
func sanitizeForLog(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T(<unmarshallable>)", v)
	}
	return logSafeReplacer.Replace(string(b))
}
