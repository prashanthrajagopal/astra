
CREATE TABLE task_dependencies (
 task_id UUID,
 depends_on UUID,
 PRIMARY KEY(task_id, depends_on)
);
