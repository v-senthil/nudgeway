package providercall

// Groups operation constants used by the WhatsApp adapter's groups.go
// helpers. Declared here so the operator-facing "Meta API logs" filter can
// enumerate them as a suggested drop-down; free-form strings from other
// adapters continue to work because the persisted column is a plain
// VARCHAR(64).
const (
	// OpListGroups covers GET /<phone_number_id>/groups.
	OpListGroups Operation = "list_groups"

	// OpGetGroup covers GET /<group_id>?fields=... .
	OpGetGroup Operation = "get_group"

	// OpListGroupMembers covers the participants sub-fetch performed by
	// GetGroup(fields=participants). It is emitted only when the adapter
	// makes a dedicated call rather than piggy-backing on OpGetGroup.
	OpListGroupMembers Operation = "list_group_members"

	// OpCreateGroup covers POST /<phone_number_id>/groups. Meta returns an
	// id synchronously; the invite_link arrives asynchronously via the
	// group_lifecycle_update webhook.
	OpCreateGroup Operation = "create_group"
)
