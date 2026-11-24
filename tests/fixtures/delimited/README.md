# CSV の共通 fixture

`cases.json` の `read` は、`source` を読んだ結果の `words` か、エラーの `error`（`key` と、行が決まるときは `line`）です。`write` は、`words` を書き出した文字列 `csv` です。
`tests/delimited.test.cjs` が読みます。
