package whatsapp

import "github.com/fullwa/fullwa/internal/ports/channel"

// Capabilities reports what this adapter supports. Groups + Calls stay off
// until dedicated phases (calls are Phase 3).
func (p *Provider) Capabilities() channel.Capabilities {
	return channel.Capabilities{
		SendText:        true,
		SendMedia:       true,
		SendTemplate:    true,
		ReceiveMessages: true,
		Templates:       true,
		Groups:          false,
		Calls:           false,
		Flows:           true,
	}
}
