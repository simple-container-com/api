// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package mongodb

import (
	"strings"
	"testing"
)

func Test_appendUserPasswordToMongoUri(t *testing.T) {
	type args struct {
		mongoUri string
		user     string
		password string
		dbName   string
	}
	tests := []struct {
		name string
		args args
		// expected substrings — we verify by string surgery because the
		// resulting multi-host MongoDB URI is intentionally not a valid
		// RFC 3986 URL under Go 1.26's `net/url.Parse`.
		wantScheme string
		wantHosts  string
		wantQuery  string
	}{
		{
			name: "happy-path",
			args: args{
				mongoUri: "mongodb://shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017/?param1=value1&param2=value2",
				user:     "test-user",
				password: "test-password",
				dbName:   "test-db",
			},
			wantScheme: "mongodb",
			wantHosts:  "shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017",
			wantQuery:  "?param1=value1&param2=value2",
		},
		{
			name: "happy-path mongodb+srv",
			args: args{
				mongoUri: "mongodb+srv://shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017/?param1=value1&param2=value2",
				user:     "test-user",
				password: "test-password",
				dbName:   "test-db",
			},
			wantScheme: "mongodb+srv",
			wantHosts:  "shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017",
			wantQuery:  "?param1=value1&param2=value2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppendUserPasswordAndDBToMongoUri(tt.args.mongoUri, tt.args.user, tt.args.password, tt.args.dbName)

			wantPrefix := tt.wantScheme + "://" + tt.args.user + ":" + tt.args.password + "@" + tt.wantHosts + "/" + tt.args.dbName
			if !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("uri prefix mismatch:\n got:  %q\n want: %q (as prefix)", got, wantPrefix)
			}
			if !strings.HasSuffix(got, tt.wantQuery) {
				t.Errorf("query suffix mismatch:\n got:  %q\n want: %q (as suffix)", got, tt.wantQuery)
			}
		})
	}
}
