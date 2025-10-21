package apikeysservice

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
)

func Test_mustGetApikeySalt(t *testing.T) {
	tests := []struct {
		name string
		salt string
		want string
	}{
		{
			name: "Test salt",
			salt: "salt",
			want: "salt",
		},
	}
	for _, tt := range tests {
		rorconfig.Set(rorconfig.ROR_API_KEY_SALT, tt.salt)
		t.Run(tt.name, func(t *testing.T) {
			if got := mustGetApikeySalt(); got != tt.want {
				t.Errorf("mustGetApikeySalt() = %v, want %v", got, tt.want)
			}
		})
	}
	// test panic
	rorconfig.Set(rorconfig.ROR_API_KEY_SALT, "")
	defer func() { _ = recover() }()
	mustGetApikeySalt()
	t.Errorf("did not panic")

}
