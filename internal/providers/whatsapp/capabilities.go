package whatsapp

import "github.com/v-senthil/nudgeway/internal/ports/channel"

// Capabilities reports what this adapter supports. Groups + Calls came online
// with Phase 2's Templates / Groups / Calling rounds. Calling capabilities
// (initiate / answer / reject / terminate / recording / transcription) are
// reported separately by CallingCapabilities() and via the CallingProvider()
// wrapper's calling.Capabilities method.
func (p *Provider) Capabilities() channel.Capabilities {
	return channel.Capabilities{
		SendText:        true,
		SendMedia:       true,
		SendTemplate:    true,
		ReceiveMessages: true,
		Templates:       true,
		Groups:          true,
		Calls:           true,
		Flows:           true,
	}
}
