# Implementation Plan — Article Service

**Based on:** PRD v1.0 · TDD v1.0.0  
**Language:** Go · PostgreSQL · Elasticsearch · Redis  
**Architecture:** Modular Monolith  
**Approach:** Dependency-driven. Implementasi mengikuti urutan dependency dan prioritas, bukan jadwal harian.

---

## 1. Consistency Review: PRD ↔ TDD

| Area | Status | Note |
|---|---|---|
| Endpoints (`POST /articles`, `GET /articles`, `PATCH /articles/{id}`) | ✅ Aligned | Method, path, dan contract cocok |
| Domain model (Article, Author, Outbox) | ✅ Aligned | Field dan tipe data konsisten |
| Ordering: `created_at DESC, id DESC` | ✅ Aligned | Deterministik, tersepakati |
| Author filter: exact normalized name, all matching IDs | ✅ Aligned | ADR-007 mengkonfirmasi PRD FR-04 |
| Pagination: keyset / `search_after`+PIT | ✅ Aligned | ADR-006 mengkonfirmasi PRD |
| Transactional Outbox | ✅ Aligned | PRD FR-06 ↔ TDD Section 8 |
| Redis cache: optional, TTL, fallback | ✅ Aligned | PRD FR-07 ↔ TDD Section 9 |
| Search: standard analyzer baseline | ✅ Aligned | PRD 8.2 ↔ TDD 7.4, ADR-008 |
| `created_at` immutable | ✅ Aligned | Dikonfirmasi di keduanya |
| No ORM | ✅ Aligned | Constraint eksplisit di kedua dokumen |
| Health/readiness endpoints | ✅ Aligned | Keduanya menyebutnya sebagai supporting |

**Kesimpulan:** PRD dan TDD konsisten. Tidak ada pertentangan fungsional.

---

## 2. Keputusan Teknis

Semua keputusan harus diselesaikan sebelum task yang bergantung padanya dikerjakan.  
Status: **Proposed** → **Approved** → **Implemented**

### D-01 · Normalisasi nama author

- **Status:** Approved
- **PRD:** "trim + case-insensitive; aksen/diakritik harus ditetapkan eksplisit."
- **TDD:** ES mapping memakai `lowercase_normalizer` (lowercase + asciifolding). PostgreSQL query memakai `LOWER(BTRIM(...))` — tanpa asciifolding.
- **Gap:** Behavior filter author di PostgreSQL (listing) dan di Elasticsearch (search) bisa berbeda jika input mengandung aksen (mis. `Büdi` vs `Budi`).
- **Rekomendasi:** Terapkan `LOWER(BTRIM(...))` tanpa asciifolding di kedua path. Hapus `asciifolding` dari `lowercase_normalizer` di ES. Aksen dianggap karakter berbeda. Dokumentasikan di ADR.
- **Blocks:** IMPL-09, IMPL-11, IMPL-14

### D-02 · Payload outbox: full payload vs load-on-process

- **Status:** Approved
- **TDD Section 8.2:** "Payload berisi cukup data untuk indexing. Alternatif: worker load artikel terbaru by ID."
- **Rekomendasi:** Worker load artikel terbaru by ID — lebih sederhana, selalu segar, tidak ada masalah payload basi.
- **Blocks:** IMPL-10, IMPL-12

### D-03 · Ukuran maksimum title / body / request body

- **Status:** Approved
- **PRD & TDD:** Disebutkan sebagai open question; belum ada nilai.
- **Rekomendasi:** title ≤ 500 karakter, body ≤ 50.000 karakter, request body ≤ 2 MB. Catat di ADR.
- **Blocks:** IMPL-07, IMPL-16

### D-04 · Cursor behavior saat artikel baru masuk di antara halaman

- **Status:** Approved
- **TDD Section 18 item 8:** Open question eksplisit.
- **Rekomendasi:** Keyset pagination tidak menjamin frozen snapshot. Dokumentasikan: "Artikel baru yang `created_at`-nya lebih baru dari cursor tidak akan muncul di halaman berikutnya dari sesi pagination yang sedang berjalan." Tidak perlu mekanisme tambahan.
- **Blocks:** IMPL-08

### D-05 · Freshness API setelah create/update

- **Status:** Approved
- **TDD:** "A successful POST/PATCH means the PostgreSQL transaction committed. It does not necessarily mean the search index has already been updated."
- **Rekomendasi:** Search visibility adalah eventually consistent. Tidak ada mekanisme bounded-wait untuk assessment. Dokumentasikan di API docs.
- **Blocks:** IMPL-13

### D-06 · Version column untuk event ordering

- **Status:** Approved
- **TDD Section 8.4:** Merekomendasikan kolom `version` monotonic di artikel atau event.
- **Rekomendasi:** Tambahkan kolom `version BIGINT NOT NULL DEFAULT 1` di tabel `articles`, increment on update dalam transaksi yang sama. Masukkan ke migration pertama.
- **Blocks:** IMPL-02, IMPL-12, IMPL-16

---

## 3. Urutan Implementasi Berdasarkan Dependency

```
[P1] Foundation
     IMPL-01 (scaffold) ──► IMPL-02 (migrations) ──► IMPL-03 (PG pool) ──► IMPL-04 (health)
     IMPL-05 (Docker Compose) — dapat paralel setelah IMPL-01

[P2] Core API — PostgreSQL path
     Requires: P1 complete
     IMPL-06 (domain) ──► IMPL-07 (POST) ──► IMPL-08 (GET listing) ──► IMPL-09 (author filter)

[P3] Outbox + Worker
     Requires: P2 complete, D-02 approved
     IMPL-10 (outbox repo) ──► IMPL-11 (ES connection) ──► IMPL-12 (worker)

[P4] Search — Elasticsearch path
     Requires: P3 complete, D-01 approved, D-05 approved
     IMPL-13 (keyword search) ──► IMPL-14 (combined filter) ──► IMPL-15 (search pagination)

[P5] PATCH dan Immutability
     Requires: P2 complete, IMPL-10 complete, D-03 approved, D-06 approved
     Note: tidak harus menunggu P4 selesai
     IMPL-16 (PATCH)

[P6] Redis Cache
     Requires: P2 dan P4 complete (jalur baca yang akan di-cache tersedia)
     IMPL-17 (cache)

[P7] Hardening dan Delivery
     Requires: semua Must Have complete
     IMPL-18 (logging & metrics) — dapat paralel dengan P5/P6
     IMPL-19 (benchmark) — setelah endpoint terkait stabil
     IMPL-20 (README & docs) — dikerjakan terakhir
```

