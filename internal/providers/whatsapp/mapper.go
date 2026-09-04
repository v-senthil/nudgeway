package whatsapp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/message"
	"github.com/v-senthil/nudgeway/internal/ports/channel"
)

// ---------- Meta wire types (adapter-local; NEVER leak out of this package) ----------

type metaWebhookEnvelope struct {
	Object string      `json:"object"`
	Entry  []metaEntry `json:"entry"`
}

type metaEntry struct {
	ID      string       `json:"id"`
	Changes []metaChange `json:"changes"`
}

type metaChange struct {
	Field string    `json:"field"`
	Value metaValue `json:"value"`
}

type metaValue struct {
	MessagingProduct string              `json:"messaging_product,omitempty"`
	Metadata         *metaValueMetadata  `json:"metadata,omitempty"`
	Contacts         []metaInboundContact `json:"contacts,omitempty"`
	Messages         []metaInboundMessage `json:"messages,omitempty"`
	Statuses         []metaStatus         `json:"statuses,omitempty"`
	Errors           []metaError          `json:"errors,omitempty"`
}

type metaValueMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number,omitempty"`
	PhoneNumberID      string `json:"phone_number_id,omitempty"`
}

type metaInboundContact struct {
	Profile struct {
		Name     string `json:"name"`
		Username string `json:"username,omitempty"`
	} `json:"profile"`
	WaID   string `json:"wa_id"`
	// UserID is the business-scoped user ID (BSUID) Meta ships alongside
	// (and will eventually replace) wa_id. Format: <CC>.<up to 128 alnum>.
	// See ~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md.
	UserID string `json:"user_id,omitempty"`
	// ParentUserID is the parent BSUID for managed businesses enrolled in
	// a parent BSUID account; usable in place of user_id across all
	// portfolios enrolled in the same parent account.
	ParentUserID string `json:"parent_user_id,omitempty"`
}

type metaInboundMessage struct {
	From      string          `json:"from"`
	// FromUserID is the sender's BSUID. Populated whenever Meta has one
	// assigned; going forward this is the durable identity — phone (`from`)
	// may disappear once username adoption completes.
	FromUserID   string `json:"from_user_id,omitempty"`
	FromParentID string `json:"from_parent_user_id,omitempty"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Context   *metaMsgContext `json:"context,omitempty"`

	Text        *metaTextMsg        `json:"text,omitempty"`
	Image       *metaMediaMsg       `json:"image,omitempty"`
	Video       *metaMediaMsg       `json:"video,omitempty"`
	Audio       *metaMediaMsg       `json:"audio,omitempty"`
	Document    *metaMediaMsg       `json:"document,omitempty"`
	Sticker     *metaMediaMsg       `json:"sticker,omitempty"`
	Location    *metaLocationMsg    `json:"location,omitempty"`
	Contacts    []metaContactCard   `json:"contacts,omitempty"`
	Interactive *metaInteractiveMsg `json:"interactive,omitempty"`
	Button      *metaButtonMsg      `json:"button,omitempty"`
	Reaction    *metaReactionMsg    `json:"reaction,omitempty"`
	System      *metaSystemMsg      `json:"system,omitempty"`
	Errors      []metaError         `json:"errors,omitempty"`
}

type metaMsgContext struct {
	From      string `json:"from,omitempty"`
	ID        string `json:"id,omitempty"`
	Forwarded bool   `json:"forwarded,omitempty"`
}

type metaTextMsg struct {
	Body string `json:"body"`
}

type metaMediaMsg struct {
	ID       string `json:"id,omitempty"`
	// URL is a short-lived direct download link Meta includes in the
	// webhook envelope (image/video/audio/document/sticker). Using this
	// with a Bearer token skips the /v20.0/{media_id} lookup.
	URL      string `json:"url,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

type metaLocationMsg struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	URL       string  `json:"url,omitempty"`
}

