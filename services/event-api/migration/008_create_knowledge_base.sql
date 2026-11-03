CREATE TABLE knowledge_documents (
    id BIGSERIAL PRIMARY KEY,

    title TEXT NOT NULL,

    source_type TEXT NOT NULL,

    source TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE TABLE knowledge_chunks (
    id BIGSERIAL PRIMARY KEY,

    document_id BIGINT NOT NULL,

    chunk_index INT NOT NULL,

    content TEXT NOT NULL,

    embedding VECTOR(768),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_knowledge_chunks_document
        FOREIGN KEY (document_id)
        REFERENCES knowledge_documents(id)
        ON DELETE CASCADE,

    CONSTRAINT unique_document_chunk
        UNIQUE (document_id, chunk_index)
);


CREATE INDEX idx_knowledge_chunks_document_id
ON knowledge_chunks(document_id);
