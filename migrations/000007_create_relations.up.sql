CREATE TABLE relations (
    coach_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    mentor_id TEXT,
    PRIMARY KEY (coach_id, user_id)
);

CREATE INDEX idx_relations_mentor_id ON relations (mentor_id);