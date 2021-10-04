package background

import (
	"testing"
	"time"
)

func Test_jiraTimeFormat(t *testing.T) {
	type args struct {
		t time.Time
	}
	t1, err := time.Parse(time.RFC3339, "2021-10-03T18:16:51.611Z")
	if err != nil {
		t.Error(err)
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "zero",
			args: args{
				t: time.Time{},
			},
			want: "0001-01-01 00:00",
		},
		{
			name: "t1",
			args: args{
				t: t1,
			},
			want: "2021-10-03 18:16",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jiraTimeFormat(tt.args.t); got != tt.want {
				t.Errorf("jiraTimeFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}
