package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeRepository struct {
	pool *pgxpool.Pool
}

func NewKnowledgeRepository(
	pool *pgxpool.Pool,
) *KnowledgeRepository {
	return &KnowledgeRepository{
		pool: pool,
	}
}

func (r *KnowledgeRepository) CreateDocument(
	ctx context.Context,
	document model.KnowledgeDocument,
) (int64, error) {

	var id int64

	err := r.pool.QueryRow(
		ctx,
		`
		INSERT INTO knowledge_documents (
			title,
			source_type,
			source
		)
		VALUES ($1, $2, $3)
		RETURNING id
		`,
		document.Title,
		document.SourceType,
		document.Source,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf(
			"failed to create knowledge document: %w",
			err,
		)
	}

	return id, nil
}

func (r *KnowledgeRepository) CreateChunk(
	ctx context.Context,
	chunk model.KnowledgeChunk,
) (int64, error) {

	var id int64

	err := r.pool.QueryRow(
		ctx,
		`
		INSERT INTO knowledge_chunks (
			document_id,
			chunk_index,
			content,
			embedding
		)
		VALUES ($1, $2, $3, $4::vector)
		RETURNING id
		`,
		chunk.DocumentID,
		chunk.ChunkIndex,
		chunk.Content,
		formatVector(chunk.Embedding),
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf(
			"failed to create knowledge chunk: %w",
			err,
		)
	}

	return id, nil
}

func formatVector(embedding []float32) string {
	var builder strings.Builder

	builder.WriteByte('[')

	for i, value := range embedding {
		if i > 0 {
			builder.WriteByte(',')
		}

		fmt.Fprintf(&builder, "%g", value)
	}

	builder.WriteByte(']')

	return builder.String()
}

func (r *KnowledgeRepository) SearchSimilarChunks(
	ctx context.Context,
	embedding []float32,
	limit int,
) ([]model.KnowledgeSearchResult, error) {

	vector := formatVector(embedding)

	rows, err := r.pool.Query(
		ctx,
		`
		SELECT
			id,
			document_id,
			chunk_index,
			content,
			embedding <=> $1::vector AS distance,
			created_at
		FROM knowledge_chunks
		ORDER BY distance
		LIMIT $2
		`,
		vector,
		limit,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to search similar knowledge chunks: %w",
			err,
		)
	}

	defer rows.Close()

	var results []model.KnowledgeSearchResult

	for rows.Next() {
		var result model.KnowledgeSearchResult

		err := rows.Scan(
			&result.ID,
			&result.DocumentID,
			&result.ChunkIndex,
			&result.Content,
			&result.Distance,
			&result.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"failed to scan knowledge search result: %w",
				err,
			)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"failed while reading knowledge search results: %w",
			err,
		)
	}

	return results, nil
}
