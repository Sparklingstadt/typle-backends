# Kotlin のバックエンド（速度比較用）

`docs/superpowers/specs/2026-11-19-backend-compare-design.md` の比較のためだけの Kotlin（Ktor、JDK 21、ふつうの JVM）の
HTTP サーバーです。本番のバックエンドではありません。Next.js の Route Handler と同じ答えを返すように、次のエンドポイントだけを
受けます。

| パス | メソッド | ほかのメソッド |
|---|---|---|
| `/api/health` | GET、HEAD | `405`（`Allow: GET, HEAD`） |
| `/api/workspace` | GET、HEAD、PUT | `405`（`Allow: GET, HEAD, PUT`） |
| `/api/workspace/records` | POST、DELETE | `405`（`Allow: POST, DELETE`） |

- 環境変数 `TYPLE_BACKEND_COMPARE` が `1` でなければ、すべてのリクエストに `404 {"error":"notFound"}` を返します。
- どの答えにもヘッダー `x-typle-backend: kotlin` を付けます。`next.config.mjs` が付ける4つのセキュリティヘッダーと
  `/api` の CSP も同じものを付けます。
- 保存の計測（`typle_save_metrics`）は書きません。

## コードの構成

```
kotlin/
  build.gradle.kts          Gradle（Kotlin DSL）。src/lib/packs と src/messages/ja/format.json をビルド時に resources へ写す
  Dockerfile                Cloud Run 用（gradle:8.14-jdk21 でビルドし、eclipse-temurin:21-jre で動かす）
  Dockerfile.dockerignore   ビルドのコンテキスト（リポジトリのルート）から、ビルドが読むものだけを通す
  src/main/kotlin/typle/
    Main.kt                 Ktor（Netty）。$PORT（既定 8080）で受ける
    lib/                    src/lib の移植: JSON（JSON.parse・JSON.stringify と同じ読み書き）、Date.parse、devices、
                            interests、wordLists、records、wordPacks、followedPacks、packs/catalog、workspaceRequest
    server/                 src/server の移植: settings・session（AUTH_SECRET からの鍵、Cookie とアプリのトークン）、
                            account（typle_session_revocations）、database（HikariCP）、records、workspace、ルート
  src/test/kotlin/typle/    テスト。共通 fixture（../tests/fixtures）をそのまま読む
```

公式パックのデータは手で写さず、Gradle の `copyAppData` が `../src/lib/packs`（`commands/manifest.json` の順）を
ビルドのたびに resources に写します。

## 環境変数

| 名前 | 内容 |
|---|---|
| `TYPLE_BACKEND_COMPARE` | `1` のときだけ答える。それ以外は全部 `404` |
| `DATABASE_URL` | Neon などの `postgres://` の URL（なければ `POSTGRES_URL`）。JDBC の URL に直して使う |
| `AUTH_SECRET` | Next.js と同じ値。ここから Cookie とアプリのトークンの鍵を作る |
| `AUTH_URL` | Next.js と同じ値。書き込みの `Origin` はこのオリジンと同じでなければならず、`https` なら Cookie の名前は `__Host-typle-session` |
| `PORT` | 受けるポート。Cloud Run が付ける。既定は `8080` |

`AUTH_URL` は、計測に使う Cookie を発行したデプロイの値にします。計測スクリプトが本番（`https://typle.sparklingstadt.com`）で
ログインした Cookie を使うなら、`AUTH_URL=https://typle.sparklingstadt.com` です。`AUTH_SECRET` もその Cookie を
発行したデプロイと同じ値でなければ、`401 loginRequired` になります。

DB の接続プールは Next.js（`src/server/database.ts`）にそろえています: 最大 5 本、使わない接続は 30 秒で閉じる、接続と
各クエリは 5 秒で諦める、TCP keep-alive。`sslmode=require`（Neon の URL）は node-postgres（pg 8）と同じく
`verify-full` として扱い、JVM の信頼するルート証明書でサーバーを確かめます。

## テスト

Java はいりません。Docker の中で動かします。`../tests/fixtures` と `../src` を読むので、リポジトリ全体をマウントします。
リポジトリのルートで:

