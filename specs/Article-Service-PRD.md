# Product Requirements Document (PRD)

## Article Service

**Status:** Proposed\
**Document type:** Product Requirements Document\
**Version:** 1.0\
**Scope:** Backend article service assessment

------------------------------------------------------------------------

## 1. Overview

### 1.1 Purpose

Dokumen ini mendefinisikan kebutuhan produk, ruang lingkup, perilaku
sistem, aturan bisnis, kebutuhan fungsional, dan kriteria penerimaan
untuk pengembangan **Article Service**.

Article Service merupakan layanan backend untuk membuat, menyimpan,
memperbarui, mencari, dan menampilkan artikel. Sistem dirancang untuk
menangani volume data besar, pertumbuhan data harian, distribusi artikel
yang tidak merata antarpenulis, serta pencarian konten berbahasa
Indonesia dan Inggris.

Dokumen ini menjadi acuan implementasi, pengujian, dan pengambilan
keputusan teknis selama pengerjaan assessment.

### 1.2 Product Goal

Membangun layanan artikel yang:

-   Menyediakan API untuk membuat, membaca, mencari, dan memperbarui
    artikel.
-   Tetap responsif pada dataset besar dan pertumbuhan data
    berkelanjutan.
-   Mendukung pencarian judul dan isi artikel.
-   Mendukung filter berdasarkan nama penulis, termasuk ketika nama yang
    sama dimiliki beberapa author ID.
-   Menjaga konsistensi data utama dan menyediakan mekanisme pemulihan
    indeks pencarian.
-   Memiliki automated tests untuk memvalidasi perilaku dan aturan
    bisnis.

### 1.3 Success Metrics

Keberhasilan implementasi dinilai berdasarkan:

1.  Seluruh endpoint wajib berjalan sesuai kontrak API.
2.  Pembuatan, pembacaan, pencarian, dan filter artikel menghasilkan
    data yang benar.
3.  Artikel ditampilkan dengan urutan terbaru lebih dahulu.
4.  Perubahan artikel tidak mengubah nilai `created_at`.
5.  Filter nama penulis menangani nama duplikat sesuai keputusan produk.
6.  Sistem dapat diuji menggunakan dataset sintetis yang
    merepresentasikan skala dan distribusi data assessment.
7.  Kegagalan sinkronisasi indeks pencarian dapat dideteksi dan
    dipulihkan.
8.  Automated tests mencakup aturan bisnis, API, dan integrasi komponen
    utama.

Target angka latency dan throughput final ditentukan melalui benchmark
karena assessment tidak menetapkan SLO numerik.

------------------------------------------------------------------------

## 2. Background and Constraints

### 2.1 Workload

Berdasarkan requirement assessment, sistem perlu mempertimbangkan
karakteristik berikut:

  -----------------------------------------------------------------------
  Area                                Requirement
  ----------------------------------- -----------------------------------
  Initial dataset                     50 juta artikel

  Data growth                         200.000 artikel baru per hari

  Read/write ratio                    Sekitar 100:1

  Traffic pattern                     Bursty / dapat mengalami lonjakan

  Dominant author                     Satu author menghasilkan sekitar
                                      30% seluruh artikel

  Long-tail authors                   Sebagian besar author lain memiliki
                                      kurang dari 100 artikel

  Content language                    Bahasa Indonesia, Inggris, atau
                                      campuran dalam satu artikel

  Language metadata                   Tidak tersedia pada schema awal

  Article updates                     Artikel dapat diperbarui

  Creation timestamp                  `created_at` tidak berubah ketika
                                      artikel diperbarui
  -----------------------------------------------------------------------

### 2.2 Technical Constraints

-   Implementasi menggunakan Golang.
-   Database dan search engine dapat dipilih sesuai pertimbangan teknis.
-   ORM tidak diperbolehkan.
-   Automated tests wajib disertakan.
-   Docker Compose bersifat opsional, tetapi direkomendasikan untuk
    mempermudah evaluasi lokal.
-   AI boleh digunakan selama penggunaannya diungkapkan dan disertai
    satu contoh kegagalan nyata beserta koreksinya.
-   Waktu pengerjaan assessment adalah tiga hari.

### 2.3 Assumptions

