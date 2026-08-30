package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	insertStrategyRoutingGraphSQL = `
		INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		VALUES (?, ?, ?, ?)`
	insertStrategyRoutingNodeSQL = `
		INSERT INTO lottery_strategy_routing_node
			(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
		VALUES (?, ?, ?, ?, ?, ?)`
	insertStrategyRoutingEdgeSQL = `
		INSERT INTO lottery_strategy_routing_edge
			(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
		VALUES (?, ?, ?, ?, ?, ?)`
	selectStrategyRoutingGraphSQL = `
		SELECT schema_version, root_node_id
		FROM lottery_strategy_routing_graph
		WHERE graph_id = ? AND revision = ?`
)

var selectStrategyRoutingNodesSQL = fmt.Sprintf(`
	SELECT node_id, node_kind, rule_code, strategy_id
	FROM lottery_strategy_routing_node
	WHERE graph_id = ? AND revision = ?
	ORDER BY node_id
	LIMIT %d`, domain.MaxStrategyRoutingGraphNodes+1)

var selectStrategyRoutingEdgesSQL = fmt.Sprintf(`
	SELECT from_node_id, branch_code, to_node_id, is_default
	FROM lottery_strategy_routing_edge
	WHERE graph_id = ? AND revision = ?
	ORDER BY from_node_id, branch_code
	LIMIT %d`, domain.MaxStrategyRoutingGraphEdges+1)

var errStoredStrategyRoutingGraphNodeLimit = errors.New("stored strategy routing graph node limit exceeded")
var errStoredStrategyRoutingGraphEdgeLimit = errors.New("stored strategy routing graph edge limit exceeded")
var errStoredStrategyRoutingNodeUnion = errors.New("stored strategy routing node union is invalid")
var errStoredStrategyRoutingEdgeDefault = errors.New("stored strategy routing edge default marker is invalid")

// StrategyRoutingGraphRepository persists complete immutable graph revisions.
// It is intentionally not runtime-composed in this lesson and exposes no
// update, upsert, delete, list, latest-revision, publication, or execution API.
type StrategyRoutingGraphRepository struct {
	database *sqlx.DB
}

var _ application.StrategyRoutingGraphCreator = (*StrategyRoutingGraphRepository)(nil)
var _ application.StrategyRoutingGraphReader = (*StrategyRoutingGraphRepository)(nil)

