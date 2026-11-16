# pin

[English](requirements.md)

## この文書について

`dockerfile-pin run`は、各イメージ参照のタグを、今指しているdigestに解決し、そのdigestを加えた対象ファイルを表示するか書き込みます。
親のProductは[dockerfile-pin](../../product.ja.md)です。

### スコープ

- `run`がどの参照を解決するか。
- dry-runと`--write`。
- `run`が表示するものと、レジストリの失敗の扱い。
- レジストリの認証。

### スコープ外

- どのファイルを読むか: [対象ファイルの探索](../target-file-discovery/requirements.ja.md)。
- 参照をどう読み、どう書き換えるか: [Dockerfileのイメージ参照](../dockerfile-image-references/requirements.ja.md)と[YAMLのイメージ参照](../yaml-image-references/requirements.ja.md)。
- `--update`と`--min-age`: [digestの更新](../digest-updates/requirements.ja.md)。
- `--ignore-images`と設定ファイル: [除外と設定](../ignore-and-configuration/requirements.ja.md)。

## ユーザーストーリー

### ファイルを変える前に確かめる

開発者として、`run`にはファイルを変えずに結果を見せてほしい。先に確認できるようにするためだ。

**受け入れ条件:**

シナリオ: `--write`がないとき、pinした内容を表示し、ファイルは変えない

- 前提: `Dockerfile`に`FROM node:20.11.1`がある。
- 操作: 開発者が`dockerfile-pin run`を実行する。
- 結果: 出力に`found 1 file(s)`、`resolving 1 unique image(s)...`、`resolved node:20.11.1 → sha256:...`の行、`--- Dockerfile`とそれに続くdigestを加えたファイル全体が含まれる。ディスク上のファイルは変わらない。

### ファイルにdigestを書き込む

開発者として、`run --write`でファイルをその場で変えたい。pinしたファイルをコミットできるようにするためだ。

**受け入れ条件:**

シナリオ: `--write`があるとき、ファイルを書き換えて数を表示する

- 前提: `Dockerfile`にpinしていない`FROM`の行が2つある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: ファイルに両方のdigestが入り、ファイルの権限は変わらず、`pinned 2 image(s) in Dockerfile`が表示される。

シナリオ: 参照にすでにdigestがあるとき、そのままにする

- 前提: ファイルに`FROM node:20@sha256:<digest>`がある。
- 操作: 開発者が`--update`なしで`dockerfile-pin run --write`を実行する。
- 結果: その行は変わらず、そのイメージは解決されない。

シナリオ: ファイルにdigestが必要なものがないとき、そのファイルは表示も書き込みもしない

- 前提: ファイルのすべての参照がpin済みかスキップである。
- 操作: 開発者が`dockerfile-pin run`を実行する。
- 結果: そのファイルの`--- <file>`のブロックは表示されない。

### レジストリが失敗しても続ける

開発者として、1つのイメージの失敗でほかを止めないでほしい。残りはpinできるようにするためだ。

**受け入れ条件:**

シナリオ: イメージを解決できないとき、警告を表示してほかをpinする

- 前提: ファイルに`node:20`と、レジストリに存在しないイメージがある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: 標準エラーに`WARN  <image>  failed to resolve: ...`が表示され、`node:20`はpinされ、存在しないイメージの行は変わらず、終了コードは0である。

シナリオ: ファイルを解析できないとき、ほかのファイルを処理する

- 前提: 対象ファイルの1つが正しくなく、もう1つは正しい。
- 操作: 開発者が`dockerfile-pin run`を実行する。
- 結果: 標準エラーに`error parsing <file>: ...`が表示され、正しいファイルは処理される。

## KPI

ありません。dockerfile-pinは利用状況のデータを集めていません。

## 機能要件

### 解決

**概要:** 異なるイメージごとに、実行全体で1回だけ解決します。

**入力:** すべての対象ファイルの参照。

**出力:** 解決できた各イメージのdigest。

#### 適用条件

スキップされず、除外されず、digestのない参照を解決します。
イメージは、書かれた名前とタグで調べます。複数のファイルにある同じイメージには、同じdigestが付きます。
解決にはHEADリクエストを使うので、Docker Hubのpull rate limitを消費しません。
認証情報は`~/.docker/config.json`から読むので、プライベートレジストリには`docker login`やcredential helperが使えます。

#### 例外時の振る舞い

解決に失敗したイメージは警告として報告し、変えません。実行全体は10分で、レジストリへの各リクエストは30秒で打ち切ります。

### 出力

**概要:** 既定はdry-runで、`--write`がファイルを変えます。

**入力:** `--write`。

**出力:** 進行状況の行と、ファイルごとの、pinした内容（dry-run）か`pinned N image(s) in <file>`の行。

#### 適用条件

dry-runは、digestが1つ以上付くファイルごとに、`--- <path>`とpinした内容全体を表示します。
`--write`はファイルの内容を置き換え、ファイルの権限は保ちます。
`run`は位置引数を受け付けません。

## データ要件

### 必要なデータ

- **解決したdigest:** 各イメージのdigest。実行の間だけ持ちます。

### 関連

イメージの名前とタグごとに1つのdigest。

### 個人情報

ありません。

## 外部サービスへの依存

- **コンテナレジストリ（Docker Hub、GHCR、GCR、ECRなど）**
  - **関連するユーザーストーリー:** すべて。
  - **依存する理由:** タグが指すdigestはレジストリにしかないため。
  - **契約と利用上の判断:** 利用者自身のレジストリのアカウントと制限が適用されます。HEADリクエストを使うので、Docker Hubのpull rate limitは消費しません。

## セキュリティ上の考慮事項

認証情報は利用者のDockerの設定から読み、表示しません。ファイルを変えるのは`--write`のときだけです。

## 未決事項

ありません。
