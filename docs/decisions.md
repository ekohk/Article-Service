# Approved Decisions D-01 to D-06

## D-01 — Author normalization

Use `LOWER(BTRIM(name))` for exact author matching. Do not use ASCII folding;
accented and unaccented names are distinct.

## D-02 — Minimal outbox payload

Outbox events contain the minimal article identity/event information. The
worker loads the latest article and author from PostgreSQL before indexing.

## D-03 — Size limits

Title is limited to 500 characters, body to 50,000 characters, and an HTTP
request body to 2 MB.

## D-04 — PostgreSQL cursor behavior

PostgreSQL keyset pagination is not a frozen snapshot. Articles inserted
after a cursor was created and newer than that cursor do not appear in later
pages of that pagination session.

## D-05 — Eventual consistency

A successful PostgreSQL write does not guarantee immediate Elasticsearch
visibility. The outbox worker provides asynchronous synchronization.

## D-06 — Monotonic article version

Articles have a `version` column starting at 1. Updates increment it in the
same transaction as the article mutation and outbox event, allowing ordered
index updates.
