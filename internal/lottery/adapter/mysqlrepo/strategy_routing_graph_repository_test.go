package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestNewStrategyRoutingGraphRepositoryRejectsNilDatabase(t *testing.T) {
	t.Parallel()

	repository, err := NewStrategyRoutingGraphRepository(nil)
	if repository != nil || !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("NewStrategyRoutingGraphRepository(nil) = %#v, %v; want nil/not configured", repository, err)
	}
}

func TestStrategyRoutingGraphRepositoryRejectsInvalidReceiverContextAndDomainBeforeSQL(t *testing.T) {
	t.Parallel()

	identity := mustStrategyRoutingGraphIdentity(t, 41, "route-v1")
	var zero StrategyRoutingGraphRepository
	if err := zero.Create(context.Background(), domain.StrategyRoutingGraph{}); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero Create() error = %v, want not configured", err)
	}
	if _, err := zero.FindByIdentity(context.Background(), identity); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero FindByIdentity() error = %v, want not configured", err)
	}
	if err := zero.Create(nil, domain.StrategyRoutingGraph{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("Create(nil) error = %v, want invalid argument", err)
	}
	if _, err := zero.FindByIdentity(nil, domain.StrategyRoutingGraphIdentity{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("FindByIdentity(nil) error = %v, want invalid argument", err)
	}

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	if err := repository.Create(context.Background(), domain.StrategyRoutingGraph{}); !errors.Is(err, domain.ErrStrategyRoutingGraphInvalid) {
		t.Fatalf("Create(zero graph) error = %v, want domain invalid", err)
	}
	if _, err := repository.FindByIdentity(context.Background(), domain.StrategyRoutingGraphIdentity{}); !errors.Is(err, domain.ErrStrategyRoutingGraphIdentityInvalid) {
		t.Fatalf("FindByIdentity(zero identity) error = %v, want identity invalid", err)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphCreateWritesCanonicalHeaderNodesEdgesInOneTransaction(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	graph := mustStrategyRoutingGraph(t, 42, "route-v1")
	expectStrategyRoutingGraphCreate(mock, graph)
	mock.ExpectCommit()

	if err := repository.Create(context.Background(), graph); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphCreateRejectsDuplicateRootAsGraphConflict(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	graph := mustStrategyRoutingGraph(t, 43, "duplicate-v1")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategyRoutingGraphSQL)).
		WithArgs(uint64(43), "duplicate-v1", uint16(1), uint64(10)).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "secret duplicate detail"})
	mock.ExpectRollback()

	err := repository.Create(context.Background(), graph)
	if !errors.Is(err, application.ErrStrategyRoutingGraphAlreadyExists) {
		t.Fatalf("Create(duplicate) error = %v, want graph already exists", err)
	}
	if errors.Is(err, application.ErrStrategyAlreadyExists) {
		t.Fatal("graph duplicate was conflated with Strategy duplicate")
	}
	if got := err.Error(); got != application.ErrStrategyRoutingGraphAlreadyExists.Error() {
		t.Fatalf("Create(duplicate) rendered %q, want safe graph class", got)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphCreateChecksRowsAffectedForEveryRowAndRollsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stage string
	}{
		{name: "header", stage: "header"},
		{name: "node", stage: "node"},
		{name: "edge", stage: "edge"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, mock := newStrategyRoutingGraphRepositoryMock(t)
			graph := mustStrategyRoutingGraph(t, 44, test.stage+"-affected-v1")
			identity := graph.Identity()
			graphID := uint64(identity.ID())
			revision := string(identity.Revision())

			mock.ExpectBegin()
			headerAffected := int64(1)
			if test.stage == "header" {
				headerAffected = 0
			}
			mock.ExpectExec(regexp.QuoteMeta(insertStrategyRoutingGraphSQL)).
				WithArgs(graphID, revision, uint16(1), uint64(10)).
				WillReturnResult(sqlmock.NewResult(0, headerAffected))
			if test.stage != "header" {
				nodes := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategyRoutingNodeSQL))
				for index, node := range graph.Nodes() {
					ruleCode, strategyID := strategyRoutingNodeColumns(node)
					affected := int64(1)
					if test.stage == "node" && index == 0 {
						affected = 0
					}
					nodes.ExpectExec().
						WithArgs(graphID, revision, uint64(node.ID()), string(node.Kind()), ruleCode, strategyID).
						WillReturnResult(sqlmock.NewResult(0, affected))
					if affected == 0 {
						break
					}
				}
				nodes.WillBeClosed()
			}
			if test.stage == "edge" {
				edges := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategyRoutingEdgeSQL))
				for index, edge := range graph.Edges() {
					affected := int64(1)
					if index == 0 {
						affected = 0
					}
					edges.ExpectExec().
						WithArgs(graphID, revision, uint64(edge.From()), string(edge.Branch()), uint64(edge.To()), boolToTinyInt(edge.IsDefault())).
						WillReturnResult(sqlmock.NewResult(0, affected))
					if affected == 0 {
						break
					}
				}
				edges.WillBeClosed()
			}
			mock.ExpectRollback()

			err := repository.Create(context.Background(), graph)
			if !errors.Is(err, application.ErrRepositoryFailure) {
				t.Fatalf("Create(%s affected) error = %v, want repository failure", test.stage, err)
			}
			assertStrategyRoutingGraphExpectations(t, mock)
		})
	}
}

