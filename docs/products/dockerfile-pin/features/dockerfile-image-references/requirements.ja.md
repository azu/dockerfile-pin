# Dockerfileのイメージ参照

[English](requirements.md)

## この文書について

この機能は、Dockerfileの`FROM`と`COPY --from=`からDockerイメージの参照を読み取り、各参照が書かれている場所にdigestを書き込みます。
`run`は読み取った参照をpinし、`check`はその参照について報告します。
親のProductは[dockerfile-pin](../../product.ja.md)です。

### スコープ

- どの命令を読み、どの参照をスキップするか。
- `FROM`で`ARG`の既定値をどう展開するか。
- すでにdigestがある参照の見分け方。
- 複数行に続く命令を含め、digestをDockerfileの文字列へ書き込む方法。

### スコープ外

- Dockerfileを探すこと。[対象ファイルの探索](../target-file-discovery/requirements.ja.md)に属します。
- digestの解決と、`run`と`check`の出力。[pin](../pinning/requirements.ja.md)と[check](../checking/requirements.ja.md)に属します。
- `ADD --from=`と`RUN --mount=...,from=...`。読み取りません。
- 名前付きビルドコンテキスト（`docker buildx build --build-context name=...`）。どのステージにも一致しない`COPY --from=<name>`は、Dockerfileだけでは区別できないので、イメージとして読みます。

## ユーザーストーリー

### ベースイメージをpinする

Dockerfileを管理する開発者として、`run`で`FROM`にdigestを付けたい。ビルドがpinしたベースイメージを使い続けるようにするためだ。

**受け入れ条件:**

シナリオ: `FROM`がタグ付きのイメージを指すとき、タグの後ろにdigestを書く

- 前提: Dockerfileに`FROM python:3.12-slim AS builder`と`FROM --platform=linux/amd64 node:20`がある。
- 操作: 開発者が`dockerfile-pin run --write -f Dockerfile`を実行する。
- 結果: それぞれの行が`FROM python:3.12-slim@sha256:<digest> AS builder`と`FROM --platform=linux/amd64 node:20@sha256:<digest>`になり、ほかの行は変わらない。

シナリオ: `FROM`が`scratch`か前のステージを指すとき、スキップする

- 前提: Dockerfileに`FROM golang:1.22 AS builder`、`FROM scratch`、`FROM builder`が順にある。
- 操作: 開発者が`dockerfile-pin check -f Dockerfile`を実行する。
- 結果: `FROM scratch`は`scratch image`付きの`SKIP`、`FROM builder`は`stage reference`付きの`SKIP`として報告され、`run`はどちらも変えない。

シナリオ: `FROM`が既定値のある`ARG`を使うとき、既定値を展開する

- 前提: Dockerfileに`ARG VERSION=1.0`と、それに続く`FROM image:${VERSION}`がある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: `image:1.0`のdigestが書かれ、その行が`FROM image:${VERSION}@sha256:<digest>`になる。

シナリオ: `FROM`が既定値のない`ARG`を使うとき、スキップする

- 前提: Dockerfileに`ARG BASE`と、それに続く`FROM ${BASE}`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`unresolved ARG variable`付きの`SKIP`として報告される。

### コピー元のイメージをpinする

外部のイメージからファイルをコピーする開発者として、`COPY --from=`のイメージもpinしたい。ベースイメージと同じ保証を得るためだ。

**受け入れ条件:**

シナリオ: `COPY --from=`がイメージを指すとき、そのフラグにdigestを書く

- 前提: Dockerfileに`COPY --chown=node:20 --from=node:20 /app /app`がある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: その行が`COPY --chown=node:20 --from=node:20@sha256:<digest> /app /app`になり、`--chown`の値は変わらない。

シナリオ: `COPY --from=`がステージかステージの番号を指すとき、スキップする

- 前提: Dockerfileに、ファイルのどこかで宣言されたステージ`builder`を使う`COPY --from=builder /a /b`と、`COPY --from=0 /a /b`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: それぞれ`stage reference`と`stage index reference`付きの`SKIP`として報告される。

シナリオ: `COPY --from=`に変数があるとき、スキップする

- 前提: Dockerfileに`COPY --from=image:${TAG} /a /b`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`unexpanded variable in COPY --from`付きの`SKIP`として報告され、`run`はその行を変えない。

シナリオ: `ONBUILD COPY --from=`がイメージを指すとき、同じ名前のステージがあってもpinする

- 前提: Dockerfileが`FROM alpine AS nginx`を宣言し、`ONBUILD COPY --from=nginx /a /b`を持つ。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: イメージ`nginx`のdigestが`ONBUILD`の行に書かれる。

### 複数行に分かれた参照をpinする

