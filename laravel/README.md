# Laravel のバックエンド（速度比較用）

> typle-r から分けたリポジトリです。文中の `.env.local`・`db/`・`next.config.mjs` は [typle-r](https://github.com/Sparklingstadt/typle-r) のものです。`tests/fixtures` と `src/` は、このリポジトリにある写しを読みます（[README](../README.md) の「typle-r からの写し」）。

速度比較のためだけの Laravel（PHP 8.4・Laravel 13・Laravel Octane + FrankenPHP）の API です。本番のバックエンドではありません。
Next.js の Route Handler と同じ答えを返すように、次のエンドポイントだけを受けます（Rails 版 `rails/` と同じ範囲です）。

| パス | メソッド | ほかのメソッド |
|---|---|---|
| `/api/health` | GET、HEAD | `405`（`Allow: GET, HEAD`） |
| `/api/workspace` | GET、HEAD、PUT | `405`（`Allow: GET, HEAD, PUT`） |
| `/api/workspace/records` | POST、DELETE | `405`（`Allow: POST, DELETE`） |

- 環境変数 `TYPLE_BACKEND_COMPARE` が `1` でなければ、すべてのリクエストに `404 {"error":"notFound"}` を返します。
- どの答えにもヘッダー `x-typle-backend: laravel` を付けます。`next.config.mjs` が付ける4つのセキュリティヘッダーと
  `/api` の CSP も同じものを付けます。Laravel のセッション・CSRF・Cookie の暗号化・CORS・入力の整形のミドルウェアは
  使わず（グローバルのミドルウェアは `BackendGate` だけ）、セッションの Cookie も `XSRF-TOKEN` も付けません。
  FrankenPHP（Caddy）の `Server` ヘッダー、PHP の `X-Powered-By`、圧縮と `Vary: Accept-Encoding` も付けません。
- typle-r の v2.68.1 に合わせています。ログインは Better Auth のセッションです。Cookie `better-auth.session_token`
  （`AUTH_URL` が `https` なら `__Secure-better-auth.session_token`）の署名を `BETTER_AUTH_SECRET` で確かめ（better-call の
  `signCookieValue` と同じ HMAC-SHA256）、`typle_auth_sessions` と `typle_auth_users` を1文で読みます。Cookie は
  Laravel の Cookie（`EncryptCookies` が復号するもの）ではなく、`Cookie` ヘッダーをそのまま読みます。
  - 署名が合わない・Cookie がない・セッションの行がない・期限が切れている（`expires_at` が今より前）ときは `401 loginRequired`。
    typle-r（`auth.api.getSession`）と同じく、断るときに Cookie は消しません。
  - `BETTER_AUTH_SECRET` がない（空白だけも）ときは、ログインが要るリクエストは `503 databaseUnavailable`（typle-r では
    Better Auth が立ち上がらないため）。
  - 共有のデモアカウント（`demo:shared`）のセッションは、`DEMO_LOGIN=1` のときだけ通ります。
  - アプリのトークン（`Authorization: Bearer`）は読みません。書き込みにはいつも同じ `Origin` が要ります。
- 保存の計測（`typle_save_metrics`）は書きません。表の形は Next.js（リポジトリの `db/`）のもので、Laravel のマイグレーションは
  ありません。

## コードの構成

```
laravel/
  bootstrap/app.php              ルート（routes/api.php と、ほかのパスの 404）、グローバルのミドルウェア（BackendGate だけ）、
                                 例外の答え（405・404・ほかは 503）
  routes/api.php                 3つのパスと、パスごとのミドルウェア、405 の行き先
  config/typle.php               比較の設定（TYPLE_BACKEND_COMPARE・AUTH_URL・BETTER_AUTH_SECRET・DEMO_LOGIN・DB の URL・
                                 TYPLE_APP_DATA）
  config/database.php            DB の接続（App\Typle\DatabaseUrl が作る）
  config/octane.php              Octane（FrankenPHP）。接続はリクエストをまたいで保つ
  app/Http/Middleware/           BackendGate（比較モードのゲートと、すべての答えに付けるヘッダー）、書き込みの前の確かめ
                                 （RequireLoginSettings・RequireWriteRequest・RequireDatabase・RequireOwner）
  app/Http/Controllers/          Health・Workspaces（show・update）・Records（store・destroy）・Errors（405・404）と、
                                 共通の答えと本文の読み取り（Controller）
  app/Http/Answer.php            JSON の答え（JSON.stringify と改行、Cache-Control・Vary）
  app/Models/                    Workspace（typle_workspaces）・PracticeRecord（typle_records）・AuthSession と AuthUser
                                 （Better Auth の typle_auth_sessions・typle_auth_users）
  app/Casts/Json.php             jsonb の列を JSON.parse・JSON.stringify と同じに読み書きする Eloquent のキャスト
  app/Typle/                     src/lib と src/server の移植（rails/app/lib/typle と1ファイルずつ対応）: JS と同じ文字・日付・数・
                                 JSON（Js.php・JsDate.php・JsonValue.php）、単語リスト・パック・記録・公式カタログの検査と変換、
                                 ログインの Cookie、保存（WorkspaceStore.php）
  app/Database/PostgresConnector.php  URL のパラメーター（sslmode など）を libpq にも渡す接続
  tests/                         PHPUnit。共通 fixture（../tests/fixtures）をそのまま読む（Lib・Integration は rails/test の
                                 lib・integration と1つずつ対応）
  Dockerfile                     Cloud Run 用（リポジトリのルートをコンテキストにしてビルドする）
  Caddyfile・php.ini             FrankenPHP と PHP の設定（Docker のイメージの中で使う）
  Dockerfile.test                テストを動かす PHP（pdo_pgsql と Composer 入り）
```

JSON は PHP の配列ではなく、オブジェクトを `stdClass`、配列を PHP のリストとして読みます（`{}` と `[]`、`{"0":…}` と配列を
区別し、キーの順を保つため）。値の扱いは Rails 版（`app/lib/typle/value.rb`）と同じで、`undefined` は `App\Typle\Undefined`
です。

公式パックのデータは写しを作らず、`../src/lib/packs` と `../src/messages/ja/format.json` を読みます。Docker のイメージでは
それを `/app-data` に写し、`TYPLE_APP_DATA=/app-data` で読みます。

### サーバー（Octane + FrankenPHP）

`php artisan serve` や、リクエストごとにアプリを起動する php-fpm では、毎回の起動（設定・ルート・サービスの読み込み）が
数字に入ってしまいます。Next.js・Go・Kotlin・Rails はどれもプロセスがアプリと DB の接続を持ち続けるので、それにそろえて
Laravel 公式の Octane を FrankenPHP の上で動かします（RoadRunner・Swoole より拡張なしで入れやすく、Laravel が勧める組み合わせ）。

- ワーカーは5つ（`OCTANE_WORKERS`）。それぞれが起動時にアプリを1回だけ用意し、自分の DB 接続を1本持ちます。Rails の
  Puma の5スレッドと、Next.js の pg のプール（最大5本）に合わせた数です。
- Octane の既定では500リクエストごとにワーカーを起動し直しますが、計測の途中で冷えたワーカーに当たらないよう、起動し直しません。
- 公式パックのカタログは、ワーカーごとに初めて要るときに1回だけ作ります（Rails はプロセスごとに1回）。

## 環境変数

| 名前 | 内容 |
|---|---|
| `TYPLE_BACKEND_COMPARE` | `1` のときだけ答える。それ以外は全部 `404` |
| `DATABASE_URL` | Neon などの `postgres://` の URL（なければ `POSTGRES_URL`） |
| `BETTER_AUTH_SECRET` | typle-r と同じ値（前後の空白は除く）。Better Auth の Cookie の署名を確かめる。なければログインが要るリクエストは `503 databaseUnavailable` |
| `AUTH_URL` | typle-r と同じ値。書き込みの `Origin` はこのオリジンと同じでなければならず、`https` なら Cookie の名前は `__Secure-better-auth.session_token`。http(s) の URL でなければ `503 authUnavailable` |
| `DEMO_LOGIN` | `1` のときだけ、共有のデモアカウント（`demo:shared`）のセッションを通す |
| `PORT` | 受けるポート。Cloud Run が付ける。イメージの既定は `8080` |
| `OCTANE_WORKERS` | Octane のワーカーの数。イメージの既定は `5` |
| `PGSSLROOTCERT` | `sslmode=verify-full` で使うルート証明書のファイル。既定は `/etc/ssl/certs/ca-certificates.crt`（イメージの中の場所） |
| `TYPLE_APP_DATA` | 公式パックのデータを読む場所。既定はリポジトリのルート。イメージでは `/app-data` |
| `TYPLE_TEST_DATABASE_URL` | テストだけ。DB のテストの接続先 |

値は環境変数の文字のまま読みます（Laravel の `env()` は `true`・`null` などを別の値に変えるので使いません）。`APP_KEY` は
要りません（暗号化するものがないため）。

`AUTH_URL` と `BETTER_AUTH_SECRET` は、計測に使う Cookie を発行したデプロイの値にします。セッションの行はそのデプロイの DB に
あるので、`DATABASE_URL` も同じ DB にします。

DB の接続は Next.js（`src/server/database.ts`）にそろえています: 接続と各文は 5 秒で諦める（`connect_timeout` と
`statement_timeout`）、TCP keep-alive。`sslmode=require`（Neon の URL）は node-postgres 8 と同じく `verify-full` として扱い、
URL のほかのパラメーター（`channel_binding` など）もそのまま libpq に渡します。`statement_timeout` とクライアントの文字コードは
接続の options で渡すので、接続したときに `SET` の文は流しません。文は名前付きの prepared statement にせず、パラメーターと一緒に
1往復で送ります（`PQexecParams`。node-postgres と同じ送り方）。

DB に流す文の数と種類は Next.js と同じです（GET は2文、PUT は6文、記録の追加は8文など。
`tests/Integration/WorkspaceTest.php` が数えています）。Eloquent で書けないところ（`IS DISTINCT FROM` の upsert、revision の
数え上げ、advisory lock、`COALESCE` の保存）は、モデルのメソッドの中の SQL です。

## テスト

PHP と Composer は Docker のものを使います。リポジトリのルートで、まずテスト用のイメージを作ります。

```bash
docker build -f laravel/Dockerfile.test -t typle-laravel-test laravel
```

```bash
docker run --rm -v "$PWD":/w -w /w/laravel typle-laravel-test sh -c 'composer install -q && vendor/bin/phpunit'
```

DB を使うテスト（`tests/fixtures/workspace-writes.json`・`record-writes.json` のシナリオ、保存の衝突、セッションの行の読み方、
文の数など）は、`TYPLE_TEST_DATABASE_URL` があるときだけ動き、なければ skip されます。DB は、typle-r の表がある PostgreSQL
です。使い捨ての PostgreSQL 17 をコンテナで立て、`scripts/migrate-test-db.sh` で表を作って流すとき:

```bash
docker network create typle-test
docker run -d --name typle-test-pg --network typle-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=typle postgres:17
docker run --rm --network typle-test -v "$PWD":/w -w /w postgres:17 sh scripts/migrate-test-db.sh postgres://postgres:test@typle-test-pg:5432/typle
docker run --rm --network typle-test -e TYPLE_TEST_DATABASE_URL=postgres://postgres:test@typle-test-pg:5432/typle \
  -v "$PWD":/w -w /w/laravel typle-laravel-test sh -c 'composer install -q && vendor/bin/phpunit'
```

（DB が立ち上がるまで数秒かかります。終わったら `docker rm -f typle-test-pg && docker network rm typle-test`。）

テストはランダムな owner で行を足し、テストごとに消します。同時に2つ保存するテストは、PHP にスレッドがないので、2つの
プロセス（`tests/Support/save.php`）で保存します。fixture と答えの JSON は、Rails のテストと同じくオブジェクトのキーの順を
問わずに比べます。

## ビルドと手元での実行

リポジトリのルートで（イメージは `src/lib/packs` などを読むので、コンテキストはルートです）:

```bash
docker build -f laravel/Dockerfile -t typle-laravel .
```

```bash
URL=$(grep '^DATABASE_URL=' .env.local | cut -d= -f2- | tr -d '"' | sed 's/127.0.0.1/host.docker.internal/')
docker run --rm -p 8083:8080 -e TYPLE_BACKEND_COMPARE=1 -e DATABASE_URL="$URL" \
  -e BETTER_AUTH_SECRET="$(grep '^BETTER_AUTH_SECRET=' .env.local | cut -d= -f2- | tr -d '"')" -e AUTH_URL=http://localhost:3000 typle-laravel
```

```bash
curl -i http://localhost:8083/api/health
```

## Cloud Run へのデプロイ（ユーザーの Mac から）

`kotlin/README.md` の「Cloud Run へのデプロイ」と同じ手順で、イメージとサービスの名前だけが違います。プロジェクト・リージョン・
Artifact Registry・Secret Manager の秘密（`typle-auth-secret`・`typle-database-url`）は Kotlin・Rails と共有できます。
`typle-auth-secret` の値は typle-r の `BETTER_AUTH_SECRET` にします（Laravel はそれを `BETTER_AUTH_SECRET` として読みます）。

```bash
IMAGE="$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-laravel:$(git rev-parse --short HEAD)"
docker buildx build --platform linux/amd64 -f laravel/Dockerfile -t "$IMAGE" --push .
```

```bash
gcloud run deploy typle-laravel \
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

設定は Kotlin・Rails と同じです（1 vCPU・2 GiB、最小 0 台、同時実行数 80、CPU はリクエストの間だけ、起動時の CPU ブーストあり）。
ワーカーは5つなので、同時に6本目以降のリクエストは待ちます。計測は1本ずつ送るので、この差は数字に出ません。

デプロイの最後に出る URL（`https://typle-laravel-….run.app`）を、Vercel の Preview 環境の `TYPLE_LARAVEL_URL` に入れます。

```bash
curl -i "https://typle-laravel-….run.app/api/health"   # 200 と x-typle-backend: laravel
```

## 比較が終わったら止める・消す

```bash
gcloud run services remove-iam-policy-binding typle-laravel --region="$REGION" --member=allUsers --role=roles/run.invoker
```

```bash
gcloud run services delete typle-laravel --region="$REGION"
```

```bash
gcloud artifacts docker images delete "$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-laravel" --delete-tags
```

Vercel の Preview 環境の `TYPLE_LARAVEL_URL` も消してください。秘密の値は、Kotlin・Rails も消し終えたら消します（`kotlin/README.md`）。

## Next.js と違うところ

答えの状態コードと JSON の値はそろえていますが、次の点は違います。Rails 版と違うところもここに書きます。

- JSON の書き出しは `JSON.stringify` に合わせています（数は `Number#toString` と同じ形、`Infinity` などは `null`、整数の値
  `1500.0` は `1500`）。読み込みは PHP の `json_decode` で、`JSON.parse` と同じものを受け付け、同じキーはあとの値が勝ちます。
  入れ子は Rails（Ruby の JSON）と同じく100段までです（Next.js には限りがありません）。
- 孤立サロゲート（`"\ud800"`）を含む JSON は、PHP のパーサーが読めないので、本文なら `400 invalidWorkspace`（記録は
  `invalidRecord`）にします（Rails と同じ）。Next.js は読めるので、ほかの検査で決まります（`listId` なら `404 recordListDeleted`、
  `recordId` なら `200`、ほかの人の `accountId` なら `401 loginAccountChanged`、lists の中なら `400 invalidWorkspace`）。
- `"\u0000"` で始まるキーのあるオブジェクトも、PHP のパーサーが読めない（オブジェクトのプロパティ名にできない）ので同じく
  `400 invalidWorkspace`（`invalidRecord`）にします。Rails と Next.js は読めるので、`lists` などの中なら同じ `400` ですが、
  本文のいちばん外の知らないキーなら、ほかの検査で決まります（`accountId` が違えば `401 loginAccountChanged`、合っていれば保存）。
- `Date.parse` は Rails・Kotlin 版と同じく、ISO 8601 の形とよく使われるいくつかの形だけを読みます（`kotlin/README.md`）。
- `toLocaleLowerCase()` は PHP の `mb_strtolower` です。ギリシャ文字の語末のシグマ（`Σ` → `ς`）は Next.js と同じく変えます
  （Rails の `downcase` は `σ` にします）。
- 本文は UTF-8 として読み、壊れたバイトは `U+FFFD` にします（`TextDecoder` と、置き換えの単位が少し違うことがあります）。
- Laravel は、`Content-Type` が JSON のリクエストの本文を、アプリに渡す前に一度 `json_decode` します（`Request::capture()`。
  Octane も同じ）。答えには関わりませんが、PUT と記録の本文は2回読むことになり、その分の時間が数字に入ります。Rails 版は
  ゲートで Rails に本文を読ませないようにしていますが、Laravel ではリクエストを作る前に割り込めないので、そのままにしています。
- Symfony の `X-HTTP-Method-Override` ヘッダーと `_method` フィールド（クエリ・フォーム・JSON の本文）によるメソッドの上書きは、
  ゲートで消します（Next.js も Rails の API モードもしないため）。
- Laravel のルートの動詞にないメソッド（`PROPFIND` など）は、3つのパスなら `405`（その `Allow`）、ほかのパスなら `404` です。
- Rails はルートの書式の拡張子（`/api/health.json`）も受けますが、Laravel は Next.js と同じく `404` です。
- 失敗（`503`）は、標準エラー出力のログに書くだけです。リクエストごとのログは書きません。文の失敗は、DB のメッセージと値を
  入れない SQL を書きます（Laravel の例外のメッセージには値が入り、セッションのトークンがログに残るため）。
- 知らないパスには `404 {"error":"notFound"}` を返します（Next.js なら 404 のページ）。
- 設定（`AUTH_URL`・`BETTER_AUTH_SECRET`・`DEMO_LOGIN`）は、値の組ごとに1回だけ読みます。
- typle-r は Cookie のキャッシュ（最大5分）でセッションの行を読まずに答えることがありますが、Laravel は毎回行を読みます（Go・
  Rails と同じ）。そのため「すべての端末からログアウト」のあとは、Laravel のほうが早く断ります。文の数はキャッシュのない typle-r と
  同じです。
- セッションの行は `LIMIT 1` を付けて読みます（Eloquent の `first`。Rails の `pick` と同じ）。トークンは一意なので、答えは同じです。
- 記録の upsert は、Rails の `VALUES` の並びではなく、Go と同じ `unnest` の配列で1文にします（記録がいくつあってもパラメーターは
  6つ）。最初の保存の `INSERT … ON CONFLICT ("owner_id") DO NOTHING RETURNING "revision"` は Rails と同じ文です。
- ヘッダーは Node と同じく1バイトを1文字（Latin-1）として読みます。
- ヘッダーの名前の大文字・小文字は FrankenPHP（Go）がそろえます（`Cache-Control` など。Rails は小文字）。どの答えにも `Date`
  ヘッダーが付きます（Node と同じ）。
- ワーカーの DB 接続は、使わない時間が続いても閉じません（Next.js のプールは30秒で閉じます）。接続は、ワーカーが初めて DB を
  使うリクエストで開きます。接続したときに流す文はありません（Rails は表の列と索引を読み、`SET` を流します）。
