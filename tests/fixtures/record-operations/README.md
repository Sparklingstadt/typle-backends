# 練習記録の操作列の共通 fixture

`lists`・`devices` のワークスペースに `steps` の操作を順に行った結果です。各操作の結果は `steps[].expected`、
最後の各リストの記録は `expected`（リストID → 記録の配列）。`tests/records.test.cjs`（`addRecord`・`deleteRecord`）が読みます。

- `add`: `expected.added`（新しく追加したか）または `expected.error`（エラーのキー。リストがないとき `recordListDeleted`）。
- `delete`: `expected.deleted`。すでに無い記録・リストの削除は `false` で、何も変えない。

1つの操作の規則は `tests/fixtures/records` にあります。