Asumsi berikut digunakan agar requirement dapat diterjemahkan menjadi
perilaku sistem:

-   Setiap artikel memiliki ID unik.
-   Setiap artikel memiliki satu `author_id`.
-   Data author disimpan terpisah dari artikel.
-   Nama author bukan identifier unik.
-   Artikel tidak dihapus dalam scope awal karena endpoint penghapusan
    tidak diminta.
-   Database relasional menjadi sumber data utama.
-   Search index dan cache merupakan komponen pendukung yang dapat
    dibangun ulang atau diisi ulang dari sumber data utama.
-   Detail schema yang tidak ditentukan assessment merupakan keputusan
    implementasi dan harus dicatat.

------------------------------------------------------------------------

## 3. Users and Use Cases

### 3.1 Primary User

Konsumen API internal atau aplikasi lain yang membutuhkan kemampuan
untuk membuat, menampilkan, mencari, dan memperbarui artikel.

Assessment tidak mensyaratkan implementasi frontend atau autentikasi
pengguna.

### 3.2 Core Use Cases

1.  Konsumen API membuat artikel baru.
2.  Konsumen API mengambil daftar artikel terbaru.
3.  Konsumen API mencari artikel berdasarkan kata atau frasa pada judul
    dan isi.
4.  Konsumen API memfilter artikel berdasarkan nama author.
5.  Konsumen API menggabungkan pencarian teks dan filter author.
6.  Konsumen API memperbarui artikel tanpa mengubah waktu pembuatan.
7.  Sistem memproses perubahan artikel agar indeks pencarian tetap
    sinkron.
8.  Sistem memulihkan indeks pencarian setelah terjadi kegagalan atau
    kehilangan data indeks.

------------------------------------------------------------------------

## 4. Scope

### 4.1 In Scope

-   REST API untuk article creation.
-   REST API untuk article listing.
-   Pencarian berdasarkan judul dan body.
-   Filter berdasarkan nama author.
-   Kombinasi query pencarian dan filter author.
-   Pagination untuk pengambilan data.
-   Pengurutan artikel terbaru lebih dahulu.
-   Update artikel sebagai kebutuhan pendukung karena artikel dapat
    diedit.
-   Penyimpanan data artikel dan author.
-   Sinkronisasi perubahan data ke search engine.
-   Caching untuk mengurangi beban pembacaan yang berulang.
-   Automated tests.
-   Dokumentasi API dan keputusan arsitektur.
-   Docker Compose untuk menjalankan dependensi lokal jika memungkinkan.

### 4.2 Out of Scope

-   Frontend atau user interface.
-   Registrasi dan manajemen author.
-   Autentikasi dan otorisasi pengguna.
-   Penghapusan artikel.
-   Komentar, kategori, tag, likes, dan fitur sosial.
-   Moderasi konten.
-   Workflow editorial atau approval.
-   Analitik produk lanjutan.
-   Multi-tenancy.
-   Penyimpanan file atau media artikel.
-   Implementasi machine learning untuk klasifikasi bahasa.

------------------------------------------------------------------------

## 5. Functional Requirements

### FR-01 --- Create Article

Sistem harus menyediakan endpoint untuk membuat artikel.

**Endpoint:** `POST /articles`

**Input minimum:**

-   `title`
-   `body`
-   `author_id`

**Expected behavior:**

-   Sistem memvalidasi field wajib.
-   Sistem memastikan author yang direferensikan tersedia.
-   Sistem membuat ID artikel yang unik.
-   Sistem menetapkan `created_at` dan `updated_at` pada saat pembuatan.
-   Artikel disimpan ke database utama.
-   Perubahan untuk kebutuhan sinkronisasi search index dicatat secara
    atomik bersama penyimpanan artikel.
-   Sistem mengembalikan representasi artikel yang berhasil dibuat.

**Acceptance criteria:**

-   Request valid menghasilkan respons sukses dan data artikel.
-   Request dengan field wajib yang hilang ditolak dengan respons
    validasi.
-   Request dengan `author_id` yang tidak tersedia ditolak.
-   Kegagalan penyimpanan tidak meninggalkan event sinkronisasi yang
    tidak memiliki artikel terkait.
-   Pembuatan artikel tidak bergantung pada keberhasilan Elasticsearch
    secara sinkron.