func TestStrategyRoutingGraphCreateRollsBackChildDriverFailure(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	graph := mustStrategyRoutingGraph(t, 45, "rollback-v1")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategyRoutingGraphSQL)).
		WithArgs(uint64(45), "rollback-v1", uint16(1), uint64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	nodes := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategyRoutingNodeSQL))
	node := graph.Nodes()[0]
	ruleCode, strategyID := strategyRoutingNodeColumns(node)
	nodes.ExpectExec().
		WithArgs(uint64(45), "rollback-v1", uint64(node.ID()), string(node.Kind()), ruleCode, strategyID).
		WillReturnError(&mysql.MySQLError{Number: 1452, Message: "secret child reference"})
	nodes.WillBeClosed()
	mock.ExpectRollback()

	err := repository.Create(context.Background(), graph)
	if !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("Create(child failure) error = %v, want repository failure", err)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphCreateCommitFailureIsUnknownOutcome(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	graph := mustStrategyRoutingGraph(t, 46, "commit-v1")
	expectStrategyRoutingGraphCreate(mock, graph)
	commitCause := errors.New("secret driver lost commit response")
	mock.ExpectCommit().WillReturnError(commitCause)

	err := repository.Create(context.Background(), graph)
	if !errors.Is(err, application.ErrCommitOutcomeUnknown) || !errors.Is(err, commitCause) {
		t.Fatalf("Create(commit failure) error = %v, want unknown outcome with cause", err)
	}
	if got := err.Error(); got != application.ErrCommitOutcomeUnknown.Error() {
		t.Fatalf("Create(commit failure) rendered %q, want safe class", got)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphFindReadsAndRestoresOneSnapshotByIdentity(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	graph := mustStrategyRoutingGraph(t, 47, "read-v1")
	expectStrategyRoutingGraphRead(mock, graph.Identity(), validStoredStrategyRoutingGraphHeader(), validStoredStrategyRoutingNodeRows(), validStoredStrategyRoutingEdgeRows())
	mock.ExpectCommit()

	loaded, err := repository.FindByIdentity(context.Background(), graph.Identity())
	if err != nil {
		t.Fatalf("FindByIdentity() error = %v", err)
	}
	assertStrategyRoutingGraphEqual(t, loaded, graph)
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphFindLocksRepeatableReadOnlySnapshotAtCallSite(t *testing.T) {
	t.Parallel()

	options := readSnapshotOptions()
	wantOptions := sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	if options == nil || *options != wantOptions {
		t.Fatalf("readSnapshotOptions() = %#v, want %#v", options, wantOptions)
	}

	file, err := parser.ParseFile(
		token.NewFileSet(),
		"strategy_routing_graph_repository.go",
		nil,
		parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatalf("parse graph repository source: %v", err)
	}
	var findUsesSnapshotOptions bool
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "FindByIdentity" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "BeginTxx" {
				return true
			}
			contextArgument, contextOK := call.Args[0].(*ast.Ident)
			optionsCall, optionsOK := call.Args[1].(*ast.CallExpr)
			if !contextOK || !optionsOK {
				return true
			}
			optionsFunction, functionOK := optionsCall.Fun.(*ast.Ident)
			if contextArgument.Name == "ctx" &&
				functionOK && optionsFunction.Name == "readSnapshotOptions" &&
				len(optionsCall.Args) == 0 {
				findUsesSnapshotOptions = true
				return false
			}
			return true
		})
	}
	if !findUsesSnapshotOptions {
		t.Fatal("FindByIdentity must begin its snapshot with readSnapshotOptions()")
	}
}

