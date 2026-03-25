package app

import "testing"

func TestTopLevelCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default run with flags", args: []string{"-config", "config/codecli-channels.json"}, want: commandRun},
		{name: "explicit run", args: []string{"run", "-config", "config/codecli-channels.json"}, want: commandRun},
		{name: "weixin", args: []string{"weixin", "setup"}, want: commandWeixin},
		{name: "help", args: []string{"help"}, want: commandHelp},
		{name: "unknown", args: []string{"unknown"}, want: commandUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := topLevelCommand(tt.args)
			if got != tt.want {
				t.Fatalf("topLevelCommand(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