type metaContactCard = map[string]any // pass-through — WhatsApp contact cards are wide

type metaInteractiveMsg struct {
	Type                string                    `json:"type"`
	ButtonReply         *metaButtonReply          `json:"button_reply,omitempty"`
	ListReply           *metaListReply            `json:"list_reply,omitempty"`
	NFMReply            *json.RawMessage          `json:"nfm_reply,omitempty"` // Flows response
	CallPermissionReply *metaCallPermissionReply  `json:"call_permission_reply,omitempty"`
}

type metaButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type metaListReply struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// metaCallPermissionReply mirrors Meta's interactive.call_permission_reply
// payload emitted when a WhatsApp user taps Accept/Decline on a call
// permission prompt. See ~/Documents/whatsapp_doc_tracker/docs/calling/
// user-call-permissions.md.
type metaCallPermissionReply struct {
	Response            string `json:"response"`
	ResponseSource      string `json:"response_source,omitempty"`
	ExpirationTimestamp int64  `json:"expiration_timestamp,omitempty"`
	IsPermanent         bool   `json:"is_permanent,omitempty"`
}

type metaButtonMsg struct {
	Payload string `json:"payload"`
	Text    string `json:"text"`
}

type metaReactionMsg struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji,omitempty"`
}

type metaSystemMsg struct {
	Body     string `json:"body,omitempty"`
	Identity string `json:"identity,omitempty"`
	NewWaID  string `json:"new_wa_id,omitempty"`
	Type     string `json:"type,omitempty"`
}

type metaStatus struct {
	ID               string      `json:"id"`
	Status           string      `json:"status"`
	Timestamp        string      `json:"timestamp"`
	RecipientID      string      `json:"recipient_id"`
	// RecipientUserID is Meta's BSUID for the customer. Present on
	// delivered / read callbacks regardless of whether the original send
	// went to a phone number or a BSUID; omitted from `failed` callbacks
	// when the send was phone-addressed.
	RecipientUserID  string      `json:"recipient_user_id,omitempty"`
	Errors           []metaError `json:"errors,omitempty"`
}