func TestStrategyRoutingGraphFindMapsMissingIdentityWithoutPartialGraph(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	identity := mustStrategyRoutingGraphIdentity(t, 48, "missing-v1")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
		WithArgs(uint64(48), "missing-v1").
		WillReturnRows(sqlmock.NewRows([]string{"schema_version", "root_node_id"}))
	mock.ExpectRollback()

	graph, err := repository.FindByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrStrategyRoutingGraphNotFound) {
		t.Fatalf("FindByIdentity(missing) error = %v, want graph not found", err)
	}
	if errors.Is(err, application.ErrStrategyNotFound) {
		t.Fatal("graph absence was conflated with Strategy absence")
	}
	assertZeroStrategyRoutingGraph(t, graph)
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphFindFailsClosedOnStoredNodeAndEdgeLimits(t *testing.T) {
	t.Parallel()

	t.Run("nodes", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		identity := mustStrategyRoutingGraphIdentity(t, 49, "too-many-nodes-v1")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
			WithArgs(uint64(49), "too-many-nodes-v1").
			WillReturnRows(validStoredStrategyRoutingGraphHeader())
		rows := sqlmock.NewRows([]string{"node_id", "node_kind", "rule_code", "strategy_id"})
		for index := 0; index < domain.MaxStrategyRoutingGraphNodes+1; index++ {
			rows.AddRow(uint64(index+1), "decision", string(domain.MembershipStrategyRoutingRuleCode), nil)
		}
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingNodesSQL)).
			WithArgs(uint64(49), "too-many-nodes-v1").
			WillReturnRows(rows)
		mock.ExpectRollback()

		graph, err := repository.FindByIdentity(context.Background(), identity)
		if !errors.Is(err, application.ErrStoredStrategyRoutingGraphInvalid) {
			t.Fatalf("FindByIdentity(129 nodes) error = %v, want stored graph invalid", err)
		}
		assertZeroStrategyRoutingGraph(t, graph)
		assertStrategyRoutingGraphExpectations(t, mock)
	})

	t.Run("edges", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		identity := mustStrategyRoutingGraphIdentity(t, 50, "too-many-edges-v1")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
			WithArgs(uint64(50), "too-many-edges-v1").
			WillReturnRows(validStoredStrategyRoutingGraphHeader())
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingNodesSQL)).
			WithArgs(uint64(50), "too-many-edges-v1").
			WillReturnRows(validStoredStrategyRoutingNodeRows())
		edges := sqlmock.NewRows([]string{"from_node_id", "branch_code", "to_node_id", "is_default"})
		for index := 0; index < domain.MaxStrategyRoutingGraphEdges+1; index++ {
			edges.AddRow(uint64(10), "premium_override", uint64(30), uint8(0))
		}
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingEdgesSQL)).
			WithArgs(uint64(50), "too-many-edges-v1").
			WillReturnRows(edges)
		mock.ExpectRollback()

		graph, err := repository.FindByIdentity(context.Background(), identity)
		if !errors.Is(err, application.ErrStoredStrategyRoutingGraphInvalid) {
			t.Fatalf("FindByIdentity(257 edges) error = %v, want stored graph invalid", err)
		}
		assertZeroStrategyRoutingGraph(t, graph)
		assertStrategyRoutingGraphExpectations(t, mock)
	})
}