Task independen dalam satu phase boleh dikerjakan paralel.

---

## 4. Execution Rules

1. Pilih task berikutnya berdasarkan dependency dan prioritas, bukan urutan waktu.
2. Hanya mulai task jika semua dependency-nya berstatus **Completed**.
3. Selesaikan dan verifikasi satu task sebelum melanjutkan task yang bergantung padanya.
4. Task independen boleh dikerjakan paralel.
5. Setelah task selesai, perbarui statusnya dan catat hasil testing.
6. Jangan mengimplementasikan fitur di luar PRD dan TDD.
7. Keputusan teknis berstatus **Proposed** harus di-approve sebelum task yang bergantung padanya dikerjakan.

---

## 5. Task Breakdown

> Konvensi ID: `IMPL-XX`  
> Prioritas: **M** = Must Have · **S** = Should Have  
> Status: **Pending** | **In Progress** | **Blocked** | **Completed**

---

### Phase 1 — Foundation

---

#### IMPL-01 · Project scaffold dan konfigurasi

**Prioritas:** M | **Status:** Completed  
**Depends on:** —  
**Tujuan:** Menyiapkan skeleton project, Go module, konfigurasi env, logging.

**Acceptance Criteria:**
- `go mod init` dengan module path yang konsisten.
- Struktur direktori sesuai TDD Section 4.
- Konfigurasi dibaca dari env variable; startup gagal jika required config hilang.
- Structured logger (slog atau zerolog) tersedia sebagai dependency injection.
- `.env.example` tersedia dan tidak menyimpan secret.

**Test Scenarios:**
- T1: Jalankan binary tanpa required env → process exit dengan pesan error yang jelas.
- T2: Jalankan binary dengan semua required env → proses start tanpa panic.

---

#### IMPL-02 · Database migrations (PostgreSQL)

**Prioritas:** M | **Status:** Completed  
**Test hasil live:** T1 UpFromEmpty ✅ · T2 UpIdempotent ✅ · T3 DownRollback ✅ · T4 UpAfterRollback ✅ (2026-11-03)
**Depends on:** IMPL-01, D-06 approved  
**Tujuan:** Membuat schema `authors`, `articles`, `outbox_events` beserta semua index.

**Acceptance Criteria:**
- Migration menggunakan Goose (versioned SQL).
- Tabel `authors`, `articles`, `outbox_events` sesuai TDD Section 6.1.
- Tabel `articles` menyertakan kolom `version BIGINT NOT NULL DEFAULT 1` (D-06).
- Tabel `outbox_events` menyertakan kolom `locked_by VARCHAR` untuk lease token dan `locked_at TIMESTAMPTZ` untuk lease expiration.
- Semua index dari TDD Section 6.2: `idx_articles_created_id`, `idx_articles_author_created_id`, `idx_authors_normalized_name`, `idx_outbox_pending`.
- `CHECK` constraint untuk blank title dan body ada di DDL.
- `goose up` dan `goose down` berjalan tanpa error dari database kosong.

**Test Scenarios:**
- T1: `goose up` dari database kosong → semua tabel dan index terbentuk, tidak ada error.
- T2: `goose up` dipanggil ulang tanpa perubahan schema → selesai tanpa error (tidak ada migrasi baru yang dijalankan).
- T3: `goose down` setelah T1 → semua tabel dan index terhapus bersih.

---

#### IMPL-03 · PostgreSQL connection pool

**Prioritas:** M | **Status:** Completed  
**Depends on:** IMPL-01  
**Tujuan:** Membuat koneksi pool ke PostgreSQL yang dapat diinjeksikan ke repository.

**Acceptance Criteria:**
- Menggunakan `pgx/v5` atau `database/sql` (pilih satu, konsisten di seluruh codebase).
- Pool config: `max_conns`, `min_conns` dibaca dari env.
- Ping dilakukan saat startup; gagal ping → service tidak ready.
- Connection pool dipakai ulang, tidak dibuat per-request.

**Test Scenarios:**
- T1 (integration): Pool connect ke PostgreSQL → ping berhasil.

---

#### IMPL-04 · Health dan Readiness endpoints

**Prioritas:** M | **Status:** Completed  
**Test hasil live:** T1 DBUp ✅ · T2 DBDown-503 ✅ · T3 Health-always-200 ✅ (compose DB 2026-11-03)
**Depends on:** IMPL-01, IMPL-03  
**Tujuan:** `GET /health` (liveness) dan `GET /ready` (readiness + dependency check).

**Acceptance Criteria:**
- `/health` → `200 OK` selama proses berjalan, tidak bergantung DB.
- `/ready` → `200 OK` hanya jika PostgreSQL reachable; `503` jika tidak.
- `/ready` tidak membocorkan detail koneksi di response body.
- Response format: `{"status": "ok"}` atau `{"status": "unavailable"}`.

**Test Scenarios:**
- T1: PostgreSQL up → `/ready` returns 200.
- T2: PostgreSQL down → `/ready` returns 503.
- T3: `/health` returns 200 terlepas dari status PostgreSQL.

---

#### IMPL-05 · Docker Compose

**Prioritas:** S | **Status:** Completed  
**Infrastruktur verified:** postgres:16.9-alpine healthy · elasticsearch:8.17.4 healthy (green) · redis:7.4.3-alpine healthy (PONG) (2026-11-03)  
**Note:** Port 5433 di-expose ke host untuk akses migration dari luar container. Port API (8080) di-expose saat `api` service di-build.
**Depends on:** IMPL-01 (untuk struktur project dan env config)  
**Paralel dengan:** IMPL-02, IMPL-03, IMPL-04  
**Tujuan:** Menjalankan semua dependency (Postgres, Redis, Elasticsearch) secara lokal.

**Acceptance Criteria:**
- Services: `api`, `worker`, `postgres`, `redis`, `elasticsearch`.
- Semua image di-pin ke versi spesifik (tidak `latest`).
- Persistent volume untuk Postgres dan Elasticsearch.
- Health check per service; `api` dan `worker` menunggu dependency-nya healthy.
- Hanya port API yang diekspos ke host.
- `docker compose up` → sistem siap tanpa langkah manual tambahan.

**Test Scenarios:**
- T1: `docker compose up -d` → semua container healthy.
- T2: `docker compose down -v && docker compose up -d` → bersih, tidak ada data lama tersisa.

---

### Phase 2 — Core API (PostgreSQL path)

---

