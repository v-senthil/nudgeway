package message

// Provider-neutral payload shapes. Adapters map their native envelopes onto
// these; the application layer + persistence write these to HBase as JSON.

// TextPayload carries a plain text message body.
type TextPayload struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url,omitempty"`
}

// MediaPayload describes an image / video / audio / document / sticker asset.
// Either MediaID (opaque handle for the provider) or URL is set. SHA256 and
// FileName are optional when known.
type MediaPayload struct {
	Kind     string `json:"kind"` // "image" | "video" | "audio" | "document" | "sticker"
	MediaID  string `json:"media_id,omitempty"`
	URL      string `json:"url,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Caption  string `json:"caption,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
}

// TemplateComponent is a rendered template component with its bound parameters.
type TemplateComponent struct {
	Type       string           `json:"type"`               // "header" | "body" | "button" | "footer"
	SubType    string           `json:"sub_type,omitempty"` // for buttons
	Index      *int             `json:"index,omitempty"`    // for buttons
	Parameters []map[string]any `json:"parameters,omitempty"`
}

// TemplatePayload is a canonical template send payload.
type TemplatePayload struct {
	Name       string              `json:"name"`
	Language   string              `json:"language"` // BCP47 or Meta locale
	Namespace  string              `json:"namespace,omitempty"`
	Components []TemplateComponent `json:"components,omitempty"`
}

// InteractiveButtonReply mirrors a tap on a reply button.
type InteractiveButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// InteractiveListReply mirrors a selection in a list message.
type InteractiveListReply struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// InteractiveCallPermissionReply mirrors a Meta interactive.call_permission_reply
// event — emitted when a WhatsApp user accepts / rejects a call permission
// prompt so the business can place a WhatsApp Business Calling API call.
type InteractiveCallPermissionReply struct {
	// Response is "accept" or "reject".
	Response string `json:"response"`
	// ResponseSource identifies how the reply was produced (e.g. "user_action").
	ResponseSource string `json:"response_source,omitempty"`
	// ExpirationTimestamp is the unix seconds at which the granted permission
	// expires. Zero when Meta did not include one (permanent permissions
	// or reject responses).
	ExpirationTimestamp int64 `json:"expiration_timestamp,omitempty"`
	// IsPermanent is true when the user granted permanent call permission
	// (introduced Nov 2025). Meta omits ExpirationTimestamp for permanent
	// grants; temporary grants carry expiration_timestamp and is_permanent=false.
	IsPermanent bool `json:"is_permanent,omitempty"`
}

// InteractivePayload captures the canonical form of an interactive message
// (either inbound reply or outbound prompt).
type InteractivePayload struct {
	Kind                string                          `json:"kind"` // "button_reply" | "list_reply" | "cta_url" | "flow" | "call_permission_reply" | ...
	ButtonReply         *InteractiveButtonReply         `json:"button_reply,omitempty"`
	ListReply           *InteractiveListReply           `json:"list_reply,omitempty"`
	CallPermissionReply *InteractiveCallPermissionReply `json:"call_permission_reply,omitempty"`
	Raw                 map[string]any                  `json:"raw,omitempty"` // outbound compositions we don't fully model yet
}

// LocationPayload carries a shared location.
type LocationPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	URL       string  `json:"url,omitempty"`
}

// ContactCard is one entry in a ContactsPayload.
type ContactCard struct {
	Name      map[string]any   `json:"name,omitempty"`
	Phones    []map[string]any `json:"phones,omitempty"`
	Emails    []map[string]any `json:"emails,omitempty"`
	Addresses []map[string]any `json:"addresses,omitempty"`
	Org       map[string]any   `json:"org,omitempty"`
	URLs      []map[string]any `json:"urls,omitempty"`
	Birthday  string           `json:"birthday,omitempty"`
}

// ContactsPayload carries one or more shared contact cards.
type ContactsPayload struct {
	Contacts []ContactCard `json:"contacts"`
}

// ReactionPayload carries an emoji reaction to a prior message. Empty Emoji
// means the reaction was removed.
type ReactionPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji,omitempty"`
}
