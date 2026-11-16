# check

[English](requirements.md)

## この文書について

`dockerfile-pin check`は、対象ファイルのすべてのイメージ参照について、digestがあるか、そのdigestがレジストリに存在するかを報告し、通らない参照があれば失敗します。
親のProductは[dockerfile-pin](../../product.ja.md)です。

### スコープ

- 状態と、それぞれの原因。
- `--syntax-only`、`--verify-tag`、`--format`、`--exit-code`。

### スコープ外

- どのファイルを読むか: [対象ファイルの探索](../target-file-discovery/requirements.ja.md)。
- 参照をどう読むか: [Dockerfileのイメージ参照](../dockerfile-image-references/requirements.ja.md)と[YAMLのイメージ参照](../yaml-image-references/requirements.ja.md)。
- 除外パターン: [除外と設定](../ignore-and-configuration/requirements.ja.md)。
- 改ざんの検出。`check`は、digestが作者の意図したイメージのものかどうかを判断できません。

## ユーザーストーリー

### pinしていない参照をCIで止める

開発者として、参照にdigestがないときはCIを失敗させたい。pinしていないイメージがマージされないようにするためだ。

**受け入れ条件:**

シナリオ: 参照にdigestがないとき、失敗する

- 前提: `Dockerfile`の1行目に`FROM node:20.11.1`がある。
- 操作: CIが`dockerfile-pin check`を実行する。
- 結果: `FAIL  Dockerfile:1    FROM node:20.11.1    missing digest`が表示され、終了コードは1である。

シナリオ: すべての参照が通るかスキップのとき、コマンドは成功する

- 前提: `Dockerfile`に`FROM python:3.12@sha256:<存在するdigest>`と`FROM scratch`がある。
- 操作: CIが`dockerfile-pin check`を実行する。
- 結果: 1つ目は`OK`、2つ目は`scratch image`付きの`SKIP`で、終了コードは0である。

### 存在しないdigestを見つける

開発者として、打ち間違えたdigestや削除されたdigestを、ビルドより前に報告してほしい。`docker build`で初めて気付くことがないようにするためだ。

**受け入れ条件:**

シナリオ: digestがレジストリにないとき、失敗する

- 前提: 参照に、形式は正しいがレジストリにないdigestがある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: `digest not found in registry`付きの`FAIL`として報告される。

シナリオ: レジストリに届かないとき、失敗にせず警告する

- 前提: レジストリへのリクエストが、ネットワークか認証のエラーで失敗する。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`registry check failed: ...`付きの`WARN`として報告され、それだけでは終了コードは0以外にならない。

シナリオ: `--syntax-only`があるとき、レジストリに問い合わせない

- 前提: 参照に何らかのdigestがある。
- 操作: 開発者が`dockerfile-pin check --syntax-only`を実行する。
- 結果: レジストリへのリクエストなしで`OK`として報告される。

### タグとdigestが合っているか確かめる

開発者として、各タグがpinしたdigestを指しているか確かめたい。別のバージョンのdigestに気付けるようにするためだ。

**受け入れ条件:**

シナリオ: タグが別のdigestを指しているとき、失敗する

- 前提: ファイルに`FROM busybox:1.37@sha256:<別のイメージのdigest>`があり、そのdigestは存在する。
- 操作: 開発者が`dockerfile-pin check --verify-tag`を実行する。
- 結果: `tag points to a different digest: <今のdigest>`付きの`FAIL`として報告される。

シナリオ: 参照にタグがないとき、確かめるものはない

- 前提: ファイルに`FROM node@sha256:<digest>`がある。
- 操作: 開発者が`dockerfile-pin check --verify-tag`を実行する。
- 結果: `no tag to verify`付きの`OK`として報告される。

シナリオ: `--verify-tag`を`--syntax-only`と組み合わせたとき、コマンドは失敗する

- 前提: 任意の対象ファイルがある。
- 操作: 開発者が`dockerfile-pin check --verify-tag --syntax-only`を実行する。
- 結果: コマンドは`--verify-tag cannot be used with --syntax-only`で終了する。

### ほかのツールから結果を読む