#### IMPL-06 · Domain model dan error types

**Prioritas:** M | **Status:** Completed  
**Depends on:** IMPL-01  
**Tujuan:** Mendefinisikan `Article`, `Author`, interface repository, dan error domain.

**Acceptance Criteria:**
- Struct `Article` dan `Author` sesuai TDD Section 5.1–5.2; `Article` menyertakan field `Version int64`.
- Interface `ArticleRepository`, `AuthorRepository`, `OutboxRepository` didefinisikan di layer domain/application.
- Error types: `ErrArticleNotFound`, `ErrAuthorNotFound`, `ErrInvalidInput`, `ErrInvalidCursor`.
- Domain package tidak mengimpor package infrastruktur apapun.

**Test Scenarios:**
*(Unit — tidak butuh DB)*
- T1: Error types dapat dibandingkan dengan `errors.Is`.

---

#### IMPL-07 · POST /articles

**Prioritas:** M | **Status:** Completed  
**Test hasil live (2026-11-03):**  
- `go build ./...` ✅ · `go vet ./...` ✅ · `gofmt -l .` CLEAN  
- Application service unit tests 7/7 ✅  
- Handler unit tests (`TestCreateHandler_*`) 5/5 ✅  
- Integration PostgreSQL (`tests/integration`, compose `postgres:16.9-alpine` :5433) 4/4 ✅ termasuk rollback outbox → artikel tidak tersimpan  
**Depends on:** IMPL-02, IMPL-03, IMPL-06, D-03 approved  
**Tujuan:** Membuat artikel baru, menyimpan ke PostgreSQL, dan mencatat outbox event dalam satu transaksi.

**Acceptance Criteria:**
- Field wajib: `author_id` (> 0), `title` (tidak blank, ≤ 500 char), `body` (tidak blank, ≤ 50.000 char).
- `author_id` yang tidak ada → `404` dengan error code `AUTHOR_NOT_FOUND`.
- Field wajib kosong atau melebihi batas → `400` dengan error code `VALIDATION_ERROR`.
- Artikel dan outbox event tersimpan dalam satu transaksi PostgreSQL.
- Rollback jika salah satu insert gagal → tidak ada artikel tanpa outbox event dan sebaliknya.
- Response `201 Created` dengan representasi artikel lengkap.
- `created_at` dan `updated_at` di-set oleh database (bukan aplikasi).
- `version` = 1 pada artikel baru.

**Test Scenarios:**
*(Unit)*
- T1: Payload valid → service memanggil repository dan mengembalikan artikel.
- T2: `author_id` = 0 → `ErrInvalidInput` sebelum menyentuh DB.
- T3: Title blank string `"   "` → `ErrInvalidInput`.
- T4: Title melebihi 500 karakter → `ErrInvalidInput`.

*(Integration — PostgreSQL)*
- T5: Insert artikel valid → artikel dan outbox event ada di DB, `version` = 1.
- T6: Simulasi outbox insert gagal → transaksi rollback, artikel tidak tersimpan.
- T7: `author_id` tidak ada di tabel `authors` → `ErrAuthorNotFound`.

---

#### IMPL-08 · GET /articles — listing tanpa keyword

**Prioritas:** M | **Status:** Completed  
**Test hasil live (2026-11-03):**  
- `gofmt -l .` CLEAN · `go vet ./...` ✅ · `go build ./...` ✅  
- Domain cursor unit tests ✅ (roundtrip, corrupt, wrong kind)  
- Application List unit tests ✅ (limit 0/101, invalid cursor, empty, has_more)  
- Handler httptest ✅ (empty 200, items+pagination, INVALID_CURSOR, limit)  
- Integration PostgreSQL compose `:5433` ✅ empty · newest-first · tie-break id DESC · pagination tanpa duplikat · D-04 artikel baru tidak muncul di halaman berikutnya  
**Depends on:** IMPL-02, IMPL-03, IMPL-06, D-04 approved  
**Tujuan:** Menampilkan daftar artikel terbaru dengan keyset pagination.

**Acceptance Criteria:**
- Default `limit` = 20; maksimum = 100; nilai di luar range → `400`.
- Urutan: `created_at DESC, id DESC` (deterministik).
- Tanpa cursor → halaman pertama.
- Dengan cursor valid → halaman berikutnya dari posisi tersebut.
- Cursor tidak valid / tampered → `400 ErrInvalidCursor`.
- `has_more: true` jika masih ada data; `next_cursor: null` jika halaman terakhir.
- Query hanya ke PostgreSQL (bukan Elasticsearch).

**Test Scenarios:**
*(Unit)*
- T1: Cursor encoding/decoding → roundtrip menghasilkan nilai sama.
- T2: `limit` = 0 → `ErrInvalidInput`.
- T3: `limit` = 101 → `ErrInvalidInput`.
- T4: Cursor yang dimodifikasi/corrupt → `ErrInvalidCursor`.

*(Integration — PostgreSQL)*
- T5: Insert 5 artikel → listing mengembalikan urutan `created_at DESC, id DESC`.
- T6: Dua artikel dengan `created_at` sama → urutan stabil berdasarkan `id DESC`.
- T7: Cursor dari halaman 1 → halaman 2 tidak ada duplikat atau skip.
- T8: Artikel baru masuk setelah cursor diambil → tidak muncul di halaman berikutnya dari sesi pagination tersebut (sesuai D-04).

---

#### IMPL-09 · GET /articles?author= — filter nama author (PostgreSQL path)

**Prioritas:** M | **Status:** Completed
**Test hasil live (2026-11-03):**
- `gofmt -l .` CLEAN · `go vet ./...` ✅ · `go build ./...` ✅ · `go test -count=1 ./...` ✅
- Unit domain/application ✅ normalisasi D-01 (trim, case-insensitive, aksen dipertahankan) · cursor terikat filter author
- Handler `httptest` ✅ meneruskan parameter `author`
- Integration PostgreSQL compose `:5433` ✅ duplicate author name · author tidak ditemukan → list kosong · aksen berbeda · pagination tanpa duplikasi/skip
**Depends on:** IMPL-08, D-01 approved  
**Tujuan:** Filter artikel berdasarkan nama author, mendukung nama duplikat dengan beberapa author ID.

**Acceptance Criteria:**
- Normalisasi: `LOWER(BTRIM(name))` (D-01; tanpa asciifolding).
- Semua `author_id` dengan nama ternormalisasi yang cocok disertakan.
- Author tidak ditemukan → list kosong (`data: []`), bukan error.
- Kombinasi dengan cursor pagination berfungsi.
- Whitespace di awal/akhir nama diabaikan.
- Perbedaan kapitalisasi diabaikan.
- Aksen dianggap karakter berbeda (dokumentasikan di ADR).