### FR-02 --- List Articles

Sistem harus menyediakan endpoint untuk mengambil daftar artikel.

**Endpoint:** `GET /articles`

**Expected behavior:**

-   Artikel ditampilkan dari yang terbaru ke yang lebih lama.
-   Pagination tersedia untuk membatasi jumlah data per request.
-   Sistem menggunakan urutan yang deterministik, termasuk tie-breaker
    ketika `created_at` sama.
-   Endpoint dapat digunakan tanpa parameter pencarian.

**Acceptance criteria:**

-   Hasil diurutkan berdasarkan `created_at` descending.
-   Artikel dengan timestamp sama tetap memiliki urutan stabil
    menggunakan ID sebagai tie-breaker.
-   Pagination tidak mengulang atau melewatkan item akibat perubahan
    offset pada data yang terus bertambah.
-   Batas maksimum ukuran halaman diterapkan.

### FR-03 --- Search Articles

Sistem harus mendukung pencarian teks pada judul dan isi artikel.

**Endpoint:** `GET /articles?query={search_term}`

**Expected behavior:**

-   Parameter `query` digunakan untuk mencari pada title dan body.
-   Pencarian dapat dikombinasikan dengan filter author.
-   Perilaku pencarian untuk bahasa Indonesia, Inggris, dan konten
    campuran harus diuji.
-   Sistem tidak mengasumsikan setiap artikel hanya memiliki satu
    bahasa.

**Acceptance criteria:**

-   Artikel yang cocok pada title dapat ditemukan.
-   Artikel yang cocok pada body dapat ditemukan.
-   Pencarian dapat dikombinasikan dengan parameter `author`.
-   Query kosong atau hanya whitespace ditangani secara konsisten.
-   Hasil mengikuti urutan relevansi atau aturan pengurutan yang
    ditetapkan pada kontrak API; jika menggunakan relevansi, aturan
    tie-breaker harus deterministik.
-   Keterlambatan sinkronisasi search index didokumentasikan.

### FR-04 --- Filter by Author Name

Sistem harus mendukung filter artikel berdasarkan nama author.

**Endpoint:** `GET /articles?author={author_name}`

**Expected behavior:**

-   Filter menggunakan nama author, bukan menganggap nama sebagai
    identifier unik.
-   Jika beberapa author ID memiliki nama yang sama, seluruh artikel
    dari author dengan nama yang cocok akan disertakan.
-   Pencocokan nama menggunakan aturan normalisasi yang konsisten.
-   Rekomendasi baseline: hapus whitespace di awal/akhir dan lakukan
    pencocokan case-insensitive.
-   Perilaku terhadap aksen/diakritik dan normalisasi Unicode harus
    ditetapkan secara eksplisit sebelum implementasi final.

**Acceptance criteria:**

-   Artikel dari author dengan nama yang cocok dikembalikan.
-   Jika nama yang sama dimiliki beberapa author ID, artikel dari
    seluruh author tersebut dikembalikan.
-   Perbedaan kapitalisasi tidak menyebabkan hasil yang tidak konsisten.
-   Whitespace di awal/akhir parameter tidak mengubah hasil.
-   Nama author yang tidak ditemukan menghasilkan daftar kosong, bukan
    error server.

### FR-05 --- Update Article

Karena artikel dapat diedit, sistem menyediakan endpoint pendukung untuk
memperbarui artikel.

**Endpoint:** `PATCH /articles/{id}`

**Expected behavior:**

-   Field yang didukung untuk diperbarui ditentukan dalam kontrak API.
-   Sistem memperbarui `updated_at`.
-   Sistem tidak mengubah `created_at`.
-   Perubahan dicatat untuk sinkronisasi search index.
-   Artikel yang tidak ditemukan menghasilkan respons not found.

**Acceptance criteria:**

-   Artikel dapat diperbarui dengan payload valid.
-   `created_at` tetap sama sebelum dan sesudah update.
-   `updated_at` berubah setelah update.
-   Search index menerima event perubahan.
-   Update terhadap ID yang tidak tersedia menghasilkan respons 404.

### FR-06 --- Search Index Synchronization

Sistem harus menyinkronkan artikel ke search engine tanpa menjadikan
search engine sebagai sumber data utama.

**Expected behavior:**