func TestStrategyRoutingGraphFindCommitsBeforeStrictCorruptRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header *sqlmock.Rows
		nodes  *sqlmock.Rows
		edges  *sqlmock.Rows
	}{
		{
			name:   "decision nullable union",
			header: validStoredStrategyRoutingGraphHeader(),
			nodes: sqlmock.NewRows([]string{"node_id", "node_kind", "rule_code", "strategy_id"}).
				AddRow(uint64(10), "decision", nil, nil).
				AddRow(uint64(20), "strategy_target", nil, uint64(100)).
				AddRow(uint64(30), "strategy_target", nil, uint64(200)),
			edges: validStoredStrategyRoutingEdgeRows(),
		},
		{
			name:   "invalid default marker",
			header: validStoredStrategyRoutingGraphHeader(),
			nodes:  validStoredStrategyRoutingNodeRows(),
			edges: sqlmock.NewRows([]string{"from_node_id", "branch_code", "to_node_id", "is_default"}).
				AddRow(uint64(10), "baseline_default", uint64(20), uint8(2)).
				AddRow(uint64(10), "premium_override", uint64(30), uint8(0)),
		},
	}

	for index, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, mock := newStrategyRoutingGraphRepositoryMock(t)
			identity := mustStrategyRoutingGraphIdentity(t, domain.StrategyRoutingGraphID(60+index), "corrupt-v1")
			expectStrategyRoutingGraphRead(mock, identity, test.header, test.nodes, test.edges)
			mock.ExpectCommit()

			graph, err := repository.FindByIdentity(context.Background(), identity)
			if !errors.Is(err, application.ErrStoredStrategyRoutingGraphInvalid) {
				t.Fatalf("FindByIdentity(corrupt) error = %v, want stored graph invalid", err)
			}
			assertZeroStrategyRoutingGraph(t, graph)
			if got := err.Error(); got != application.ErrStoredStrategyRoutingGraphInvalid.Error() {
				t.Fatalf("FindByIdentity(corrupt) rendered %q, want safe class", got)
			}
			assertStrategyRoutingGraphExpectations(t, mock)
		})
	}
}

func TestStrategyRoutingGraphFindRejectsUnknownSchemaBeforeNodeOrEdgeQueries(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	identity := mustStrategyRoutingGraphIdentity(t, 63, "unknown-schema-v1")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
		WithArgs(uint64(63), "unknown-schema-v1").
		WillReturnRows(sqlmock.NewRows([]string{"schema_version", "root_node_id"}).
			AddRow(uint16(2), uint64(10)))
	mock.ExpectRollback()

	graph, err := repository.FindByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrStoredStrategyRoutingGraphInvalid) {
		t.Fatalf("FindByIdentity(unknown schema) error = %v, want stored graph invalid", err)
	}
	assertZeroStrategyRoutingGraph(t, graph)
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphFindScansNullableUnsignedMaxUint64BeforeRestore(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategyRoutingGraphRepositoryMock(t)
	identity := mustStrategyRoutingGraphIdentity(t, 64, "max-uint64-v1")
	maxUnsignedDecimal := []byte("18446744073709551615")
	nodes := sqlmock.NewRows([]string{"node_id", "node_kind", "rule_code", "strategy_id"}).
		AddRow(uint64(10), "decision", string(domain.MembershipStrategyRoutingRuleCode), nil).
		AddRow(uint64(20), "strategy_target", nil, maxUnsignedDecimal).
		AddRow(uint64(30), "strategy_target", nil, maxUnsignedDecimal)
	expectStrategyRoutingGraphRead(
		mock,
		identity,
		validStoredStrategyRoutingGraphHeader(),
		nodes,
		validStoredStrategyRoutingEdgeRows(),
	)
	mock.ExpectCommit()

	graph, err := repository.FindByIdentity(context.Background(), identity)
	if err != nil {
		t.Fatalf("FindByIdentity(max uint64) error = %v", err)
	}
	for _, nodeID := range []domain.StrategyRoutingNodeID{20, 30} {
		node, found := graph.Node(nodeID)
		if !found || uint64(node.StrategyID()) != uint64(math.MaxUint64) {
			t.Fatalf("node %d StrategyID = %d, found %t; want MaxUint64", nodeID, node.StrategyID(), found)
		}
	}
	root, found := graph.Node(10)
	if !found || root.Kind() != domain.StrategyRoutingNodeKindDecision || root.StrategyID() != 0 {
		t.Fatalf("decision node = %#v, found %t; want scanned NULL Strategy target", root, found)
	}
	assertStrategyRoutingGraphExpectations(t, mock)
}

