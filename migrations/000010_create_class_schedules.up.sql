CREATE TABLE class_schedules (
  id          SERIAL PRIMARY KEY,
  student_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
  start_time  TEXT NOT NULL,
  timezone    TEXT NOT NULL
);

CREATE INDEX idx_class_schedules_student ON class_schedules (student_id);
