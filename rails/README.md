# Rails のバックエンド（速度比較用）

> typle-r から分けたリポジトリです。文中の `.env.local`・`db/`・`npm run verify`・`next.config.mjs` は [typle-r](https://github.com/Sparklingstadt/typle-r) のものです。`tests/fixtures` と `src/` は、このリポジトリにある写しを読みます（[README](../README.md) の「typle-r からの写し」）。

`docs/superpowers/specs/2026-11-21-rails-backend-design.md` の速度比較のためだけの Rails（Ruby 3.4.1・Rails 8.1・Puma、
YJIT オン）の API です。本番のバックエンドではありません。Next.js の Route Handler と同じ答えを返すように、次のエンドポイント
だけを受けます（Kotlin 版 `kotlin/` と同じ範囲です）。

| パス | メソッド | ほかのメソッド |
|---|---|---|
| `/api/health` | GET、HEAD | `405`（`Allow: GET, HEAD`） |
| `/api/workspace` | GET、HEAD、PUT | `405`（`Allow: GET, HEAD, PUT`） |
| `/api/workspace/records` | POST、DELETE | `405`（`Allow: POST, DELETE`） |

- 環境変数 `TYPLE_BACKEND_COMPARE` が `1` でなければ、すべてのリクエストに `404 {"error":"notFound"}` を返します。
- どの答えにもヘッダー `x-typle-backend: rails` を付けます。`next.config.mjs` が付ける4つのセキュリティヘッダーと
  `/api` の CSP も同じものを付けます。Rails が既定で付けるヘッダーと、`Vary: Accept` は付けません。
- typle-r の v2.68.1 に合わせています。ログインは Better Auth のセッションです。Cookie `better-auth.session_token`
  （`AUTH_URL` が `https` なら `__Secure-better-auth.session_token`）の署名を `BETTER_AUTH_SECRET` で確かめ（better-call の
  `signCookieValue` と同じ HMAC-SHA256）、`typle_auth_sessions` と `typle_auth_users` を1文で読みます。
  - 署名が合わない・Cookie がない・セッションの行がない・期限が切れている（`expires_at` が今より前）ときは `401 loginRequired`。
    typle-r（`auth.api.getSession`）と同じく、断るときに Cookie は消しません。
  - `BETTER_AUTH_SECRET` がない（空白だけも）ときは、ログインが要るリクエストは `503 databaseUnavailable`（typle-r では
    Better Auth が立ち上がらないため）。
  - 共有のデモアカウント（`demo:shared`）のセッションは、`DEMO_LOGIN=1` のときだけ通ります。
  - アプリのトークン（`Authorization: Bearer`）は読みません。書き込みにはいつも同じ `Origin` が要ります。
- 保存の計測（`typle_save_metrics`）は書きません。表の形は Next.js（リポジトリの `db/`）のもので、Rails のマイグレーションは
  ありません。

## コードの構成

```
rails/
  config/routes.rb               3つのパスと、405・404 の行き先
  config/database.yml            DB の URL と接続プール（Typle::DatabaseUrl が作る）
  config/puma.rb                 1 プロセス・5 スレッド
  app/middleware/typle/          比較モードのゲートと、すべての答えに付けるヘッダー（Rack のミドルウェア）
  app/controllers/               health・workspaces（show・update）・records（create・destroy）と、共通の確認
                                 （ログイン・書き込みの Origin と Content-Type・本文の読み取り）
  app/models/                    Workspace（typle_workspaces）・PracticeRecord（typle_records）・AuthSession と AuthUser
                                 （Better Auth の typle_auth_sessions・typle_auth_users）
  app/lib/typle/                 src/lib と src/server の移植: JS と同じ文字・日付・数・JSON（js.rb・js_date.rb・
                                 json_value.rb）、単語リスト・パック・記録・公式カタログの検査と変換、ログインの Cookie、
                                 保存（workspace_store.rb）
  test/                          Minitest。共通 fixture（../tests/fixtures）をそのまま読む
  Dockerfile                     Cloud Run 用（リポジトリのルートをコンテキストにしてビルドする）
```

公式パックのデータは写しを作らず、`../src/lib/packs` と `../src/messages/ja/format.json` を読みます。Docker のイメージでは
それを `/app-data` に写し、`TYPLE_APP_DATA=/app-data` で読みます。

## 環境変数

| 名前 | 内容 |
|---|---|
| `TYPLE_BACKEND_COMPARE` | `1` のときだけ答える。それ以外は全部 `404` |
| `DATABASE_URL` | Neon などの `postgres://` の URL（なければ `POSTGRES_URL`） |
| `BETTER_AUTH_SECRET` | typle-r と同じ値（前後の空白は除く）。Better Auth の Cookie の署名を確かめる。なければログインが要るリクエストは `503 databaseUnavailable` |
| `AUTH_URL` | typle-r と同じ値。書き込みの `Origin` はこのオリジンと同じでなければならず、`https` なら Cookie の名前は `__Secure-better-auth.session_token`。http(s) の URL でなければ `503 authUnavailable` |
| `DEMO_LOGIN` | `1` のときだけ、共有のデモアカウント（`demo:shared`）のセッションを通す |
| `PORT` | 受けるポート。Cloud Run が付ける。イメージの既定は `8080` |
| `PGSSLROOTCERT` | `sslmode=verify-full` で使うルート証明書のファイル。既定は `/etc/ssl/certs/ca-certificates.crt`（イメージの中の場所） |
| `TYPLE_TEST_DATABASE_URL` | テストだけ。DB のテストの接続先 |

`AUTH_URL` と `BETTER_AUTH_SECRET` は、計測に使う Cookie を発行したデプロイの値にします。セッションの行はそのデプロイの DB に
あるので、`DATABASE_URL` も同じ DB にします。

DB の接続は Next.js（`src/server/database.ts`）にそろえています: 最大 5 本、使わない接続は 30 秒で閉じる、接続と各文は 5 秒で
諦める（`statement_timeout`）、TCP keep-alive。`sslmode=require`（Neon の URL）は node-postgres 8 と同じく `verify-full` と
して扱います。`database.yml` に `url:` を書いているので、Rails が `DATABASE_URL` をそのまま混ぜることはありません。

DB に流す文の数と種類は Next.js と同じです（GET は2文、PUT は6文、記録の追加は8文など。
`test/integration/workspace_test.rb` が数えています）。ActiveRecord で書けないところ（`IS DISTINCT FROM` の upsert、
revision の数え上げ、advisory lock）は、モデルのメソッドの中の SQL です。

## テスト

手元の Ruby（3.4.1）で動かします。gem は `rails/vendor/bundle` に入れます（コミットしません）。

```bash
cd rails && bundle config set --local path vendor/bundle && bundle install
```

```bash
cd rails && bin/rails test
```

DB を使うテスト（`tests/fixtures/workspace-writes.json`・`record-writes.json` のシナリオ、保存の衝突、セッションの行の読み方、
文の数など）は、`TYPLE_TEST_DATABASE_URL` があるときだけ動き、なければ skip されます。手元の Docker の PostgreSQL を使うとき:

```bash
cd rails && TYPLE_TEST_DATABASE_URL=$(grep '^DATABASE_URL=' ../.env.local | cut -d= -f2- | tr -d '"') bin/rails test
```

テストはランダムな owner で行を足し、終わったら消します。`npm run verify` にはこのテストを入れていません（Kotlin と同じ）。
比較の作業をしたときは、上のコマンドを手で動かしてください。

Ruby がないときは Docker で動かせます（リポジトリのルートで）。

```bash
docker run --rm -v "$PWD":/w -w /w/rails -e BUNDLE_APP_CONFIG=/tmp/bundle-config -e BUNDLE_PATH=/tmp/bundle ruby:3.4.1 sh -c 'bundle install -j4 >/dev/null && bin/rails test'
```

## ビルドと手元での実行

リポジトリのルートで（イメージは `src/lib/packs` などを読むので、コンテキストはルートです）:

```bash
docker build -f rails/Dockerfile -t typle-rails .
```

```bash
URL=$(grep '^DATABASE_URL=' .env.local | cut -d= -f2- | tr -d '"' | sed 's/127.0.0.1/host.docker.internal/')
docker run --rm -p 8082:8080 -e TYPLE_BACKEND_COMPARE=1 -e DATABASE_URL="$URL" \
  -e BETTER_AUTH_SECRET="$(grep '^BETTER_AUTH_SECRET=' .env.local | cut -d= -f2- | tr -d '"')" -e AUTH_URL=http://localhost:3000 typle-rails
```

```bash
curl -i http://localhost:8082/api/health
```

## Cloud Run へのデプロイ（ユーザーの Mac から）

`kotlin/README.md` の「Cloud Run へのデプロイ」と同じ手順で、イメージとサービスの名前だけが違います。プロジェクト・リージョン・
Artifact Registry・Secret Manager の秘密（`typle-auth-secret`・`typle-database-url`）は Kotlin と共有できます。`typle-auth-secret`
の値は typle-r の `BETTER_AUTH_SECRET` にします（Rails はそれを `BETTER_AUTH_SECRET` として読みます）。

```bash
IMAGE="$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-rails:$(git rev-parse --short HEAD)"
docker buildx build --platform linux/amd64 -f rails/Dockerfile -t "$IMAGE" --push .
```

```bash
gcloud run deploy typle-rails \
  --image="$IMAGE" \
  --region="$REGION" \
  --cpu=1 --memory=2Gi \
  --min-instances=0 \
  --concurrency=80 \
  --cpu-throttling \
  --cpu-boost \
  --port=8080 \
  --set-secrets=BETTER_AUTH_SECRET=typle-auth-secret:latest,DATABASE_URL=typle-database-url:latest \
  --set-env-vars=TYPLE_BACKEND_COMPARE=1,AUTH_URL=https://typle.sparklingstadt.com \
  --allow-unauthenticated
```

設定は Kotlin と同じです（1 vCPU・2 GiB、最小 0 台、同時実行数 80、CPU はリクエストの間だけ、起動時の CPU ブーストあり）。
Puma は5スレッドなので、同時に6本目以降のリクエストは待ちます。計測は1本ずつ送るので、この差は数字に出ません。

デプロイの最後に出る URL（`https://typle-rails-….run.app`）を、Vercel の Preview 環境の `TYPLE_RAILS_URL` に入れます。

```bash
curl -i "https://typle-rails-….run.app/api/health"   # 200 と x-typle-backend: rails
```

## 比較が終わったら止める・消す

```bash
gcloud run services remove-iam-policy-binding typle-rails --region="$REGION" --member=allUsers --role=roles/run.invoker
```

```bash
gcloud run services delete typle-rails --region="$REGION"
```

```bash
gcloud artifacts docker images delete "$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-rails" --delete-tags
```

Vercel の Preview 環境の `TYPLE_RAILS_URL` も消してください。秘密の値は、Kotlin も消し終えたら消します（`kotlin/README.md`）。

## Next.js と違うところ

答えの状態コードと JSON の値はそろえていますが、次の点は違います。

- JSON の書き出しは `JSON.stringify` に合わせています（数は `Number#toString` と同じ形、`Infinity` などは `null`、整数の値
  `1500.0` は `1500`）。読み込みは Ruby の JSON で、`JSON.parse` と同じものを受け付け、同じキーはあとの値が勝ちます。
- 孤立サロゲート（`"\ud800"`）を含む JSON は、Ruby のパーサーが読めないので、本文なら `400 invalidWorkspace`（記録は
  `invalidRecord`）にします。Next.js は読めるので、ほかの検査で決まります（`listId` なら `404 recordListDeleted`、
  `recordId` なら `200`、ほかの人の `accountId` なら `401 loginAccountChanged`、lists の中なら `400 invalidWorkspace`）。
- `Date.parse` は Kotlin 版と同じく、ISO 8601 の形とよく使われるいくつかの形だけを読みます（`kotlin/README.md`）。
- `toLocaleLowerCase()` は Ruby の `downcase` です。
- 本文は UTF-8 として読み、壊れたバイトは `U+FFFD` にします（`TextDecoder` と、置き換えの単位が少し違うことがあります）。
- 失敗（`503`）は、Rails のログ（標準出力）に書くだけです。リクエストごとのログは書きません。
- 知らないパスには `404 {"error":"notFound"}` を返します（Next.js なら 404 のページ）。
- 設定（`AUTH_URL`・`BETTER_AUTH_SECRET`・`DEMO_LOGIN`）は、値の組ごとに1回だけ読みます。
- typle-r は Cookie のキャッシュ（最大5分）でセッションの行を読まずに答えることがありますが、Rails は毎回行を読みます（Go と
  同じ）。そのため「すべての端末からログアウト」のあとは、Rails のほうが早く断ります。文の数はキャッシュのない typle-r と同じです。
- セッションの行は `LIMIT 1` を付けて読みます（ActiveRecord の `pick`）。トークンは一意なので、答えは同じです。
- ヘッダーは Node と同じく1バイトを1文字（Latin-1）として読みます。`Content-Type` は Rails には見せず（ゲートが
  `application/octet-stream` に置き換える）、ハンドラーが自分で確かめます。Rails が本文をフォームとして読んだり、MIME 型として
  読めない値で断ったりしないためです。
- プロセスやつなぎ直した接続の最初のリクエストでは、Rails が表の列と索引を読み、接続の設定（`SET`）を流します。Next.js には
  ない仕事なので、`--warmup 0` のコールドスタートの数字にはこれが入ります。温めのあとの数字には入りません。