**Test Scenarios:**
*(Unit)*
- T1: Normalisasi `"  Budi Santoso  "` → `"budi santoso"`.
- T2: Normalisasi `"BUDI"` → `"budi"`.
- T3: Normalisasi `"Büdi"` → `"büdi"` (bukan "budi"; aksen dipertahankan).

*(Integration — PostgreSQL)*
- T4: Dua author berbeda ID, nama sama → artikel dari kedua author muncul.
- T5: Nama author tidak ada → response `data: []`.
- T6: Filter `author` + cursor → pagination konsisten tanpa duplikat atau skip.

---

### Phase 3 — Outbox + Worker

---

#### IMPL-10 · Outbox repository

**Prioritas:** M | **Status:** Completed
**File berubah:** `migrations/00002_outbox_dead_letter.sql`, `internal/domain/article/entity.go`, `internal/repository/postgres/outbox_repository.go`, `internal/config/config.go`, `cmd/api/main.go`, serta integration dan migration tests.
**Test hasil live (2026-11-04):**
- `gofmt -l .` CLEAN · `go vet ./...` ✅ · `go build ./...` ✅
- `go test -count=1 -p 1 ./...` ✅ dalam Go Linux container karena kebijakan Windows memblokir binary test; terhubung ke PostgreSQL Docker Compose pada jaringan yang sama.
- Migration PostgreSQL up/down/up ✅; dead-letter schema tervalidasi.
- Integration PostgreSQL ✅ concurrent claim tanpa overlap · retry state/lease clear · lease aktif/expired · dead-letter pada max attempts · MarkProcessed terminal · regression POST transactional outbox.
**Depends on:** IMPL-02, IMPL-03, IMPL-06, D-02 approved  
**Tujuan:** Menyimpan dan membaca outbox event; mendukung atomic claim dengan lease token.

**Acceptance Criteria:**
- `InsertEvent(tx, event)` → insert dalam transaksi yang diberikan (atomic dengan artikel).
- `ClaimBatch(ctx, batchSize, workerID, leaseDuration)` → `SELECT ... FOR UPDATE SKIP LOCKED`; set `locked_by = workerID` dan `locked_at = now()` secara atomic dalam satu statement/transaksi.
- `MarkProcessed(ctx, id)` → set `processed_at = now()`.
- `MarkFailed(ctx, id, errMsg, nextAvailableAt)` → increment `attempts`, set `last_error`, set `available_at = nextAvailableAt`, clear `locked_by` dan `locked_at`.
- Recovery event kedaluwarsa: `ClaimBatch` tidak mengambil event yang `locked_at` sudah melewati `leaseDuration` dari worker lain; event dengan `locked_at` expired dapat diklaim ulang.
- Dead-letter: setelah `attempts >= OUTBOX_MAX_ATTEMPTS`, set status eksplisit (mis. kolom boolean `is_dead` atau `available_at` sangat jauh ke depan) agar tidak diklaim worker normal.

**Test Scenarios:**
*(Integration — PostgreSQL)*
- T1: `ClaimBatch` pada 2 worker konkuren dengan workerID berbeda → tidak ada event yang diklaim dua kali.
- T2: `MarkFailed` → `attempts` bertambah 1, `available_at` berubah ke nilai yang diberikan, `locked_by` dan `locked_at` di-clear.
- T3: Event dengan `locked_at` sudah expired (simulasi worker crash) → dapat diklaim ulang oleh worker lain.
- T4: Setelah `attempts >= MAX_ATTEMPTS` → event tidak lagi diklaim oleh `ClaimBatch` normal.

---

#### IMPL-11 · Elasticsearch connection dan mapping

**Prioritas:** M | **Status:** Completed
**File berubah:** `go.mod`, `go.sum`, `internal/platform/elasticsearch/client.go`, `internal/platform/elasticsearch/client_test.go`, `cmd/worker/main.go`.
**Implementasi:**
- Client resmi `github.com/elastic/go-elasticsearch/v8` v8.17.0 digunakan bersama Elasticsearch server `8.17.4` yang dipin di Docker Compose.
- `New` memvalidasi URL dan nama index dari konfigurasi; `Ping` memverifikasi konektivitas.
- `EnsureIndex` membuat index secara idempoten dengan field artikel sesuai TDD, standard analyzer, tiga shard, tanpa replica untuk local single-node, dan `lowercase_normalizer` yang hanya memakai filter `lowercase` sesuai D-01.
- Alias `articles-read` dibuat/diterapkan ke index terkonfigurasi.
- Worker menginisialisasi client, memeriksa konektivitas, dan memastikan index siap; pemrosesan outbox tetap menjadi scope IMPL-12.
**Test hasil live (2026-11-04):**
- Unit test validasi konfigurasi client ✅.
- Integration Elasticsearch nyata dari runner Linux pada network Docker Compose ✅: index dibuat dari scratch, mapping dan alias diverifikasi, pemanggilan kedua idempotent, tidak ada `asciifolding`.
- Full suite serial dengan PostgreSQL dan Elasticsearch nyata: `go test -count=1 -p 1 ./...` ✅.
- `gofmt -l .` CLEAN · `go vet ./...` ✅ · `go build ./...` ✅ · `git diff --check` ✅.
**Depends on:** IMPL-01, D-01 approved  
**Paralel dengan:** IMPL-10  
**Tujuan:** Inisialisasi client ES dan membuat index dengan mapping dari TDD Section 7.1–7.2.

**Acceptance Criteria:**
- Client terhubung ke URL dari env.
- Index `articles-v1` dibuat dengan mapping sesuai TDD: fields `article_id`, `author_id`, `author_name_normalized`, `title`, `body`, `created_at`, `updated_at`.
- `author_name_normalized` menggunakan normalizer `lowercase_normalizer` **tanpa** `asciifolding` (D-01).
- Alias `articles-read` menunjuk ke `articles-v1`.
- Versi Elasticsearch client kompatibel dengan versi server yang di-pin di Docker Compose.
- Index creation idempotent (sudah ada → tidak error).

**Test Scenarios:**
*(Integration — Elasticsearch)*
- T1: Buat index dari scratch → alias tersedia, mapping sesuai spec.
- T2: Jalankan index creation dua kali → tidak error, mapping tidak berubah.

---

