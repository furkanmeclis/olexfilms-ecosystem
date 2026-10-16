-- TEC-762: durable preview and ownership of nodes created by a location import.
CREATE TABLE warehouse_location_import_rows (
    job_id BIGINT NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    row_number INTEGER NOT NULL,
    path JSONB NOT NULL,
    status VARCHAR(24) NOT NULL,
    error_code VARCHAR(64),
    target VARCHAR(255),
    PRIMARY KEY (job_id, row_number)
);

CREATE TABLE warehouse_location_import_nodes (
    job_id BIGINT NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL,
    node_id BIGINT NOT NULL,
    depth INTEGER NOT NULL,
    undone BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (job_id, kind, node_id)
);