type metaError struct {
	Code    int    `json:"code"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
	Href    string `json:"href,omitempty"`
	Data    any    `json:"error_data,omitempty"`
}

// metaSendResponse mirrors the Cloud API /messages response body.
type metaSendResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID            string `json:"id"`
		MessageStatus string `json:"message_status,omitempty"`
	} `json:"messages"`
}

// mediaLookupResponse mirrors GET /<media_id>.
type mediaLookupResponse struct {
	MessagingProduct string `json:"messaging_product"`
	URL              string `json:"url"`
	MimeType         string `json:"mime_type"`
	SHA256           string `json:"sha256"`
	FileSize         int64  `json:"file_size"`
	ID               string `json:"id"`
}

// ---------- Inbound: Meta → canonical ----------

// canonicalize converts one Meta inbound message into canonical fields. The
// caller wraps these into an events.MessageReceivedPayload.
func canonicalize(msg metaInboundMessage) (message.Type, any) {
	switch msg.Type {
	case "text":
		body := ""
		if msg.Text != nil {
			body = msg.Text.Body
		}
		return message.TypeText, message.TextPayload{Body: body}
	case "image":
		return message.TypeImage, mediaPayload("image", msg.Image)
	case "video":
		return message.TypeVideo, mediaPayload("video", msg.Video)
	case "audio":
		return message.TypeAudio, mediaPayload("audio", msg.Audio)
	case "document":
		return message.TypeDocument, mediaPayload("document", msg.Document)
	case "sticker":
		return message.TypeSticker, mediaPayload("sticker", msg.Sticker)
	case "location":
		if msg.Location == nil {
			return message.TypeLocation, message.LocationPayload{}
		}
		return message.TypeLocation, message.LocationPayload{
			Latitude:  msg.Location.Latitude,
			Longitude: msg.Location.Longitude,
			Name:      msg.Location.Name,
			Address:   msg.Location.Address,
			URL:       msg.Location.URL,
		}
	case "contacts":
		cards := make([]message.ContactCard, 0, len(msg.Contacts))
		for _, c := range msg.Contacts {
			cards = append(cards, contactCard(c))
		}
		return message.TypeContacts, message.ContactsPayload{Contacts: cards}
	case "interactive":
		return message.TypeInteractive, interactivePayload(msg.Interactive)
	case "button":
		if msg.Button == nil {
			return message.TypeButton, message.InteractivePayload{Kind: "button"}
		}
		return message.TypeButton, message.InteractivePayload{
			Kind:        "button",
			ButtonReply: &message.InteractiveButtonReply{ID: msg.Button.Payload, Title: msg.Button.Text},
		}
	case "reaction":
		if msg.Reaction == nil {
			return message.TypeReaction, message.ReactionPayload{}
		}
		return message.TypeReaction, message.ReactionPayload{
			MessageID: msg.Reaction.MessageID,
			Emoji:     msg.Reaction.Emoji,
		}
	case "system":
		return message.TypeSystem, msg.System
	default:
		// Unknown / unsupported type — preserve raw for future-proofing.
		return message.TypeUnknown, map[string]any{"raw_type": msg.Type}
	}
}

func mediaPayload(kind string, m *metaMediaMsg) message.MediaPayload {
	if m == nil {
		return message.MediaPayload{Kind: kind}
	}
	return message.MediaPayload{
		Kind:     kind,
		MediaID:  m.ID,
		URL:      m.URL,
		MIMEType: m.MimeType,
		SHA256:   m.SHA256,
		Caption:  m.Caption,
		FileName: m.Filename,
		Voice:    m.Voice,
	}
}

func contactCard(raw map[string]any) message.ContactCard {
	card := message.ContactCard{}
	if v, ok := raw["name"].(map[string]any); ok {
		card.Name = v
	}
	if v, ok := raw["phones"].([]any); ok {
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				card.Phones = append(card.Phones, m)
			}
		}
	}
	if v, ok := raw["emails"].([]any); ok {
		for _, e := range v {
			if m, ok := e.(map[string]any); ok {
				card.Emails = append(card.Emails, m)
			}
		}
	}
	if v, ok := raw["addresses"].([]any); ok {
		for _, a := range v {
			if m, ok := a.(map[string]any); ok {
				card.Addresses = append(card.Addresses, m)
			}
		}
	}
	if v, ok := raw["urls"].([]any); ok {
		for _, u := range v {
			if m, ok := u.(map[string]any); ok {
				card.URLs = append(card.URLs, m)
			}
		}
	}
	if v, ok := raw["org"].(map[string]any); ok {
		card.Org = v
	}
	if v, ok := raw["birthday"].(string); ok {
		card.Birthday = v
	}
	return card
}

func interactivePayload(m *metaInteractiveMsg) message.InteractivePayload {
	if m == nil {
		return message.InteractivePayload{}
	}
	out := message.InteractivePayload{Kind: m.Type}
	if m.ButtonReply != nil {
		out.ButtonReply = &message.InteractiveButtonReply{ID: m.ButtonReply.ID, Title: m.ButtonReply.Title}
	}
	if m.ListReply != nil {
		out.ListReply = &message.InteractiveListReply{
			ID: m.ListReply.ID, Title: m.ListReply.Title, Description: m.ListReply.Description,
		}
	}
	if m.CallPermissionReply != nil {
		out.CallPermissionReply = &message.InteractiveCallPermissionReply{
			Response:            m.CallPermissionReply.Response,
			ResponseSource:      m.CallPermissionReply.ResponseSource,
			ExpirationTimestamp: m.CallPermissionReply.ExpirationTimestamp,
			IsPermanent:         m.CallPermissionReply.IsPermanent,
		}
	}
	return out
}

// parseTimestamp parses a Meta unix-string timestamp. Empty / invalid strings
// return time.Time{}; callers substitute time.Now if they care.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	var secs int64
	_, err := fmt.Sscanf(s, "%d", &secs)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(secs, 0).UTC()
}

// ---------- Outbound: canonical → Meta ----------

// canonicalSendToMeta builds the Meta /messages POST body for a canonical
// send request. The request's Body is expected to be a JSON encoding of one
// of the shapes in internal/domain/message/payload.go (matched by MessageType).
func canonicalSendToMeta(req channel.SendRequest) ([]byte, error) {
	base := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                req.To,
	}
	switch req.MessageType {
	case string(message.TypeText):
		var p message.TextPayload
		if err := json.Unmarshal(req.Body, &p); err != nil {
			return nil, fmt.Errorf("whatsapp: decode text payload: %w", err)
		}
		base["type"] = "text"
		base["text"] = map[string]any{"body": p.Body, "preview_url": p.PreviewURL}
	case string(message.TypeImage), string(message.TypeVideo), string(message.TypeAudio),
		string(message.TypeDocument), string(message.TypeSticker):
		var p message.MediaPayload
		if err := json.Unmarshal(req.Body, &p); err != nil {
			return nil, fmt.Errorf("whatsapp: decode media payload: %w", err)
		}
		kind := req.MessageType
		media := map[string]any{}
		if p.MediaID != "" {
			media["id"] = p.MediaID
		} else if p.URL != "" {
			media["link"] = p.URL
		} else {
			return nil, fmt.Errorf("whatsapp: media payload requires media_id or url")
		}
		if p.Caption != "" && (kind == string(message.TypeImage) || kind == string(message.TypeVideo) || kind == string(message.TypeDocument)) {
			media["caption"] = p.Caption
		}
		if p.FileName != "" && kind == string(message.TypeDocument) {
			media["filename"] = p.FileName
		}
		base["type"] = kind
		base[kind] = media
	case string(message.TypeTemplate):
		var p message.TemplatePayload
		if err := json.Unmarshal(req.Body, &p); err != nil {
			return nil, fmt.Errorf("whatsapp: decode template payload: %w", err)
		}
		tmpl := map[string]any{
			"name":     p.Name,
			"language": map[string]any{"code": p.Language},
		}
		if len(p.Components) > 0 {
			tmpl["components"] = p.Components
		}
		base["type"] = "template"
		base["template"] = tmpl
	case string(message.TypeLocation):
		var p message.LocationPayload
		if err := json.Unmarshal(req.Body, &p); err != nil {
			return nil, fmt.Errorf("whatsapp: decode location payload: %w", err)
		}
		base["type"] = "location"
		base["location"] = map[string]any{
			"latitude": p.Latitude, "longitude": p.Longitude,
			"name": p.Name, "address": p.Address,
		}
	case string(message.TypeReaction):
		var p message.ReactionPayload
		if err := json.Unmarshal(req.Body, &p); err != nil {
			return nil, fmt.Errorf("whatsapp: decode reaction payload: %w", err)
		}
		base["type"] = "reaction"
		base["reaction"] = map[string]any{"message_id": p.MessageID, "emoji": p.Emoji}
	case string(message.TypeInteractive):
		// Pass-through: caller has already composed the interactive object
		// as JSON — we forward it verbatim under the "interactive" key.
		var raw json.RawMessage
		if err := json.Unmarshal(req.Body, &raw); err != nil {
			return nil, fmt.Errorf("whatsapp: decode interactive payload: %w", err)
		}
		base["type"] = "interactive"
		base["interactive"] = raw
	default:
		return nil, fmt.Errorf("whatsapp: unsupported message type %q", req.MessageType)
	}
	if req.IdempotencyKey != "" {
		base["biz_opaque_callback_data"] = req.IdempotencyKey
	}
	return json.Marshal(base)
}
