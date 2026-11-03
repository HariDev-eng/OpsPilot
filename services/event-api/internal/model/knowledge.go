package model

import "time"

type KnowledgeDocument struct {
	ID         int64
	Title      string
	SourceType string
	Source     *string
	CreatedAt  time.Time
}

type KnowledgeChunk struct {
	ID         int64
	DocumentID int64
	ChunkIndex int
	Content    string
	Embedding  []float32
	CreatedAt  time.Time
}

type KnowledgeSearchResult struct {
	KnowledgeChunk
	Distance float64
}