-   Database utama dan event sinkronisasi disimpan dalam satu transaksi.
-   Worker memproses event secara asynchronous.
-   Event dapat dicoba ulang ketika terjadi kegagalan sementara.
-   Pemrosesan event harus idempotent.
-   Mekanisme mencegah event lama menimpa versi artikel yang lebih baru.
-   Sistem menyediakan strategi reindex atau backfill.

**Acceptance criteria:**

-   Artikel yang berhasil dibuat atau diperbarui pada database memiliki
    event sinkronisasi terkait.
-   Kegagalan search engine tidak menghilangkan data artikel dari
    database utama.
-   Event yang gagal dapat dicoba ulang.
-   Pemrosesan ulang event tidak membuat duplikasi dokumen.
-   Reindex dapat dijalankan dari database utama.

### FR-07 --- Caching

Sistem dapat menggunakan cache untuk mengurangi beban query berulang.

**Expected behavior:**

-   Cache tidak menjadi sumber data utama.
-   Cache dapat dihapus atau diperbarui ketika artikel berubah.
-   Cache memiliki TTL.
-   Sistem tetap dapat melayani request ketika cache tidak tersedia,
    dengan mengambil data dari sumber utama atau search engine sesuai
    jenis request.
-   Mekanisme cache mempertimbangkan cache stampede.

**Acceptance criteria:**

-   Cache hit mengembalikan hasil yang valid.
-   Cache miss mengambil data dari sumber yang benar.
-   Update artikel tidak membuat cache lama bertahan melebihi kebijakan
    konsistensi yang ditetapkan.
-   Gangguan Redis tidak menyebabkan kehilangan data atau kegagalan
    permanen layanan.

------------------------------------------------------------------------

## 6. API Requirements

### 6.1 Endpoint Summary

  Method   Endpoint              Purpose                          Priority
  -------- --------------------- -------------------------------- ------------
  POST     `/articles`           Membuat artikel                  Must
  GET      `/articles`           Mengambil daftar artikel         Must
  GET      `/articles?query=`    Mencari title dan body           Must
  GET      `/articles?author=`   Filter berdasarkan nama author   Must
  PATCH    `/articles/{id}`      Memperbarui artikel              Supporting
  GET      `/health`             Liveness check                   Supporting
  GET      `/ready`              Readiness check                  Supporting

### 6.2 List Query Parameters

  Parameter   Description                               Proposed default
  ----------- ----------------------------------------- ------------------
  `query`     Kata/frasa untuk mencari title dan body   Tidak ada
  `author`    Nama author untuk filter                  Tidak ada
  `limit`     Jumlah item per halaman                   20
  `cursor`    Token pagination                          Tidak ada

Batas maksimum `limit` yang diusulkan adalah 100. Nilai final dapat
disesuaikan berdasarkan hasil benchmark.

### 6.3 Response and Error Contract

API harus menggunakan HTTP status code secara konsisten.

Contoh kategori respons:

-   `200 OK`: pembacaan atau update berhasil.
-   `201 Created`: artikel berhasil dibuat.
-   `400 Bad Request`: parameter atau payload tidak valid.
-   `404 Not Found`: artikel atau resource yang dituju tidak ditemukan.
-   `422 Unprocessable Entity`: payload valid secara format tetapi
    melanggar aturan bisnis, jika dipilih sebagai konvensi API.
-   `500 Internal Server Error`: kegagalan yang tidak terduga.
-   `503 Service Unavailable`: layanan belum siap atau dependensi wajib
    tidak tersedia.

Format error harus konsisten dan tidak membocorkan detail internal.

------------------------------------------------------------------------

## 7. Data Requirements

### 7.1 Article

Atribut minimum yang diusulkan:

  Field          Description
  -------------- --------------------------
  `id`           Identifier unik artikel
  `author_id`    Referensi ke author
  `title`        Judul artikel
  `body`         Isi artikel
  `created_at`   Waktu pembuatan
  `updated_at`   Waktu pembaruan terakhir

Kolom tambahan seperti `version` dapat digunakan untuk mengendalikan
urutan event sinkronisasi.

### 7.2 Author

Atribut minimum yang diusulkan:

  Field    Description
  -------- ------------------------
  `id`     Identifier unik author
  `name`   Nama author