#### IMPL-12 · Outbox worker

**Prioritas:** M | **Status:** Completed
**Depends on:** IMPL-10, IMPL-11, D-02 approved, D-06 approved  
**Tujuan:** Worker yang memproses outbox event dan mengindeks artikel ke Elasticsearch dengan ordering protection.

**Acceptance Criteria:**
- Poll interval dikonfigurasi via env (`OUTBOX_POLL_INTERVAL`).
- Worker load artikel terbaru from PostgreSQL by `article_id` (D-02); jika artikel tidak ditemukan, event dimark processed (skip, bukan retry).
- Indexing ke ES menggunakan document ID = `article_id`.
- Version ordering: sertakan `version` artikel saat indexing; gunakan ES conditional update (mis. `if_seq_no`/`if_primary_term` atau external versioning) sehingga event dengan `version` lebih lama tidak menimpa dokumen dengan `version` lebih baru.
- Retry dengan exponential backoff + jitter pada transient ES error.
- Event sukses → `MarkProcessed`.
- Event gagal transient → `MarkFailed` dengan `nextAvailableAt` sesuai backoff.
- Graceful shutdown: selesaikan batch yang sedang diproses sebelum exit.
- Worker tidak menahan transaksi DB saat menunggu ES.

**Test Scenarios:**
*(Unit)*
- T1: Worker memproses event sukses → memanggil ES index dan `MarkProcessed`.
- T2: ES gagal transient → event tidak dimark processed, `MarkFailed` dipanggil.

*(Integration)*
- T3: Insert artikel → worker memproses → dokumen ada di ES dengan `version` yang benar.
- T4: Update artikel (version 2) → worker memproses → ES dokumen menunjukkan version 2.
- T5: Event lama (version 1) diproses setelah event baru (version 2) sudah di ES → ES dokumen tetap version 2.
- T6: Worker crash di tengah batch (simulasi: lease expired) → event diklaim ulang oleh worker berikutnya dan diproses.

**Implementation Notes:**
- `internal/worker` memproses batch dengan dependency injection, memuat artikel dan author terbaru dari PostgreSQL, serta menyelesaikan batch yang telah diklaim saat shutdown.
- `internal/platform/elasticsearch` mengindeks dengan document ID artikel dan `external_gte` versioning.
- Kegagalan dijadwalkan kembali melalui `MarkFailed` menggunakan exponential backoff dengan jitter; artikel yang hilang dimark processed.
- `cmd/worker` sekarang membuka PostgreSQL, memastikan index Elasticsearch, dan menjalankan worker dengan signal-based graceful shutdown.

**Tests and Validation:**
- Unit test worker: sukses/`MarkProcessed`, artikel hilang/skip, kegagalan indexing/`MarkFailed` dengan backoff, dan penyelesaian batch saat shutdown.
- Integration test nyata: PostgreSQL + Elasticsearch memverifikasi indexing version 1, update version 2, serta stale version tidak menimpa dokumen baru.
- `gofmt -l .`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
- `git diff --check`: PASS.
- `go test -count=1 -p 1 ./...`: PASS pada runner Linux Docker di network `deployments_backend`.
- Test host Windows tanpa network Docker: integration Elasticsearch gagal DNS (`elasticsearch: no such host`); tidak digunakan sebagai hasil validasi final.

---

### Phase 4 — Search (Elasticsearch path)

---

#### IMPL-13 · GET /articles?query= — keyword search

**Prioritas:** M | **Status:** Completed
**Depends on:** IMPL-11, IMPL-12, D-05 approved  
**Tujuan:** Pencarian full-text pada title dan body menggunakan Elasticsearch.

**Acceptance Criteria:**
- `multi_match` pada `title^2` dan `body` (type: `best_fields`).
- Hasil: ID dan sort values dari ES → fetch artikel dari PostgreSQL dalam satu batch.
- Urutan ES dipertahankan saat artikel di-return.
- Artikel yang ada di ES tapi tidak ditemukan di PostgreSQL → dilewati, dicatat di log/metric.
- Query kosong atau hanya whitespace → dihandle konsisten; pilih satu: fallback ke listing tanpa keyword atau return `400`. Dokumentasikan pilihan ini.
- Eventual consistency didokumentasikan: artikel baru mungkin belum muncul di hasil pencarian.

**Test Scenarios:**
*(Integration — Elasticsearch)*
- T1: Artikel dengan kata di `title` → muncul di hasil search.
- T2: Artikel dengan kata di `body` → muncul di hasil search.
- T3: Kata bahasa Indonesia ("pemrograman") → muncul.
- T4: Kata bahasa Inggris ("programming") → muncul.
- T5: Artikel campuran dua bahasa → muncul untuk query salah satu bahasa.
- T6: Dokumen ada di ES tapi tidak ada di PostgreSQL → dilewati, respons tidak error.

**Implementation Notes:**
- `internal/platform/elasticsearch` menambahkan `multi_match` `best_fields` pada `title^2` dan `body`, dengan urutan relevansi dan `article_id` sebagai tie-breaker.
- `internal/application/article` mengambil ID dari Elasticsearch, melakukan satu kali hydration `GetByIDs` ke PostgreSQL, lalu mengembalikan artikel dalam urutan Elasticsearch.
- Hit yang tidak lagi ada di PostgreSQL dilewati dan dicatat melalui structured logger.
- Query kosong/whitespace menggunakan fallback listing PostgreSQL; kombinasi keyword dan author tetap menjadi scope IMPL-14.
- Search visibility bersifat eventual consistency sesuai D-05.

**Tests and Validation:**
- Unit test service mempertahankan urutan hasil search dan handler meneruskan query.
- Unit test client memverifikasi request `multi_match` dan parsing document ID.
- Integration Elasticsearch nyata memverifikasi pencarian istilah bahasa Indonesia, Inggris, title, dan body.
- `gofmt -l .`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
- `git diff --check`: PASS.
- `go test -count=1 -p 1 ./...`: PASS pada runner Linux Docker dengan PostgreSQL dan Elasticsearch nyata.

---

#### IMPL-14 · Kombinasi ?query= dan ?author=

**Prioritas:** M | **Status:** Completed
**Depends on:** IMPL-13, IMPL-09, D-01 approved  
**Tujuan:** Mendukung kombinasi keyword search + author filter dalam satu request.

