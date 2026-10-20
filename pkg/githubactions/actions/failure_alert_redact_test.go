// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A failed Pulumi run returns its whole stdout in the error, and the failure
// alert goes to chat; the key in a preview diff must not go with it, wherever
// the truncation of a long error cuts it.
func TestFailureMessageRedactsPrivateKeys(t *testing.T) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	body := strings.Split(key, "\n")[5]
	for _, pad := range []int{0, 300, 2000} {
		runErr := fmt.Errorf("failed to run update: exit status 255\ncode: 255\nstdout: %s  ~ credentials: %q\nResources: 1 to update\nstderr: ", strings.Repeat("~ resource\n", pad/10), key)
		msg := (&Executor{}).getFailureMessage(OperationConfig{Type: OperationDeploy, StackName: "app"}, runErr, time.Second)
		if strings.Contains(msg, body[4:24]) {
			t.Errorf("pad %d: the alert carries key material:\n%s", pad, msg)
		}
	}
}