Nama author tidak wajib unik. Sistem tidak boleh menggunakan `name`
sebagai primary key atau menganggapnya mengidentifikasi satu author
saja.

### 7.3 Data Volume

Sistem perlu diuji dengan:

-   Dataset awal sekitar 50 juta artikel.
-   Pertumbuhan sekitar 200.000 artikel per hari.
-   Distribusi author dengan satu author menyumbang sekitar 30% artikel.
-   Author long-tail yang sebagian besar memiliki kurang dari 100
    artikel.
-   Konten berbahasa Indonesia, Inggris, dan campuran.

Dataset uji harus dibuat secara sintetis atau melalui metode lain yang
tidak menggunakan data pribadi tanpa izin.

------------------------------------------------------------------------

## 8. Search and Language Requirements

### 8.1 Search Scope

Pencarian mencakup:

-   Article title.
-   Article body.

Pencarian harus mendukung kombinasi dengan filter author.

### 8.2 Mixed-language Content

Karena bahasa tidak disimpan dalam schema dan satu artikel dapat berisi
lebih dari satu bahasa:

-   Sistem tidak boleh bergantung pada satu field bahasa per artikel.
-   Baseline awal menggunakan analyzer umum/standard pada search engine.
-   Kualitas pencarian harus dievaluasi menggunakan corpus uji
    Indonesia, Inggris, dan campuran.
-   Language-specific analyzer dapat dipertimbangkan setelah hasil
    evaluasi menunjukkan kebutuhan.
-   Dukungan stemming, stop words, tokenisasi, dan accent folding harus
    diputuskan berdasarkan hasil pengujian, bukan diasumsikan selalu
    menguntungkan.

### 8.3 Search Result Consistency

Database utama menjadi sumber data artikel yang otoritatif. Search
engine digunakan untuk menemukan kandidat dan urutan hasil.

Untuk pencarian berbasis search engine, sistem dapat mengambil ID dan
urutan hasil dari search index, lalu mengambil data artikel terbaru dari
database utama. Urutan dari search engine harus dipertahankan ketika
data utama dikembalikan.

Konsekuensi desain:

-   Hasil pencarian dapat mengalami eventual consistency setelah
    create/update.
-   Dokumen yang belum terindeks mungkin belum muncul pada pencarian.
-   Dokumen yang sudah tidak sinkron perlu ditangani secara aman.
-   Sistem membutuhkan mekanisme retry, monitoring, dan reindex.

------------------------------------------------------------------------

## 9. Non-Functional Requirements

### 9.1 Performance and Scalability

-   Sistem harus dirancang dengan mempertimbangkan 50 juta artikel dan
    pertumbuhan 200.000 artikel per hari.
-   Read-heavy workload menjadi prioritas optimasi.
-   Burst traffic harus dipertimbangkan dalam kapasitas API, database
    connection pool, worker, cache, dan search engine.
-   Pagination harus menghindari pemindaian atau pengambilan data yang
    tidak perlu.
-   Benchmark wajib dilakukan sebelum menetapkan klaim kapasitas atau
    latency.
-   Partitioning tidak menjadi keputusan awal yang wajib; penerapannya
    harus didukung hasil benchmark dan kebutuhan operasional.

### 9.2 Reliability

-   Database utama harus tetap menjadi sumber data yang dapat
    dipulihkan.
-   Kegagalan Redis atau Elasticsearch tidak boleh menyebabkan hilangnya
    artikel yang sudah tersimpan.
-   Worker harus mendukung retry dan pemrosesan ulang.
-   Proses reindex harus tersedia untuk pemulihan search index.

### 9.3 Consistency

-   Operasi create/update pada database utama harus atomik.
-   Event sinkronisasi dicatat dalam transaksi yang sama dengan
    perubahan artikel.
-   Search index bersifat eventually consistent.
-   Cache harus memiliki kebijakan invalidasi atau TTL yang jelas.
-   `created_at` bersifat immutable setelah artikel dibuat.

### 9.4 Maintainability

-   Kode menggunakan pemisahan tanggung jawab yang jelas.
-   Business rules tidak ditempatkan langsung di HTTP handler.
-   Database access menggunakan SQL eksplisit, bukan ORM.
-   Migration dikelola dengan tool versioned migration.
-   API dan keputusan teknis didokumentasikan.
-   Automated tests dijalankan secara konsisten.