**Acceptance Criteria:**
- ES query: `bool.must` untuk keyword, `bool.filter.term` untuk `author_name_normalized`.
- Author filter menggunakan nilai ternormalisasi `LOWER(BTRIM(name))` tanpa asciifolding (D-01); nilai yang sama digunakan saat indexing di worker.
- Tanpa keyword + ada author → gunakan PostgreSQL path (IMPL-09), bukan ES.
- Ada keyword + ada author → gunakan ES path dengan filter.
- Ada keyword + tanpa author → ES path tanpa filter author (IMPL-13).

**Test Scenarios:**
*(Integration)*
- T1: `query=golang&author=Budi` → hanya artikel dari author bernama "Budi" yang mengandung "golang".
- T2: `author=Budi` (tanpa query) → PostgreSQL path digunakan, bukan ES.
- T3: `query=golang` (tanpa author) → ES path tanpa filter author.
- T4: Dua author berbeda ID dengan nama sama + keyword → artikel dari kedua author muncul.

**Implementation Notes:**
- Search Elasticsearch menggunakan `bool.must` untuk `multi_match` dan `bool.filter.term` pada `author_name_normalized`.
- Nilai author dinormalisasi dengan `LOWER(BTRIM(name))` melalui helper domain, tanpa `asciifolding` (D-01).
- Keyword tanpa author tetap menggunakan ES; author tanpa keyword tetap menggunakan PostgreSQL listing.
- Query kosong/whitespace tetap mengikuti fallback listing PostgreSQL dari IMPL-13.

**Tests and Validation:**
- Unit test memverifikasi normalisasi author, routing query+author, dan bentuk filter term Elasticsearch.
- Integration PostgreSQL + Elasticsearch nyata memverifikasi keyword+author, author-only fallback, no-match, serta dua author dengan normalized name sama.
- `gofmt -l .`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
- `git diff --check`: PASS.
- `go test -count=1 -p 1 ./...`: PASS pada runner Linux Docker dengan PostgreSQL dan Elasticsearch nyata.

---

#### IMPL-15 · Search pagination (search_after + PIT)

**Prioritas:** M | **Status:** Completed  
**Depends on:** IMPL-13  
**Tujuan:** Pagination stabil untuk hasil pencarian ES menggunakan PIT + search_after.

**Acceptance Criteria:**
- Gunakan Point-in-Time (PIT) + `search_after` untuk deep pagination.
- Cursor opaque; berisi: PIT ID, sort values, dan fingerprint query+filter agar cursor tidak dipakai untuk query yang berbeda.
- Cursor dari query/filter berbeda → `400 ErrInvalidCursor`.
- PIT dibuka saat request pertama dan PIT ID disertakan dalam cursor untuk request berikutnya.
- TTL PIT dikonfigurasi; expired PIT menghasilkan error yang ditangani dengan baik (bukan 500).
- PIT ditutup secara eksplisit saat halaman terakhir tercapai (`has_more: false`) atau saat terjadi error fatal.
- `has_more` dan `next_cursor` konsisten dengan hasil aktual.

**Test Scenarios:**
*(Integration)*
- T1: 25 dokumen di ES, `limit=10` → halaman 1 (10 item, `has_more: true`), halaman 2 (10 item, `has_more: true`), halaman 3 (5 item, `has_more: false`, `next_cursor: null`).
- T2: Cursor dari satu query digunakan untuk query berbeda → `400 ErrInvalidCursor`.
- T3: PIT expired sebelum halaman berikutnya diminta → error informatif, bukan panic atau 500.

**Implementation:**
- Search menggunakan PIT dengan keep-alive `2m`, sort `_score desc` lalu `article_id desc`, dan `search_after`.
- Cursor opaque base64url JSON dengan kind `es_search`, PIT ID, sort values, serta SHA-256 fingerprint query+author.
- Cursor mismatch/malformed dan PIT yang tidak tersedia dipetakan ke `ErrInvalidCursor` (HTTP 400); PIT ditutup pada halaman final, hydration error, dan error halaman lanjutan.
- PostgreSQL hydration tetap dilakukan satu kali per halaman dan urutan Elasticsearch dipertahankan.

**Files and Validation:**
- `internal/platform/elasticsearch/client.go`: PIT open/search/close dan search-after.
- `internal/application/article/search_cursor.go`, `service.go`: cursor validation, fingerprint, pagination metadata.
- `internal/domain/article/search.go`, `internal/handler/http/article_handler.go`: search page contract dan forwarding cursor.
- Unit cursor tests dan real Elasticsearch PIT integration test (5 dokumen, 3 halaman).
- `gofmt -l .`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
- `git diff --check`: PASS.
- `go test -count=1 -p 1 ./...`: PASS di Docker network dengan PostgreSQL dan Elasticsearch nyata.

---

### Phase 5 — PATCH dan Immutability

---

#### IMPL-16 · PATCH /articles/{id}

**Prioritas:** M | **Status:** Completed  
**Depends on:** IMPL-06, IMPL-07, IMPL-10, D-03 approved, D-06 approved  
**Note:** Tidak harus menunggu Phase 4 selesai.  
**Tujuan:** Memperbarui title dan/atau body artikel; `created_at` tidak berubah.

**Acceptance Criteria:**
- Setidaknya satu field (`title` atau `body`) harus ada di payload; jika tidak ada → `400`.
- Field omitted dibedakan dari field bernilai empty string; empty string → `400`.
- Title blank atau melebihi 500 karakter → `400`.
- Body blank atau melebihi 50.000 karakter → `400`.
- `created_at` tidak pernah diupdate di SQL statement (`UPDATE ... SET title=..., body=..., updated_at=..., version=version+1`).
- `updated_at` diperbarui ke waktu sekarang.
- `version` di-increment (+1) dalam transaksi yang sama (D-06).
- Outbox event `article.updated` disimpan dalam transaksi yang sama.
- Article ID tidak ada → `404`.
- Response `200 OK` dengan representasi artikel yang sudah diperbarui.

**Test Scenarios:**
*(Unit)*
- T1: Payload `{}` (tidak ada field) → `ErrInvalidInput`.
- T2: Payload `{"title": ""}` → `ErrInvalidInput`.
- T3: Payload `{"title": "New Title"}` → service mengupdate hanya `title`, `body` tidak berubah.

*(Integration — PostgreSQL)*
- T4: PATCH valid → nilai `created_at` sebelum dan sesudah identik.
- T5: PATCH valid → `updated_at` sesudah > `updated_at` sebelum.
- T6: PATCH valid → `version` bertambah tepat 1.
- T7: PATCH valid → outbox event `article.updated` tersimpan dalam transaksi yang sama.
- T8: PATCH article ID tidak ada → `404`.

