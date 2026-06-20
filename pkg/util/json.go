// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package util

import "encoding/json"

func ToObjectViaJson[T any](from any, to *T) (*T, error) {
	if bytes, err := json.Marshal(from); err == nil {
		if err = json.Unmarshal(bytes, &to); err != nil {
			return nil, err
		} else {
			return to, nil
		}
	} else {
		return nil, err
	}
}
