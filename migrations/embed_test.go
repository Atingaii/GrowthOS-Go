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

func TestStrategySnapshotMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000006_create_lottery_strategy_snapshot.up.sql":       "e0a0d825c8dc9d1a763ec1f5dffe066d7afb19fd0668e44645f69f8204188027",
		"sql/000007_create_lottery_strategy_snapshot_award.up.sql": "69d5593f4f4731dd02365d477ec2f402699b0e2739f29dcd0a11a164f8164afc",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded Strategy snapshot migration: %v", err)
			}
			checksum := sha256.Sum256(contents)
			if got := hex.EncodeToString(checksum[:]); got != wantChecksum {
				t.Fatalf("checksum = %s, want immutable checksum %s; add a new migration instead of rewriting history", got, wantChecksum)
			}
			if got := strings.Count(string(contents), ";"); got != 1 {
				t.Fatalf("statement terminators = %d, want one DDL statement per migration", got)
			}
			if strings.Contains(strings.ToLower(string(contents)), "updated_at") {
				t.Fatal("Strategy snapshot migration contains updated_at, want create-only schema")
			}
		})
	}
}

func TestActivityPublicationMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000008_create_marketing_activity.up.sql":                      "416c210bf59f225fc922de12bcac91717513f57afb9611e72e425ec72bf8189b",
		"sql/000009_create_marketing_activity_publication.up.sql":          "3abf5885bf3f966eb2438181712b39c7b721be72582df1ba6ddcb899f834f856",
		"sql/000010_create_marketing_activity_publication_strategy.up.sql": "5c8007ccd43d5f0a54f0e6419089c41ef2edfcbb850ae9c5a66922c176940d91",
		"sql/000011_add_marketing_activity_active_publication_fk.up.sql":   "97bcd95eecee1aa5d0b1b9012caded312dc054ff5633de99d8885bc12018fc60",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded Activity publication migration: %v", err)
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

func TestIdentityMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000012_create_identity_workforce_account.up.sql":       "0dde0a2a01ba5671df2b11a025e19b16c20b61ba107b275a03bf5df2533e7771",
		"sql/000013_create_identity_session.up.sql":                 "d568d6268c622cbed0e7fd4137e0d19c8b993fb20df72d0aed40711fd9d32adc",
		"sql/000014_create_identity_authentication_throttle.up.sql": "f30d58b12151884f50b4d8d5949552c85c7740715a2c8e3680ccfa33d8dd86fc",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded Identity migration: %v", err)
			}
			checksum := sha256.Sum256(contents)
			if got := hex.EncodeToString(checksum[:]); got != wantChecksum {
				t.Fatalf(
					"checksum = %s, want immutable checksum %s; add a new migration instead of rewriting history",
					got,
					wantChecksum,
				)
			}
			if got := strings.Count(string(contents), ";"); got != 1 {
				t.Fatalf("statement terminators = %d, want one DDL statement per migration", got)
			}
		})
	}
}

func TestGovernanceMigrationsRemainImmutable(t *testing.T) {
	t.Parallel()

	immutable := map[string]string{
		"sql/000015_create_governance_policy_revision.up.sql":           "1b0d7d73bb1d5f4caeb82993d0b1d8e18581f997c9c62992d9d3ff0255dbce73",
		"sql/000016_create_governance_policy_role.up.sql":               "0190e3510cdb49a8b75281c614ff28a8283424523029b4d40580064d9a44cde5",
		"sql/000017_create_governance_policy_role_permission.up.sql":    "37684956d15497b86a384c1676ddafcc9694d66f2b9ef0e72f0892aad8a2fc72",
		"sql/000018_create_governance_policy_role_binding.up.sql":       "501ca439f160155a443a87e5d75af9ebbf829a781d18defc9cc8a2a17c8e2dae",
		"sql/000019_create_governance_policy_activation.up.sql":         "64430c1b53ebfb3971d5607b50dffb3e258927678bf8ae62aa9f912c07e62e1b",
		"sql/000020_create_governance_active_policy.up.sql":             "9da0d8bbca2fc9fdfbae5cd1daf6055fd24377fa1d99cc4fe52083905888c85d",
		"sql/000021_create_governance_authorization_audit.up.sql":       "421e809166e6f0e8f6199f433e317a0d5721ec8352111b44ad59e31ba1a23a61",
		"sql/000022_create_governance_authorization_audit_match.up.sql": "115e12f72232f697488622641dc06b46609ffba3c6acf16b50c27b382e9ed7a4",
	}

	for name, wantChecksum := range immutable {
		name := name
		wantChecksum := wantChecksum
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := fs.ReadFile(Files, name)
			if err != nil {
				t.Fatalf("read embedded Governance migration: %v", err)
			}
			checksum := sha256.Sum256(contents)
			if got := hex.EncodeToString(checksum[:]); got != wantChecksum {
				t.Fatalf(
					"checksum = %s, want immutable checksum %s; add a new migration instead of rewriting history",
					got,
					wantChecksum,
				)
			}
			if got := strings.Count(string(contents), ";"); got != 1 {
				t.Fatalf("statement terminators = %d, want one DDL statement per migration", got)
			}
		})
	}
}

func TestEmbeddedMigrationInventoryEndsAtVersionTwentyTwo(t *testing.T) {
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
		"000006_create_lottery_strategy_snapshot.up.sql",
		"000007_create_lottery_strategy_snapshot_award.up.sql",
		"000008_create_marketing_activity.up.sql",
		"000009_create_marketing_activity_publication.up.sql",
		"000010_create_marketing_activity_publication_strategy.up.sql",
		"000011_add_marketing_activity_active_publication_fk.up.sql",
		"000012_create_identity_workforce_account.up.sql",
		"000013_create_identity_session.up.sql",
		"000014_create_identity_authentication_throttle.up.sql",
		"000015_create_governance_policy_revision.up.sql",
		"000016_create_governance_policy_role.up.sql",
		"000017_create_governance_policy_role_permission.up.sql",
		"000018_create_governance_policy_role_binding.up.sql",
		"000019_create_governance_policy_activation.up.sql",
		"000020_create_governance_active_policy.up.sql",
		"000021_create_governance_authorization_audit.up.sql",
		"000022_create_governance_authorization_audit_match.up.sql",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("embedded migrations = %q, want exact current inventory %q", names, want)
	}
}