### 9.5 Observability

Sistem sebaiknya menyediakan:

-   Structured logging.
-   Request ID atau correlation ID.
-   Metrics untuk request rate, error rate, dan latency.
-   Metrics untuk queue/outbox backlog dan retry.
-   Metrics untuk cache hit/miss.
-   Metrics untuk search latency dan sinkronisasi index.
-   Health dan readiness endpoint.

------------------------------------------------------------------------

## 10. Proposed Solution Direction

Bagian ini adalah arah solusi awal, bukan satu-satunya implementasi yang
diperbolehkan assessment.

### 10.1 Architecture Style

Menggunakan **modular monolith** untuk menjaga implementasi tetap
sederhana dan mudah diuji dalam batas waktu assessment, dengan pemisahan
modul berdasarkan tanggung jawab.

### 10.2 Proposed Technology

  Concern                 Proposed technology
  ----------------------- -------------------------------
  Programming language    Go
  HTTP framework          Gin
  Primary database        PostgreSQL
  Database access         pgx atau `database/sql`
  Search engine           Elasticsearch
  Cache                   Redis
  Database migration      Goose
  Async synchronization   Transactional Outbox + worker
  API documentation       OpenAPI / Swagger
  Performance testing     k6
  Local environment       Docker Compose

Pemilihan teknologi dapat ditinjau kembali jika hasil benchmark atau
keterbatasan lingkungan menunjukkan alternatif yang lebih sesuai.

### 10.3 Data Flow

**Create/update flow:**

1.  Client mengirim request ke HTTP API.
2.  Handler melakukan parsing dan validasi input.
3.  Application layer menjalankan aturan bisnis.
4.  Repository menyimpan perubahan artikel ke PostgreSQL.
5.  Outbox event disimpan dalam transaksi database yang sama.
6.  API mengembalikan respons berdasarkan hasil transaksi database.
7.  Worker mengambil event dari outbox.
8.  Worker memperbarui dokumen di Elasticsearch.
9.  Cache terkait dihapus atau diperbarui sesuai kebijakan.

**Read flow:**

-   Listing tanpa keyword dapat menggunakan PostgreSQL.
-   Pencarian keyword menggunakan Elasticsearch untuk memperoleh
    kandidat dan urutan hasil.
-   Data artikel yang ditampilkan diambil dari PostgreSQL agar isi yang
    dikembalikan mencerminkan sumber data utama.
-   Filter author harus konsisten dengan aturan nama author yang telah
    ditetapkan.

### 10.4 Pagination

Gunakan cursor/keyset pagination untuk listing yang diurutkan
berdasarkan `created_at DESC` dan ID sebagai tie-breaker.

Untuk pencarian Elasticsearch, pertimbangkan `search_after` dan Point in
Time (PIT) jika diperlukan agar pagination stabil pada indeks yang
berubah.

------------------------------------------------------------------------

## 11. Testing Requirements

### 11.1 Unit Tests

Minimal mencakup:

-   Validasi create article.
-   Validasi update article.
-   Aturan `created_at` yang tidak berubah.
-   Normalisasi nama author.
-   Resolusi nama author duplikat.
-   Validasi pagination cursor.
-   Pemetaan error domain ke respons API.

### 11.2 Integration Tests

Minimal mencakup:

-   Create article ke PostgreSQL.
-   List dan sorting artikel.
-   Filter author dengan nama duplikat.
-   Kombinasi query dan author.
-   Update artikel.
-   Outbox event tersimpan atomik.
-   Worker memproses event.
-   Retry ketika search engine gagal.
-   Cache invalidation atau expiration.

### 11.3 Search Quality Tests

Siapkan corpus yang memuat:

-   Artikel berbahasa Indonesia.
-   Artikel berbahasa Inggris.
-   Artikel dengan campuran kedua bahasa.
-   Kata yang muncul pada title.
-   Kata yang muncul pada body.
-   Variasi kapitalisasi dan tanda baca.
-   Kasus kata umum dan istilah khusus.

Hasil pengujian digunakan untuk mengevaluasi analyzer dan strategi
pencarian.

### 11.4 Performance Tests

