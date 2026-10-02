-- TEC-211: the organization import list shows the uploaded file name.
ALTER TABLE import_jobs
    ADD COLUMN source_filename VARCHAR(255) NOT NULL DEFAULT '';
