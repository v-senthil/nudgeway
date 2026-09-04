package hbase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tsuna/gohbase"
	"github.com/tsuna/gohbase/hrpc"
)

// EnsureSchema creates the attachments table with two column families:
//
//	d — data. Holds the raw blob under column d:bytes.
//	m — metadata. Holds content_type, size, sha256, filename, and
//	    per-integration provider handles (media_id_<provider>_<integ_id>).
//
// The row key is the SHA-256 hex digest of the blob so writes are
// content-addressed and idempotent. See docs/architecture.md.
//
// If namespace is non-empty the table is created as "<namespace>:<table>";
// otherwise the un-namespaced table name is used. gohbase does not
// currently expose a CreateNamespace RPC — operators must pre-create the
// namespace with the HBase shell if isolation is desired.
//
// EnsureSchema is idempotent: a "TableExistsException" error from HBase
// is logged at WARN and swallowed so the caller can treat a successful
// return as "table is ready to use".
func EnsureSchema(ctx context.Context, admin gohbase.AdminClient, namespace, table string) error {
	if admin == nil {
		return fmt.Errorf("hbase: EnsureSchema: nil admin client")
	}
	if table == "" {
		return fmt.Errorf("hbase: EnsureSchema: table name is required")
	}
	fq := table
	if namespace != "" {
		fq = namespace + ":" + table
	}
	families := map[string]map[string]string{
		"d": {},
		"m": {},
	}
	create := hrpc.NewCreateTable(ctx, []byte(fq), families)
	if err := admin.CreateTable(create); err != nil {
		msg := err.Error()
		// HBase surfaces "already exists" as either TableExistsException
		// or NamespaceExistException — swallow both. Anything else is
		// a hard failure the operator needs to see.
		if strings.Contains(msg, "TableExistsException") || strings.Contains(msg, "already exists") {
			slog.Default().Info("hbase: attachments table already exists",
				slog.String("table", fq))
			return nil
		}
		return fmt.Errorf("hbase: create table %s: %w", fq, err)
	}
	slog.Default().Info("hbase: attachments table created", slog.String("table", fq))
	return nil
}