Gunakan dataset dan distribusi author yang merepresentasikan assessment.

Skenario benchmark minimal:

-   List artikel terbaru.
-   List dengan filter author dominan.
-   List dengan filter author long-tail.
-   Search keyword.
-   Search dengan filter author.
-   Burst traffic untuk read-heavy workload.
-   Create/update pada laju yang relevan.

Catat throughput, latency persentil, error rate, penggunaan resource,
dan query plan database. Jangan menyatakan sistem mampu menangani target
tertentu tanpa hasil pengukuran.

------------------------------------------------------------------------

## 12. Acceptance Criteria --- Overall

Implementasi dapat dianggap memenuhi PRD apabila:

-   [ ] `POST /articles` dapat membuat artikel valid.
-   [ ] `GET /articles` menampilkan artikel terbaru lebih dahulu.
-   [ ] `query` mencari title dan body.
-   [ ] `author` memfilter berdasarkan nama author.
-   [ ] Nama author duplikat menghasilkan artikel dari semua author ID
    yang cocok.
-   [ ] Pagination berjalan secara deterministik.
-   [ ] Artikel dapat diperbarui.
-   [ ] `created_at` tidak berubah setelah update.
-   [ ] Perubahan artikel dicatat untuk sinkronisasi search index.
-   [ ] Kegagalan search engine tidak menghilangkan data utama.
-   [ ] Automated tests tersedia dan lulus.
-   [ ] Strategi pencarian konten campuran Indonesia/Inggris diuji.
-   [ ] Benchmark menggunakan distribusi data yang relevan tersedia.
-   [ ] Dokumentasi API dan keputusan teknis tersedia.
-   [ ] Penggunaan AI diungkapkan dan satu kegagalan nyata beserta
    koreksinya didokumentasikan.

------------------------------------------------------------------------

## 13. Risks and Mitigations

  -----------------------------------------------------------------------
  Risk                    Impact                  Mitigation
  ----------------------- ----------------------- -----------------------
  Dataset sangat besar    Query lambat dan        Index yang sesuai,
                          penggunaan resource     keyset pagination,
                          tinggi                  benchmark, query plan
                                                  review

  Author sangat dominan   Distribusi data tidak   Uji workload skew,
                          merata dan filter       evaluasi index dan
                          tertentu berat          query plan

  Nama author tidak unik  Hasil filter tidak      Resolusi nama ke
                          lengkap jika nama       seluruh author ID yang
                          dianggap unik           cocok

  Konten multi-bahasa     Relevansi pencarian     Corpus uji campuran dan
                          tidak konsisten         evaluasi analyzer

  Search index tertinggal Hasil pencarian tidak   Transactional Outbox,
                          langsung mencerminkan   retry, monitoring,
                          update                  reindex

  Cache stale             Data lama dapat         TTL dan invalidasi
                          dikembalikan            cache

  Burst traffic           Latency dan error       Connection pool,
                          meningkat               caching, load test,
                                                  backpressure

  Event diproses tidak    Dokumen lama menimpa    Versioning atau
  berurutan               dokumen baru            mekanisme ordering yang
                                                  idempotent
  -----------------------------------------------------------------------

------------------------------------------------------------------------

## 14. Open Questions and Decisions to Validate

Hal berikut belum ditentukan secara eksplisit oleh assessment dan perlu
dipastikan atau dicatat sebagai keputusan implementasi:

1.  Batas maksimum panjang title dan body.
2.  Apakah pencarian harus exact, prefix, fuzzy, atau full-text.
3.  Apakah filter author harus mengabaikan aksen/diakritik.
4.  Apakah pencarian harus menampilkan hasil segera setelah
    create/update atau eventual consistency dapat diterima.
5.  Apakah urutan hasil pencarian berdasarkan relevansi atau tanggal
    terbaru.
6.  Target latency dan throughput yang akan digunakan dalam benchmark.
7.  Detail kapasitas infrastruktur dan konfigurasi search cluster.
8.  Apakah partitioning diperlukan setelah benchmark.
9.  Kebijakan retensi dan pemulihan outbox event.
10. Bagaimana initial backfill 50 juta artikel dilakukan tanpa membebani
    sistem secara berlebihan.

