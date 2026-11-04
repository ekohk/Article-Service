-- Run with psql against the benchmark database.
-- Replace the author name and cursor values when inspecting a specific page.
\timing on

EXPLAIN (ANALYZE, BUFFERS)
SELECT a.id, a.author_id, a.title, a.body, a.version, a.created_at, a.updated_at
FROM articles AS a
ORDER BY a.created_at DESC, a.id DESC
LIMIT 21;

EXPLAIN (ANALYZE, BUFFERS)
SELECT a.id, a.author_id, a.title, a.body, a.version, a.created_at, a.updated_at
FROM articles AS a
JOIN authors AS au ON au.id = a.author_id
WHERE LOWER(BTRIM(au.name)) = LOWER(BTRIM('Benchmark Author 0001'))
ORDER BY a.created_at DESC, a.id DESC
LIMIT 21;