```bash
docker run --rm -v "$PWD":/w -v typle-kotlin-gradle:/home/gradle/.gradle -w /w/kotlin gradle:8.14-jdk21 ./gradlew test --no-daemon
```

DB を使うテスト（`tests/fixtures/workspace-writes.json`・`record-writes.json` のシナリオ、保存の衝突、ログインの取り消しなど）は、
`TYPLE_TEST_DATABASE_URL` があるときだけ動き、なければ skip されます。手元の Docker の PostgreSQL（`.env.local` の
`DATABASE_URL`）を使うときは、コンテナーの中から見えるように `127.0.0.1` を `host.docker.internal` に変えて渡します。
テストはランダムな owner で行を足し、終わったら消します。

```bash
URL=$(grep '^DATABASE_URL=' .env.local | cut -d= -f2- | tr -d '"' | sed 's/127.0.0.1/host.docker.internal/;s/localhost/host.docker.internal/')
docker run --rm -e TYPLE_TEST_DATABASE_URL="$URL" -v "$PWD":/w -v typle-kotlin-gradle:/home/gradle/.gradle -w /w/kotlin \
  gradle:8.14-jdk21 ./gradlew test --rerun --no-daemon
```

`npm run verify` にはこのテストを入れていません。比較の作業をしたときは、上のコマンドを手で動かしてください。

## ビルドと手元での実行

リポジトリのルートで（ビルドは `src/lib/packs` などを読むので、コンテキストはルートです）:

```bash
docker build -f kotlin/Dockerfile -t typle-kotlin .
URL=$(grep '^DATABASE_URL=' .env.local | cut -d= -f2- | tr -d '"' | sed 's/127.0.0.1/host.docker.internal/')
docker run --rm -p 8080:8080 -e TYPLE_BACKEND_COMPARE=1 -e DATABASE_URL="$URL" \
  -e AUTH_SECRET="$(grep '^AUTH_SECRET=' .env.local | cut -d= -f2- | tr -d '"')" -e AUTH_URL=http://localhost:3000 typle-kotlin
curl -i http://localhost:8080/api/health
```

## Cloud Run へのデプロイ（ユーザーの Mac から）

クラウドのセッションには資格情報がないので、`gcloud` で手で行います。`PROJECT_ID` と `REGION` は置き換えてください。

**リージョン**: Neon の DB に一番近いところにします（例: Neon が `aws-ap-southeast-1`（シンガポール）なら
`asia-southeast1`、`aws-us-east-1`（バージニア）なら `us-east4`）。Neon のコンソールで DB のリージョンを確かめ、
Vercel の関数のリージョン（プロジェクトの Settings → Functions）も同じくらい近いかを確かめて、計測の結果と一緒に
書き残してください。

```bash
PROJECT_ID=your-project
REGION=asia-southeast1   # ← Neon の DB に一番近いリージョンを選ぶ
gcloud config set project "$PROJECT_ID"
gcloud services enable run.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com

# イメージの置き場所（初回だけ）
gcloud artifacts repositories create typle --repository-format=docker --location="$REGION"
gcloud auth configure-docker "$REGION-docker.pkg.dev"

# Cloud Run は amd64 で動くので、Apple シリコンの Mac では --platform を付けてビルドする（リポジトリのルートで）
IMAGE="$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-kotlin:$(git rev-parse --short HEAD)"
docker buildx build --platform linux/amd64 -f kotlin/Dockerfile -t "$IMAGE" --push .

# 秘密の値は Secret Manager に置く（初回だけ。値は標準入力から渡し、履歴に残さない）
printf '%s' "$AUTH_SECRET_VALUE" | gcloud secrets create typle-auth-secret --data-file=-
printf '%s' "$DATABASE_URL_VALUE" | gcloud secrets create typle-database-url --data-file=-
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
for s in typle-auth-secret typle-database-url; do
  gcloud secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:$PROJECT_NUMBER-compute@developer.gserviceaccount.com" --role=roles/secretmanager.secretAccessor
done

gcloud run deploy typle-kotlin \
  --image="$IMAGE" \
  --region="$REGION" \
  --cpu=1 --memory=2Gi \
  --min-instances=0 \
  --concurrency=80 \
  --cpu-throttling \
  --cpu-boost \
  --port=8080 \
  --set-secrets=AUTH_SECRET=typle-auth-secret:latest,DATABASE_URL=typle-database-url:latest \
  --set-env-vars=TYPLE_BACKEND_COMPARE=1,AUTH_URL=https://typle.sparklingstadt.com \
  --allow-unauthenticated
```

