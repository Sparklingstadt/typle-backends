# typle-backends

[Typle](https://github.com/Sparklingstadt/typle-r)（タイピングアプリ）の API の一部を、Go・Kotlin・Rails で実装し直したものです。同じ API を4つの言語で書き、Next.js の Route Handler と速さを比べるために作りました。

本番で動いているのは typle-r の Next.js だけです。ここにあるのは比較用の実装で、本番のバックエンドではありません。

| 実装 | 場所 | 動かす場所 |
|---|---|---|
| Next.js（Route Handler） | [typle-r](https://github.com/Sparklingstadt/typle-r) の `src/app/api`・`src/server` | Vercel |
| Go | `server/`・`api/go.go`・`go.mod` | Vercel の Go 関数。手元では `go run ./server/cmd/typle-web` |
| Kotlin（Ktor、JDK 21） | `kotlin/`（[README](kotlin/README.md)） | Google Cloud Run |
| Rails（Ruby 3.4.1・Rails 8.1） | `rails/`（[README](rails/README.md)） | Google Cloud Run |

## 実装している API

3つとも、次のパスだけを受けます。ログインは Next.js が発行した Cookie を読むだけで、ログインの処理そのものはありません。

| パス | メソッド | 内容 |
|---|---|---|
| `/api/health` | GET | DB につながるか |
| `/api/workspace` | GET・PUT | アカウントの単語リスト・パック・端末の読み込みと、`revision` による保存 |
| `/api/workspace/records` | POST・DELETE | 練習の記録を1件ずつ足す・消す |

## 同じ動きにそろえる方法

- 4つの実装は、同じ fixture（`tests/fixtures`）を読んで同じ答えを返すことを、それぞれのテストで確かめます。保存の衝突（409）、上限、不正な本文の断り方、記録の追加と削除のシナリオが入っています。
- Rails は Rails らしい書き方（ActiveRecord・コントローラー）で書きつつ、DB に流す文の数と種類を Next.js と同じにしています。
- JSON としての値は4つとも同じですが、バイトの細部は違うことがあります（Go は保存された値をそのまま返すので、`2.50` の末尾のゼロが残ります）。

## テスト

リポジトリのルートで走らせます。DB を使うテストは `TYPLE_TEST_DATABASE_URL` があるときだけ動き、なければ飛ばします。DB は、typle-r の表がある PostgreSQL です（typle-r で `npm run db:migrate` を流した DB）。テストはランダムな owner の行を足して、終わりに消します。

```bash
go vet ./... && go test ./...
```

```bash
docker run --rm -v "$PWD":/w -v typle-kotlin-gradle:/home/gradle/.gradle -w /w/kotlin gradle:8.14-jdk21 ./gradlew test --no-daemon
```

```bash
cd rails && bundle config set --local path vendor/bundle && bundle install && bin/rails test
```

```bash
node --test tests/*.test.cjs
```

2026-11-25 に、分けたあとのこのリポジトリで4つとも通ることを確かめています（DB あり。Kotlin 91件、Rails 101件）。

## typle-r からの写し

テストとビルドが読むので、typle-r の次のファイルを同じ相対パスに写してあります。元は typle-r の `6f1a764d`（v2.60.4）です。

| 場所 | 内容 |
|---|---|
| `tests/fixtures/` | 4つの実装が共通で読むテストデータ |
| `src/lib/packs/`（`samples.json`・`countries.json`・`commands/*.json`） | 公式の単語リスト |
| `src/messages/ja/format.json` | 既定の端末の名前 |
| `docs/api/workspace-records.openapi.json` | 記録の API の定義 |
| `scripts/records-rollback.sql` | Go のテストが流す SQL |

typle-r の側が変わったら、次で写し直します。

```bash
scripts/sync-from-typle-r.sh <typle-r のチェックアウト>
```

Go は公式の単語リストの写しを自分でも持っています（`server/internal/workspace/packs/`）。`src/lib/packs` と違えば、`go test` の `TestPacksMatchApp` が失敗します。

## 計測

`scripts/bench-backends.cjs` が、同じリクエストを各実装へ順に送り、p50・p95・最大を出します。手順と計測の中身は [docs/backend-compare.md](docs/backend-compare.md) にあります。

**今の typle-r では、Vercel を経由する計測はできません。** typle-r は v2.60.5 で、リクエストヘッダー `x-typle-backend` で実装を振り分ける仕組みと、本番を Go に切り替える `TYPLE_BACKEND=go` を外しました。

- 今も測れるもの：Cloud Run のサービスへ直接送る `kotlin-direct`・`rails-direct`。
- 測れないもの：`next`・`go`・`kotlin`・`rails`（どれも typle-r のデプロイを通る）。測るには、振り分けのある typle-r（v2.60.4 まで）のデプロイが要ります。
- `scripts/backendCompare.cjs` と [docs/backend-switch.md](docs/backend-switch.md) は、その振り分けの記録として残しています。

## 文書

| 文書 | 内容 |
|---|---|
| [docs/backend-compare.md](docs/backend-compare.md) | 比較の仕組みと手順（typle-r の v2.60.4 までのもの） |
| [docs/backend-switch.md](docs/backend-switch.md) | 本番を Go に切り替える手順（同じく v2.60.4 まで） |
| [docs/superpowers/specs/](docs/superpowers/specs) | 比較と Rails の実装の設計 |
| [kotlin/README.md](kotlin/README.md)・[rails/README.md](rails/README.md) | それぞれの構成・テスト・Cloud Run へのデプロイ |

## 経緯

2026-11-25 に typle-r から分けました。`server/`・`api/`・`kotlin/`・`rails/` と比較の仕組みに触れたコミット（136件）の履歴を、そのまま持ってきています。コミットメッセージの PR 番号は、typle-r のものと分かるように `Sparklingstadt/typle-r#123` の形に書き換えました。

- Go は、typle-r の本番のバックエンドでした（v0.37.0〜v2.46.11）。v2.47.0 で Next.js に移したあと、v2.52.0 で比較用に戻したものです。
- Kotlin は v2.52.0、Rails は v2.57.0 で比較に加えました。

## ライセンス

コードは MIT ライセンスです（[LICENSE](LICENSE)）。公式の単語リストの写し（`src/lib/packs`・`server/internal/workspace/packs`）は対象外です。出典は typle-r の [docs/word-list-sources.md](https://github.com/Sparklingstadt/typle-r/blob/main/docs/word-list-sources.md) にあります。
