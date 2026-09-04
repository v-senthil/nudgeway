package hbase

import (
	"fmt"
	"strings"

	"github.com/tsuna/gohbase"
)

// NewClient dials an HBase cluster via the ZooKeeper quorum and returns
// both a regular client (used for row Get/Put/Delete) and an admin client
// (used at boot for CreateTable). Both clients speak the native HBase RPC
// protocol — Thrift is not required.
//
// zkQuorum is the list of host:port entries for the ZooKeeper ensemble
// that publishes the HBase master + region servers (e.g.
// []string{"127.0.0.1:2181"}). zkNode is the znode path HBase publishes
// under (default "/hbase"; some distributions use "/hbase-unsecure").
//
// Returns wrapped errors when the quorum list is empty. Actual dial /
// meta lookups happen lazily on the first RPC — callers should treat a
// nil error as "config accepted" rather than "cluster reachable" and
// verify reachability with an admin ping (e.g. an EnsureSchema call).
func NewClient(zkQuorum []string, zkNode string) (gohbase.Client, gohbase.AdminClient, error) {
	if len(zkQuorum) == 0 {
		return nil, nil, fmt.Errorf("hbase: NewClient: zk quorum is required")
	}
	quorum := strings.Join(zkQuorum, ",")
	opts := []gohbase.Option{}
	if zkNode != "" {
		opts = append(opts, gohbase.ZookeeperRoot(zkNode))
	}
	client := gohbase.NewClient(quorum, opts...)
	admin := gohbase.NewAdminClient(quorum, opts...)
	return client, admin, nil
}

// Close releases resources held by a gohbase.Client (region cache, TCP
// pools, in-flight goroutines). Safe to call multiple times.
func Close(c gohbase.Client) {
	if c == nil {
		return
	}
	c.Close()
}