func TestStrategyRoutingGraphRepositoryCancellationAndDriverClassification(t *testing.T) {
	t.Parallel()

	t.Run("pre-canceled create and find do no SQL", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		graph := mustStrategyRoutingGraph(t, 70, "canceled-v1")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := repository.Create(ctx, graph); !errors.Is(err, context.Canceled) {
			t.Fatalf("Create(canceled) error = %v, want context canceled", err)
		}
		if _, err := repository.FindByIdentity(ctx, graph.Identity()); !errors.Is(err, context.Canceled) {
			t.Fatalf("FindByIdentity(canceled) error = %v, want context canceled", err)
		}
		assertStrategyRoutingGraphExpectations(t, mock)
	})

	t.Run("create begin deadlock is retryable", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		graph := mustStrategyRoutingGraph(t, 71, "deadlock-v1")
		mock.ExpectBegin().WillReturnError(&mysql.MySQLError{Number: 1213, Message: "secret deadlock"})
		err := repository.Create(context.Background(), graph)
		if !errors.Is(err, application.ErrRepositoryRetryable) {
			t.Fatalf("Create(deadlock) error = %v, want retryable", err)
		}
		assertStrategyRoutingGraphExpectations(t, mock)
	})

	t.Run("read query lock wait is retryable", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		identity := mustStrategyRoutingGraphIdentity(t, 72, "lock-wait-v1")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
			WithArgs(uint64(72), "lock-wait-v1").
			WillReturnError(&mysql.MySQLError{Number: 1205, Message: "secret lock wait"})
		mock.ExpectRollback()
		_, err := repository.FindByIdentity(context.Background(), identity)
		if !errors.Is(err, application.ErrRepositoryRetryable) {
			t.Fatalf("FindByIdentity(lock wait) error = %v, want retryable", err)
		}
		assertStrategyRoutingGraphExpectations(t, mock)
	})

	t.Run("read commit driver failure is classified and not outcome unknown", func(t *testing.T) {
		t.Parallel()

		repository, mock := newStrategyRoutingGraphRepositoryMock(t)
		identity := mustStrategyRoutingGraphIdentity(t, 73, "read-commit-v1")
		expectStrategyRoutingGraphRead(mock, identity, validStoredStrategyRoutingGraphHeader(), validStoredStrategyRoutingNodeRows(), validStoredStrategyRoutingEdgeRows())
		commitCause := errors.New("secret read commit failure")
		mock.ExpectCommit().WillReturnError(commitCause)
		_, err := repository.FindByIdentity(context.Background(), identity)
		if !errors.Is(err, application.ErrRepositoryFailure) || errors.Is(err, application.ErrCommitOutcomeUnknown) {
			t.Fatalf("FindByIdentity(commit failure) error = %v, want repository failure only", err)
		}
		assertStrategyRoutingGraphExpectations(t, mock)
	})
}

