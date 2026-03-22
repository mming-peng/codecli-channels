package channel

import "testing"

func TestConversationScopeKey(t *testing.T) {
	scope := ConversationScope{
		Key:      "qq-default:user:u1",
		Kind:     "dm",
		ChatID:   "u1",
		UserID:   "u1",
		ThreadID: "",
	}
	if scope.Key != "qq-default:user:u1" {
		t.Fatalf("unexpected key: %s", scope.Key)
	}
	if scope.Kind != "dm" {
		t.Fatalf("unexpected kind: %s", scope.Kind)
	}
}
