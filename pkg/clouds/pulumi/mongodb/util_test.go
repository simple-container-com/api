package mongodb

import "testing"

func Test_appendUserPasswordToMongoUri(t *testing.T) {
	type args struct {
		mongoUri string
		user     string
		password string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "happy-path",
			args: args{
				mongoUri: "mongodb://ac-gvptdqa-shard-00-00.qo081kw.mongodb.net:27017,ac-gvptdqa-shard-00-01.qo081kw.mongodb.net:27017,ac-gvptdqa-shard-00-02.qo081kw.mongodb.net:27017/?param1=value1&param2=value2",
				user:     "test-user",
				password: "test-password",
			},
			want: "mongodb://test-user:test-password@ac-gvptdqa-shard-00-00.qo081kw.mongodb.net:27017,ac-gvptdqa-shard-00-01.qo081kw.mongodb.net:27017,ac-gvptdqa-shard-00-02.qo081kw.mongodb.net:27017/?param1=value1&param2=value2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendUserPasswordToMongoUri(tt.args.mongoUri, tt.args.user, tt.args.password); got != tt.want {
				t.Errorf("appendUserPasswordToMongoUri() = %v, want %v", got, tt.want)
			}
		})
	}
}
