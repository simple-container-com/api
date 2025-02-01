package k8s

import "testing"

func Test_toMebibytesFormat(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{
			name: "bytes",
			size: 100,
			want: "100",
		},
		{
			name: "kilobytes",
			size: 100 * 1024,
			want: "100K",
		},
		{
			name: "megabytes",
			size: 100 * 1024 * 1024,
			want: "100M",
		},
		{
			name: "gigabytes",
			size: 100 * 1024 * 1024 * 1024,
			want: "100G",
		},
		{
			name: "terabytes",
			size: 100 * 1024 * 1024 * 1024 * 1024,
			want: "100T",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesSizeToHuman(tt.size); got != tt.want {
				t.Errorf("bytesSizeToHuman() = %v, want %v", got, tt.want)
			}
		})
	}
}
