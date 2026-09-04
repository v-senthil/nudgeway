package audit_test

import (
	"testing"

	"github.com/v-senthil/nudgeway/internal/domain/audit"
)

func TestActionConstants(t *testing.T) {
	cases := map[audit.Action]string{
		audit.IntegrationCreated:     "integration.created",
		audit.IntegrationDeleted:     "integration.deleted",
		audit.IntegrationTested:      "integration.tested",
		audit.MessageSent:            "message.sent",
		audit.MessageMarkedRead:      "message.marked_read",
		audit.ConversationMarkedRead: "conversation.marked_read",
		audit.AttachmentUploaded:     "attachment.uploaded",
		audit.UserLoggedIn:           "user.logged_in",
		audit.UserLoggedOut:          "user.logged_out",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("action mismatch: got %q want %q", got, want)
		}
	}
}