**Implementation:**
- Menambahkan `PATCH /articles/{id}` dengan payload pointer fields agar omitted dan empty string dibedakan.
- Service memvalidasi title/body, mempertahankan field yang tidak dikirim, meng-update title/body melalui transaksi, menaikkan version, dan membuat outbox `article.updated` pada transaksi yang sama.
- Repository tidak mengubah `created_at`; `updated_at` diisi PostgreSQL dengan `NOW()`.
- Error article tidak ditemukan dipetakan ke HTTP 404 dan response sukses menggunakan representasi artikel terbaru.

**Files and Validation:**
- `internal/application/article/service.go`: update use case dan transactional outbox.
- `internal/handler/http/article_handler.go`, `router.go`: PATCH route dan request parsing.
- `tests/integration/post_article_test.go`: PostgreSQL verification untuk created_at, updated_at, version, outbox payload, dan 404.
- Unit tests untuk title-only update, empty payload, dan PATCH handler.

---

### Phase 6 — Redis Cache

---

#### IMPL-17 · Redis cache untuk listing dan search

**Prioritas:** S | **Status:** Completed  
**Depends on:** IMPL-08, IMPL-09, IMPL-13, IMPL-14 (jalur baca yang akan di-cache tersedia)  
**Tujuan:** Cache GET responses untuk mengurangi beban PostgreSQL/ES pada read-heavy traffic.

**Acceptance Criteria:**
- Cache key deterministik: `articles:v1:q=<normalized>:author=<normalized>:limit=<n>:cursor=<token>`.
- TTL dikonfigurasi via env (default 45s).
- Cache hit → return tanpa menyentuh DB/ES.
- Cache miss → fetch dari source, simpan ke cache.
- Cache gagal (Redis down/timeout) → fallback ke source, log error, tidak return error ke client.
- Invalidasi: gunakan namespace versioning atau TTL pendek; pilih satu pendekatan, dokumentasikan.
- Stampede protection: gunakan `singleflight` untuk request konkuren ke key yang sama saat cache miss.

**Test Scenarios:**
*(Integration — Redis)*
- T1: Request sama dua kali → hasil identik; request kedua tidak menyentuh DB/ES.
- T2: TTL expired → cache miss, fetch dari source berhasil.
- T3: Redis unavailable → request tetap berhasil dari source tanpa error ke client.
- T4: Dua request konkuren ke key sama saat cache miss → hanya satu yang ke DB/ES (`singleflight`).
- T5: Artikel diupdate → cache lama tidak dikembalikan setelah TTL atau invalidasi.

**Implementation:**
- Menambahkan adapter Redis opsional dengan konfigurasi `REDIS_ADDR`, password, DB, dan `CACHE_TTL` (default 45 detik).
- Listing dan keyword search memakai key deterministik `articles:v1:q=<normalized>:author=<normalized>:limit=<n>:cursor=<token>`.
- Cache hit menghindari source query; cache miss mengambil source lalu menyimpan hasil.
- Error Redis atau payload cache invalid tidak menggagalkan request; source tetap digunakan dan error dicatat.
- `singleflight.Group` mencegah request konkuren pada cache miss yang sama memanggil source berulang.
- Kebijakan invalidasi menggunakan TTL pendek dan namespace key `articles:v1`; update artikel tidak menghapus cache secara aktif.

**Files and Validation:**
- `internal/platform/cache/redis.go`: Redis adapter.
- `internal/application/article/service.go`: cache key, fallback, TTL, singleflight.
- `cmd/api/main.go`: optional Redis wiring.
- Unit tests cache hit/fallback dan real Redis set/get integration test.

---

### Phase 7 — Hardening dan Delivery

---

#### IMPL-18 · Structured logging dan metrics

**Prioritas:** S | **Status:** Completed  
**Depends on:** IMPL-04  
**Paralel dengan:** IMPL-16, IMPL-17  
**Tujuan:** Logging dan metrics dasar sesuai TDD Section 13.

**Acceptance Criteria:**
- Setiap request dicatat: method, route, status code, duration, request ID.
- Worker log mencatat: event ID, article ID, attempts, error category.
- Tidak log body artikel atau secret.
- Metrics minimal (log fields atau Prometheus counter): request count, error rate, cache hit/miss, outbox pending count, search latency.
- Request ID di-generate per request dan disertakan di response header (`X-Request-ID`).

**Implementation:**
- Middleware observability mencatat method, route template, status, duration, request ID, dan error category tanpa body request.
- Request ID memakai incoming `X-Request-ID` bila tersedia atau ID baru yang unik per process; selalu dikembalikan pada response.
- Registry metrics process-local memakai atomic counters untuk request/error, cache hit/miss, search latency, dan outbox pending count.
- Worker mencatat event ID, article ID, attempts, error category, serta event sukses; pending outbox dihitung dari PostgreSQL bila tersedia.
- Search service mengukur latency source search, sementara cache adapter tetap fallback tanpa membocorkan payload.

**Files and Validation:**
- `internal/handler/http/observability.go`, `router.go`: request middleware dan wiring.
- `internal/platform/metrics/metrics.go`: atomic metrics registry.
- `internal/application/article/service.go`: cache dan search instrumentation.
- `internal/worker/outbox_worker.go`, `internal/repository/postgres/outbox_repository.go`: worker logs dan pending count.
- `cmd/api/main.go`, `cmd/worker/main.go`: dependency injection.
- Unit tests untuk registry dan HTTP request observability.
- Validasi: `gofmt -l .`, `go vet ./...`, `go build ./...`, `git diff --check`, dan serial integration suite dengan PostgreSQL/Elasticsearch/Redis.

---

#### IMPL-19 · Dataset sintetis dan benchmark k6

**Prioritas:** S | **Status:** Completed  
**Depends on:** IMPL-08, IMPL-09, IMPL-13, IMPL-14 stabil (endpoint terkait sudah Completed)  
**Tujuan:** Memvalidasi performa dengan data yang merepresentasikan distribusi assessment.

**Acceptance Criteria:**
- Script seed: ukuran dataset configurable (tidak dipaksakan 50 juta di lingkungan lokal); distribusi 1 author dominan (~30%), long-tail authors.
- Skenario k6 sesuai TDD Section 12.6: listing, author dominant, author long-tail, keyword search, kombinasi, cache hit, mixed read/write.
- Record: p50, p95, p99 latency, throughput, error rate.
- `EXPLAIN (ANALYZE, BUFFERS)` untuk query utama dijalankan dan hasilnya didokumentasikan.
- Keterbatasan benchmark (dataset size yang digunakan vs target 50 juta) didokumentasikan secara eksplisit.