設定の意味（設計の「Kotlin のコード」の節のとおり）:

- `--cpu=1 --memory=2Gi`: Vercel の関数の既定（Standard、1 vCPU・2 GB）に合わせる。
- `--min-instances=0`: 使わないときは 0 台。
- `--concurrency=80`: Cloud Run の既定。
- `--cpu-throttling`: CPU はリクエストの間だけ割り当てる。
- `--cpu-boost`: 起動時の CPU ブーストはオン。
- JVM には `-XX:MaxRAMPercentage=75` だけを付けています（`Dockerfile`）。GraalVM のネイティブイメージや CRaC は使いません。

デプロイの最後に出る URL（`https://typle-kotlin-….run.app`）を、Vercel の Preview 環境の `TYPLE_KOTLIN_URL` に入れます。
確かめるには:

```bash
curl -i "https://typle-kotlin-….run.app/api/health"   # 200 と x-typle-backend: kotlin
```

## 比較が終わったら止める・消す

公開 URL で受けているので、比較が終わったらすぐに止めます。動きは本番の API と同じで、ログインの Cookie かトークンがなければ
何も読めず書けませんが、置いたままにはしません。

```bash
# すぐに誰からも呼べなくする（サービスは残す）
gcloud run services remove-iam-policy-binding typle-kotlin --region="$REGION" \
  --member=allUsers --role=roles/run.invoker

# サービスごと消す
gcloud run services delete typle-kotlin --region="$REGION"

# イメージと秘密の値も消す
gcloud artifacts docker images delete "$REGION-docker.pkg.dev/$PROJECT_ID/typle/typle-kotlin" --delete-tags
gcloud secrets delete typle-auth-secret
gcloud secrets delete typle-database-url
```

Vercel の Preview 環境の `TYPLE_KOTLIN_URL` も消してください。

## Next.js と違うところ

答えはそろえていますが、次の点は JVM の上で同じにしきれないか、比較に関係しないので違います。

- `Date.parse` は ISO 8601 の形（日付だけは UTC、時刻があってオフセットがなければ UTC として読む。Cloud Run と Vercel は
  どちらも UTC）と、よく使われるいくつかの形（`2026-09-07 10:00:00`、`2026/09/07`、`Mon, 07 Sep 2026 10:00:00 GMT`、
  `Mon Sep 07 2026 10:00:00 GMT+0900 (JST)`、`Sep 7, 2026`）だけを読みます。V8 はもっと多くの形を読むので、珍しい形の
  日付は Next.js では通り、ここでは `invalidRecord` や `savedDataCorrupted` になることがあります。アプリは
  `toISOString()` の形しか送りません。
- `toLocaleLowerCase()` はロケールに依らない小文字化（`Locale.ROOT`）です。Node の既定のロケールでも、トルコ語など以外は
  同じ結果です。
- 4000 段より深く入れ子になった JSON の本文は、読めない本文として `400` にします。
- 失敗（`503`）は標準エラーに書くだけで、Sentry には送りません。
- 知らないパスには `404 {"error":"notFound"}` を返します（Next.js なら 404 のページ）。末尾の `/` の付いたパスを
  リダイレクトはしません。
- 本文は、ログインを確かめる前に最大 1 MiB + 1 バイトまで読みます（それより大きければ `413` と分かれば足りるため）。
  答えは同じです。
- HEAD の答えには `Content-Length: 0` が付きます。
- 設定（`AUTH_SECRET`・`AUTH_URL`）は起動時に1回だけ読みます（Next.js はリクエストごとに読む）。
- DB の URL の `channel_binding` は JDBC に渡しません。