`check`をCIに組み込む開発者として、機械で読める出力と、変えられる終了コードがほしい。

**受け入れ条件:**

シナリオ: `--format json`があるとき、JSONの配列を表示する

- 前提: pinしていない参照が1つあるファイルがある。
- 操作: 開発者が`dockerfile-pin check --format json`を実行する。
- 結果: `file`、`line`、`image`、`status`、`message`、`original`を持つオブジェクトの配列が表示され、`status`は`"fail"`である。

シナリオ: `--exit-code`があるとき、失敗時にそのコードを使う

- 前提: pinしていない参照が1つあるファイルがある。
- 操作: 開発者が`dockerfile-pin check --exit-code 2`を実行する。
- 結果: 終了コードは2である。

## KPI

ありません。dockerfile-pinは利用状況のデータを集めていません。

## 機能要件

### 状態

**概要:** 各参照には、ちょうど1つの状態が付きます。

**入力:** 対象ファイルの参照とオプション。

**出力:** 参照ごとに1つの結果。ファイルと参照の順に並びます。

#### 適用条件

最初に当てはまった行が状態を決めます。

| 順序 | 条件 | 状態 | メッセージ |
| --- | --- | --- | --- |
| 1 | 読み取りがその参照をスキップしている | `SKIP` | スキップの理由 |
| 2 | イメージが除外パターンに一致する | `SKIP` | `ignored` |
| 3 | digestがない | `FAIL` | `missing digest` |
| 4 | `--syntax-only` | `OK` | |
| 5 | レジストリへのリクエストが失敗する | `WARN` | `registry check failed: <error>` |
| 6 | digestがレジストリにない | `FAIL` | `digest not found in registry` |
| 7 | それ以外 | `OK` | |

`--verify-tag`があると、行7に達した参照をさらに確かめます。タグがなければ`no tag to verify`付きの`OK`のままです。タグの解決に失敗すると`tag resolve failed: <error>`付きの`WARN`になります。タグのdigestが違うと`tag points to a different digest: <digest>`付きの`FAIL`になります。
`localhost:5000/app`のように最後の`/`より前にあるコロンは、レジストリのポートで、タグではありません。

終了コードは、`FAIL`の結果が1つでもあれば`--exit-code`の値（既定は1）で、なければ0です。

#### 例外時の振る舞い

読めないファイルや解析できないファイルは、標準エラーに`error processing <file>: ...`として報告します。ほかのファイルの検査は続け、そのエラーだけでは終了コードは変わりません。

### 出力の形式

**概要:** 既定はテキストで、`--format json`はJSONを表示します。

**入力:** `--format`。

**出力:** テキストでは、結果ごとに1行です。状態、`<file>:<line>`、命令かキーを付けた書かれたままの参照（たとえば`FROM node:20`、`image: node:20`、`uses: docker://alpine:3`、`- postgres:16`）、メッセージが並びます。JSONでは、`file`、`line`、`image`（digestなし）、`status`（`ok`、`fail`、`skip`、`warn`）、`message`、`original`を持つオブジェクトの配列です。

## データ要件

### 必要なデータ

- **checkの結果:** ファイル、行、イメージ、状態、メッセージ、元の文字列。コマンドの実行中だけ存在します。

### 関連

参照ごとに1つの結果。

### 個人情報

ありません。

## 外部サービスへの依存

- **コンテナレジストリ**
  - **関連するユーザーストーリー:** 存在しないdigestを見つける、タグとdigestが合っているか確かめる。
  - **依存する理由:** digestが存在するかと、タグがどこを指すかは、レジストリにしか分からないため。
  - **契約と利用上の判断:** HEADリクエストを使うので、Docker Hubのpull rate limitを消費しません。`--verify-tag`は、タグのある参照ごとにリクエストを1つ増やします。

## セキュリティ上の考慮事項

`--verify-tag`は、digestを、タグが今指している先と比べます。レジストリはタグが過去に指していた先を返さないので、不一致が意味するのは今違うということだけです。上流の再ビルドと、digestの差し替えを区別できません。既定で無効にしているのはこのためです。

## 未決事項

ありません。
