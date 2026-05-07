package mongodb

import (
	"net/url"
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
	}{
		{
			name: "happy-path",
			args: args{
				mongoUri: "mongodb://shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017/?param1=value1&param2=value2",
				user:     "test-user",
				password: "test-password",
				dbName:   "test-db",
			},
		},
		{
			name: "happy-path mongodb+srv",
			args: args{
				mongoUri: "mongodb+srv://shard-00-00.example.com:27017,shard-00-01.example.com:27017,shard-00-02.example.com:27017/?param1=value1&param2=value2",
				user:     "test-user",
				password: "test-password",
				dbName:   "test-db",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppendUserPasswordAndDBToMongoUri(tt.args.mongoUri, tt.args.user, tt.args.password, tt.args.dbName)

			gotURL, err := url.Parse(got)
			if err != nil {
				t.Fatalf("AppendUserPasswordAndDBToMongoUri returned a value that failed to parse: %v", err)
			}
			inputURL, err := url.Parse(tt.args.mongoUri)
			if err != nil {
				t.Fatalf("test setup: input URI did not parse: %v", err)
			}

			if gotURL.Scheme != inputURL.Scheme {
				t.Errorf("scheme mutated: got %q, want %q", gotURL.Scheme, inputURL.Scheme)
			}
			if gotURL.Host != inputURL.Host {
				t.Errorf("host mutated: got %q, want %q", gotURL.Host, inputURL.Host)
			}
			if gotURL.RawQuery != inputURL.RawQuery {
				t.Errorf("query mutated: got %q, want %q", gotURL.RawQuery, inputURL.RawQuery)
			}
			if got, want := gotURL.User.Username(), tt.args.user; got != want {
				t.Errorf("username: got %q, want %q", got, want)
			}
			if pw, ok := gotURL.User.Password(); !ok || pw != tt.args.password {
				t.Errorf("password: got %q (set=%v), want %q", pw, ok, tt.args.password)
			}
			if got, want := gotURL.Path, "/"+tt.args.dbName; got != want {
				t.Errorf("path: got %q, want %q", got, want)
			}
		})
	}
}