func TestStrategyRoutingGraphSQLIsExplicitBoundedAndHasNoMutationSurface(t *testing.T) {
	t.Parallel()

	queries := map[string]string{
		"header": selectStrategyRoutingGraphSQL,
		"nodes":  selectStrategyRoutingNodesSQL,
		"edges":  selectStrategyRoutingEdgesSQL,
	}
	selectStar := regexp.MustCompile(`(?i)\bSELECT\s+\*`)
	for name, query := range queries {
		if selectStar.MatchString(query) {
			t.Fatalf("%s query uses SELECT *: %s", name, query)
		}
	}
	if !strings.Contains(selectStrategyRoutingNodesSQL, fmt.Sprintf("LIMIT %d", domain.MaxStrategyRoutingGraphNodes+1)) {
		t.Fatalf("node query is not bounded at %d: %s", domain.MaxStrategyRoutingGraphNodes+1, selectStrategyRoutingNodesSQL)
	}
	if !strings.Contains(selectStrategyRoutingEdgesSQL, fmt.Sprintf("LIMIT %d", domain.MaxStrategyRoutingGraphEdges+1)) {
		t.Fatalf("edge query is not bounded at %d: %s", domain.MaxStrategyRoutingGraphEdges+1, selectStrategyRoutingEdgesSQL)
	}
	for name, statement := range map[string]string{
		"header insert": insertStrategyRoutingGraphSQL,
		"node insert":   insertStrategyRoutingNodeSQL,
		"edge insert":   insertStrategyRoutingEdgeSQL,
	} {
		upper := strings.ToUpper(statement)
		if strings.Contains(upper, "UPDATE") || strings.Contains(upper, "DELETE") || strings.Contains(upper, "ON DUPLICATE") {
			t.Fatalf("%s contains a forbidden mutation/upsert clause: %s", name, statement)
		}
	}
}

func newStrategyRoutingGraphRepositoryMock(
	t *testing.T,
) (*StrategyRoutingGraphRepository, sqlmock.Sqlmock) {
	t.Helper()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository, err := NewStrategyRoutingGraphRepository(sqlx.NewDb(database, "sqlmock"))
	if err != nil {
		t.Fatalf("NewStrategyRoutingGraphRepository() error = %v", err)
	}
	return repository, mock
}

func expectStrategyRoutingGraphCreate(
	mock sqlmock.Sqlmock,
	graph domain.StrategyRoutingGraph,
) {
	identity := graph.Identity()
	graphID := uint64(identity.ID())
	revision := string(identity.Revision())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategyRoutingGraphSQL)).
		WithArgs(graphID, revision, uint16(graph.SchemaVersion()), uint64(graph.RootNodeID())).
		WillReturnResult(sqlmock.NewResult(0, 1))
	nodes := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategyRoutingNodeSQL))
	for _, node := range graph.Nodes() {
		ruleCode, strategyID := strategyRoutingNodeColumns(node)
		nodes.ExpectExec().
			WithArgs(graphID, revision, uint64(node.ID()), string(node.Kind()), ruleCode, strategyID).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	nodes.WillBeClosed()
	edges := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategyRoutingEdgeSQL))
	for _, edge := range graph.Edges() {
		edges.ExpectExec().
			WithArgs(graphID, revision, uint64(edge.From()), string(edge.Branch()), uint64(edge.To()), boolToTinyInt(edge.IsDefault())).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	edges.WillBeClosed()
}

func expectStrategyRoutingGraphRead(
	mock sqlmock.Sqlmock,
	identity domain.StrategyRoutingGraphIdentity,
	header *sqlmock.Rows,
	nodes *sqlmock.Rows,
	edges *sqlmock.Rows,
) {
	graphID := uint64(identity.ID())
	revision := string(identity.Revision())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingGraphSQL)).
		WithArgs(graphID, revision).
		WillReturnRows(header)
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingNodesSQL)).
		WithArgs(graphID, revision).
		WillReturnRows(nodes)
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategyRoutingEdgesSQL)).
		WithArgs(graphID, revision).
		WillReturnRows(edges)
}

func validStoredStrategyRoutingGraphHeader() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"schema_version", "root_node_id"}).
		AddRow(uint16(1), uint64(10))
}

func validStoredStrategyRoutingNodeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"node_id", "node_kind", "rule_code", "strategy_id"}).
		AddRow(uint64(10), "decision", string(domain.MembershipStrategyRoutingRuleCode), nil).
		AddRow(uint64(20), "strategy_target", nil, uint64(100)).
		AddRow(uint64(30), "strategy_target", nil, uint64(200))
}

func validStoredStrategyRoutingEdgeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"from_node_id", "branch_code", "to_node_id", "is_default"}).
		AddRow(uint64(10), "baseline_default", uint64(20), uint8(1)).
		AddRow(uint64(10), "premium_override", uint64(30), uint8(0))
}