**Implementation:**
- `scripts/benchmark/seed.go` membuat jumlah author dan article yang dapat dikonfigurasi.
- Author pertama menerima sepertiga data (33.4% pada dataset uji 1,000 artikel); sisa data didistribusikan acak ke long-tail authors.
- `scripts/benchmark/article_service.js` menyediakan skenario listing, dominant author, long-tail author, keyword, combined filter, cache hit, cache miss, bursty concurrent reads, dan mixed read/write.
- `scripts/benchmark/explain.sql` menyediakan query `EXPLAIN (ANALYZE, BUFFERS)` untuk listing dan author filtering.

**Actual validation (2026-11-04):**
- Dataset: 1,000 articles, 20 authors, dominant author 334 articles (33.4%), 19 long-tail authors.
- k6 dijalankan terhadap API image melalui Docker network dengan PostgreSQL, Elasticsearch, dan Redis nyata. Semua 9 scenarios selesai tanpa interrupted iterations.
- Aggregate k6 result: p50 `0.922565 ms`, p95 `1.9767635 ms`, p99 `20.2779695 ms`, throughput `28.3527247 req/s`, error rate `0`.
- PostgreSQL listing `EXPLAIN`: index scan menggunakan `idx_articles_created_id`, 21 rows, execution `0.375 ms`, shared buffers hit `3`.
- PostgreSQL dominant-author `EXPLAIN`: author lookup dan `idx_articles_author_created_id`, 21 rows returned after top-N sort, execution `0.449 ms`, shared buffers hit `20`.
- Validation commands: `gofmt -l .`, `go vet ./...`, `go build ./...`, `git diff --check`, and `go test -count=1 -p 1 ./...` passed.

**Limitations:**
- Local benchmark uses 1,000 articles rather than the 50-million target; results are not a capacity claim for 50 million rows.
- k6 result is an aggregate across scenarios at local load and does not replace a production-sized capacity test.

---

#### IMPL-20 · README dan dokumentasi final

**Prioritas:** M | **Status:** Completed  
**Depends on:** Semua Must Have task Completed  
**Tujuan:** Dokumentasi yang cukup untuk evaluator menjalankan dan menilai service.

**Acceptance Criteria:**
- Cara menjalankan: prerequisites, `docker compose up`, migrate, seed.
- Cara menjalankan tests: unit, integration, benchmark.
- API contract ringkas atau link ke OpenAPI spec.
- ADR untuk setiap keputusan teknis signifikan: ADR-001 s/d ADR-008 dari TDD, ditambah keputusan D-01 s/d D-06 dari dokumen ini.
- Penggunaan AI: tools yang dipakai, bagian yang dibantu, cara output diverifikasi, satu contoh kegagalan AI nyata beserta koreksinya.
- Keterbatasan yang diketahui (known limitations).

**Implementation:**
- `README.md` mendokumentasikan arsitektur, konfigurasi, Docker Compose,
  migration, seed, API contract, tests, benchmark, ADR, AI verification, dan
  known limitations.
- `docs/adr/ADR-001` sampai `ADR-008` mendokumentasikan keputusan arsitektur
  aktual dari TDD.
- `docs/decisions.md` mendokumentasikan D-01 sampai D-06 yang telah disetujui.
- README menyatakan secara eksplisit bahwa tidak ada OpenAPI generated file,
  migration dijalankan eksplisit, dan benchmark lokal 1.000 artikel bukan
  klaim kapasitas 50 juta artikel.

**Validation:**
- Semua Must Have IMPL-01 sampai IMPL-16 berstatus Completed.
- IMPL-18 dan IMPL-19 juga berstatus Completed.
- Dokumentasi diverifikasi terhadap source tree, `.env.example`, Makefile,
  Docker Compose, migration, TDD API contract, hasil benchmark aktual, dan
  serial test suite yang telah lulus.

### Production readiness audit (2026-11-04)

**Status:** Not Production Ready. The repository is deployment-prepared for
an external production platform, but critical controls remain external:

- Authentication and authorization are not implemented; an identity-aware
  gateway is mandatory before public exposure.
- The development Compose file intentionally uses plaintext local credentials
  and disables Elasticsearch security. It is not a production topology.
- The production Compose file now requires HTTPS Elasticsearch, PostgreSQL TLS,
  Elasticsearch credentials, Redis password, non-root/read-only containers,
  dropped capabilities, and restart policies. Secrets must be injected by an
  external secret manager.
- `config.Load` now rejects invalid pool/batch/TTL values and insecure
  production dependency settings.
- `.dockerignore` excludes local env files, tests, specs, docs, binaries, and
  temporary files from the build context.

**Validated:** production Compose interpolation with required placeholders,
production image build, config security tests, `go vet`, `go build`,
`git diff --check`, and the serial full test suite with real PostgreSQL,
Elasticsearch, and Redis. No production deployment was performed.

**Remaining blockers:** external authentication/authorization, secret
manager, managed TLS certificates, backup/PITR service and restore drill,
ingress/rate limiting/WAF, alerting, and production-scale capacity testing.

---

## 6. Prioritas

### Must Have — wajib memenuhi assessment

| ID | Task |
|---|---|
| IMPL-01 | Project scaffold & config |
| IMPL-02 | DB migrations |
| IMPL-03 | PostgreSQL connection pool |
| IMPL-04 | Health/readiness endpoints |
| IMPL-06 | Domain model & errors |
| IMPL-07 | POST /articles |
| IMPL-08 | GET /articles listing |
| IMPL-09 | GET /articles?author= (PG path) |
| IMPL-10 | Outbox repository |
| IMPL-11 | ES connection & mapping |
| IMPL-12 | Outbox worker |
| IMPL-13 | GET /articles?query= (ES search) |
| IMPL-14 | Kombinasi query + author |
| IMPL-15 | Search pagination (PIT) |
| IMPL-16 | PATCH /articles/{id} |
| IMPL-20 | README & dokumentasi final |

### Should Have — nilai tambah, tidak blocker

| ID | Task |
|---|---|
| IMPL-05 | Docker Compose |
| IMPL-17 | Redis cache |
| IMPL-18 | Structured logging & metrics |
| IMPL-19 | Dataset sintetis & k6 benchmark |

---

*Dokumen ini hanya mencakup scope assessment. Tidak ada task di luar PRD dan TDD yang ditambahkan.*