Jika tidak ada klarifikasi dari pemberi assessment, keputusan yang
diambil harus ditulis dalam README atau Architecture Decision Records
(ADR).

------------------------------------------------------------------------

## 15. AI Usage and Engineering Transparency

Penggunaan AI diperbolehkan dalam assessment, tetapi keputusan akhir dan
kualitas implementasi tetap menjadi tanggung jawab developer.

Dokumentasi akhir harus mencantumkan:

-   Tool AI yang digunakan.
-   Bagian pekerjaan yang dibantu AI.
-   Cara output AI diverifikasi.
-   Minimal satu contoh kegagalan nyata dari output AI.
-   Perbaikan yang dilakukan dan alasan teknisnya.

AI tidak boleh diperlakukan sebagai sumber kebenaran otomatis. Semua
kode, query, konfigurasi, dan keputusan arsitektur harus ditinjau serta
diuji.

------------------------------------------------------------------------

## 16. Delivery Plan

Rencana berikut disusun agar pekerjaan dapat diselesaikan dalam batas
waktu assessment tiga hari.

### Day 1 --- Foundation and Core API

-   Finalisasi keputusan data model dan API contract.
-   Siapkan struktur project dan konfigurasi.
-   Buat migration PostgreSQL.
-   Implementasikan create article.
-   Implementasikan list artikel dan pagination.
-   Tambahkan unit dan integration tests dasar.

### Day 2 --- Search and Synchronization

-   Implementasikan search title/body.
-   Implementasikan filter author termasuk nama duplikat.
-   Implementasikan outbox dan worker.
-   Implementasikan update article.
-   Tambahkan test untuk mixed-language corpus dan konsistensi
    sinkronisasi.

### Day 3 --- Hardening and Delivery

-   Implementasikan atau rapikan cache jika waktu dan benchmark
    mendukung.
-   Jalankan integration tests.
-   Jalankan benchmark representatif.
-   Perbaiki query dan index berdasarkan hasil pengukuran.
-   Siapkan Docker Compose, README, API documentation, dan ADR.
-   Dokumentasikan penggunaan AI dan contoh kegagalan yang benar-benar
    terjadi.

------------------------------------------------------------------------

## 17. Definition of Done

Sebuah fitur dinyatakan selesai apabila:

1.  Perilaku fitur sesuai requirement dan acceptance criteria.
2.  Validasi input dan error handling tersedia.
3.  Unit atau integration tests yang relevan tersedia.
4.  Tidak ada penggunaan ORM.
5.  Perubahan schema memiliki migration.
6.  API contract dan dokumentasi diperbarui.
7.  Logging tidak membocorkan data sensitif.
8.  Kegagalan dependency ditangani secara eksplisit.
9.  Kode telah ditinjau dan diuji.
10. Keputusan atau kompromi penting didokumentasikan.

------------------------------------------------------------------------

## Appendix A --- Terminology

  -----------------------------------------------------------------------
  Term                                Meaning
  ----------------------------------- -----------------------------------
  Article                             Konten yang dibuat dan dikelola
                                      melalui service

  Author                              Pemilik/pembuat artikel

  Author ID                           Identifier unik author

  Long-tail author                    Author dengan jumlah artikel
                                      relatif sedikit

  Dominant author                     Author dengan porsi artikel sangat
                                      besar

  Search index                        Struktur data pada search engine
                                      untuk pencarian

  Outbox                              Tabel/event store untuk mencatat
                                      perubahan yang perlu diproses
                                      asynchronous

  Eventual consistency                Kondisi ketika sistem pendukung
                                      menyusul perubahan sumber data
                                      utama setelah jeda waktu

  Keyset pagination                   Pagination menggunakan nilai kunci
                                      urutan, bukan offset besar
  -----------------------------------------------------------------------

## Appendix B --- Relationship to Technical Design

PRD ini menjelaskan **apa** yang harus dibangun dan **perilaku apa**
yang diharapkan. Detail implementasi seperti struktur package Go, schema
SQL final, mapping Elasticsearch, strategi retry worker, konfigurasi
Redis, serta deployment dirinci dalam dokumen Technical Design Document
(TDD).

PRD dan TDD harus dijaga tetap konsisten. Jika keputusan teknis mengubah
perilaku produk atau acceptance criteria, PRD perlu diperbarui.