func mustStrategyRoutingGraph(
	t *testing.T,
	id domain.StrategyRoutingGraphID,
	revision string,
) domain.StrategyRoutingGraph {
	t.Helper()

	root, err := domain.NewStrategyRoutingDecisionNode(10, domain.MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("NewStrategyRoutingDecisionNode() error = %v", err)
	}
	baseline, err := domain.NewStrategyRoutingTargetNode(20, 100)
	if err != nil {
		t.Fatalf("NewStrategyRoutingTargetNode(baseline) error = %v", err)
	}
	premium, err := domain.NewStrategyRoutingTargetNode(30, 200)
	if err != nil {
		t.Fatalf("NewStrategyRoutingTargetNode(premium) error = %v", err)
	}
	premiumEdge, err := domain.NewStrategyRoutingEdge(10, 30, domain.MembershipRoutingBranchPremiumOverride)
	if err != nil {
		t.Fatalf("NewStrategyRoutingEdge(premium) error = %v", err)
	}
	baselineEdge, err := domain.NewStrategyRoutingEdge(10, 20, domain.MembershipRoutingBranchBaselineDefault)
	if err != nil {
		t.Fatalf("NewStrategyRoutingEdge(baseline) error = %v", err)
	}
	graph, err := domain.NewStrategyRoutingGraph(
		id,
		revision,
		10,
		[]domain.StrategyRoutingNode{premium, root, baseline},
		[]domain.StrategyRoutingEdge{premiumEdge, baselineEdge},
	)
	if err != nil {
		t.Fatalf("NewStrategyRoutingGraph() error = %v", err)
	}
	return graph
}

func mustStrategyRoutingGraphIdentity(
	t *testing.T,
	id domain.StrategyRoutingGraphID,
	revision string,
) domain.StrategyRoutingGraphIdentity {
	t.Helper()

	identity, err := domain.NewStrategyRoutingGraphIdentity(id, revision)
	if err != nil {
		t.Fatalf("NewStrategyRoutingGraphIdentity() error = %v", err)
	}
	return identity
}

func assertStrategyRoutingGraphEqual(
	t *testing.T,
	got domain.StrategyRoutingGraph,
	want domain.StrategyRoutingGraph,
) {
	t.Helper()

	if got.Identity() != want.Identity() ||
		got.SchemaVersion() != want.SchemaVersion() ||
		got.RootNodeID() != want.RootNodeID() ||
		got.Depth() != want.Depth() {
		t.Fatalf("graph envelope = %#v, want %#v", got, want)
	}
	gotNodes, wantNodes := got.Nodes(), want.Nodes()
	if len(gotNodes) != len(wantNodes) {
		t.Fatalf("graph nodes = %d, want %d", len(gotNodes), len(wantNodes))
	}
	for index := range gotNodes {
		if gotNodes[index] != wantNodes[index] {
			t.Fatalf("graph node[%d] = %#v, want %#v", index, gotNodes[index], wantNodes[index])
		}
	}
	gotEdges, wantEdges := got.Edges(), want.Edges()
	if len(gotEdges) != len(wantEdges) {
		t.Fatalf("graph edges = %d, want %d", len(gotEdges), len(wantEdges))
	}
	for index := range gotEdges {
		if gotEdges[index] != wantEdges[index] {
			t.Fatalf("graph edge[%d] = %#v, want %#v", index, gotEdges[index], wantEdges[index])
		}
	}
}

func assertZeroStrategyRoutingGraph(t *testing.T, graph domain.StrategyRoutingGraph) {
	t.Helper()

	if graph.ID() != 0 || graph.Revision() != "" || graph.SchemaVersion() != 0 ||
		graph.RootNodeID() != 0 || graph.Depth() != 0 || len(graph.Nodes()) != 0 || len(graph.Edges()) != 0 {
		t.Fatalf("operation returned partial graph %#v", graph)
	}
}

func assertStrategyRoutingGraphExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("strategy routing graph SQL expectations: %v", err)
	}
}
