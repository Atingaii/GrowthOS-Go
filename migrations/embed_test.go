package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
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
