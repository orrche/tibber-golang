//go:build tibberpush
// +build tibberpush

package tibber

import "testing"

func TestPush(t *testing.T) {
	token := string(helperLoadBytes(t, "token.txt"))
	tc := NewClient(token)
	_, err := tc.SendPushNotification("Golang Test", "Running golang test")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
}