// NewStrategyRoutingGraphRepository constructs the graph adapter without
// taking ownership of the shared pool.
func NewStrategyRoutingGraphRepository(
	database *sqlx.DB,
) (*StrategyRoutingGraphRepository, error) {
	if database == nil {
		return nil, application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	return &StrategyRoutingGraphRepository{database: database}, nil
}

// Create stores one complete graph revision in canonical header-node-edge
// order. A duplicate GraphID/revision is a conflict, never idempotent success.
func (repository *StrategyRoutingGraphRepository) Create(
	ctx context.Context,
	graph domain.StrategyRoutingGraph,
) error {
	if ctx == nil {
		return application.WrapRepositoryError(
			application.ErrRepositoryInvalidArgument,
			errNilContext,
		)
	}
	if repository == nil || repository.database == nil {
		return application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	if err := graph.Validate(); err != nil {
		return err
	}

	identity := graph.Identity()
	graphID := uint64(identity.ID())
	revision := string(identity.Revision())
	tx, err := repository.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		insertStrategyRoutingGraphSQL,
		graphID,
		revision,
		uint16(graph.SchemaVersion()),
		uint64(graph.RootNodeID()),
	)
	if err != nil {
		return classifyStrategyRoutingGraphRootInsertError(err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	nodeStatement, err := tx.PrepareContext(ctx, insertStrategyRoutingNodeSQL)
	if err != nil {
		return classifyOperationError(err)
	}
	for _, node := range graph.Nodes() {
		ruleCode, strategyID := strategyRoutingNodeColumns(node)
		result, err = nodeStatement.ExecContext(
			ctx,
			graphID,
			revision,
			uint64(node.ID()),
			string(node.Kind()),
			ruleCode,
			strategyID,
		)
		if err != nil {
			_ = nodeStatement.Close()
			return classifyOperationError(err)
		}
		if err := requireOneAffectedRow(result); err != nil {
			_ = nodeStatement.Close()
			return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
		}
	}
	if err := nodeStatement.Close(); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	edgeStatement, err := tx.PrepareContext(ctx, insertStrategyRoutingEdgeSQL)
	if err != nil {
		return classifyOperationError(err)
	}
	for _, edge := range graph.Edges() {
		result, err = edgeStatement.ExecContext(
			ctx,
			graphID,
			revision,
			uint64(edge.From()),
			string(edge.Branch()),
			uint64(edge.To()),
			boolToTinyInt(edge.IsDefault()),
		)
		if err != nil {
			_ = edgeStatement.Close()
			return classifyOperationError(err)
		}
		if err := requireOneAffectedRow(result); err != nil {
			_ = edgeStatement.Close()
			return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
		}
	}
	if err := edgeStatement.Close(); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// FindByIdentity reads one complete graph revision from a repeatable-read,
// read-only snapshot. Domain restoration begins only after the snapshot ends.
func (repository *StrategyRoutingGraphRepository) FindByIdentity(
	ctx context.Context,
	identity domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraph, error) {
	if ctx == nil {
		return domain.StrategyRoutingGraph{}, application.WrapRepositoryError(
			application.ErrRepositoryInvalidArgument,
			errNilContext,
		)
	}
	if repository == nil || repository.database == nil {
		return domain.StrategyRoutingGraph{}, application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	if err := identity.Validate(); err != nil {
		return domain.StrategyRoutingGraph{}, err
	}

	tx, err := repository.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.StrategyRoutingGraph{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	header, err := loadStoredStrategyRoutingGraph(ctx, tx, identity)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.StrategyRoutingGraph{}, application.WrapRepositoryError(
				application.ErrStrategyRoutingGraphNotFound,
				err,
			)
		}
		return domain.StrategyRoutingGraph{}, classifyOperationError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.StrategyRoutingGraph{}, err
	}
	if domain.StrategyRoutingGraphSchemaVersion(header.SchemaVersion) !=
		domain.StrategyRoutingGraphSchemaVersionV1 {
		return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(
			domain.ErrStrategyRoutingGraphSchemaUnsupported,
		)
	}

	nodes, err := loadStoredStrategyRoutingNodes(ctx, tx, identity)
	if err != nil {
		if errors.Is(err, errStoredStrategyRoutingGraphNodeLimit) {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		return domain.StrategyRoutingGraph{}, classifyOperationError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.StrategyRoutingGraph{}, err
	}
	edges, err := loadStoredStrategyRoutingEdges(ctx, tx, identity)
	if err != nil {
		if errors.Is(err, errStoredStrategyRoutingGraphEdgeLimit) {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		return domain.StrategyRoutingGraph{}, classifyOperationError(err)
	}

	if err := ctx.Err(); err != nil {
		return domain.StrategyRoutingGraph{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.StrategyRoutingGraph{}, classifyReadCommitError(ctx, err)
	}
	return restoreStoredStrategyRoutingGraph(identity, header, nodes, edges)
}

type storedStrategyRoutingGraph struct {
	SchemaVersion uint16 `db:"schema_version"`
	RootNodeID    uint64 `db:"root_node_id"`
}

type storedStrategyRoutingNode struct {
	NodeID     uint64           `db:"node_id"`
	NodeKind   string           `db:"node_kind"`
	RuleCode   sql.NullString   `db:"rule_code"`
	StrategyID sql.Null[uint64] `db:"strategy_id"`
}

type storedStrategyRoutingEdge struct {
	FromNodeID uint64 `db:"from_node_id"`
	BranchCode string `db:"branch_code"`
	ToNodeID   uint64 `db:"to_node_id"`
	IsDefault  uint8  `db:"is_default"`
}

func loadStoredStrategyRoutingGraph(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	identity domain.StrategyRoutingGraphIdentity,
) (storedStrategyRoutingGraph, error) {
	var header storedStrategyRoutingGraph
	err := sqlx.GetContext(
		ctx,
		queryer,
		&header,
		selectStrategyRoutingGraphSQL,
		uint64(identity.ID()),
		string(identity.Revision()),
	)
	return header, err
}

func loadStoredStrategyRoutingNodes(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	identity domain.StrategyRoutingGraphIdentity,
) ([]storedStrategyRoutingNode, error) {
	rows, err := queryer.QueryxContext(
		ctx,
		selectStrategyRoutingNodesSQL,
		uint64(identity.ID()),
		string(identity.Revision()),
	)
	if err != nil {
		return nil, err
	}
	storedNodes := make([]storedStrategyRoutingNode, 0)
	for rows.Next() {
		var node storedStrategyRoutingNode
		if err := rows.StructScan(&node); err != nil {
			_ = rows.Close()
			return nil, err
		}
		storedNodes = append(storedNodes, node)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(storedNodes) > domain.MaxStrategyRoutingGraphNodes {
		return nil, errStoredStrategyRoutingGraphNodeLimit
	}
	return storedNodes, nil
}

func loadStoredStrategyRoutingEdges(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	identity domain.StrategyRoutingGraphIdentity,
) ([]storedStrategyRoutingEdge, error) {
	rows, err := queryer.QueryxContext(
		ctx,
		selectStrategyRoutingEdgesSQL,
		uint64(identity.ID()),
		string(identity.Revision()),
	)
	if err != nil {
		return nil, err
	}
	storedEdges := make([]storedStrategyRoutingEdge, 0)
	for rows.Next() {
		var edge storedStrategyRoutingEdge
		if err := rows.StructScan(&edge); err != nil {
			_ = rows.Close()
			return nil, err
		}
		storedEdges = append(storedEdges, edge)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(storedEdges) > domain.MaxStrategyRoutingGraphEdges {
		return nil, errStoredStrategyRoutingGraphEdgeLimit
	}
	return storedEdges, nil
}

func restoreStoredStrategyRoutingGraph(
	identity domain.StrategyRoutingGraphIdentity,
	header storedStrategyRoutingGraph,
	nodeRows []storedStrategyRoutingNode,
	edgeRows []storedStrategyRoutingEdge,
) (domain.StrategyRoutingGraph, error) {
	nodes := make([]domain.StrategyRoutingNode, 0, len(nodeRows))
	for _, row := range nodeRows {
		ruleCode, strategyID, err := restoreStrategyRoutingNodeUnion(row)
		if err != nil {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		node, err := domain.RestoreStrategyRoutingNode(
			domain.StrategyRoutingNodeID(row.NodeID),
			domain.StrategyRoutingNodeKind(row.NodeKind),
			domain.MembershipRoutingRuleCode(ruleCode),
			domain.StrategyID(strategyID),
		)
		if err != nil {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		nodes = append(nodes, node)
	}

	edges := make([]domain.StrategyRoutingEdge, 0, len(edgeRows))
	for _, row := range edgeRows {
		isDefault, err := tinyIntToBool(row.IsDefault)
		if err != nil {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		edge, err := domain.RestoreStrategyRoutingEdge(
			domain.StrategyRoutingNodeID(row.FromNodeID),
			domain.StrategyRoutingNodeID(row.ToNodeID),
			domain.MembershipRoutingBranch(row.BranchCode),
			isDefault,
		)
		if err != nil {
			return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
		}
		edges = append(edges, edge)
	}

	graph, err := domain.RestoreStrategyRoutingGraph(
		identity.ID(),
		identity.Revision(),
		domain.StrategyRoutingGraphSchemaVersion(header.SchemaVersion),
		domain.StrategyRoutingNodeID(header.RootNodeID),
		nodes,
		edges,
	)
	if err != nil {
		return domain.StrategyRoutingGraph{}, storedStrategyRoutingGraphInvalid(err)
	}
	return graph, nil
}

func strategyRoutingNodeColumns(node domain.StrategyRoutingNode) (any, any) {
	if node.Kind() == domain.StrategyRoutingNodeKindDecision {
		return string(node.RuleCode()), nil
	}
	return nil, uint64(node.StrategyID())
}

func restoreStrategyRoutingNodeUnion(
	row storedStrategyRoutingNode,
) (string, uint64, error) {
	switch domain.StrategyRoutingNodeKind(row.NodeKind) {
	case domain.StrategyRoutingNodeKindDecision:
		if !row.RuleCode.Valid || row.StrategyID.Valid {
			return "", 0, errStoredStrategyRoutingNodeUnion
		}
		return row.RuleCode.String, 0, nil
	case domain.StrategyRoutingNodeKindStrategyTarget:
		if row.RuleCode.Valid || !row.StrategyID.Valid {
			return "", 0, errStoredStrategyRoutingNodeUnion
		}
		return "", row.StrategyID.V, nil
	default:
		return "", 0, errStoredStrategyRoutingNodeUnion
	}
}

func boolToTinyInt(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func tinyIntToBool(value uint8) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errStoredStrategyRoutingEdgeDefault
	}
}

func classifyStrategyRoutingGraphRootInsertError(err error) error {
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return application.WrapRepositoryError(
			application.ErrStrategyRoutingGraphAlreadyExists,
			err,
		)
	}
	return classifyOperationError(err)
}

func storedStrategyRoutingGraphInvalid(cause error) error {
	return application.WrapRepositoryError(
		application.ErrStoredStrategyRoutingGraphInvalid,
		cause,
	)
}
