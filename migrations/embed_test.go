package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

func TestInitialLotteryMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000001_create_lottery_strategy.up.sql":       "2816792cd7ebaaf70c986c56f89f8207d1cac599deee5d1342d20af7768dcefc",
		"sql/000002_create_lottery_strategy_award.up.sql": "396aa84751e30f66fa7751bf79e389050e16fd1faf54dec0e7836b953efe60e4",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded migration: %v", err)
			}
			checksum := sha256.Sum256(contents)
			if got := hex.EncodeToString(checksum[:]); got != wantChecksum {
				t.Fatalf("checksum = %s, want immutable checksum %s; add a new migration instead of rewriting history", got, wantChecksum)
			}
			if got := strings.Count(string(contents), ";"); got != 1 {
				t.Fatalf("statement terminators = %d, want one DDL statement per migration", got)
			}
		})
	}
}

func TestRoutingGraphMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000003_create_lottery_strategy_routing_graph.up.sql": "b05da9a3166a8eb0ac0c99cfb9b193344b330b5240c00b5b1b00c9b7c127836a",
		"sql/000004_create_lottery_strategy_routing_node.up.sql":  "8a6925b25827c73e9874776796932feee87047f17297ec1c4bc92d11dd9b414d",
		"sql/000005_create_lottery_strategy_routing_edge.up.sql":  "80772ed2d1674a88e665dd494c48fb0f0cd34853bd734f7d2379d8263d7a33c5",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded routing migration: %v", err)
			}
			checksum := sha256.Sum256(contents)
			if got := hex.EncodeToString(checksum[:]); got != wantChecksum {
				t.Fatalf("checksum = %s, want immutable checksum %s; add a new migration instead of rewriting history", got, wantChecksum)
			}
			if got := strings.Count(string(contents), ";"); got != 1 {
				t.Fatalf("statement terminators = %d, want one DDL statement per migration", got)
			}
			if strings.Contains(strings.ToLower(string(contents)), "updated_at") {
				t.Fatal("routing graph migration contains updated_at, want create-only schema")
			}
		})
	}
}

func TestEmbeddedLotteryMigrationInventoryEndsAtVersionFive(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(Files, "sql")
	if err != nil {
		t.Fatalf("read embedded SQL directory: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	want := []string{
		"000001_create_lottery_strategy.up.sql",
		"000002_create_lottery_strategy_award.up.sql",
		"000003_create_lottery_strategy_routing_graph.up.sql",
		"000004_create_lottery_strategy_routing_node.up.sql",
		"000005_create_lottery_strategy_routing_edge.up.sql",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("embedded migrations = %q, want exact current inventory %q", names, want)
	}
}