長い命令を行継続で折り返す開発者として、digestが正しい場所に入ってほしい。ファイルが気付かないうちにpinされないままになるのを防ぐためだ。

**受け入れ条件:**

シナリオ: 参照が行継続で分かれているとき、参照が終わる場所にdigestを書く

- 前提: Dockerfileに、`--from=`の値が`\`で2行に分かれた`COPY`がある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: 参照が終わる行の参照の末尾にdigestが入り、行継続は残る。

シナリオ: 参照がソースに見つからないとき、その行を変えずに報告する

- 前提: digestを解決できた参照が、その命令のソースの文字列に現れない。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: ファイル、行、イメージと`reference not found in the source line, left unchanged`を含む`WARN`が表示され、そのイメージはpinした数に入らない。

## KPI

ありません。dockerfile-pinは利用状況のデータを集めていません。

## 機能要件

### 参照の読み取り

**概要:** すべての`FROM`、`--from=`フラグのあるすべての`COPY`、`--from=`フラグのあるすべての`ONBUILD COPY`が、それぞれ1つの参照になります。

**入力:** Dockerfileの内容。

**出力:** ファイルの順に並んだ参照。各参照は、書かれたままの文字列、`ARG`を展開しdigestを除いたイメージ、既存のdigest、命令がまたがる行、命令の文字列、スキップするかどうかとその理由を持ちます。

#### 適用条件

##### FROM

参照は、`FROM`とそのフラグの後にある最初の語です。`--platform`と`AS <name>`は書かれたまま残します。
`$VAR`と`${VAR}`は、その`FROM`より上に書かれた`ARG`の既定値で置き換えます。`FROM`より下に書かれた`ARG`は、その`FROM`に影響しません。
名前がステージ参照になるのは、その名前のステージが`FROM`より上で宣言されているときだけです。ステージ名は大文字と小文字を区別せずに比べます。

##### COPY --from

フラグ名は`--from=`と完全に一致するものだけを読みます。`--FROM=`は読みません。
整数の値はステージの番号です。`$`を含む値は展開しません。
その名前のステージがファイルのどこかで宣言されていれば、名前はステージ参照です。
`ONBUILD COPY`には、このファイルのステージ名を当てはめません。そのトリガーは、このイメージをベースに使うビルドの中で動くためです。

##### digestとステージ

`@`を含む参照は、すでにdigestを持っています。最後の`@`より前がイメージで、後ろがdigestです。
digest付きの参照は、ステージ参照にはなりません。digestを含む値全体をステージ名と比べるためです。

##### escapeディレクティブ

`# escape=`のパーサーディレクティブは行継続の文字を変えます。読み取りと書き込みの両方でこれに従います。

#### 例外時の振る舞い

##### スキップする参照

| 参照 | 表示する理由 |
| --- | --- |
| 値のない`FROM`か`--from=` | `missing image reference` |
| `scratch` | `scratch image` |
| ステージ名 | `stage reference` |
| `COPY --from=`のステージの番号 | `stage index reference` |
| 既定値のない`FROM`の変数 | `unresolved ARG variable` |
| `COPY --from=`の変数 | `unexpanded variable in COPY --from` |

スキップした参照は`check`が報告し、`run`は変えません。

##### 解析できないDockerfile

Dockerfileのパーサーが受け付けないファイルは、ファイル名付きのエラーとして報告し、ほかの対象ファイルの処理は続けます。

### digestの書き込み

**概要:** digestは、Dockerfileの文字列の中で参照のすぐ後ろに書きます。

**入力:** 内容、参照、各参照に選んだdigest。

**出力:** 選んだ各参照が`<digestを除いた書かれたままの参照>@<digest>`になった内容と、ソースに見つからなかった参照の一覧。

#### 適用条件

既存のdigestは置き換えます。`COPY`では`--from=`の後ろの文字列だけを変えるので、同じ行のほかの場所にある同じ文字列は書かれたままです。
変数は書かれたまま残します。`FROM image:${VERSION}`は`FROM image:${VERSION}@<digest>`になります。
コメント、行継続、ほかの行を含め、それ以外の文字列は変えません。

## データ要件

### 必要なデータ

- **イメージ参照:** 参照の読み取りに挙げた項目。コマンドの実行中だけ存在します。

### 関連

各参照は1つのDockerfileに属します。

### 個人情報

ありません。

## 外部サービスへの依存

ありません。レジストリへの問い合わせは、pinとcheckの機能に属します。

## セキュリティ上の考慮事項

ビルドがdockerfile-pinとは違う読み方をする参照を、pin済みとして報告してはいけません。スキップの規則はBuildKitが`FROM`と`COPY --from`を読む方法に合わせてあり、pinしていないイメージをpullする行を`check`が`OK`と報告しないようにしています。

## 未決事項

ありません。
