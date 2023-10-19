CREATE KEYSPACE posts WITH replication = {'class': 'SimpleStrategy', 'replication_factor':1};

CREATE TYPE AttachmentId (
    id bigint,
    owner_id bigint,
    type int
);

CREATE TABLE posts (
    bucket bigint,
    id bigint,
    message text,
    attachments list<frozen <AttachmentId>>,
    owner_id bigint,
    PRIMARY KEY (bucket, id)
) WITH CLUSTERING ORDER BY (id DESC);

CREATE MATERIALIZED VIEW posts_by_owner_id AS
    SELECT * FROM posts
    WHERE owner_id IS NOT NULL AND bucket IS NOT NULL AND id IS NOT NULL
    PRIMARY KEY (owner_id, id, bucket)
WITH CLUSTERING ORDER BY (id DESC, bucket DESC);


CREATE TABLE likes (
    post_id bigint,
    owner_id bigint,
    PRIMARY KEY (post_id, owner_id)
) WITH CLUSTERING ORDER BY (owner_id DESC);

CREATE INDEX ON likes (owner_id);

CREATE TABLE comments (
    id bigint,
    post_id bigint,
    owner_id bigint,
    message text,
    attachments list<text>,
    PRIMARY KEY (post_id, id)
) WITH CLUSTERING ORDER BY (id DESC);
CREATE INDEX ON comments (owner_id);